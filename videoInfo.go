package m3u8d

import (
	"bytes"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-mpeg2"
	"os"
)

type TsVideoInfo struct {
	Width  uint32
	Height uint32
	Fps    int
}

// GetTsVideoInfo
// 参考: https://github.com/yapingcat/gomedia/issues/154
func GetTsVideoInfo(tsPath string) (info TsVideoInfo) {
	demuxer := mpeg2.NewTSDemuxer()
	demuxer.OnFrame = func(cid mpeg2.TS_STREAM_TYPE, frame []byte, pts uint64, dts uint64) {
		if cid == mpeg2.TS_STREAM_H264 {
			codec.SplitFrameWithStartCode(frame, func(nalu []byte) bool {
				naluType := codec.H264NaluType(nalu)
				if naluType == codec.H264_NAL_SPS {
					info.Width, info.Height = codec.GetH264Resolution(nalu)
					start, sc := codec.FindStartCode(nalu, 0)
					sodb := codec.CovertRbspToSodb(nalu[start+int(sc)+1:])
					bs := codec.NewBitStream(sodb)
					var s codec.SPS
					s.Decode(bs)
					if s.VuiParameters.NumUnitsInTick > 0 {
						info.Fps = int(s.VuiParameters.TimeScale / s.VuiParameters.NumUnitsInTick / 2)
					}
					return false
				}
				return true
			})
		}
	}

	data, err := os.ReadFile(tsPath)
	if err != nil {
		return TsVideoInfo{}
	}

	err = demuxer.Input(bytes.NewReader(data))
	if err != nil {
		return TsVideoInfo{}
	}

	return info
}
