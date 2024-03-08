package m3u8d

import (
	"testing"
)

func TestSetProxyFormat(t *testing.T) {
	runOne := func(origin string, expectAfter string) {
		after, _, errMsg := ParseProxyFormat(origin)
		if errMsg != "" {
			panic(errMsg)
		}
		if after != expectAfter {
			panic(after)
		}
	}
	runOne("httP://127.0.0.1:1234", "http://127.0.0.1:1234")
	runOne("127.0.0.1:1234", "http://127.0.0.1:1234")
	runOne("socKs5://127.0.0.1:1080", "socks5://127.0.0.1:1080")
	_, _, errMsg := ParseProxyFormat("htt://123.com")
	if errMsg == "" {
		t.Fatal("TestSetProxyFormat")
	}
}
