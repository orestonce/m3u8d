package m3u8d

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

type ParseCurl_Resp struct {
	ErrMsg      string
	DownloadReq RunDownload_Req
}

func ParseCurlStr(s string) (resp ParseCurl_Resp) {
	r := strings.NewReader(strings.ReplaceAll(s, "\\\n", ""))
	var tmp []string
	for {
		b, _, err := r.ReadRune()
		if err != nil {
			break
		}
		if unicode.IsSpace(b) {
			continue
		}
		if b == '"' || b == '\'' {
			str := parseQuotedStr(r, b)
			tmp = append(tmp, str)
		} else {
			var buf bytes.Buffer
			buf.WriteRune(b)
			for {
				b, _, err = r.ReadRune()
				if err != nil || unicode.IsSpace(b) {
					break
				}
				buf.WriteRune(b)
			}
			tmp = append(tmp, buf.String())
		}
	}
	if len(tmp) > 0 && strings.ToLower(tmp[0]) == "curl" {
		tmp = tmp[1:]
	}
	return ParseCurl(tmp)
}

func parseQuotedStr(r *strings.Reader, b rune) string {
	var buf bytes.Buffer
	var prevHasQ = false

	for {
		nextB, _, err := r.ReadRune()
		if err != nil {
			break
		}
		if nextB == b && prevHasQ == false {
			break
		}
		if nextB == '\\' && b == '"' && prevHasQ == false {
			prevHasQ = true
			continue
		}
		prevHasQ = false
		buf.WriteRune(nextB)
	}
	return buf.String()
}

func ParseCurl(cmdList []string) (resp ParseCurl_Resp) {
	header := http.Header{}

	isHeader := false
	isMethod := false
	isProxy := false

	for idx := 0; idx < len(cmdList); idx++ {
		value := cmdList[idx]
		if isHeader {
			idx1 := strings.Index(value, ": ")
			if idx1 >= 0 {
				k := value[:idx1]
				v := value[idx1+2:]
				header.Set(k, v)
			}
			isHeader = false
			continue
		}
		if isMethod {
			if strings.ToUpper(value) != http.MethodGet {
				resp.ErrMsg = "不支持的method: " + strconv.Quote(value)
				return resp
			}
			isMethod = false
			continue
		}
		if isProxy {
			resp.DownloadReq.SetProxy = value
			isProxy = false
			continue
		}
		valueLow := strings.ToLower(value)
		switch valueLow {
		case "-h":
			isHeader = true
		case "--compressed":
		case "-x":
			if value == "-X" {
				isMethod = true
			} else {
				isProxy = true
			}
		case "-k", "--insecure":
			resp.DownloadReq.Insecure = true
		default:
			if strings.HasPrefix(valueLow, "-") { // 不认识的flag, 跳过
				continue
			}
			if resp.DownloadReq.M3u8Url != "" {
				resp.ErrMsg = "重复的url"
				return resp
			}
			resp.DownloadReq.M3u8Url = value
		}
	}
	resp.DownloadReq.HeaderMap = header
	return resp
}

func RunDownload_Req_ToCurlStr(req RunDownload_Req) string {
	if req.M3u8Url == "" {
		return ""
	}
	var buf bytes.Buffer
	buf.WriteString("curl " + strconv.Quote(req.M3u8Url))
	if req.Insecure {
		buf.WriteString(" \\\n -insecure")
	}
	if req.SetProxy != "" {
		buf.WriteString(" \\\n -X " + req.SetProxy)
	}
	for key, vList := range req.HeaderMap {
		if len(vList) == 0 {
			continue
		}
		buf.WriteString(" \\\n -H ")
		buf.WriteString(`'` + key + `: ` + vList[0] + `'`)
	}
	return buf.String()
}
