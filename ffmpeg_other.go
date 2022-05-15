//+build !windows

package m3u8d

import "errors"

func UnzipFfmpegToLocal(exeFileDir string) (targetFile string, err error) {
	return "", errors.New("UnzipFfmpegToLocal not implement.")
}
