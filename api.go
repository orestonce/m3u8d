package m3u8d

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const logFileName = `skip_by_http_code.txt`

func (this *DownloadEnv) StartDownload(req StartDownload_Req) (errMsg string) {
	this.status.Locker.Lock()
	defer this.status.Locker.Unlock()

	if this.status.IsRunning {
		errMsg = "正在下载"
		return errMsg
	}
	skipInfo, errMsg := ParseSkipTsExpr(req.SkipTsExpr)
	if errMsg != "" {
		return "解析跳过ts的表达式错误: " + errMsg
	}

	var proxyUrlObj *url.URL
	req.SetProxy, proxyUrlObj, errMsg = ParseProxyFormat(req.SetProxy)
	if errMsg != "" {
		return errMsg
	}
	this.setupClient(req, proxyUrlObj)
	errMsg = this.prepareReqAndHeader(&req)
	if errMsg != "" {
		return errMsg
	}

	this.status.clearStatusNoLock()

	this.status.ProgressBarShow = req.ProgressBarShow
	this.ctx, this.cancelFn = context.WithCancel(context.Background())
	this.status.IsRunning = true
	go func() {
		this.runDownload(req, skipInfo)
		this.status.SetProgressBarTitle("下载进度")
		this.status.DrawProgressBar(1, 0)

		this.status.Locker.Lock()
		//this.cancelFn()
		this.status.IsRunning = false
		this.status.Locker.Unlock()

		this.logFileClose()
	}()
	return ""
}

type SkipTsUnit struct {
	StartIdx uint32 // 包含
	EndIdx   uint32 // 包含
}

type SkipByTimeUnit struct {
	StartSec uint32
	EndSec   uint32
}

type SkipTsInfo struct {
	HttpCodeList      []int
	SkipList          []SkipTsUnit
	IfHttpCodeMergeTs bool
	SkipByTimeList    []SkipByTimeUnit
}

func ParseSkipTsExpr(expr string) (info SkipTsInfo, errMsg string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return info, ""
	}
	list := strings.Split(expr, ",")
	singleRe := regexp.MustCompile(`^([0-9]+)$`)
	betweenRe := regexp.MustCompile(`^([0-9]+) *- *([0-9]+)$`)
	httpCodeRe := regexp.MustCompile(`^http.code *= *([0-9]+)$`)
	betweenTimeRe := regexp.MustCompile(`^time *: *(\d{2}:\d{2}:\d{2}) *- *(\d{2}:\d{2}:\d{2})$`)

	for _, one := range list {
		one = strings.TrimSpace(one)
		var groups []string
		var ok = false

		if groups = singleRe.FindStringSubmatch(one); len(groups) > 0 {
			i, err := strconv.Atoi(groups[1])
			if err == nil || i > 0 {
				ok = true
				info.SkipList = skipListAddUnit(info.SkipList, SkipTsUnit{
					StartIdx: uint32(i),
					EndIdx:   uint32(i),
				})
			}
		} else if groups = betweenRe.FindStringSubmatch(one); len(groups) > 0 {
			i1, err1 := strconv.Atoi(groups[1])
			i2, err2 := strconv.Atoi(groups[2])
			if err1 == nil && err2 == nil && i1 > 0 && i2 > 0 && i1 <= i2 {
				ok = true
				info.SkipList = skipListAddUnit(info.SkipList, SkipTsUnit{
					StartIdx: uint32(i1),
					EndIdx:   uint32(i2),
				})
			}
		} else if groups = httpCodeRe.FindStringSubmatch(one); len(groups) > 0 {
			i, err := strconv.Atoi(groups[1])
			if err == nil && i > 0 {
				ok = true
				info.HttpCodeList = append(info.HttpCodeList, i)
			}
		} else if one == `if-http.code-merge_ts` {
			info.IfHttpCodeMergeTs = true
			ok = true
		} else if groups = betweenTimeRe.FindStringSubmatch(one); len(groups) > 0 {
			startSec, err1 := getTimeSecFromStr(groups[1])
			endSec, err2 := getTimeSecFromStr(groups[2])
			if err1 == nil && err2 == nil && startSec < endSec {
				ok = true
				info.SkipByTimeList = append(info.SkipByTimeList, SkipByTimeUnit{
					StartSec: startSec,
					EndSec:   endSec,
				})
			}
		}
		if ok == false {
			return info, "parse expr part invalid " + strconv.Quote(one)
		}
	}
	sort.Slice(info.SkipList, func(i, j int) bool {
		a, b := info.SkipList[i], info.SkipList[j]
		return a.StartIdx < b.StartIdx
	})
	sort.Ints(info.HttpCodeList)
	return info, ""
}

