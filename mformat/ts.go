package mformat

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// TsInfo 用于保存 ts 文件的下载地址和文件名
type TsInfo struct {
	Name                    string
	Url                     string // 后续填充
	URI                     string
	Seq                     uint64  // 如果是aes加密并且没有iv, 这个seq需要充当iv
	TimeSec                 float64 // 此ts片段占用多少秒
	Idx_EXT_X_DISCONTINUITY int     // 分段编号
	Key                     TsKeyInfo

	SkipByHttpCode bool
	HttpCode       int
}

type TsKeyInfo struct {
	Method     string
	KeyContent []byte // 后续填充
	KeyURI     string
	Iv         []byte
}

// IsNestedPlaylists 返回是否为 嵌套播放列表
func (this *M3U8File) IsNestedPlaylists() bool {
	for _, one := range this.PartList {
		if one.Playlist != nil {
			return true
		}
	}
	return false
}

func (this *M3U8File) ContainsMediaSegment() bool {
	for _, one := range this.PartList {
		if one.Segment != nil {
			return true
		}
	}
	return false
}

// LookupHDPlaylist 找个最高清的播放列表
func (this *M3U8File) LookupHDPlaylist() (playlist *M3U8Playlist) {
	for _, one := range this.PartList {
		if one.Playlist != nil {
			if playlist == nil {
				playlist = one.Playlist
			} else if one.Playlist.Bandwidth != playlist.Bandwidth {
				if one.Playlist.Bandwidth > playlist.Bandwidth {
					playlist = one.Playlist
				}
			} else if one.Playlist.Resolution.Width != playlist.Resolution.Width {
				if one.Playlist.Resolution.Width > playlist.Resolution.Width {
					playlist = one.Playlist
				}
			}
		}
	}
	return playlist
}

// GetTsList 获取ts文件列表, 此处不组装url, 不下载key内容
func (this *M3U8File) GetTsList() (list []TsInfo) {
	var beginSeq = uint64(this.MediaSequence)
	var index = 0
	var discontinutyIdx = 0
	var curKey *M3U8Key

	for _, part := range this.PartList {
		if part.Is_EXT_X_DISCONTINUITY && len(list) > 0 {
			discontinutyIdx++
		}
		if part.Key != nil {
			if part.Key.Method == EncryptMethod_NONE {
				curKey = nil
			} else {
				curKey = part.Key
			}
		}
		if part.Segment != nil {
			var seg = part.Segment
			index++
			var info = TsInfo{
				Name:                    fmt.Sprintf("%05d.ts", index), // ts视频片段命名规则
				URI:                     seg.URI,
				Url:                     "", // 之后填充
				Seq:                     beginSeq + uint64(index-1),
				TimeSec:                 seg.Duration,
				Idx_EXT_X_DISCONTINUITY: discontinutyIdx,
			}
			if curKey != nil {
				iv := []byte(strings.TrimPrefix(strings.ToLower(curKey.IV), "0x"))
				if len(iv) == 0 {
					if curKey.Method == EncryptMethod_AES128 {
						iv = make([]byte, 16)
						binary.BigEndian.PutUint64(iv[8:], info.Seq)
					}
				} else {
					var err error
					iv, err = hex.DecodeString(string(iv))
					if err != nil {
						iv = nil
					}
				}
				info.Key = TsKeyInfo{
					Method:     curKey.Method,
					KeyContent: nil, // 之后填充
					KeyURI:     curKey.URI,
					Iv:         iv,
				}
			}
			list = append(list, info)
		}
	}
	return list
}

// AesDecrypt 解密加密后的ts文件
func AesDecrypt(encrypted []byte, key TsKeyInfo) ([]byte, error) {
	block, err := aes.NewCipher(key.KeyContent)
	if err != nil {
		return nil, err
	}
	iv := key.Iv
	if len(iv) == 0 {
		return nil, errors.New("key URI " + key.KeyURI + ", invalid iv len(iv) == 0")
	}
	blockMode := cipher.NewCBCDecrypter(block, iv)
	if len(iv) == 0 || len(encrypted)%len(iv) != 0 {
		return nil, errors.New("invalid encrypted data len " + strconv.Itoa(len(encrypted)))
	}
	origData := make([]byte, len(encrypted))
	blockMode.CryptBlocks(origData, encrypted)
	length := len(origData)
	unpadding := int(origData[length-1])
	if length-unpadding < 0 {
		return nil, fmt.Errorf(`invalid length of unpadding %v - %v`, length, unpadding)
	}
	return origData[:(length - unpadding)], nil
}
