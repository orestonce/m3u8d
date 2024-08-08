package main

import (
	"fmt"
	"github.com/orestonce/go2cpp"
	"github.com/orestonce/m3u8d"
	"github.com/orestonce/m3u8d/m3u8dcpp"
	"golang.org/x/text/encoding/simplifiedchinese"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

func main() {
	if os.Getenv("GITHUB_ACTIONS") == "" { // 本地编译
		BuildCliBinaryAllVersion()             // 编译命令行版本
		CreateLibForQtUi("amd64", "c-archive") // 创建Qt需要使用的.a库文件
	} else if len(os.Args) >= 2 { // github actions 编译
		switch os.Args[1] {
		case "build-cli":
			BuildCliBinaryAllVersion() // 编译命令行版本
		case "update-qt-version-rc":
			refName := os.Getenv("GITHUB_REF_NAME")
			WriteVersionDotRc(refName)
		case "create-qt-lib":
			goarch := os.Args[2]
			buildMode := os.Args[3]
			CreateLibForQtUi(goarch, buildMode)
		default:
			fmt.Println("help:")
			fmt.Println("build-cli")
			fmt.Println("update-qt-version-rc")
			fmt.Println("create-qt-lib [goarch] [buildMode]")
			fmt.Println("              goarch: 386, amd64, arm64...")
			fmt.Println("              buildMode: c-shared, c-archive")
		}
		//if runtime.GOOS == "darwin" { // 编译darwin版本的dmg
		//	CreateLibForQtUi("arm64", "c-shared")
		//} else { // 编译windows版本的exe
		//	CreateLibForQtUi("386", "c-archive")
		//}
	}
}

type BuildCfg struct {
	GOOS   string
	GOARCH string
	Ext    string
}

func BuildCliBinaryAllVersion() {
	var list = []BuildCfg{
		{
			GOOS:   "linux",
			GOARCH: "386",
		},
		{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
		{
			GOOS:   "linux",
			GOARCH: "arm",
		},
		{
			GOOS:   "linux",
			GOARCH: "arm64",
		},
		{
			GOOS:   "android",
			GOARCH: "arm64",
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
		{
			GOOS:   "windows",
			GOARCH: "amd64",
			Ext:    ".exe",
		},
	}
	for _, cfg := range list {
		BuildCliVersion(cfg)
	}
}

func BuildCliVersion(cfg BuildCfg) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	name := "m3u8d_" + cfg.GOOS + "_" + cfg.GOARCH + "_cli" + cfg.Ext
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

func CreateLibForQtUi(goarch string, buildmode string) {
	ctx := go2cpp.NewGo2cppContext(go2cpp.NewGo2cppContext_Req{
		CppBaseName:                 "m3u8d",
		EnableQtClass_RunOnUiThread: true,
		EnableQtClass_Toast:         true,
	})
	ctx.Generate1(m3u8dcpp.StartDownload)
	ctx.Generate1(m3u8dcpp.CloseOldEnv)
	ctx.Generate1(m3u8dcpp.GetStatus)
	ctx.Generate1(m3u8dcpp.WaitDownloadFinish)
	ctx.Generate1(m3u8d.GetWd)
	ctx.Generate1(m3u8d.ParseCurlStr)
	ctx.Generate1(m3u8d.RunDownload_Req_ToCurlStr)
	ctx.Generate1(m3u8d.GetFileNameFromUrl)
	ctx.Generate1(m3u8dcpp.MergeTsDir)
	ctx.Generate1(m3u8dcpp.MergeStop)
	ctx.Generate1(m3u8dcpp.MergeGetProgressPercent)
	ctx.Generate1(m3u8d.FindUrlInStr)

	var optionList []string
	if runtime.GOOS == "darwin" {
		optionList = []string{"-ldflags=-w"}
	}
	ctx.MustCreateLibrary("m3u8d-qt", goarch, buildmode, optionList...)
}

var gVersionReg = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func WriteVersionDotRc(version string) {
	groups := gVersionReg.FindStringSubmatch(version)
	if len(groups) == 0 {
		panic("version invalid: " + strconv.Quote(version))
	}
	versionPart3 := append(groups[1:], "0")
	v1 := strings.Join(versionPart3, ",")
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
