package m3u8d

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	v, err := getHost(`https://example.com:65/3kb/hls/index.m3u8`)
	if err != nil {
		panic(err)
	}
	if v != `https://example.com:65` {
		panic(v)
	}
	// 相对根目录
	tGetTsList(`https://example.com:65/3kb/hls/index.m3u8`, `/3kb/hls/JJG.ts`, "https://example.com:65/3kb/hls/JJG.ts")
	// 相对自己
	tGetTsList("https://example.xyz/k/data1/SD/index.m3u8", `0.ts`, `https://example.xyz/k/data1/SD/0.ts`)
	// 绝对路径
	tGetTsList("https://example.xyz/k/data1/SD/index.m3u8", `https://exampe2.com/0.ts`, `https://exampe2.com/0.ts`)
}

func tGetTsList(m3u8Url string, m3u8Content string, expectTs0Url string) {
	realUrl, errMsg := ResolveRefUrl(m3u8Url, m3u8Content)
	if errMsg != "" {
		panic(errMsg)
	}
	if realUrl != expectTs0Url {
		panic(realUrl)
	}
}

//go:embed testdata/TestFull
var sDataTestFull embed.FS

func TestFull(t *testing.T) {
	subFs, err := fs.Sub(sDataTestFull, "testdata/TestFull")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(subFs)))
	server := httptest.NewServer(mux)
	m3u8Url := server.URL + "/jhxy.01.m3u8"
	resp, err := http.Get(m3u8Url)
	if err != nil {
		panic(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		panic(resp.Status + " " + m3u8Url)
	}
	saveDir := filepath.Join(GetWd(), "testdata/save_dir")
	err = os.RemoveAll(saveDir)
	if err != nil {
		panic(err)
	}
	var instance DownloadEnv
	errMsg := instance.StartDownload(StartDownload_Req{
		M3u8Url:     m3u8Url,
		SaveDir:     saveDir,
		FileName:    "all",
		ThreadCount: 8,
	})
	if errMsg != "" {
		panic(errMsg)
	}
	status := instance.WaitDownloadFinish()
	if status.ErrMsg != "" {
		panic(status.ErrMsg)
	}
	fState, err := os.Stat(filepath.Join(saveDir, "all.mp4"))
	if err != nil {
		panic(err)
	}
	if fState.Size() <= 100*1000 { // 100KB
		panic("state error")
	}
}

func TestGetFileName(t *testing.T) {
	u1 := "https://example.com/video.m3u8"
	u2 := "https://example.com/video.m3u8?query=1"
	u3 := "https://example.com/video-name"

	if GetFileNameFromUrl(u1) != "video" {
		t.Fail()
	}

	if GetFileNameFromUrl(u2) != "video" {
		t.Fail()
	}

	if GetFileNameFromUrl(u3) != "video-name" {
		t.Fail()
	}
}