func getTimeSecFromStr(str string) (sec uint32, err error) {
	var h, m, s uint32

	_, err = fmt.Sscanf(str, `%d:%d:%d`, &h, &m, &s)
	if err != nil {
		return 0, err
	}
	if m >= 60 || s >= 60 {
		return 0, errors.New("invalid str " + strconv.Quote(str))
	}
	sec = h*60*60 + m*60 + s
	return sec, nil
}

func maxUint32(a uint32, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func minUint32(a uint32, b uint32) uint32 {
	if a > b {
		return b
	}
	return a
}

func skipListAddUnit(skipList []SkipTsUnit, unit SkipTsUnit) (after []SkipTsUnit) {
	for idx, one := range skipList {
		// 交集的开始索引
		jStart := maxUint32(one.StartIdx, unit.StartIdx)
		// 交集的结束索引
		jEnd := minUint32(one.EndIdx, unit.EndIdx)
		// 有交集, 或者正好拼接为一个大区间10-20,21-30 => 10-30
		if jStart <= jEnd || jStart == jEnd-1 {
			unit.StartIdx = minUint32(one.StartIdx, unit.StartIdx)
			unit.EndIdx = maxUint32(one.EndIdx, unit.EndIdx)
			var pre, post []SkipTsUnit // 前面部分，后面部分
			pre = skipList[:idx]
			if len(skipList) > idx+1 {
				post = skipList[idx+1:]
			}
			skipList = append(pre, post...)
			return skipListAddUnit(skipList, unit)
		}
	}
	// 都无交集
	skipList = append(skipList, unit)
	return skipList
}

func (this *DownloadEnv) GetStatus() (resp GetStatus_Resp) {
	var sleepTh int32

	sleepTh = atomic.LoadInt32(&this.sleepTh)
	resp.Percent = this.status.GetPercent()
	resp.Title = this.status.GetTitle()
	if resp.Title == "" {
		resp.Title = "正在下载"
	}
	{
		this.status.Locker.Lock()
		resp.IsDownloading = this.status.IsRunning
		resp.ErrMsg = this.status.errMsg
		resp.IsSkipped = this.status.isSkipped
		resp.SaveFileTo = this.status.saveFileTo
		this.status.Locker.Unlock()
	}

	var speed SpeedInfo
	if resp.IsDownloading {
		speed = this.status.SpeedRecent5sGetAndUpdate()
	}
	resp.StatusBar = speed.ToString()
	if sleepTh > 0 {
		if resp.StatusBar != "" {
			resp.StatusBar += ", "
		}
		resp.StatusBar += "有 " + strconv.Itoa(int(sleepTh)) + "个线程正在休眠."
	}

	if resp.ErrMsg != "" {
		resp.IsCancel = this.GetIsCancel()
	}
	return resp
}

func (this *DownloadEnv) WaitDownloadFinish() GetStatus_Resp {
	for {
		status := this.GetStatus()
		if status.IsDownloading {
			time.Sleep(time.Millisecond * 100)
			continue
		}
		return status
	}
}

func (this *DownloadEnv) setErrMsg(errMsg string) {
	this.status.Locker.Lock()
	this.status.errMsg = errMsg
	this.status.Locker.Unlock()

	if errMsg != "" {
		this.logToFile("errMsg " + errMsg)
	}
}

func (this *DownloadEnv) setSaveFileTo(to string, isSkipped bool) {
	this.status.Locker.Lock()
	this.status.saveFileTo = to
	this.status.isSkipped = isSkipped
	this.status.Locker.Unlock()
}

var debugLogNo uint32

func (this *DownloadEnv) runDownload(req StartDownload_Req, skipInfo SkipTsInfo) {
	if !strings.HasPrefix(req.M3u8Url, "http") || req.M3u8Url == "" {
		this.setErrMsg("M3u8Url not valid " + strconv.Quote(req.M3u8Url))
		return
	}
	var err error
	downloadingDir := filepath.Join(req.TsTempDir, "downloading")
	for _, dir := range []string{req.SaveDir, req.TsTempDir, downloadingDir} {
		if isDirExists(dir) {
			continue
		}
		err = os.Mkdir(dir, 0755)
		if err != nil {
			this.setErrMsg("os.Mkdir " + strconv.Quote(dir) + " error: " + err.Error())
			return
		}
	}
	var tmpDebugFilePath string
	if req.DebugLog {
		tmpDebugFilePath = filepath.Join(downloadingDir, fmt.Sprintf("temp_debuglog_%08d-%05d.txt", os.Getpid(), atomic.AddUint32(&debugLogNo, 1)))
		this.logFile, err = os.OpenFile(tmpDebugFilePath, os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			this.setErrMsg("os.WriteUrl error: " + err.Error())
			return
		}
		this.logToFile("version: " + GetVersion())
		this.logToFile("origin m3u8 url: " + req.M3u8Url)
	}

	this.status.SetProgressBarTitle("[1/4]嗅探m3u8")
	var m3u8Body []byte
	var errMsg string
	req.M3u8Url, m3u8Body, errMsg = this.sniffM3u8(req.M3u8Url)
	if errMsg != "" {
		this.setErrMsg("sniffM3u8: " + errMsg)
		return
	}
	videoId := req.getVideoId()
	tsSaveDir := filepath.Join(downloadingDir, videoId)
	if !isDirExists(tsSaveDir) {
		err = os.MkdirAll(tsSaveDir, os.ModePerm)
		if err != nil {
			this.setErrMsg("os.MkdirAll error: " + err.Error())
			return
		}
	}

	if this.logFile != nil {
		this.logFile.Sync()
		this.logFile.Close()
		persistDebugFilePath := filepath.Join(tsSaveDir, "debuglog.txt")
		err = os.Rename(tmpDebugFilePath, persistDebugFilePath)
		if err != nil {
			this.setErrMsg("os.Rename set persistDebugFilePath " + strconv.Quote(persistDebugFilePath) + " error : " + err.Error())
			return
		}
		this.logFile, err = os.OpenFile(persistDebugFilePath, os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			this.setErrMsg("os.Open persistDebugFilePath " + strconv.Quote(persistDebugFilePath) + " error: " + err.Error())
			return
		}
		this.logToFile("refresh m3u8 url: " + req.M3u8Url)
	}

	beginSeq := parseBeginSeq(m3u8Body)
	// 获取m3u8地址的内容体
	encInfo, err := this.getEncryptInfo(req.M3u8Url, string(m3u8Body))
	if err != nil {
		this.setErrMsg("getEncryptInfo: " + err.Error())
		return
	}
	this.status.SetProgressBarTitle("[2/4]获取ts列表")
	tsList, errMsg := getTsList(beginSeq, req.M3u8Url, string(m3u8Body))
	if errMsg != "" {
		this.setErrMsg("获取ts列表错误: " + errMsg)
		return
	}
	tsList = skipApplyFilter(tsList, skipInfo, req.Skip_EXT_X_DISCONTINUITY)
	if len(tsList) <= 0 {
		this.setErrMsg("需要下载的文件为空")
		return
	}
	// 下载ts
	this.status.SetProgressBarTitle("[3/4]下载ts")
	this.status.SpeedResetBytes()
	err = this.downloader(tsList, skipInfo, tsSaveDir, encInfo, req.ThreadCount)
	this.status.SpeedResetBytes()
	if err != nil {
		this.setErrMsg("下载ts文件错误: " + err.Error())
		return
	}
	this.status.DrawProgressBar(1, 1)
	var tsFileList []string
	var skipByHttpCodeLog bytes.Buffer
	var skipCount int
	for _, one := range tsList {
		if one.SkipByHttpCode {
			skipCount++
			fmt.Fprintf(&skipByHttpCodeLog, "http.code=%v,filename=%v,url=%v\n", one.HttpCode, one.Name, one.Url)
			continue
		}
		fileNameFull := filepath.Join(tsSaveDir, one.Name)
		tsFileList = append(tsFileList, fileNameFull)
	}
	// 写入ffmpeg合并命令
	if this.writeFfmpegCmd(tsSaveDir, tsList) == false {
		return
	}
	if skipByHttpCodeLog.Len() > 0 {
		// 写入通过http.code跳过的ts文件列表
		err = os.WriteFile(filepath.Join(tsSaveDir, logFileName), skipByHttpCodeLog.Bytes(), 0666)
		if err != nil {
			this.setErrMsg("写入" + logFileName + "失败, " + err.Error())
			return
		}
		if skipInfo.IfHttpCodeMergeTs == false {
			this.setErrMsg("使用http.code跳过了" + strconv.Itoa(skipCount) + "条ts记录，请自行合并")
			return
		}
	}
	if req.SkipMergeTs {
		return
	}
	var name string
	var tmpOutputName string

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
			tmpOutputName = name + ".temp"
			break
		}
		if idx > 10000 { // 超过1万就不找了
			this.setErrMsg("自动寻找文件名失败")
			return
		}
	}
	this.status.SetProgressBarTitle("[4/4]合并ts为mp4")
	err = MergeTsFileListToSingleMp4(MergeTsFileListToSingleMp4_Req{
		TsFileList: tsFileList,
		OutputMp4:  tmpOutputName,
		Ctx:        this.ctx,
		Status:     &this.status,
	})
	this.status.SpeedResetBytes()
	if err != nil {
		this.setErrMsg("合并错误: " + err.Error())
		return
	}

	err = os.Rename(tmpOutputName, name)
	if err != nil {
		this.setErrMsg("重命名失败: " + err.Error())
		return
	}
	if skipByHttpCodeLog.Len() > 0 {
		// 写入通过http.code跳过的ts文件列表
		saveFileName := name + "_" + logFileName
		err = os.WriteFile(saveFileName, skipByHttpCodeLog.Bytes(), 0666)
		if err != nil {
			this.setErrMsg("写入" + saveFileName + "失败, " + err.Error())
			return
		}
	}
	if req.SkipRemoveTs == false {
		this.logFileClose()
		err = os.RemoveAll(tsSaveDir)
		if err != nil {
			this.setErrMsg("删除下载目录失败: " + err.Error())
			return
		}
		// 如果downloading目录为空,就删除掉,否则忽略
		_ = os.Remove(downloadingDir)
	}
	this.setSaveFileTo(name, false)
	return
}
func isSkipByTsTime(beginSec float64, endSec float64, list []SkipByTimeUnit) bool {
	for _, unit := range list {
		newBegin := math.Max(float64(unit.StartSec), beginSec)
		newEnd := math.Min(float64(unit.EndSec), endSec)

		if newEnd > newBegin {
			return true
		}
	}
	return false
}

