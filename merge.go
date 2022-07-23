package m3u8d

import (
	"bytes"
	"context"
	"errors"
	"github.com/yapingcat/gomedia/mp4"
	"github.com/yapingcat/gomedia/mpeg2"
	"io/ioutil"
	"os"
	"strconv"
)

type MergeTsFileListToSingleMp4_Req struct {
	TsFileList []string
	OutputMp4  string
	ctx        context.Context
}

func MergeTsFileListToSingleMp4(req MergeTsFileListToSingleMp4_Req) (err error) {
	mp4file, err := os.OpenFile(req.OutputMp4, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer mp4file.Close()

	muxer, err := mp4.CreateMp4Muxer(mp4file)
	if err != nil {
		return err
	}
	vtid := muxer.AddVideoTrack(mp4.MP4_CODEC_H264)
	atid := muxer.AddAudioTrack(mp4.MP4_CODEC_AAC)

	demuxer := mpeg2.NewTSDemuxer()
	var OnFrameErr error
	demuxer.OnFrame = func(cid mpeg2.TS_STREAM_TYPE, frame []byte, pts uint64, dts uint64) {
		if OnFrameErr != nil {
			return
		}
		if cid == mpeg2.TS_STREAM_AAC {
			OnFrameErr = muxer.Write(atid, frame, pts, dts)
		} else if cid == mpeg2.TS_STREAM_H264 {
			OnFrameErr = muxer.Write(vtid, frame, pts, dts)
		} else {
			OnFrameErr = errors.New("unknown cid " + strconv.Itoa(int(cid)))
		}
	}

	for idx, tsFile := range req.TsFileList {
		select {
		case <-req.ctx.Done():
			return req.ctx.Err()
		default:
		}
		DrawProgressBar(len(req.TsFileList), idx)
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
	}

	err = muxer.WriteTrailer()
	if err != nil {
		return err
	}
	err = mp4file.Sync()
	if err != nil {
		return err
	}
	DrawProgressBar(1, 1)
	return nil
}
