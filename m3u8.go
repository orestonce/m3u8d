package m3u8d

import (
	"strconv"
	"strings"
)

type EncryptInfo struct {
	Method string
	Key    []byte
	Iv     []byte
}

type M3u8Content struct {
	PartList []M3u8Part `json:",omitempty"`
	TsList   []string   `json:",omitempty"`
}

type M3u8Part struct {
	Tag      string            `json:",omitempty"`
	TextFull string            `json:",omitempty"`
	KeyValue map[string]string `json:",omitempty"`
}

func (info M3u8Content) GetPart(tag string) M3u8Part {
	for _, one := range info.PartList {
		if one.Tag == tag {
			return one
		}
	}
	return M3u8Part{}
}

func M3u8Parse(content string) (info M3u8Content) {
	for _, line := range splitLineWithTrimSpace(content) {
		if strings.HasPrefix(line, "#") == false {
			continue
		}
		tmp := strings.SplitN(line, ":", 2)
		if len(tmp) < 2 {
			info.PartList = append(info.PartList, M3u8Part{
				Tag: tmp[0],
			})
			continue
		}
		var p M3u8Part
		p.Tag = tmp[0]
		p.TextFull = strings.TrimSpace(tmp[1])
		for _, kv := range strings.Split(tmp[1], ",") {
			data := strings.SplitN(kv, "=", 2)
			if len(data) < 2 {
				continue
			}
			key := data[0]
			value := strings.TrimSpace(data[1])
			if strings.HasPrefix(value, "\"") {
				var err error
				value, err = strconv.Unquote(value)
				if err != nil {
					continue
				}
			}
			if p.KeyValue == nil {
				p.KeyValue = map[string]string{}
			}
			p.KeyValue[key] = value
		}
		info.PartList = append(info.PartList, p)
	}
	return info
}
