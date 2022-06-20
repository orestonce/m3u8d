package m3u8d

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/net/proxy"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func newDialContext(setProxy string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if setProxy == "" {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		} else if strings.HasPrefix(setProxy, "http") {
			return proxyByHttp(setProxy, ctx, addr)
		} else { // socks5
			urlObj, err := url.Parse(setProxy)
			if err != nil {
				return nil, err
			}
			dialer, err := proxy.FromURL(urlObj, nil)
			if err != nil {
				return nil, err
			}
			return dialer.Dial(network, addr)
		}
	}

}

func proxyByHttp(setProxy string, ctx context.Context, to string) (net.Conn, error) {
	// https://github.com/aidenesco/connect/blob/master/proxy.go
	urlObj, err := url.Parse(setProxy)
	if err != nil {
		return nil, err
	}
	var tConn net.Conn
	if strings.HasPrefix(setProxy, "https://") {
		host := urlObj.Host
		_, _, err = net.SplitHostPort(host)
		if err != nil {
			host = host + ":443"
		}
		tConn, err = (&tls.Dialer{}).DialContext(ctx, "tcp", host)
	} else {
		host := urlObj.Host
		_, _, err = net.SplitHostPort(host)
		if err != nil {
			host = host + ":80"
		}
		tConn, err = (&net.Dialer{}).DialContext(ctx, "tcp", host)
	}

	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tConn.Close()
		}
	}()

	buf0 := bytes.NewBuffer(nil)
	buf0.WriteString(`CONNECT ` + to + ` HTTP/1.1` + "\r\n")

	if u := urlObj.User; u != nil {
		username := u.Username()
		password, _ := u.Password()
		buf0.WriteString("Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)) + "\r\n")
	}
	buf0.WriteString("\r\n")
	_, err = tConn.Write(buf0.Bytes())
	if err != nil {
		return nil, err
	}

	bufr := bufio.NewReader(tConn)

	var response *http.Response
	response, err = http.ReadResponse(bufr, nil)
	if err != nil {
		return nil, err
	}

	switch response.StatusCode {
	case http.StatusOK:
		return tConn, nil
	case http.StatusProxyAuthRequired:
		return nil, errors.New("connect: invalid or missing \"Proxy-Authorization\" header")
	default:
		return nil, fmt.Errorf("connect: unexpected CONNECT response status \"%s\" (expect 200 OK)", response.Status)
	}
}
