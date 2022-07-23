package m3u8d

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/orestonce/gopool"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TsInfo 用于保存 ts 文件的下载地址和文件名
type TsInfo struct {
	Name string
	Url  string
}

var gProgressBarTitle string
var gProgressPercent int
var gProgressPercentLocker sync.Mutex

type GetProgress_Resp struct {
	Percent int
	Title   string
	SleepTh string
}

func GetProgress() (resp GetProgress_Resp) {
	gProgressPercentLocker.Lock()
	resp.Percent = gProgressPercent
	resp.Title = gProgressBarTitle
	gProgressPercentLocker.Unlock()
	if resp.Title == "" {
		resp.Title = "正在下载"
	}
	var sleepTh int32
	gOldEnvLocker.Lock()
	if gOldEnv != nil {
		sleepTh = atomic.LoadInt32(&gOldEnv.sleepTh)
	}
	gOldEnvLocker.Unlock()
	if sleepTh > 0 {
		resp.SleepTh = "有 " + strconv.Itoa(int(sleepTh)) + "个线程正在休眠."
	}
	return resp
}

func SetProgressBarTitle(title string) {
	gProgressPercentLocker.Lock()
	defer gProgressPercentLocker.Unlock()
	gProgressBarTitle = title
}

type RunDownload_Resp struct {
	ErrMsg     string
	IsSkipped  bool
	IsCancel   bool
	SaveFileTo string
}

type RunDownload_Req struct {
	M3u8Url             string
	Insecure            bool   // "是否允许不安全的请求(默认为false)"
	SaveDir             string // "文件保存路径(默认为当前路径)"
	FileName            string // 文件名
	SkipTsCountFromHead int    // 跳过前面几个ts
	SetProxy            string
	HeaderMap           map[string][]string
}

type downloadEnv struct {
	cancelFn func()
	ctx      context.Context
	client   *http.Client
	header   http.Header
	sleepTh  int32
}

func (this *downloadEnv) RunDownload(req RunDownload_Req) (resp RunDownload_Resp) {
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
	host, err := getHost(req.M3u8Url)
	if err != nil {
		resp.ErrMsg = "getHost0: " + err.Error()
		return resp
	}
	this.header = http.Header{
		"User-Agent":      []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/79.0.3945.88 Safari/537.36"},
		"Connection":      []string{"keep-alive"},
		"Accept":          []string{"*/*"},
		"Accept-Encoding": []string{"*"},
		"Accept-Language": []string{"zh-CN,zh;q=0.9, en;q=0.8, de;q=0.7, *;q=0.5"},
		"Referer":         []string{host},
	}
	for key, valueList := range req.HeaderMap {
		this.header[key] = valueList
	}
	SetProgressBarTitle("[1/6]嗅探m3u8")
	var m3u8Body []byte
	var errMsg string
	req.M3u8Url, m3u8Body, errMsg = this.sniffM3u8(req.M3u8Url)
	if errMsg != "" {
		resp.ErrMsg = "sniffM3u8: " + errMsg
		resp.IsCancel = this.GetIsCancel()
		return resp
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
		SetProgressBarTitle("[2/6]检查是否已下载")
		latestNameFullPath, found := info.SearchVideoInDir(req.SaveDir)
		if found {
			resp.IsSkipped = true
			resp.SaveFileTo = latestNameFullPath
			return resp
		}
	}
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
	// 获取m3u8地址的内容体
	tsKey, err := this.getM3u8Key(req.M3u8Url, string(m3u8Body))
	if err != nil {
		resp.ErrMsg = "getM3u8Key: " + err.Error()
		resp.IsCancel = this.GetIsCancel()
		return resp
	}
	SetProgressBarTitle("[3/6]获取ts列表")
	tsList, errMsg := getTsList(req.M3u8Url, string(m3u8Body))
	if errMsg != "" {
		resp.ErrMsg = "获取ts列表错误: " + errMsg
		return resp
	}
	if len(tsList) <= req.SkipTsCountFromHead {
		resp.ErrMsg = "需要下载的文件为空"
		return resp
	}
	tsList = tsList[req.SkipTsCountFromHead:]
	// 下载ts
	SetProgressBarTitle("[4/6]下载ts")
	err = this.downloader(tsList, downloadDir, tsKey)
	if err != nil {
		resp.ErrMsg = "下载ts文件错误: " + err.Error()
		resp.IsCancel = this.GetIsCancel()
		return resp
	}
	DrawProgressBar(1, 1)
	var tsFileList []string
	for _, one := range tsList {
		tsFileList = append(tsFileList, filepath.Join(downloadDir, one.Name))
	}
	var tmpOutputName string
	var contentHash string
	tmpOutputName = filepath.Join(downloadDir, "all.merge.mp4")

	SetProgressBarTitle("[5/6]合并ts为mp4")
	err = MergeTsFileListToSingleMp4(MergeTsFileListToSingleMp4_Req{
		TsFileList: tsFileList,
		OutputMp4:  tmpOutputName,
		Ctx:        this.ctx,
	})
	if err != nil {
		resp.ErrMsg = "合并错误: " + err.Error()
		return resp
	}
	SetProgressBarTitle("[6/6]计算文件hash")
	contentHash = getFileSha256(tmpOutputName)
	if contentHash == "" {
		resp.ErrMsg = "无法计算摘要信息: " + tmpOutputName
		return resp
	}
	var name string
	for idx := 0; ; idx++ {
		idxS := strconv.Itoa(idx)
		if len(idxS) < 4 {
			idxS = strings.Repeat("0", 4-len(idxS)) + idxS
		}
		idxS = "_" + idxS
		if idx == 0 {
			name = filepath.Join(req.SaveDir, req.FileName+".mp4")
		} else {
			name = filepath.Join(req.SaveDir, req.FileName+idxS+".mp4")
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
	// 如果downloading目录为空,就删除掉,否则忽略
	_ = os.Remove(filepath.Join(req.SaveDir, "downloading"))
	return resp
}

var gOldEnv *downloadEnv
var gOldEnvLocker sync.Mutex

func RunDownload(req RunDownload_Req) (resp RunDownload_Resp) {
	req.SetProxy = strings.ToLower(req.SetProxy)
	env := &downloadEnv{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: req.Insecure,
				},
				DialContext: newDialContext(req.SetProxy),
			},
			Timeout: time.Second * 10,
		},
	}
	env.ctx, env.cancelFn = context.WithCancel(context.Background())

	gOldEnvLocker.Lock()
	if gOldEnv != nil {
		gOldEnv.cancelFn()
	}
	gOldEnv = env
	gOldEnvLocker.Unlock()
	resp = env.RunDownload(req)
	SetProgressBarTitle("下载进度")
	DrawProgressBar(1, 0)
	return resp
}

