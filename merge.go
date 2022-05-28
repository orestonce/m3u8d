package m3u8d

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

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