func skipApplyFilter(list []TsInfo, skipInfo SkipTsInfo, skip_EXT_X_DISCONTINUITY bool) (after []TsInfo) {
	var hasEmptyExtinf bool
	for _, ts := range list {
		if ts.TimeSec < 1e-5 {
			hasEmptyExtinf = true
		}
	}
	isSkipByTsIndex := func(idx uint32) bool {
		for _, unit := range skipInfo.SkipList {
			if unit.StartIdx <= idx && idx <= unit.EndIdx {
				return true
			}
		}
		return false
	}

	var timeBegin float64
	var timeEnd float64

	for idx, ts := range list {
		if idx > 0 {
			timeBegin += list[idx-1].TimeSec
		}
		timeEnd += ts.TimeSec

		if isSkipByTsIndex(uint32(idx) + 1) {
			continue
		}
		if skip_EXT_X_DISCONTINUITY && ts.Between_EXT_X_DISCONTINUITY {
			continue
		}
		if hasEmptyExtinf == false && isSkipByTsTime(timeBegin, timeEnd, skipInfo.SkipByTimeList) {
			continue
		}
		after = append(after, ts)
	}
	return after
}

func (this *DownloadEnv) setupClient(req StartDownload_Req, proxyUrlObj *url.URL) {
	if this.nowClient == nil {
		this.nowClient = &http.Client{}
	}
	//关闭以前的空闲链接
	this.nowClient.CloseIdleConnections()

	if this.nowClient.Transport == nil {
		this.nowClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: req.Insecure,
			},
			Proxy: http.ProxyURL(proxyUrlObj),
		}
	} else {
		transport := this.nowClient.Transport.(*http.Transport)
		transport.TLSClientConfig.InsecureSkipVerify = req.Insecure
		transport.Proxy = http.ProxyURL(proxyUrlObj)
	}
}

