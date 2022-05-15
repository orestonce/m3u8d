//@author:llychao<lychao_vip@163.com>
//@contributor: Junyi<me@junyi.pw>
//@date:2020-02-18
//@功能:golang m3u8 video Downloader
package m3u8d

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/orestonce/gopool"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/levigross/grequests"
)

// TsInfo 用于保存 ts 文件的下载地址和文件名
type TsInfo struct {
	Name string
	Url  string
}

var gProgressPercent int
var gProgressPercentLocker sync.Mutex

func GetProgress() int {
	gProgressPercentLocker.Lock()
	tmp := gProgressPercent
	gProgressPercentLocker.Unlock()
	return tmp
}

type RunDownload_Resp struct {
	ErrMsg     string
	IsSkipped  bool
	SaveFileTo string
}

type RunDownload_Req struct {
	M3u8Url             string `json:",omitempty"`
	HostType            string `json:",omitempty"` // "设置getHost的方式(apiv1: `http(s):// + url.Host + filepath.Dir(url.Path)`; apiv2: `http(s)://+ u.Host`"
	Insecure            bool   `json:"-"`          // "是否允许不安全的请求(默认为false)"
	SaveDir             string `json:"-"`          // "文件保存路径(默认为当前路径)"
	FileName            string `json:"-"`          // 文件名
	UserFfmpegMerge     bool   `json:",omitempty"` // 使用ffmpeg合并分段视频
	SkipTsCountFromHead int    `json:",omitempty"` // 跳过前面几个ts
}

func RunDownload(req RunDownload_Req) (resp RunDownload_Resp) {
	if req.HostType == "" {
		req.HostType = "apiv1"
	}
	if req.SaveDir == "" {
		var err error
		req.SaveDir, err = os.Getwd()
		if err != nil {
			resp.ErrMsg = "os.Getwd error: " + err.Error()
			return resp
		}
	}
	if req.FileName == "" {
		req.FileName = "all"
	}
	if req.SkipTsCountFromHead < 0 {
		req.SkipTsCountFromHead = 0
	}
	id, err := req.getVideoId()
	if err != nil {
		resp.ErrMsg = "getVideoId: " + err.Error()
		return resp
	}
	info, err := cacheRead(req.SaveDir, id)
	if err != nil {
		resp.ErrMsg = "cacheRead: " + err.Error()
		return resp
	}
	if info != nil {
		latestNameFullPath, found := info.SearchVideoInDir(req.SaveDir)
		if found {
			resp.IsSkipped = true
			resp.SaveFileTo = latestNameFullPath
			return resp
		}
	}
	var ffmpegExe string
	if req.UserFfmpegMerge {
		ffmpegExe, err = SetupFfmpeg()
		if err != nil {
			resp.ErrMsg = "SetupFfmpeg error: " + err.Error()
			return resp
		}
	}
	host, err := getHost(req.M3u8Url, "apiv2")
	if err != nil {
		resp.ErrMsg = "getHost0: " + err.Error()
		return resp
	}
	ro := &grequests.RequestOptions{
		UserAgent:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/79.0.3945.88 Safari/537.36",
		RequestTimeout: 10 * time.Second, //请求头超时时间
		Headers: map[string]string{
			"Connection":      "keep-alive",
			"Accept":          "*/*",
			"Accept-Encoding": "*",
			"Accept-Language": "zh-CN,zh;q=0.9, en;q=0.8, de;q=0.7, *;q=0.5",
		},
	}
	ro.Headers["Referer"] = host
	ro.InsecureSkipVerify = req.Insecure
	if !strings.HasPrefix(req.M3u8Url, "http") || req.M3u8Url == "" {
		resp.ErrMsg = "M3u8Url not valid " + strconv.Quote(req.M3u8Url)
		return resp
	}
	downloadDir := filepath.Join(req.SaveDir, "downloading", id)
	if !isDirExists(downloadDir) {
		err = os.MkdirAll(downloadDir, os.ModePerm)
		if err != nil {
			resp.ErrMsg = "os.MkdirAll error: " + err.Error()
			return resp
		}
	}
	m3u8Host, err := getHost(req.M3u8Url, req.HostType)
	if err != nil {
		resp.ErrMsg = "getHost1: " + err.Error()
		return resp
	}
	// 获取m3u8地址的内容体
	r, err := grequests.Get(req.M3u8Url, ro)
	if err != nil {
		resp.ErrMsg = "getM3u8Body: " + err.Error()
		return resp
	}
	m3u8Body := r.String()
	ts_key, err := getM3u8Key(ro, m3u8Host, m3u8Body)
	if err != nil {
		resp.ErrMsg = "getM3u8Key: " + err.Error()
		return resp
	}
	ts_list := getTsList(m3u8Host, m3u8Body)
	if len(ts_list) <= req.SkipTsCountFromHead {
		resp.ErrMsg = "需要下载的文件为空"
		return resp
	}
	ts_list = ts_list[req.SkipTsCountFromHead:]
	// 下载ts
	err = downloader(ro, ts_list, downloadDir, ts_key)
	if err != nil {
		resp.ErrMsg = "下载ts文件错误: " + err.Error()
		return resp
	}
	DrawProgressBar(1)
	var tsFileList []string
	for _, one := range ts_list {
		tsFileList = append(tsFileList, filepath.Join(downloadDir, one.Name))
	}
	var tmpOutputName string
	var contentHash string
	if req.UserFfmpegMerge {
		tmpOutputName = filepath.Join(downloadDir, "all.merge.mp4")
		contentHash, err = mergeTsFileList_Ffmpeg(ffmpegExe, tsFileList, tmpOutputName)
	} else {
		tmpOutputName = filepath.Join(downloadDir, "all.merge.ts")
		contentHash, err = mergeTsFileList_Raw(tsFileList, tmpOutputName)
	}
	if err != nil {
		resp.ErrMsg = "合并错误: " + err.Error()
		return resp
	}
	var name string
	for idx := 0; ; idx++ {
		idxS := strconv.Itoa(idx)
		if len(idxS) < 4 {
			idxS = strings.Repeat("0", 4-len(idxS)) + idxS
		}
		idxS = "_" + idxS
		if req.UserFfmpegMerge {
			if idx == 0 {
				name = filepath.Join(req.SaveDir, req.FileName+".mp4")
			} else {
				name = filepath.Join(req.SaveDir, req.FileName+idxS+".mp4")
			}
		} else { // 直接合并的就是ts,不是mp4
			if idx == 0 {
				name = filepath.Join(req.SaveDir, req.FileName+".ts")
			} else {
				name = filepath.Join(req.SaveDir, req.FileName+idxS+".ts")
			}
		}
		if !isFileExists(name) {
			resp.SaveFileTo = name
			break
		}
		if idx > 10000 { // 超过1万就不找了
			resp.ErrMsg = "自动寻找文件名失败"
			return resp
		}
	}
	err = os.Rename(tmpOutputName, name)
	if err != nil {
		resp.ErrMsg = "重命名失败: " + err.Error()
		return resp
	}
	err = cacheWrite(req.SaveDir, id, req, resp.SaveFileTo, contentHash)
	if err != nil {
		resp.ErrMsg = "cacheWrite: " + err.Error()
		return resp
	}
	err = os.RemoveAll(downloadDir)
	if err != nil {
		resp.ErrMsg = "删除下载目录失败: " + err.Error()
		return resp
	}
	return resp
}

