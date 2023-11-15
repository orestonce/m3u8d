package m3u8d

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/orestonce/gopool"
)

// TsInfo 用于保存 ts 文件的下载地址和文件名
type TsInfo struct {
	Name string
	Url  string
	Seq  uint64 // 如果是aes加密并且没有iv, 这个seq需要充当iv
}

type GetProgress_Resp struct {
	Percent   int
	Title     string
	StatusBar string
}

var PNG_SIGN = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func GetProgress() (resp GetProgress_Resp) {
	var sleepTh int32
	var speedV string

	gOldEnvLocker.Lock()
	if gOldEnv != nil {
		sleepTh = atomic.LoadInt32(&gOldEnv.sleepTh)
		gOldEnv.progressLocker.Lock()
		resp.Percent = gOldEnv.progressPercent
		resp.Title = gOldEnv.progressBarTitle
		gOldEnv.progressLocker.Unlock()
		if resp.Title == "" {
			resp.Title = "正在下载"
		}
		speedV = gOldEnv.speedRecent5sGetAndUpdate()
	}
	gOldEnvLocker.Unlock()
	resp.StatusBar = speedV
	if sleepTh > 0 {
		if resp.StatusBar != "" {
			resp.StatusBar += ", "
		}
		resp.StatusBar += "有 " + strconv.Itoa(int(sleepTh)) + "个线程正在休眠."
	}
	return resp
}

func (this *downloadEnv) SetProgressBarTitle(title string) {
	this.progressLocker.Lock()
	this.progressBarTitle = title
	this.progressLocker.Unlock()
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
	SkipRemoveTs        bool
	ProgressBarShow     bool
	ThreadCount         int
}

type downloadEnv struct {
	cancelFn         func()
	ctx              context.Context
	nowClient        *http.Client
	header           http.Header
	sleepTh          int32
	progressLocker   sync.Mutex
	progressBarTitle string
	progressPercent  int
	progressBarShow  bool
	speedBytesLocker sync.Mutex
	speedBeginTime   time.Time
	speedBytesMap    map[time.Time]int64
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
		req.FileName = GetFileNameFromUrl(req.M3u8Url)
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
	this.SetProgressBarTitle("[1/6]嗅探m3u8")
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
		this.SetProgressBarTitle("[2/6]检查是否已下载")
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

	downloadingFilePath := filepath.Join(downloadDir, "downloading.txt")
	if !isFileExists(downloadingFilePath) {
		err = ioutil.WriteFile(downloadingFilePath, []byte(req.M3u8Url), 0666)
		if err != nil {
			resp.ErrMsg = "os.WriteUrl error: " + err.Error()
			return resp
		}
	}

	beginSeq := parseBeginSeq(m3u8Body)
	// 获取m3u8地址的内容体
	encInfo, err := this.getEncryptInfo(req.M3u8Url, string(m3u8Body))
	if err != nil {
		resp.ErrMsg = "getEncryptInfo: " + err.Error()
		resp.IsCancel = this.GetIsCancel()
		return resp
	}
	this.SetProgressBarTitle("[3/6]获取ts列表")
	tsList, errMsg := getTsList(beginSeq, req.M3u8Url, string(m3u8Body))
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
	this.SetProgressBarTitle("[4/6]下载ts")
	this.speedSetBegin()
	err = this.downloader(tsList, downloadDir, encInfo, req.ThreadCount)
	this.speedClearBytes()
	if err != nil {
		resp.ErrMsg = "下载ts文件错误: " + err.Error()
		resp.IsCancel = this.GetIsCancel()
		return resp
	}
	this.DrawProgressBar(1, 1)
	var tsFileList []string
	for _, one := range tsList {
		tsFileList = append(tsFileList, filepath.Join(downloadDir, one.Name))
	}
	var tmpOutputName string
	var contentHash string
	tmpOutputName = filepath.Join(downloadDir, "all.merge.mp4")

	this.SetProgressBarTitle("[5/6]合并ts为mp4")
	this.speedSetBegin()
	err = MergeTsFileListToSingleMp4(MergeTsFileListToSingleMp4_Req{
		TsFileList: tsFileList,
		OutputMp4:  tmpOutputName,
		Ctx:        this.ctx,
	})
	this.speedClearBytes()
	if err != nil {
		resp.ErrMsg = "合并错误: " + err.Error()
		return resp
	}
	this.SetProgressBarTitle("[6/6]计算文件hash")
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
	if req.SkipRemoveTs == false {
		err = os.RemoveAll(downloadDir)
		if err != nil {
			resp.ErrMsg = "删除下载目录失败: " + err.Error()
			return resp
		}
		// 如果downloading目录为空,就删除掉,否则忽略
		_ = os.Remove(filepath.Join(req.SaveDir, "downloading"))
	}
	return resp
}

func parseBeginSeq(body []byte) uint64 {
	data := M3u8Parse(string(body))
	seq := data.GetPart(`#EXT-X-MEDIA-SEQUENCE`).TextFull
	u, err := strconv.ParseUint(seq, 10, 64)
	if err != nil {
		return 0
	}
	return u
}

var gOldEnv *downloadEnv
var gOldEnvLocker sync.Mutex

func RunDownload(req RunDownload_Req) (resp RunDownload_Resp) {
	var proxyUrlObj *url.URL
	req.SetProxy, proxyUrlObj, resp.ErrMsg = SetProxyFormat(req.SetProxy)
	if resp.ErrMsg != "" {
		return resp
	}
	env := &downloadEnv{
		nowClient: &http.Client{
			Timeout: time.Second * 20,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: req.Insecure,
				},
				Proxy: http.ProxyURL(proxyUrlObj),
			},
		},
		speedBytesMap:   map[time.Time]int64{},
		progressBarShow: req.ProgressBarShow,
	}
	env.ctx, env.cancelFn = context.WithCancel(context.Background())

	gOldEnvLocker.Lock()
	if gOldEnv != nil {
		gOldEnv.cancelFn()
	}
	gOldEnv = env
	gOldEnvLocker.Unlock()
	resp = env.RunDownload(req)
	env.SetProgressBarTitle("下载进度")
	env.DrawProgressBar(1, 0)
	return resp
}

