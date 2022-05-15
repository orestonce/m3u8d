package m3u8d

import (
	"bytes"
	_ "embed"
	"errors"
	"github.com/saracen/go7z"
	"io"
	"os"
	"path"
	"path/filepath"
)

//go:embed ffmpeg-2022-05-04-git-0914e3a14a-essentials_build.7z
var g7zipFileContent []byte

const FfmpegSha256Value = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func UnzipFfmpegToLocal(exeFileDir string) (targetFile string, err error) {
	targetFile = filepath.Join(exeFileDir, "ffmpeg-2022-05-04.exe")
	if isFileExists(targetFile) && isSha256Match(targetFile) {
		return targetFile, nil
	}
	sz, err := go7z.NewReader(bytes.NewReader(g7zipFileContent), int64(len(g7zipFileContent)))
	if err != nil {
		return "", err
	}

	for {
		hdr, err := sz.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return "", err
		}

		// If empty stream (no contents) and isn't specifically an empty file...
		// then it's a directory.
		if hdr.IsEmptyStream && !hdr.IsEmptyFile {
			continue
		}

		if path.Base(hdr.Name) == "ffmpeg.exe" {
			if !isDirExists(exeFileDir) {
				err = os.MkdirAll(exeFileDir, 0777)
				if err != nil {
					return "", err
				}
			}
			// Create file
			f, err := os.Create(targetFile)
			if err != nil {
				return "", err
			}

			if _, err := io.Copy(f, sz); err != nil {
				f.Close()
				return "", err
			}
			err = f.Close()
			if err != nil {
				return "", err
			}
			if !isSha256Match(targetFile) {
				return "", errors.New("ffmpeg的sha256 校验不通过")
			}
			return targetFile, nil
		}
	}
	return "", errors.New("7z文件内未找到 ffmpeg.exe")
}

func isSha256Match(targetFile string) bool {
	return getFileSha256(targetFile) == FfmpegSha256Value
}
