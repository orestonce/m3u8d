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
	OriginReq   RunDownload_Req
}

func (this *RunDownload_Req) getVideoId() (id string, err error) {
	b, err := json.Marshal(this)
	if err != nil {
		return "", err
	}
	tmp1 := sha256.Sum256(b)
	return hex.EncodeToString(tmp1[:]), nil
}

func cacheRead(dir string, id string) (info *DbVideoInfo, err error) {
	value, err := dbRead(dir, id)
	if err != nil {
		return nil, err
	}
	if len(value) == 0 {
		return nil, nil
	}
	var obj DbVideoInfo
	err = json.Unmarshal(value, &obj)
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

func cacheWrite(dir string, id string, originReq RunDownload_Req, videoNameFullPath string, contentHash string) (err error) {
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
	return dbWrite(dir, id, content)
}

func dbRead(dir string, key string) (content []byte, err error) {
	db, err := cdb.OpenFile(filepath.Join(dir, "m3u8d_cache.cdb"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer db.Close()
	content, err = db.GetValue([]byte(key))
	if err != nil {
		if err == cdb.ErrNoData {
			return nil, nil
		}
		return nil, err
	}
	return content, nil
}

func dbWrite(dir string, key string, value []byte) (err error) {
	cdbFileName := filepath.Join(dir, "m3u8d_cache.cdb")

	reader, err := cdb.OpenFile(cdbFileName)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	tmpCdbFileName := cdbFileName + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	writer, err := cdb.NewFileWriter(tmpCdbFileName)
	if err != nil {
		if reader != nil {
			reader.Close()
		}
		return err
	}

	if reader != nil {
		for it := reader.BeginIterator(); it != nil; {
			tmpKey, tmpValue, err := it.ReadNextKeyValue()
			if err != nil {
				if err == cdb.ErrNoData {
					break
				}
				reader.Close()
				writer.Close()
				os.Remove(tmpCdbFileName)
				return err
			}
			if string(tmpKey) == key {
				continue
			}
			err = writer.WriteKeyValue(tmpKey, tmpValue)
			if err != nil {
				reader.Close()
				writer.Close()
				os.Remove(tmpCdbFileName)
				return err
			}
		}
		reader.Close()
	}
	err = writer.WriteKeyValue([]byte(key), value)
	if err != nil {
		writer.Close()
		return err
	}
	err = writer.Close()
	if err != nil {
		return err
	}
	return os.Rename(tmpCdbFileName, cdbFileName)
}
