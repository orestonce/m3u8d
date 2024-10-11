package mformat

import (
	"bufio"
	"bytes"
	"io"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
)

// https://datatracker.ietf.org/doc/html/rfc8216#section-4.3.2.4
const (
	EncryptMethod_NONE       = `NONE`
	EncryptMethod_AES128     = `AES-128`
	EncryptMethod_SIMPLE_AES = `SAMPLE-AES` // TODO
)

type M3U8File struct {
	Version        int
	MediaSequence  int
	TargetDuration float64 // 秒
	PartList       []M3U8Part
}

type M3U8Part struct {
	Key                    *M3U8Key
	Segment                *M3U8Segment
	Playlist               *M3U8Playlist
	Is_EXT_X_DISCONTINUITY bool //#EXT-X-DISCONTINUITY
	Is_EXT_X_ENDLIST       bool //#EXT-X-ENDLIST
}

type M3U8Key struct {
	Method string
	URI    string
	IV     string
}

type M3U8Segment struct {
	URI      string
	Duration float64 // 秒
	Title    string
}

type M3U8Playlist struct {
	URI        string
	Bandwidth  int
	Resolution M3U8Resolution
}

type M3U8Resolution struct {
	Width  int
	Height int
}

func M3U8Parse(content []byte) (info M3U8File, ok bool) {
	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(content)))
	line, err := reader.ReadLine()
	if err != nil {
		return info, false
	}
	line = strings.TrimSpace(line)
	if line != "#EXTM3U" {
		return info, false
	}

	rVersion := regexp.MustCompile(`^#EXT-X-VERSION:([0-9]+)`)
	rSeq := regexp.MustCompile(`^#EXT-X-MEDIA-SEQUENCE:([0-9]+)`)
	rDur := regexp.MustCompile(`^#EXT-X-TARGETDURATION:([0-9.]+)`)
	rKey := regexp.MustCompile(`^#EXT-X-KEY:`)
	rDiscontinuity := regexp.MustCompile(`^#EXT-X-DISCONTINUITY`)
	rMedia := regexp.MustCompile(`^#EXTINF:`)
	rEndList := regexp.MustCompile(`^#EXT-X-ENDLIST`)
	rPlaylist := regexp.MustCompile(`^#EXT-X-STREAM-INF:`)

	var curSeg *M3U8Segment
	var curPlaylist *M3U8Playlist

	for {
		line, err = reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				return info, true
			}
			return info, false
		}
		line = strings.TrimSpace(line)
		switch {
		case rVersion.MatchString(line):
			var version int
			version, err = strconv.Atoi(rVersion.FindStringSubmatch(line)[1])
			if err == nil {
				info.Version = version
			}
		case rSeq.MatchString(line):
			var seq int
			seq, err = strconv.Atoi(rSeq.FindStringSubmatch(line)[1])
			if err == nil {
				info.MediaSequence = seq
			}
		case rDur.MatchString(line):
			var dur float64
			dur, err = strconv.ParseFloat(rDur.FindStringSubmatch(line)[1], 64)
			if err == nil {
				info.TargetDuration = dur
			}
		case rKey.MatchString(line):
			var key M3U8Key
			for _, part := range splitTagPropertyPart(line) {
				if strings.HasPrefix(part, "METHOD=") {
					key.Method = strings.TrimPrefix(part, "METHOD=")
				}
				if strings.HasPrefix(part, "URI=") {
					key.URI = strings.TrimPrefix(part, "URI=")
					if strings.HasPrefix(key.URI, `"`) {
						key.URI, _ = strconv.Unquote(key.URI)
					}
				}
				if strings.HasPrefix(part, "IV=") {
					key.IV = strings.TrimPrefix(part, "IV=")
				}
			}
			info.PartList = append(info.PartList, M3U8Part{
				Key: &key,
			})
		case rDiscontinuity.MatchString(line):
			info.PartList = append(info.PartList, M3U8Part{
				Is_EXT_X_DISCONTINUITY: true,
			})
		case rEndList.MatchString(line):
			info.PartList = append(info.PartList, M3U8Part{
				Is_EXT_X_ENDLIST: true,
			})
			return info, true
		case rMedia.MatchString(line):
			if curSeg == nil {
				curSeg = &M3U8Segment{}
			}
			temp := splitTagPropertyPart(line)
			if len(temp) > 0 {
				var dur float64
				dur, err = strconv.ParseFloat(temp[0], 64)
				if err == nil {
					curSeg.Duration = dur
				}
			}
			if len(temp) > 1 {
				curSeg.Title = temp[1]
			}
		case rPlaylist.MatchString(line):
			if curPlaylist == nil {
				curPlaylist = &M3U8Playlist{}
			}
			rResolution := regexp.MustCompile(`^RESOLUTION=([0-9]+)x([0-9]+)$`)
			rBandwidth := regexp.MustCompile(`^BANDWIDTH=([0-9]+)$`)
			for _, part := range splitTagPropertyPart(line) {
				if rResolution.MatchString(part) {
					groups := rResolution.FindStringSubmatch(part)
					w, err1 := strconv.Atoi(groups[1])
					h, err2 := strconv.Atoi(groups[2])
					if err1 == nil && err2 == nil {
						curPlaylist.Resolution = M3U8Resolution{
							Width:  w,
							Height: h,
						}
					}
				}
				if rBandwidth.MatchString(part) {
					var bw int
					bw, err = strconv.Atoi(rBandwidth.FindStringSubmatch(part)[1])
					if err == nil {
						curPlaylist.Bandwidth = bw
					}
				}
			}
		default:
			if strings.HasPrefix(line, "#") {
				break
			}
			if curSeg == nil && curPlaylist == nil {
				curSeg = &M3U8Segment{}
			}
			if curSeg != nil {
				curSeg.URI = line
				info.PartList = append(info.PartList, M3U8Part{
					Segment: curSeg,
				})
			} else if curPlaylist != nil {
				curPlaylist.URI = line
				info.PartList = append(info.PartList, M3U8Part{
					Playlist: curPlaylist,
				})
			}
			curSeg = nil
			curPlaylist = nil
		}
	}
}

// splitTagPropertyPart
// 以逗号为分隔符拆分冒号以后的部分为字符串列表
//
//	例子 #EXT-X-KEY:METHOD=AES-128,URI="/20230502/xthms/2000kb/hls/key.key"
//			METHOD=AES-128
//			URI="/20230502/xthms/2000kb/hls/key.key"
func splitTagPropertyPart(line string) (list []string) {
	temp := strings.SplitN(line, ":", 2)
	if len(temp) != 2 {
		return nil
	}
	for _, part := range strings.Split(temp[1], ",") {
		part = strings.TrimSpace(part)
		list = append(list, part)
	}
	return list
}