func CloseOldEnv() {
	gOldEnvLocker.Lock()
	defer gOldEnvLocker.Unlock()
	if gOldEnv != nil {
		gOldEnv.cancelFn()
	}
	gOldEnv = nil
}

// 获取m3u8地址的host
func getHost(Url string) (host string, err error) {
	u, err := url.Parse(Url)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

func GetWd() string {
	wd, _ := os.Getwd()
	return wd
}

// 获取m3u8加密的密钥
func (this *downloadEnv) getM3u8Key(m3u8Url string, html string) (key string, err error) {
	key = ""
	for _, line := range splitLineWithTrimSpace(html) {
		if strings.Contains(line, "#EXT-X-KEY") == false {
			continue
		}
		uriPos := strings.Index(line, "URI")
		quotationMarkPos := strings.LastIndex(line, "\"")
		keyUrl := strings.Split(line[uriPos:quotationMarkPos], "\"")[1]
		if !strings.Contains(line, "http") {
			var errMsg string
			keyUrl, errMsg = resolveRefUrl(m3u8Url, line)
			if errMsg != "" {
				return "", errors.New(errMsg)
			}
		}
		var res []byte
		res, err = this.doGetRequest(keyUrl)
		if err != nil {
			return "", err
		}
		return string(res), nil
	}
	return "", nil
}

func splitLineWithTrimSpace(s string) []string {
	tmp := strings.Split(s, "\n")
	for idx, str := range tmp {
		str = strings.TrimSpace(str)
		tmp[idx] = str
	}
	return tmp
}

func getTsList(m38uUrl string, body string) (tsList []TsInfo, errMsg string) {
	index := 0

	for _, line := range splitLineWithTrimSpace(body) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") && line != "" {
			index++
			var after string
			after, errMsg = resolveRefUrl(m38uUrl, line)
			if errMsg != "" {
				return nil, errMsg
			}
			tsList = append(tsList, TsInfo{
				Name: fmt.Sprintf("%05d.ts", index), // ts视频片段命名规则
				Url:  after,
			})
		}
	}
	return tsList, ""
}

