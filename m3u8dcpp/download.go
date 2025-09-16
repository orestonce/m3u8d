package m3u8dcpp

import (
	"github.com/orestonce/m3u8d"
)

var gOldEnv m3u8d.DownloadEnv

func StartDownload(req m3u8d.StartDownload_Req) bool {
	return gOldEnv.StartDownload(req)
}

func CloseOldEnv() {
	gOldEnv.CloseEnv()
}

func GetStatus() (resp m3u8d.GetStatus_Resp) {
	return gOldEnv.GetStatus()
}

func WaitDownloadFinish() (resp m3u8d.GetStatus_Resp) {
	return gOldEnv.WaitDownloadFinish()
}
