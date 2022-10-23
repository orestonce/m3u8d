package main

import (
	"fmt"
	"github.com/orestonce/go2cpp"
	"golang.org/x/text/encoding/simplifiedchinese"
	"io/ioutil"
	"m3u8d"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func main() {
	BuildCliBinary()                       // 编译命令行版本
	if os.Getenv("GITHUB_ACTIONS") == "" { // 本地编译
		CreateLibForQtUi("amd64-static") // 创建Qt需要使用的.a库文件
		WriteVersionDotRc("1.5.20")
	} else { // github actions 编译
		if runtime.GOOS == "darwin" { // 编译darwin版本的dmg
			CreateLibForQtUi("amd64-shared")
		} else { // 编译windows版本的exe
			CreateLibForQtUi("386-static")
		}
		if len(os.Args) <= 1 || os.Args[1] != "check-only" {
			version := strings.TrimPrefix(os.Getenv("GITHUB_REF_NAME"), "v")
			WriteVersionDotRc(version)
		}
	}
}

func BuildCliBinary() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	type buildCfg struct {
		GOOS   string
		GOARCH string
		Ext    string
	}
	var list = []buildCfg{
		{
			GOOS:   "linux",
			GOARCH: "386",
		},
		{
			GOOS:   "linux",
			GOARCH: "arm",
		},
		{
			GOOS:   "linux",
			GOARCH: "mipsle",
		},
		{
			GOOS:   "darwin",
			GOARCH: "amd64",
		},
		{
			GOOS:   "windows",
			GOARCH: "386",
			Ext:    ".exe",
		},
	}
	for _, cfg := range list {
		name := "m3u8d_cli_" + cfg.GOOS + "_" + cfg.GOARCH + cfg.Ext
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", filepath.Join(wd, "bin", name))
		cmd.Dir = filepath.Join(wd, "cmd")
		cmd.Env = append(os.Environ(), "GOOS="+cfg.GOOS)
		cmd.Env = append(cmd.Env, "GOARCH="+cfg.GOARCH)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			fmt.Println(cmd.Dir)
			panic(err)
		}
		fmt.Println("done", name)
	}
}

func CreateLibForQtUi(mode string) {
	ctx := go2cpp.NewGo2cppContext(go2cpp.NewGo2cppContext_Req{
		CppBaseName:                 "m3u8d",
		EnableQtClass_RunOnUiThread: true,
		EnableQtClass_Toast:         true,
	})
	ctx.Generate1(m3u8d.RunDownload)
	ctx.Generate1(m3u8d.CloseOldEnv)
	ctx.Generate1(m3u8d.GetProgress)
	ctx.Generate1(m3u8d.GetWd)
	ctx.Generate1(m3u8d.ParseCurlStr)
	ctx.Generate1(m3u8d.RunDownload_Req_ToCurlStr)
	ctx.Generate1(m3u8d.GetFileNameFromUrl)
	if mode == "amd64-static" {
		ctx.MustCreateAmd64LibraryInDir("m3u8d-qt")
	} else if mode == "386-static" {
		ctx.MustCreate386LibraryInDir("m3u8d-qt")
	} else if mode == "amd64-shared" {
		ctx.MustCreateAmd64CSharedInDir("m3u8d-qt")
	} else {
		panic(mode)
	}
}

func WriteVersionDotRc(version string) {
	tmp := strings.Split(version, ".")
	ok := len(tmp) == 3
	for _, v := range tmp {
		vi, err := strconv.Atoi(v)
		if err != nil {
			ok = false
			break
		}
		if vi < 0 {
			ok = false
			break
		}
	}
	if ok == false {
		panic("version invalid: " + strconv.Quote(version))
	}
	tmp = append(tmp, "0")
	v1 := strings.Join(tmp, ",")
	// TODO: 这里写中文github action会乱码, 有空研究一下
	data := []byte(`IDI_ICON1 ICON "favicon.ico"

#if defined(UNDER_CE)
#include <winbase.h>
#else
#include <winver.h>
#endif

VS_VERSION_INFO VERSIONINFO
    FILEVERSION ` + v1 + `
    PRODUCTVERSION ` + v1 + `
    FILEFLAGSMASK 0x3fL
#ifdef _DEBUG
    FILEFLAGS VS_FF_DEBUG
#else
    FILEFLAGS 0x0L
#endif
    FILEOS VOS__WINDOWS32
    FILETYPE VFT_DLL
    FILESUBTYPE 0x0L
    BEGIN
        BLOCK "StringFileInfo"
        BEGIN
            BLOCK "080404b0"
            BEGIN
                VALUE "ProductVersion", "` + version + `.0\0"
                VALUE "ProductName", "m3u8 downloader\0"
                VALUE "LegalCopyright", "https://github.com/orestonce/m3u8d\0"
                VALUE "FileDescription", "m3u8 downloader\0"
           END
        END

        BLOCK "VarFileInfo"
        BEGIN
            VALUE "Translation", 0x804, 1200
        END
    END
`)
	data, err := simplifiedchinese.GBK.NewEncoder().Bytes(data)
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile("m3u8d-qt/version.rc", data, 0777)
	if err != nil {
		panic(err)
	}
}