// 下载ts文件
// @modify: 2020-08-13 修复ts格式SyncByte合并不能播放问题
func (this *downloadEnv) downloadTsFile(ts TsInfo, download_dir, key string) (err error) {
	currPath := fmt.Sprintf("%s/%s", download_dir, ts.Name)
	if isFileExists(currPath) {
		return nil
	}
	data, err := this.doGetRequest(ts.Url)
	if err != nil {
		return err
	}
	// 校验长度是否合法
	var origData []byte
	origData = data
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

func (this *downloadEnv) SleepDur(d time.Duration) {
	select {
	case <-time.After(d):
	case <-this.ctx.Done():
	}
}

func (this *downloadEnv) downloader(tsList []TsInfo, downloadDir string, key string) (err error) {
	task := gopool.NewThreadPool(1)
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
				if i > 0 {
					atomic.AddInt32(&this.sleepTh, 1)
					this.SleepDur(time.Second * time.Duration(i))
					atomic.AddInt32(&this.sleepTh, -1)
				}
				lastErr = this.downloadTsFile(ts, downloadDir, key)
				if lastErr == nil {
					break
				}
				if this.GetIsCancel() {
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
			DrawProgressBar(tsLen, downloadCount)
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
func DrawProgressBar(total int, current int) {
	if total == 0 {
		return
	}
	proportion := float32(current) / float32(total)
	gProgressPercentLocker.Lock()
	gProgressPercent = int(proportion * 100)
	title := gProgressBarTitle
	gProgressPercentLocker.Unlock()

	gShowProgressBarLocker.Lock()
	if gShowProgressBar {
		width := 50
		pos := int(proportion * float32(width))
		fmt.Printf(title+" %s%*s %6.2f%%\r", strings.Repeat("■", pos), width-pos, "", proportion*100)
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

// ============================== 加解密相关 ==============================

func PKCS7UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func AesDecrypt(crypted, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
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

func (this *downloadEnv) sniffM3u8(urlS string) (afterUrl string, content []byte, errMsg string) {
	for idx := 0; idx < 5; idx++ {
		var err error
		content, err = this.doGetRequest(urlS)
		if err != nil {
			return "", nil, err.Error()
		}
		if UrlHasSuffix(urlS, ".m3u8") {
			// 看这个是不是嵌套的m3u8
			var m3u8Url string
			containsTs := false
			for _, line := range splitLineWithTrimSpace(string(content)) {
				if strings.HasPrefix(line, "#") {
					continue
				}
				if UrlHasSuffix(line, ".m3u8") {
					m3u8Url = line
					break
				}
				if UrlHasSuffix(line, ".ts") {
					containsTs = true
					break
				}
			}
			if containsTs {
				return urlS, content, ""
			}
			if m3u8Url == "" {
				return "", nil, "未发现m3u8资源_1"
			}
			urlS, errMsg = resolveRefUrl(urlS, m3u8Url)
			if errMsg != "" {
				return "", nil, errMsg
			}
			continue
		}
		groups := regexp.MustCompile(`http[s]://[a-zA-Z0-9/\\.%_-]+.m3u8`).FindSubmatch(content)
		if len(groups) == 0 {
			return "", nil, "未发现m3u8资源_2"
		}
		urlS = string(groups[0])
	}
	return "", nil, "未发现m3u8资源_3"
}

func resolveRefUrl(baseUrl string, extUrl string) (after string, errMsg string) {
	urlObj, err := url.Parse(baseUrl)
	if err != nil {
		return "", err.Error()
	}
	lineObj, err := url.Parse(extUrl)
	if err != nil {
		return "", err.Error()
	}
	return urlObj.ResolveReference(lineObj).String(), ""
}

func UrlHasSuffix(urlS string, suff string) bool {
	urlObj, err := url.Parse(urlS)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(urlObj.Path), suff)
}

func (this *downloadEnv) doGetRequest(urlS string) (data []byte, err error) {
	req, err := http.NewRequest(http.MethodGet, urlS, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(this.ctx)
	req.Header = this.header
	resp, err := this.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return content, errors.New("resp.Status: " + resp.Status + " " + urlS)
	}
	return content, nil
}

func (this *downloadEnv) GetIsCancel() bool {
	select {
	case <-this.ctx.Done():
		return true
	default:
		return false
	}
}
