package m3u8d

import (
	"net/url"
	"strings"
)

func SetProxyFormat(origin string) (after string, urlObj *url.URL, errMsg string) {
	after = strings.ToLower(strings.TrimSpace(origin))
	if after == "" {
		return after, nil, ""
	}
	if strings.Contains(after, "://") == false {
		after = "http://" + after // 默认http
	}
	urlObj, err := url.Parse(after)
	if err != nil {
		return "", nil, "SetProxyFormat1: " + err.Error()
	}
	for _, vp := range []string{"http", "https", "socks5"} {
		if urlObj.Scheme == vp {
			return after, urlObj, ""
		}
	}
	return "", nil, "SetProxyFormat2: unknown schema " + urlObj.Scheme
}
