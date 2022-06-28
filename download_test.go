package m3u8d

import "testing"

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
