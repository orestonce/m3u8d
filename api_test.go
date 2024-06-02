package m3u8d

import (
	"fmt"
	"strconv"
	"testing"
)

func TestFindUrlInStr(t *testing.T) {
	urlStr := FindUrlInStr(" https://www.baidu.com/url1.jsp?sxjs=1&s=2&io=%%112 xs")
	if urlStr != "https://www.baidu.com/url1.jsp?sxjs=1&s=2&io=%%112" {
		fmt.Println(strconv.Quote(urlStr))
		t.Fail()
	}
}
