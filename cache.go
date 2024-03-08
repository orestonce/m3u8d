package m3u8d

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/orestonce/cdb"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
)

type DbVideoInfo struct {
	VideoId     string
	ContentHash string
	FileSize    int64 // 加快搜索速度
	OriginReq   StartDownload_Req
}

func (this *StartDownload_Req) getVideoId() (id string, err error) {
	b, err := json.Marshal([]string{this.M3u8Url, strconv.Itoa(this.SkipTsCountFromHead)})
	if err != nil {
		return "", err
	}
	tmp1 := sha256.Sum256(b)
	return hex.EncodeToString(tmp1[:]), nil
}

func cacheRead(dir string, id string) (info *DbVideoInfo, err error) {
	value, err := cdb.FileGetValueString(filepath.Join(dir, "m3u8d_cache.cdb"), id)
	if err != nil {
		if err == cdb.ErrNoData {
			return nil, nil
		}
		return nil, err
	}
	var obj DbVideoInfo
	err = json.Unmarshal([]byte(value), &obj)
	if err != nil {
		return nil, err
	}
	info = &obj
	return info, nil
}

func (this *DbVideoInfo) SearchVideoInDir(dir string) (latestNameFullPath string, found bool) {
	fileList, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, one := range fileList {
		if this.FileSize != one.Size() || !one.Mode().IsRegular() {
			continue
		}
		tmp := filepath.Join(dir, one.Name())
		if this.ContentHash == getFileSha256(tmp) {
			return tmp, true
		}
	}
	return "", false
}

func cacheWrite(dir string, id string, originReq StartDownload_Req, videoNameFullPath string, contentHash string) (err error) {
	var info = &DbVideoInfo{
		VideoId:     id,
		OriginReq:   originReq,
		ContentHash: contentHash,
		FileSize:    0,
	}
	stat, err := os.Stat(videoNameFullPath)
	if err != nil {
		return err
	}
	info.FileSize = stat.Size()
	content, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return err
	}
	return cdb.FileRewriteKeyValue(filepath.Join(dir, "m3u8d_cache.cdb"), id, string(content))
}
