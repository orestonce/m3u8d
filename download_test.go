package m3u8d

import (
	"testing"
)

func TestUrlHasSuffix(t *testing.T) {
	if UrlHasSuffix("/0001.ts", ".ts") == false {
		t.Fatal()
		return
	}
	if UrlHasSuffix("/0001.Ts", ".ts") == false {
		t.Fatal()
		return
	}
	if UrlHasSuffix("/0001.ts?v=123", ".ts") == false {
		t.Fatal()
		return
	}
	if UrlHasSuffix("https://www.example.com/0001.m3u8?hsd=12", "hsd") {
		t.Fatal()
		return
	}
	if UrlHasSuffix("https://www.example.com/0001.m3U8?hsd=12", ".m3u8") == false {
		t.Fatal()
		return
	}
}

func TestGetTsList(t *testing.T) {
	v, err := getHost(`https://example.com:65/3kb/hls/index.m3u8`, `apiv1`)
	if err != nil {
		panic(err)
	}
	if v != `https://example.com:65/3kb/hls` {
		panic(v)
	}
	list, errMsg := getTsList(`https://example.com:65/3kb/hls`, `#EXTINF:3.753,
/3kb/hls/JJG.ts`)
	if errMsg != "" {
		panic(errMsg)
	}
	if len(list) != 1 {
		panic(len(list))
	}
	if list[0].Url != "https://example.com:65/3kb/hls/JJG.ts" {
		panic(list[0].Url)
	}
}
