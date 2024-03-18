package m3u8d

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
		this.runDownload(req)
		this.status.SetProgressBarTitle("下载进度")
		this.status.DrawProgressBar(1, 0)

		this.status.Locker.Lock()
		this.cancelFn()
		this.status.IsRunning = false
		this.status.Locker.Unlock()
	}()
	return ""
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
}

func (this *DownloadEnv) setSaveFileTo(to string, isSkipped bool) {
	this.status.Locker.Lock()
	this.status.saveFileTo = to
	this.status.isSkipped = isSkipped
	this.status.Locker.Unlock()
}

func (this *DownloadEnv) runDownload(req StartDownload_Req) {
	this.status.SetProgressBarTitle("[1/6]嗅探m3u8")
	var m3u8Body []byte
	var errMsg string
	req.M3u8Url, m3u8Body, errMsg = this.sniffM3u8(req.M3u8Url)
	if errMsg != "" {
		this.setErrMsg("sniffM3u8: " + errMsg)
		return
	}
	videoId, err := req.getVideoId()
	if err != nil {
		this.setErrMsg("getVideoId: " + err.Error())
		return
	}

	if req.SkipCacheCheck == false && req.SkipMergeTs == false {
		var info *DbVideoInfo
		info, err = cacheRead(req.SaveDir, videoId)
		if err != nil {
			this.setErrMsg("cacheRead: " + err.Error())
			return
		}
		if info != nil {
			this.status.SetProgressBarTitle("[2/6]检查是否已下载")
			latestNameFullPath, found := info.SearchVideoInDir(req.SaveDir)
			if found {
				this.setSaveFileTo(latestNameFullPath, true)
				return
			}
		}
	}
	if !strings.HasPrefix(req.M3u8Url, "http") || req.M3u8Url == "" {
		this.setErrMsg("M3u8Url not valid " + strconv.Quote(req.M3u8Url))
		return
	}
	downloadDir := filepath.Join(req.SaveDir, "downloading", videoId)
	if !isDirExists(downloadDir) {
		err = os.MkdirAll(downloadDir, os.ModePerm)
		if err != nil {
			this.setErrMsg("os.MkdirAll error: " + err.Error())
			return
		}
	}

	downloadingFilePath := filepath.Join(downloadDir, "downloading.txt")
	if !isFileExists(downloadingFilePath) {
		err = ioutil.WriteFile(downloadingFilePath, []byte(req.M3u8Url), 0666)
		if err != nil {
			this.setErrMsg("os.WriteUrl error: " + err.Error())
			return
		}
	}

	beginSeq := parseBeginSeq(m3u8Body)
	// 获取m3u8地址的内容体
	encInfo, err := this.getEncryptInfo(req.M3u8Url, string(m3u8Body))
	if err != nil {
		this.setErrMsg("getEncryptInfo: " + err.Error())
		return
	}
	this.status.SetProgressBarTitle("[3/6]获取ts列表")
	tsList, errMsg := getTsList(beginSeq, req.M3u8Url, string(m3u8Body), req.Skip_EXT_X_DISCONTINUITY)
	if errMsg != "" {
		this.setErrMsg("获取ts列表错误: " + errMsg)
		return
	}
	if len(tsList) <= req.SkipTsCountFromHead {
		this.setErrMsg("需要下载的文件为空")
		return
	}
	tsList = tsList[req.SkipTsCountFromHead:]
	// 下载ts
	this.status.SetProgressBarTitle("[4/6]下载ts")
	this.status.SpeedResetBytes()
	err = this.downloader(tsList, downloadDir, encInfo, req.ThreadCount)
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
		tsFileList = append(tsFileList, filepath.Join(downloadDir, one.Name))
	}
	var tmpOutputName string
	tmpOutputName = filepath.Join(downloadDir, "all.merge.mp4")

	this.status.SetProgressBarTitle("[5/6]合并ts为mp4")
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
	var contentHash string
	if req.SkipCacheCheck == false {
		this.status.SetProgressBarTitle("[6/6]计算文件hash")
		contentHash = getFileSha256(tmpOutputName)
		if contentHash == "" {
			this.setErrMsg("无法计算摘要信息: " + tmpOutputName)
			return
		}
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
	if req.SkipCacheCheck == false {
		err = cacheWrite(req.SaveDir, videoId, req, name, contentHash)
		if err != nil {
			this.setErrMsg("cacheWrite: " + err.Error())
			return
		}
	}
	if req.SkipRemoveTs == false {
		err = os.RemoveAll(downloadDir)
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

func (this *DownloadEnv) setupClient(req StartDownload_Req, proxyUrlObj *url.URL) {
	if this.nowClient == nil {
		this.nowClient = &http.Client{}
	}
	//关闭以前的空闲链接
	this.nowClient.CloseIdleConnections()
	this.nowClient.Timeout = time.Second * 20

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
	if req.SkipTsCountFromHead < 0 {
		req.SkipTsCountFromHead = 0
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
