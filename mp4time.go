package m3u8d

import (
	"encoding/binary"
	"errors"
	"github.com/xiaoqidun/setft"
	"github.com/yapingcat/gomedia/go-mp4"
	"io"
	"os"
	"time"
)

func (this *DownloadEnv) updateMp4Time(firstTsName string, mp4FileName string) bool {
	stat, err := os.Stat(firstTsName)
	if err != nil {
		this.setErrMsg("读取文件状态失败: " + err.Error())
		return false
	}
	mTime := stat.ModTime()
	this.logToFile("更新文件时间为:" + mTime.String())
	err = updateMp4CreateTime(mp4FileName, mTime)
	if err != nil {
		this.setErrMsg("更新mp4创建时间失败: " + err.Error())
		return false
	}

	err = setft.SetFileTime(mp4FileName, mTime, mTime, mTime)
	if err != nil {
		this.setErrMsg("更新文件时间属性失败: " + err.Error())
		return false
	}
	this.logToFile("更新成功")
	return true
}

func mov_tag(tag [4]byte) uint32 {
	return binary.LittleEndian.Uint32(tag[:])
}

func updateMp4CreateTime(mp4Path string, ctime time.Time) error {
	timeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(timeBuf, uint32(ctime.Unix()))

	mp4Fd, err := os.OpenFile(mp4Path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}
	defer mp4Fd.Close()

	var posList []int64
	var zeroPosList []int64

	for err == nil {
		basebox := mp4.BasicBox{}
		_, err = basebox.Decode(mp4Fd)
		if err != nil {
			break
		}
		if basebox.Size < mp4.BasicBoxLen {
			err = errors.New("mp4 Parser error")
			break
		}
		tagName := mov_tag(basebox.Type)
		switch tagName {
		case mov_tag([4]byte{'m', 'o', 'o', 'v'}):
			break
		case mov_tag([4]byte{'m', 'v', 'h', 'd'}):
			var offset int64
			if offset, err = mp4Fd.Seek(0, io.SeekCurrent); err != nil {
				break
			}
			mvhd := mp4.MovieHeaderBox{Box: new(mp4.FullBox)}
			if _, err = mvhd.Decode(mp4Fd); err != nil {
				break
			}
			if mvhd.Box.Version == 0 {
				posList = append(posList, offset+4) //create time
				posList = append(posList, offset+8) //modify time
			} else {
				zeroPosList = append(zeroPosList, offset+4)
				posList = append(posList, offset+8) //create time
				zeroPosList = append(zeroPosList, offset+12)
				posList = append(posList, offset+16) //modify time
			}
		case mov_tag([4]byte{'t', 'r', 'a', 'k'}):
			break
		case mov_tag([4]byte{'m', 'd', 'i', 'a'}):
			break
		case mov_tag([4]byte{'m', 'd', 'h', 'd'}):
			var offset int64
			if offset, err = mp4Fd.Seek(0, io.SeekCurrent); err != nil {
				break
			}
			mdhd := mp4.MediaHeaderBox{Box: new(mp4.FullBox)}
			if _, err = mdhd.Decode(mp4Fd); err != nil {
				break
			}
			if mdhd.Box.Version == 0 {
				posList = append(posList, offset+4) //create time
				posList = append(posList, offset+8) //modify time
			} else {
				zeroPosList = append(zeroPosList, offset+4)
				posList = append(posList, offset+8) //create time
				zeroPosList = append(zeroPosList, offset+12)
				posList = append(posList, offset+16) //modify time
			}
		case mov_tag([4]byte{'t', 'k', 'h', 'd'}):
			var offset int64
			if offset, err = mp4Fd.Seek(0, io.SeekCurrent); err != nil {
				break
			}
			tkhd := mp4.TrackHeaderBox{Box: new(mp4.FullBox)}
			if _, err = tkhd.Decode(mp4Fd); err != nil {
				break
			}
			if tkhd.Box.Version == 0 {
				posList = append(posList, offset+4) //create time
				posList = append(posList, offset+8) //modify time
			} else {
				zeroPosList = append(zeroPosList, offset+4)
				posList = append(posList, offset+8) //create time
				zeroPosList = append(zeroPosList, offset+12)
				posList = append(posList, offset+16) //modify time
			}
		default:
			_, err = mp4Fd.Seek(int64(basebox.Size)-mp4.BasicBoxLen, io.SeekCurrent)
		}
	}
	if err != io.EOF {
		return err
	}

	for _, pos := range posList {
		_, err = mp4Fd.Seek(pos, io.SeekStart)
		if err != nil {
			return err
		}
		_, err = mp4Fd.Write(timeBuf)
		if err != nil {
			return err
		}
	}

	for _, pos := range zeroPosList {
		_, err = mp4Fd.Seek(pos, io.SeekStart)
		if err != nil {
			return err
		}
		_, err = mp4Fd.Write([]byte{0, 0, 0, 0})
		if err != nil {
			return err
		}
	}

	return nil
}