// 获取m3u8地址的host
func getHost(Url, ht string) (host string, err error) {
	u, err := url.Parse(Url)
	if err != nil {
		return "", err
	}
	switch ht {
	case "apiv1":
		return u.Scheme + "://" + u.Host + filepath.Dir(u.EscapedPath()), nil
	case "apiv2":
		return u.Scheme + "://" + u.Host, nil
	default:
		return "", errors.New("getHost invalid ht " + strconv.Quote(ht))
	}
}

func GetWd() string {
	wd, _ := os.Getwd()
	return wd
}

// 获取m3u8加密的密钥
func getM3u8Key(ro *grequests.RequestOptions, host, html string) (key string, err error) {
	lines := strings.Split(html, "\n")
	key = ""
	for _, line := range lines {
		if strings.Contains(line, "#EXT-X-KEY") {
			uri_pos := strings.Index(line, "URI")
			quotation_mark_pos := strings.LastIndex(line, "\"")
			key_url := strings.Split(line[uri_pos:quotation_mark_pos], "\"")[1]
			if !strings.Contains(line, "http") {
				key_url = fmt.Sprintf("%s/%s", host, key_url)
			}
			res, err := grequests.Get(key_url, ro)
			if err != nil {
				return "", err
			}
			if res.StatusCode == 200 {
				key = res.String()
			}
		}
	}
	return key, nil
}