func prepareDir(dir string) (dirAbs string, errMsg string) {
	if filepath.IsAbs(dir) == false {
		var err error
		dir, err = filepath.Abs(dir)
		if err != nil {
			return "", "filepath.Abs error: " + err.Error()
		}
	}
	dir = filepath.Clean(dir)
	return dir, ""
}

func (this *DownloadEnv) prepareReqAndHeader(req *StartDownload_Req) (errMsg string) {
	if req.SaveDir == "" {
		var err error
		req.SaveDir, err = os.Getwd()
		if err != nil {
			return "os.Getwd error: " + err.Error()
		}
	} else {
		req.SaveDir, errMsg = prepareDir(req.SaveDir)
		if errMsg != "" {
			return errMsg
		}
	}
	if req.TsTempDir == "" {
		req.TsTempDir = req.SaveDir
	} else {
		req.TsTempDir, errMsg = prepareDir(req.TsTempDir)
		if errMsg != "" {
			return errMsg
		}
	}
	if req.FileName == "" {
		req.FileName = GetFileNameFromUrl(req.M3u8Url)
	}

	host, err := getHost(req.M3u8Url)
	if err != nil {
		return "getHost0: " + err.Error()
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
	return ""
}

func (this *StartDownload_Req) getVideoId() (id string) {
	tmp1 := sha256.Sum256([]byte(this.M3u8Url))
	return hex.EncodeToString(tmp1[:])
}

func FindUrlInStr(str string) string {
	re := regexp.MustCompile(`https?://[^\s/$.?#].\S*`)
	return re.FindString(str)
}
