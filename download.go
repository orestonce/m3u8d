package m3u8d

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/andybalholm/brotli"
	"github.com/orestonce/m3u8d/mformat"
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

type GetStatus_Resp struct {
	Percent       int
	Title         string
	StatusBar     string
	IsDownloading bool
	IsCancel      bool
	ErrMsg        string
	IsSkipped     bool
	SaveFileTo    string
	TaskId        string	// 任务id, 库用户自己传入的 StartDownload_Req.TaskId
}

var PNG_SIGN = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

type StartDownload_Req struct {
	M3u8Url           string
	Insecure          bool                // "是否允许不安全的请求(默认为false)"
	SaveDir           string              // "文件保存路径(默认为当前路径)"
	FileName          string              // 文件名
	SkipTsExpr        string              // 跳过ts信息，ts编号从1开始，可以以逗号","为分隔符跳过多部分ts，例如: 1,92-100 表示跳过第1号ts、跳过92到100号ts
	SetProxy          string              //代理
	HeaderMap         map[string][]string // 自定义http头信息
	SkipRemoveTs      bool                // 不删除ts文件
	ProgressBarShow   bool                // 在控制台打印进度条
	ThreadCount       int                 // 线程数
	SkipCacheCheck    bool                // 不缓存已下载的m3u8的文件信息
	SkipMergeTs       bool                // 不合并ts为mp4
	DebugLog          bool                // 调试日志
	TsTempDir         string              // 临时ts文件目录
	UseServerSideTime bool                // 使用服务端提供的文件时间
	WithSkipLog       bool                // 在mp4旁记录跳过ts文件的信息
	TaskId            string			  // 用户自定义的任务id, GetStatus会原样传回来
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

// 获取m3u8地址的host
func getHost(Url string) (host string, err error) {
	u, err := url.Parse(Url)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

func updateTsUrl(m3u8Url string, tsList []mformat.TsInfo) (errMsg string) {
	for idx := range tsList {
		ts := &tsList[idx]
		ts.Url, errMsg = ResolveRefUrl(m3u8Url, ts.URI)
		if errMsg != "" {
			return "ts.URI = " + ts.URI + ", error " + errMsg
		}
	}
	return ""
}

// 更新秘钥(key)的url和内容，方便后续下载
func UpdateMediaKeyContent(m3u8Url string, tsList []mformat.TsInfo, getFunc func(urlStr string) (content []byte, err error)) (errMsg string) {
	uriToContentMap := map[string][]byte{}

	for idx := range tsList {
		ts := &tsList[idx]
		if ts.Key.Method != `` {
			keyContent, ok := uriToContentMap[ts.Key.KeyURI]
			if ok == false {
				var keyUrl string
				keyUrl, errMsg = ResolveRefUrl(m3u8Url, ts.Key.KeyURI)
				logPrefix := "m3u8Url = " + strconv.Quote(m3u8Url) + ", ts.Key.KeyURI = " + ts.Key.KeyURI

				if errMsg != "" {
					return logPrefix + ", error " + errMsg
				}
				var err error
				keyContent, err = getFunc(keyUrl)
				if err != nil {
					return logPrefix + ", http error " + err.Error()
				}
				if ts.Key.Method == mformat.EncryptMethod_AES128 { // Aes 128
					switch len(keyContent) {
					case 16:
					case 32:
						var temp []byte
						temp, err = hex.DecodeString(string(keyContent))
						if err == nil && len(temp) == 16 {
							keyContent = temp
							break
						}
						fallthrough
					default:
						return logPrefix + ", invalid key " + strconv.Quote(string(keyContent))
					}
				}
				uriToContentMap[ts.Key.KeyURI] = keyContent
			}
			ts.Key.KeyContent = keyContent
		}
	}
	return ""
}

// 下载ts文件
// @modify: 2020-08-13 修复ts格式SyncByte合并不能播放问题
func (this *DownloadEnv) downloadTsFile(ts *mformat.TsInfo, skipInfo SkipTsInfo, downloadDir string, useServerSideTime bool) (err error) {
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
	if ts.Key.Method != "" {
		//解密 ts 文件，算法：aes 128 cbc pack5
		origData, err = mformat.AesDecrypt(origData, ts.Key)
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

func (this *DownloadEnv) downloader(tsList []mformat.TsInfo, skipInfo SkipTsInfo, downloadDir string, req StartDownload_Req) (err error) {
	if req.ThreadCount <= 0 || req.ThreadCount > 1000 {
		return errors.New("DownloadEnv.threadCount invalid: " + strconv.Itoa(req.ThreadCount))
	}
	task := gopool.NewThreadPool(req.ThreadCount)
	var locker sync.Mutex

	this.status.SpeedResetTotalBlockCount(len(tsList))

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
				lastErr = this.downloadTsFile(ts, skipInfo, downloadDir, req.UseServerSideTime)
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

func (this *DownloadEnv) sniffM3u8(urlS string) (afterUrl string, info mformat.M3U8File, errMsg string) {
	for idx := 0; idx < 5; idx++ {
		content, httpResp, err := this.doGetRequest(urlS, true)
		if err != nil {
			return "", info, err.Error()
		}
		if httpResp.StatusCode != 200 {
			return "", info, "invalid httpCode " + strconv.Itoa(httpResp.StatusCode)
		}
		var ok bool
		info, ok = mformat.M3U8Parse(content)
		if ok {
			// 看这个是不是嵌套的m3u8
			if info.IsNestedPlaylists() {
				playlist := info.LookupHDPlaylist()
				if playlist == nil {
					return "", info, "lookup playlist failed"
				}
				urlS, errMsg = ResolveRefUrl(urlS, playlist.URI)
				if errMsg != "" {
					return "", info, errMsg
				}
				continue
			}
			if info.ContainsMediaSegment() {
				return urlS, info, ""
			}
			return "", info, "未发现m3u8资源_1"
		}
		groups := regexp.MustCompile(`http[s]://[a-zA-Z0-9/\\.%_-]+.m3u8`).FindSubmatch(content)
		if len(groups) == 0 {
			return "", info, "未发现m3u8资源_2"
		}
		urlS = string(groups[0])
	}
	return "", info, "未发现m3u8资源_3"
}

func ResolveRefUrl(baseUrl string, extUrl string) (after string, errMsg string) {
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
	case "br":
		readCloser = io.NopCloser(brotli.NewReader(resp.Body))
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
	this.logFileLocker.Lock()
	skip := this.logFile == nil
	this.logFileLocker.Unlock()

	if skip {
		return
	}

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

func (this *DownloadEnv) writeFfmpegCmd(downloadingDir string, list []mformat.TsInfo) (err error) {
	const listFileName = "filelist.txt"

	var fileListLog bytes.Buffer
	for _, one := range list {
		if one.SkipByHttpCode {
			continue
		}
		fileListLog.WriteString("file " + one.Name + "\n")
	}
	err = os.WriteFile(filepath.Join(downloadingDir, listFileName), fileListLog.Bytes(), 0777)
	if err != nil {
		return errors.New("写入" + listFileName + "失败, " + err.Error())
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
		return errors.New("写入" + ffmpegCmdFileName + "失败, " + err.Error())
	}

	return nil
}

func (this *DownloadEnv) writeSkipByHttpCodeInfoTxt(tsSaveDir string, list []mformat.TsInfo) error {
	var skipByHttpCodeLog bytes.Buffer
	for _, one := range list {
		fmt.Fprintf(&skipByHttpCodeLog, "http.code=%v,filename=%v,url=%v\n", one.HttpCode, one.Name, one.Url)
	}
	return os.WriteFile(filepath.Join(tsSaveDir, logFileName), skipByHttpCodeLog.Bytes(), 0666)
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
