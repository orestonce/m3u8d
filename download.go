package m3u8d

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/orestonce/gopool"
)

// TsInfo 用于保存 ts 文件的下载地址和文件名
type TsInfo struct {
	Name                        string
	Url                         string
	Seq                         uint64  // 如果是aes加密并且没有iv, 这个seq需要充当iv
	TimeSec                     float64 // 此ts片段占用多少秒
	Between_EXT_X_DISCONTINUITY bool
	SkipByHttpCode              bool
	HttpCode                    int
}

type GetStatus_Resp struct {
	Percent       int
	Title         string
	StatusBar     string
	IsDownloading bool
	IsCancel      bool
	ErrMsg        string
	IsSkipped     bool
	SaveFileTo    string
}

var PNG_SIGN = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

type StartDownload_Req struct {
	M3u8Url                  string
	Insecure                 bool                // "是否允许不安全的请求(默认为false)"
	SaveDir                  string              // "文件保存路径(默认为当前路径)"
	FileName                 string              // 文件名
	SkipTsExpr               string              // 跳过ts信息，ts编号从1开始，可以以逗号","为分隔符跳过多部分ts，例如: 1,92-100 表示跳过第1号ts、跳过92到100号ts
	SetProxy                 string              //代理
	HeaderMap                map[string][]string // 自定义http头信息
	SkipRemoveTs             bool                // 不删除ts文件
	ProgressBarShow          bool                // 在控制台打印进度条
	ThreadCount              int                 // 线程数
	SkipCacheCheck           bool                // 不缓存已下载的m3u8的文件信息
	SkipMergeTs              bool                // 不合并ts为mp4
	Skip_EXT_X_DISCONTINUITY bool                // 跳过 #EXT-X-DISCONTINUITY 标签包裹的ts
	DebugLog                 bool                // 调试日志
	TsTempDir                string              // 临时ts文件目录
	UseServerSideTime        bool                // 使用服务端提供的文件时间
}

