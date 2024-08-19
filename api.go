package m3u8d

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
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

func (this *DownloadEnv) StartDownload(req StartDownload_Req) (errMsg string) {
	this.status.Locker.Lock()
	defer this.status.Locker.Unlock()

	if this.status.IsRunning {
		errMsg = "正在下载"
		return errMsg
	}
	skipTsList, errMsg := ParseSkipTsExpr(req.SkipTsExpr)
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

	this.status.progressBarShow = req.ProgressBarShow
	this.ctx, this.cancelFn = context.WithCancel(context.Background())
	this.status.IsRunning = true
	go func() {
		this.runDownload(req, skipTsList)
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

func ParseSkipTsExpr(expr string) (skipList []SkipTsUnit, errMsg string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, ""
	}
	list := strings.Split(expr, ",")
	for _, one := range list {
		part := strings.SplitN(one, "-", 2)

		var unit SkipTsUnit
		value, err := strconv.ParseUint(strings.TrimSpace(part[0]), 10, 32)
		if err != nil {
			return nil, "parse expr part1 failed " + strconv.Quote(one)
		}
		unit.StartIdx = uint32(value)

		if len(part) == 2 {
			value, err = strconv.ParseUint(strings.TrimSpace(part[1]), 10, 32)
			if err != nil {
				return nil, "parse expr part2 failed " + strconv.Quote(one)
			}
			unit.EndIdx = uint32(value)
		} else {
			unit.EndIdx = unit.StartIdx
		}
		if unit.StartIdx == 0 || unit.EndIdx < unit.StartIdx {
			return nil, "parse expr part invalid " + strconv.Quote(one)
		}
		skipList = skipListAddUnit(skipList, unit)
	}
	sort.Slice(skipList, func(i, j int) bool {
		a, b := skipList[i], skipList[j]
		return a.StartIdx < b.StartIdx
	})
	return skipList, ""
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

func (this *DownloadEnv) runDownload(req StartDownload_Req, skipList []SkipTsUnit) {
	if !strings.HasPrefix(req.M3u8Url, "http") || req.M3u8Url == "" {
		this.setErrMsg("M3u8Url not valid " + strconv.Quote(req.M3u8Url))
		return
	}
	var err error
	downloadingDir := filepath.Join(req.SaveDir, "downloading")
	if !isDirExists(downloadingDir) {
		err = os.MkdirAll(downloadingDir, os.ModePerm)
		if err != nil {
			this.setErrMsg("os.MkdirAll error0: " + err.Error())
			return
		}
	}
	var tempDebugFilePath string
	if req.DebugLog {
		tempDebugFilePath = filepath.Join(downloadingDir, fmt.Sprintf("temp_debuglog_%08d-%05d.txt", os.Getpid(), atomic.AddUint32(&debugLogNo, 1)))
		this.logFile, err = os.OpenFile(tempDebugFilePath, os.O_APPEND|os.O_CREATE, 0666)
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
	videoDownloadDir := filepath.Join(req.SaveDir, "downloading", videoId)
	if !isDirExists(videoDownloadDir) {
		err = os.MkdirAll(videoDownloadDir, os.ModePerm)
		if err != nil {
			this.setErrMsg("os.MkdirAll error: " + err.Error())
			return
		}
	}

	if this.logFile != nil {
		this.logFile.Sync()
		this.logFile.Close()
		persistDebugFilePath := filepath.Join(videoDownloadDir, "debuglog.txt")
		err = os.Rename(tempDebugFilePath, persistDebugFilePath)
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
	tsList = skipApplyFilter(tsList, skipList, req.Skip_EXT_X_DISCONTINUITY)
	if len(tsList) <= 0 {
		this.setErrMsg("需要下载的文件为空")
		return
	}
	// 下载ts
	this.status.SetProgressBarTitle("[3/4]下载ts")
	this.status.SpeedResetBytes()
	err = this.downloader(tsList, videoDownloadDir, encInfo, req.ThreadCount)
	this.status.SpeedResetBytes()
	if err != nil {
		this.setErrMsg("下载ts文件错误: " + err.Error())
		return
	}
	this.status.DrawProgressBar(1, 1)
	if req.SkipMergeTs {
		return
	}
	var tsFileList []string
	for _, one := range tsList {
		tsFileList = append(tsFileList, filepath.Join(videoDownloadDir, one.Name))
	}
	var tmpOutputName string
	tmpOutputName = filepath.Join(videoDownloadDir, "all.merge.mp4")

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
			break
		}
		if idx > 10000 { // 超过1万就不找了
			this.setErrMsg("自动寻找文件名失败")
			return
		}
	}
	err = os.Rename(tmpOutputName, name)
	if err != nil {
		this.setErrMsg("重命名失败: " + err.Error())
		return
	}
	if req.SkipRemoveTs == false {
		this.logFileClose()
		err = os.RemoveAll(videoDownloadDir)
		if err != nil {
			this.setErrMsg("删除下载目录失败: " + err.Error())
			return
		}
		// 如果downloading目录为空,就删除掉,否则忽略
		_ = os.Remove(filepath.Join(req.SaveDir, "downloading"))
	}
	this.setSaveFileTo(name, false)
	return
}

func skipApplyFilter(list []TsInfo, skipList []SkipTsUnit, skip_EXT_X_DISCONTINUITY bool) (after []TsInfo) {
	isSkip := func(idx uint32) bool {
		for _, unit := range skipList {
			if unit.StartIdx <= idx && idx <= unit.EndIdx {
				return true
			}
		}
		return false
	}
	for idx, ts := range list {
		if isSkip(uint32(idx) + 1) {
			continue
		}
		if skip_EXT_X_DISCONTINUITY && ts.Between_EXT_X_DISCONTINUITY {
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

func (this *DownloadEnv) prepareReqAndHeader(req *StartDownload_Req) (errMsg string) {
	if req.SaveDir == "" {
		var err error
		req.SaveDir, err = os.Getwd()
		if err != nil {
			return "os.Getwd error: " + err.Error()
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
