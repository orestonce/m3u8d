package m3u8d

import (
	"bytes"
	"context"
	"errors"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-mp4"
	"github.com/yapingcat/gomedia/go-mpeg2"
	"io/ioutil"
	"os"
	"strconv"
)

type MergeTsFileListToSingleMp4_Req struct {
	TsFileList []string
	OutputMp4  string
	Status     *SpeedStatus
	Ctx        context.Context
}

func MergeTsFileListToSingleMp4(req MergeTsFileListToSingleMp4_Req) (err error) {
	mp4file, err := os.OpenFile(req.OutputMp4, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer mp4file.Close()

	if req.Status != nil {
		req.Status.SpeedResetBytes()
	}

	muxer, err := mp4.CreateMp4Muxer(mp4file)
	if err != nil {
		return err
	}
	vtid := muxer.AddVideoTrack(mp4.MP4_CODEC_H264)
	atid := muxer.AddAudioTrack(mp4.MP4_CODEC_AAC)

	demuxer := mpeg2.NewTSDemuxer()
	var OnFrameErr error
	var audioTimestamp uint64 = 0
	aacSampleRate := -1
	demuxer.OnFrame = func(cid mpeg2.TS_STREAM_TYPE, frame []byte, pts uint64, dts uint64) {
		if OnFrameErr != nil {
			return
		}
		if cid == mpeg2.TS_STREAM_AAC {
			audioTimestamp = pts
			codec.SplitAACFrame(frame, func(aac []byte) {
				if aacSampleRate == -1 {
					adts := codec.NewAdtsFrameHeader()
					adts.Decode(aac)
					aacSampleRate = codec.AACSampleIdxToSample(int(adts.Fix_Header.Sampling_frequency_index))
				}
				err = muxer.Write(atid, aac, audioTimestamp, audioTimestamp)
				audioTimestamp += uint64(1024 * 1000 / aacSampleRate) //每帧aac采样固定为1024。aac_sampleRate 为采样率
				if err != nil {
					OnFrameErr = err
					return
				}
			})
		} else if cid == mpeg2.TS_STREAM_H264 {
			err = muxer.Write(vtid, frame, pts, dts)
			if err != nil {
				OnFrameErr = err
				return
			}
		} else {
			OnFrameErr = errors.New("unknown cid " + strconv.Itoa(int(cid)))
			return
		}
	}

	if req.Status != nil {
		req.Status.ResetTotalBlockCount(len(req.TsFileList))
	}
	for _, tsFile := range req.TsFileList {
		select {
		case <-req.Ctx.Done():
			return req.Ctx.Err()
		default:
		}
		var buf []byte
		buf, err = ioutil.ReadFile(tsFile)
		if err != nil {
			return err
		}
		err = demuxer.Input(bytes.NewReader(buf))
		if err != nil {
			return err
		}
		if OnFrameErr != nil {
			return OnFrameErr
		}
		if req.Status != nil {
			req.Status.SpeedAdd1Block(len(buf))
		}
	}

	err = muxer.WriteTrailer()
	if err != nil {
		return err
	}
	err = mp4file.Sync()
	if err != nil {
		return err
	}
	if req.Status != nil {
		req.Status.DrawProgressBar(1, 1)
	}
	return nil
}