type DownloadEnv struct {
	cancelFn      func()
	ctx           context.Context
	nowClient     *http.Client
	header        http.Header
	sleepTh       int32
	status        SpeedStatus
	logFile       *os.File
	logFileLocker sync.Mutex
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

// 获取m3u8地址的host
func getHost(Url string) (host string, err error) {
	u, err := url.Parse(Url)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

// 获取m3u8加密的密钥, 可能存在iv
func (this *DownloadEnv) getEncryptInfo(m3u8Url string, html string) (info *EncryptInfo, err error) {
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
	var keyContent []byte
	var httpResp *http.Response
	keyContent, httpResp, err = this.doGetRequest(keyUrl, true)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != 200 {
		return nil, errors.New("getEncryptInfo httpCode error " + strconv.Itoa(httpResp.StatusCode))
	}
	if method == EncryptMethod_AES128 && len(keyContent) != 16 { // Aes 128
		return nil, errors.New("getEncryptInfo invalid key " + strconv.Quote(string(keyContent)))
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
		Key:    keyContent,
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

	var between_EXT_X_DISCONTINUITY = false // 正在跳过 #EXT-X-DISCONTINUITY 标签间的ts
	var timeSec float64
	var reExtInf = regexp.MustCompile(`^#EXTINF *: *([0-9.]+)`)

	for _, line := range splitLineWithTrimSpace(body) {
		line = strings.TrimSpace(line)
		if line == "#EXT-X-DISCONTINUITY" {
			if len(tsList) > 0 {
				between_EXT_X_DISCONTINUITY = !between_EXT_X_DISCONTINUITY
			}
			continue
		}
		if groups := reExtInf.FindStringSubmatch(line); len(groups) > 0 {
			f, err := strconv.ParseFloat(groups[1], 64)
			if err == nil {
				timeSec = f
			}
			continue
		}
		if !strings.HasPrefix(line, "#") && line != "" {
			index++
			var after string
			after, errMsg = resolveRefUrl(m38uUrl, line)
			if errMsg != "" {
				return nil, errMsg
			}
			tsList = append(tsList, TsInfo{
				Name:                        fmt.Sprintf("%05d.ts", index), // ts视频片段命名规则
				Url:                         after,
				Seq:                         beginSeq + uint64(index-1),
				Between_EXT_X_DISCONTINUITY: between_EXT_X_DISCONTINUITY,
				TimeSec:                     timeSec,
			})
			timeSec = 0
		}
	}
	return tsList, ""
}

// 下载ts文件
// @modify: 2020-08-13 修复ts格式SyncByte合并不能播放问题
func (this *DownloadEnv) downloadTsFile(ts *TsInfo, skipInfo SkipTsInfo, downloadDir string, encInfo *EncryptInfo, useServerSideTime bool) (err error) {
	currPath := filepath.Join(downloadDir, ts.Name)
	var stat os.FileInfo
	stat, err = os.Stat(currPath)
	if err == nil && stat.Mode().IsRegular() {
		this.status.SpeedAdd1Block(stat.ModTime(), int(stat.Size()))
		return nil
	}
	beginTime := time.Now()
	data, httpResp, err := this.doGetRequest(ts.Url, false)
	if err != nil {
		return err
	}
	if httpResp.StatusCode != 200 {
		if len(skipInfo.HttpCodeList) > 0 && isInIntSlice(httpResp.StatusCode, skipInfo.HttpCodeList) {
			this.status.SpeedAdd1Block(beginTime, 0)
			ts.SkipByHttpCode = true
			ts.HttpCode = httpResp.StatusCode
			this.logToFile("skip ts " + strconv.Quote(ts.Name) + " byHttpCode: " + strconv.Itoa(httpResp.StatusCode))
			return nil
		}
		return errors.New(`invalid http status code: ` + strconv.Itoa(httpResp.StatusCode) + ` url: ` + ts.Url)
	}
	var mTime time.Time
	if mStr := httpResp.Header.Get("Last-Modified"); mStr != "" && useServerSideTime {
		this.logToFile("get mtime " + strconv.Quote(mStr))
		mTime, err = time.Parse(time.RFC1123, mStr)
		// 这个错误不重要, 所以只记录日志
		if err != nil {
			this.logToFile("parse mtime error " + err.Error())
			mTime = time.Time{}
		}
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
	if mTime.IsZero() == false {
		err = os.Chtimes(currPath, mTime, mTime)
		// 这个错误不重要, 所以只记录日志
		if err != nil {
			this.logToFile("os.Chtimes error " + err.Error())
		}
	}
	this.status.SpeedAdd1Block(beginTime, len(origData))
	return nil
}

func isInIntSlice(i int, list []int) bool {
	for _, one := range list {
		if i == one {
			return true
		}
	}
	return false
}

func (this *DownloadEnv) SleepDur(d time.Duration) {
	select {
	case <-time.After(d):
	case <-this.ctx.Done():
	}
}

func (this *DownloadEnv) downloader(tsList []TsInfo, skipInfo SkipTsInfo, downloadDir string, encInfo *EncryptInfo, req StartDownload_Req) (err error) {
	if req.ThreadCount <= 0 || req.ThreadCount > 1000 {
		return errors.New("DownloadEnv.threadCount invalid: " + strconv.Itoa(req.ThreadCount))
	}
	task := gopool.NewThreadPool(req.ThreadCount)
	var locker sync.Mutex

	this.status.ResetTotalBlockCount(len(tsList))

	for idx := range tsList {
		ts := &tsList[idx]
		task.AddJob(func() {
			var lastErr error
			for i := 0; i < 5; i++ {
				if this.GetIsCancel() {
					break
				}

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
				lastErr = this.downloadTsFile(ts, skipInfo, downloadDir, encInfo, req.UseServerSideTime)
				if lastErr == nil {
					break
				}
			}
			if lastErr != nil {
				locker.Lock()
				if err == nil {
					err = fmt.Errorf("%v: %v", ts.Name, lastErr.Error())
				}
				locker.Unlock()

				this.status.setTsNotWriteReason(ts, "download: "+lastErr.Error())
			} else if ts.SkipByHttpCode {
				this.status.setTsNotWriteReason(ts, "skipByHttpCode: "+strconv.Itoa(ts.HttpCode))
			} else if this.GetIsCancel() {
				this.status.setTsNotWriteReason(ts, "用户取消")
			}
		})
	}
	task.CloseAndWait()

	return err
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

func AesDecrypt(seq uint64, encrypted []byte, encInfo *EncryptInfo) ([]byte, error) {
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
	if len(iv) == 0 || len(encrypted)%len(iv) != 0 {
		return nil, errors.New("AesDecrypt invalid encrypted data len " + strconv.Itoa(len(encrypted)))
	}
	origData := make([]byte, len(encrypted))
	blockMode.CryptBlocks(origData, encrypted)
	length := len(origData)
	unpadding := int(origData[length-1])
	if length-unpadding < 0 {
		return nil, fmt.Errorf(`invalid length of unpadding %v - %v`, length, unpadding)
	}
	return origData[:(length - unpadding)], nil
}

func (this *DownloadEnv) sniffM3u8(urlS string) (afterUrl string, content []byte, errMsg string) {
	for idx := 0; idx < 5; idx++ {
		var err error
		var httpResp *http.Response
		content, httpResp, err = this.doGetRequest(urlS, true)
		if err != nil {
			return "", nil, err.Error()
		}
		if httpResp.StatusCode != 200 {
			return "", nil, "invalid httpCode " + strconv.Itoa(httpResp.StatusCode)
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
				for _, supportSuffix := range []string{".ts", ".png", ".jpeg", ".jpg"} {
					if UrlHasSuffix(line, supportSuffix) {
						containsTs = true
						break
					}
				}
				if containsTs {
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

func (this *DownloadEnv) doGetRequest(urlS string, dumpRespBody bool) (data []byte, resp *http.Response, err error) {
	req, err := http.NewRequest(http.MethodGet, urlS, nil)
	if err != nil {
		return nil, nil, err
	}
	req = req.WithContext(this.ctx)
	req.Header = this.header

	var logBuf *bytes.Buffer

	this.logFileLocker.Lock()
	if this.logFile != nil {
		logBuf = bytes.NewBuffer(nil)
		logBuf.WriteString("http get url " + strconv.Quote(urlS) + "\n")
		reqBytes, _ := httputil.DumpRequest(req, false)
		logBuf.WriteString("httpReq:\n" + string(reqBytes) + "\n")
	}
	this.logFileLocker.Unlock()

	beginTime := time.Now()

	resp, err = this.nowClient.Do(req)
	if logBuf != nil && resp != nil {
		respBytes, _ := httputil.DumpResponse(resp, false)
		logBuf.WriteString("httpResp:\n" + string(respBytes) + "\n")
		logBuf.WriteString("time1: " + time.Since(beginTime).String() + "\n")
		beginTime = time.Now()
	}

	if err != nil {
		if logBuf != nil {
			logBuf.WriteString("error1:" + err.Error() + "\n")
			this.logToFile(logBuf.String())
		}
		return nil, nil, err
	}
	defer resp.Body.Close()

	var content []byte
	var readCloser io.ReadCloser

	contentEncoding := resp.Header.Get("Content-Encoding")
	switch contentEncoding {
	case "gzip":
		readCloser, err = gzip.NewReader(resp.Body)
		if err != nil {
			err = errors.New("error2: gzip.new error " + err.Error())
			if logBuf != nil {
				logBuf.WriteString(err.Error() + "\n")
				this.logToFile(logBuf.String())
			}
			return nil, nil, err
		}
		defer readCloser.Close()
	case "deflate":
		readCloser = flate.NewReader(resp.Body)
		defer readCloser.Close()
	case "":
		readCloser = resp.Body
	default:
		err = errors.New("error3: unsupported Content-Encoding " + strconv.Quote(contentEncoding))
		if logBuf != nil {
			logBuf.WriteString(err.Error() + "\n")
			this.logToFile(logBuf.String())
		}
		return nil, nil, err
	}
	content, err = this.status.SpeedReadAll(readCloser)
	if logBuf != nil {
		logBuf.WriteString("time4: " + time.Since(beginTime).String() + ", bytes: " + strconv.Itoa(len(content)) + "\n")
	}
	if err != nil {
		if logBuf != nil {
			logBuf.WriteString("error4:" + err.Error() + "\n")
			this.logToFile(logBuf.String())
		}
		return nil, nil, err
	}
	if logBuf != nil && dumpRespBody {
		logBuf.WriteString("httpRespBody:\n" + string(content))
	}
	if logBuf != nil {
		this.logToFile(logBuf.String())
	}
	return content, resp, nil
}

func (this *DownloadEnv) logToFile(body string) {
	this.logFileLocker.Lock()
	defer this.logFileLocker.Unlock()

	if this.logFile == nil {
		return
	}

	timeStr := time.Now().Format("2006-01-02_15:04:05")
	this.logFile.WriteString("===>time: " + timeStr + "\n")
	this.logFile.WriteString(body)
	if strings.HasSuffix(body, "\n") == false {
		this.logFile.WriteString("\n")
	}
}

func (this *DownloadEnv) logToFile_TsNotWriteReason() {
	var list []tsNotWriteReasonUnit

	this.status.Locker.Lock()
	for _, one := range this.status.tsNotWriteReasonMap {
		list = append(list, one)
	}
	this.status.Locker.Unlock()

	if len(list) == 0 {
		return
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].fileName < list[j].fileName
	})

	this.logFileLocker.Lock()
	defer this.logFileLocker.Unlock()

	if this.logFile == nil {
		return
	}

	this.logFile.WriteString("---- ERROR TS ----\n")
	for idx, one := range list {
		if idx > 0 {
			this.logFile.WriteString("\n")
		}
		this.logFile.WriteString(one.fileName)
		this.logFile.WriteString("\n")
		this.logFile.WriteString(one.downloadUrl)
		this.logFile.WriteString("\n")
		this.logFile.WriteString(one.reason)
		this.logFile.WriteString("\n")
	}
	this.logFile.WriteString("---- END ----\n")
}

func (this *DownloadEnv) GetIsCancel() bool {
	this.status.Locker.Lock()
	defer this.status.Locker.Unlock()
	return IsContextCancel(this.ctx)
}

func GetWd() string {
	wd, _ := os.Getwd()
	return wd
}

func (this *DownloadEnv) CloseEnv() {
	this.status.Locker.Lock()
	if !this.status.IsRunning {
		this.status.Locker.Unlock()
		return
	}

	if this.cancelFn != nil {
		this.cancelFn()
	}
	ctx := this.ctx
	this.status.Locker.Unlock()

	if ctx == nil {
		return
	}
	<-ctx.Done()
}

func (this *DownloadEnv) logFileClose() {
	this.logFileLocker.Lock()
	defer this.logFileLocker.Unlock()

	if this.logFile != nil {
		this.logFile.Close()
		this.logFile = nil
	}
}

func (this *DownloadEnv) writeFfmpegCmd(downloadingDir string, list []TsInfo) bool {
	const listFileName = "filelist.txt"

	var fileListLog bytes.Buffer
	for _, one := range list {
		if one.SkipByHttpCode {
			continue
		}
		fileListLog.WriteString("file " + one.Name + "\n")
	}
	err := os.WriteFile(filepath.Join(downloadingDir, listFileName), fileListLog.Bytes(), 0777)
	if err != nil {
		this.setErrMsg("写入" + listFileName + "失败, " + err.Error())
		return false
	}

	var ffmpegCmdContent = "ffmpeg -f concat -i " + listFileName + " -c copy -y output.mp4"
	var ffmpegCmdFileName = "merge-by-ffmpeg"
	if runtime.GOOS == `windows` {
		ffmpegCmdContent += "\r\npause"
		ffmpegCmdFileName += ".bat"
	} else {
		ffmpegCmdFileName += ".sh"
	}

	err = os.WriteFile(filepath.Join(downloadingDir, ffmpegCmdFileName), []byte(ffmpegCmdContent), 0777)
	if err != nil {
		this.setErrMsg("写入" + ffmpegCmdFileName + "失败, " + err.Error())
		return false
	}

	return true
}

func IsContextCancel(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	select {
	case <-ctx.Done():
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