func getOldEnv() *downloadEnv {
	gOldEnvLocker.Lock()
	defer gOldEnvLocker.Unlock()
	return gOldEnv
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

// 获取m3u8加密的密钥, 可能存在iv
func (this *downloadEnv) getEncryptInfo(m3u8Url string, html string) (info *EncryptInfo, err error) {
	keyPart := M3u8Parse(html).GetPart("#EXT-X-KEY")
	uri := keyPart.KeyValue["URI"]
	if uri == "" {
		return nil, nil
	}
	method := keyPart.KeyValue["METHOD"]
	if method == EncryptMethod_NONE {
		return nil, nil
	}
	keyUrl, errMsg := resolveRefUrl(m3u8Url, uri)
	if errMsg != "" {
		return nil, errors.New(errMsg)
	}
	var res []byte
	res, err = this.doGetRequest(keyUrl)
	if err != nil {
		return nil, err
	}
	if method == EncryptMethod_AES128 && len(res) != 16 { // Aes 128
		return nil, errors.New("getEncryptInfo invalid key " + strconv.Quote(string(res)))
	}
	var iv []byte
	ivs := keyPart.KeyValue["IV"]
	if ivs != "" {
		iv, err = hex.DecodeString(strings.TrimPrefix(ivs, "0x"))
		if err != nil {
			return nil, err
		}
	}
	return &EncryptInfo{
		Method: method,
		Key:    res,
		Iv:     iv,
	}, nil
}

func splitLineWithTrimSpace(s string) []string {
	tmp := strings.Split(s, "\n")
	for idx, str := range tmp {
		str = strings.TrimSpace(str)
		tmp[idx] = str
	}
	return tmp
}

func getTsList(beginSeq uint64, m38uUrl string, body string) (tsList []TsInfo, errMsg string) {
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
				Seq:  beginSeq + uint64(index-1),
			})
		}
	}
	return tsList, ""
}

// 下载ts文件
// @modify: 2020-08-13 修复ts格式SyncByte合并不能播放问题
func (this *downloadEnv) downloadTsFile(ts TsInfo, download_dir string, encInfo *EncryptInfo) (err error) {
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
	if encInfo != nil {
		//解密 ts 文件，算法：aes 128 cbc pack5
		origData, err = AesDecrypt(ts.Seq, origData, encInfo)
		if err != nil {
			return err
		}
	}

	// Detect Fake png file
	if bytes.HasPrefix(origData, PNG_SIGN) {
		origData = origData[8:]
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
	err = os.Rename(tmpPath, currPath)
	if err != nil {
		return err
	}
	this.speedAddBytes(len(origData))
	return nil
}

func (this *downloadEnv) SleepDur(d time.Duration) {
	select {
	case <-time.After(d):
	case <-this.ctx.Done():
	}
}

func (this *downloadEnv) downloader(tsList []TsInfo, downloadDir string, encInfo *EncryptInfo, threadCount int) (err error) {
	if threadCount <= 0 || threadCount > 1000 {
		return errors.New("downloadEnv.threadCount invalid: " + strconv.Itoa(threadCount))
	}
	task := gopool.NewThreadPool(threadCount)
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
				lastErr = this.downloadTsFile(ts, downloadDir, encInfo)
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
			this.DrawProgressBar(tsLen, downloadCount)
			locker.Unlock()
		})
	}
	task.CloseAndWait()

	return err
}

// 进度条
func (this *downloadEnv) DrawProgressBar(total int, current int) {
	if total == 0 {
		return
	}
	proportion := float32(current) / float32(total)

	this.progressLocker.Lock()
	this.progressPercent = int(proportion * 100)
	title := this.progressBarTitle
	if this.progressBarShow {
		width := 50
		pos := int(proportion * float32(width))
		fmt.Printf(title+" %s%*s %6.2f%%\r", strings.Repeat("■", pos), width-pos, "", proportion*100)
	}
	this.progressLocker.Unlock()
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

func AesDecrypt(seq uint64, crypted []byte, encInfo *EncryptInfo) ([]byte, error) {
	block, err := aes.NewCipher(encInfo.Key)
	if err != nil {
		return nil, err
	}
	iv := encInfo.Iv
	if len(iv) == 0 {
		if encInfo.Method == EncryptMethod_AES128 {
			iv = make([]byte, 16)
			binary.BigEndian.PutUint64(iv[8:], seq)
		} else {
			return nil, errors.New("AesDecrypt method not support " + strconv.Quote(encInfo.Method))
		}
	}
	blockMode := cipher.NewCBCDecrypter(block, iv)
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	length := len(origData)
	unpadding := int(origData[length-1])
	if length-unpadding < 0 {
		return nil, fmt.Errorf(`invalid length of unpadding %v - %v`, length, unpadding)
	}
	return origData[:(length - unpadding)], nil
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
				// Support fake png ts
				if UrlHasSuffix(line, ".png") {
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
	resp, err := this.nowClient.Do(req)
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

func GetFileNameFromUrl(u string) string {
	urlObj, err := url.Parse(u)
	if err != nil || urlObj.Scheme == "" {
		return "all"
	}
	name := path.Base(urlObj.Path)
	if name == "" {
		return "all"
	}
	ext := path.Ext(name)
	if len(name) > len(ext) {
		return strings.TrimSuffix(name, ext)
	}
	return name
}