func getTsList(host, body string) (tsList []TsInfo) {
	lines := strings.Split(body, "\n")
	index := 0

	const TS_NAME_TEMPLATE = "%05d.ts" // ts视频片段命名规则

	var ts TsInfo
	for _, line := range lines {
		if !strings.HasPrefix(line, "#") && line != "" {
			//有可能出现的二级嵌套格式的m3u8,请自行转换！
			index++
			if strings.HasPrefix(line, "http") {
				ts = TsInfo{
					Name: fmt.Sprintf(TS_NAME_TEMPLATE, index),
					Url:  line,
				}
				tsList = append(tsList, ts)
			} else {
				ts = TsInfo{
					Name: fmt.Sprintf(TS_NAME_TEMPLATE, index),
					Url:  fmt.Sprintf("%s/%s", host, line),
				}
				ts.Url = strings.ReplaceAll(ts.Url, `\`, `/`)
				tsList = append(tsList, ts)
			}
		}
	}
	return
}

// 下载ts文件
// @modify: 2020-08-13 修复ts格式SyncByte合并不能播放问题
func downloadTsFile(ro *grequests.RequestOptions, ts TsInfo, download_dir, key string) (err error) {
	currPath := fmt.Sprintf("%s/%s", download_dir, ts.Name)
	if isFileExists(currPath) {
		return nil
	}
	res, err := grequests.Get(ts.Url, ro)
	if err != nil {
		return err
	}
	if !res.Ok {
		return errors.New("!res.Ok")
	}
	// 校验长度是否合法
	var origData []byte
	origData = res.Bytes()
	contentLen := 0
	contentLenStr := res.Header.Get("Content-Length")
	if contentLenStr != "" {
		contentLen, _ = strconv.Atoi(contentLenStr)
	}
	if len(origData) == 0 || (contentLen > 0 && len(origData) < contentLen) || res.Error != nil {
		msg := ""
		if res.Error != nil {
			msg = res.Error.Error()
		}
		return errors.New("[warn] File: " + ts.Name + "res origData invalid or err：" + msg)
	}
	// 解密出视频 ts 源文件
	if key != "" {
		//解密 ts 文件，算法：aes 128 cbc pack5
		origData, err = AesDecrypt(origData, []byte(key))
		if err != nil {
			return err
		}
	}
	// https://en.wikipedia.org/wiki/MPEG_transport_stream
	// Some TS files do not start with SyncByte 0x47, they can not be played after merging,
	// Need to remove the bytes before the SyncByte 0x47(71).
	syncByte := uint8(71) //0x47
	bLen := len(origData)
	for j := 0; j < bLen; j++ {
		if origData[j] == syncByte {
			origData = origData[j:]
			break
		}
	}
	tmpPath := currPath + ".tmp"
	err = ioutil.WriteFile(tmpPath, origData, 0666)
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, currPath)
}

func downloader(ro *grequests.RequestOptions, tsList []TsInfo, downloadDir string, key string) (err error) {
	task := gopool.NewThreadPool(8)
	tsLen := len(tsList)
	downloadCount := 0
	var locker sync.Mutex

	for _, ts := range tsList {
		ts := ts
		task.AddJob(func() {
			var lastErr error
			for i := 0; i < 5; i++ {
				locker.Lock()
				if err != nil {
					locker.Unlock()
					return
				}
				locker.Unlock()
				lastErr = downloadTsFile(ro, ts, downloadDir, key)
				if lastErr == nil {
					break
				}
			}
			if lastErr != nil {
				locker.Lock()
				if err == nil {
					err = lastErr
				}
				locker.Unlock()
			}
			locker.Lock()
			downloadCount++
			DrawProgressBar(float32(downloadCount) / float32(tsLen))
			locker.Unlock()
		})
	}
	task.CloseAndWait()

	return err
}

var gShowProgressBar bool
var gShowProgressBarLocker sync.Mutex

func SetShowProgressBar() {
	gShowProgressBarLocker.Lock()
	gShowProgressBar = true
	gShowProgressBarLocker.Unlock()
}

// 进度条
func DrawProgressBar(proportion float32) {
	gProgressPercentLocker.Lock()
	gProgressPercent = int(proportion * 100)
	gProgressPercentLocker.Unlock()

	gShowProgressBarLocker.Lock()
	if gShowProgressBar {
		width := 50
		pos := int(proportion * float32(width))
		fmt.Printf("[下载进度] %s%*s %6.2f%%\r", strings.Repeat("■", pos), width-pos, "", proportion*100)
	}
	gShowProgressBarLocker.Unlock()
}

func isFileExists(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && stat.Mode().IsRegular()
}

func isDirExists(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && stat.IsDir()
}

// 判断文件是否存在
func pathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// ============================== 加解密相关 ==============================

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func AesEncrypt(origData, key []byte, ivs ...[]byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	var iv []byte
	if len(ivs) == 0 {
		iv = key
	} else {
		iv = ivs[0]
	}
	origData = PKCS7Padding(origData, blockSize)
	blockMode := cipher.NewCBCEncrypter(block, iv[:blockSize])
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	return crypted, nil
}

func AesDecrypt(crypted, key []byte, ivs ...[]byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	var iv []byte
	if len(ivs) == 0 {
		iv = key
	} else {
		iv = ivs[0]
	}
	blockMode := cipher.NewCBCDecrypter(block, iv[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	origData = PKCS7UnPadding(origData)
	return origData, nil
}

func getFileSha256(targetFile string) (v string) {
	fin, err := os.Open(targetFile)
	if err != nil {
		return ""
	}
	defer fin.Close()

	sha256Obj := sha256.New()
	_, err = io.Copy(sha256Obj, fin)
	if err != nil {
		return ""
	}
	tmp := sha256Obj.Sum(nil)
	return hex.EncodeToString(tmp[:])
}

func getStringSha256(s string) (v string) {
	tmp := sha256.Sum256([]byte(s))
	return hex.EncodeToString(tmp[:])
}
