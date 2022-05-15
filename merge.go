package m3u8d

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type SetupFfmpeg_Resp struct {
	ErrMsg     string
	FfmpegPath string
}

var gFfmpegExePath string
var gFfmpegExePathLocker sync.Mutex

func SetupFfmpeg() (p string, err error) {
	gFfmpegExePathLocker.Lock()
	defer gFfmpegExePathLocker.Unlock()

	if gFfmpegExePath != "" {
		return gFfmpegExePath, nil
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		ffmpegPath, err = UnzipFfmpegToLocal(getTempDir())
		if err != nil {
			return "", err
		}
		gFfmpegExePath = ffmpegPath
		return ffmpegPath, nil
	}
	gFfmpegExePath = ffmpegPath
	return ffmpegPath, nil
}

func getTempDir() string {
	dir := filepath.Join(os.TempDir(), "m3u8d")
	if !isDirExists(dir) {
		_ = os.MkdirAll(dir, 0777)
	}
	return dir
}

func mergeTsFileList_Ffmpeg(ffmpegPath string, tsFileList []string, outputMp4 string) (contentHash string, err error) {
	outputMp4Temp := outputMp4 + ".tmp"
	cmd := exec.Command(ffmpegPath, "-i", "-", "-acodec", "copy", "-vcodec", "copy", "-f", "mp4", outputMp4Temp)
	ip, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	if isFileExists(outputMp4Temp) {
		err = os.Remove(outputMp4Temp)
		if err != nil {
			return "", err
		}
	}
	//cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	setupCmd(cmd)
	err = cmd.Start()
	if err != nil {
		return "", err
	}
	for _, name := range tsFileList {
		fin, err := os.Open(name)
		if err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			return "", errors.New("read error: " + err.Error())
		}
		_, err = io.Copy(ip, fin)
		if err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			fin.Close()
			return "", errors.New("write error: " + err.Error())
		}
		fin.Close()
	}
	err = ip.Close()
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return "", errors.New("ip.Close error: " + err.Error())
	}
	err = cmd.Wait()
	if err != nil {
		return "", err
	}
	err = os.Rename(outputMp4Temp, outputMp4)
	if err != nil {
		return "", err
	}
	return getFileSha256(outputMp4), nil
}

func mergeTsFileList_Raw(tsFileList []string, outputTs string) (hash string, err error) {
	fout, err := os.Create(outputTs)
	if err != nil {
		return "", err
	}
	hashWriter := sha256.New()
	multiW := io.MultiWriter(fout, hashWriter)

	for _, tsName := range tsFileList {
		var fin *os.File
		fin, err = os.Open(tsName)
		if err != nil {
			fout.Close()
			_ = os.Remove(outputTs)
			return "", err
		}
		_, err = io.Copy(multiW, fin)
		if err != nil {
			fin.Close()
			fout.Close()
			_ = os.Remove(outputTs)
			return "", err
		}
		fin.Close()
	}
	err = fout.Close()
	if err != nil {
		return "", err
	}
	tmp := hashWriter.Sum(nil)
	return hex.EncodeToString(tmp[:]), nil
}
