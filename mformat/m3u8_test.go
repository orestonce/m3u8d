package mformat

import (
	"reflect"
	"strconv"
	"testing"
)

func TestM3U8Parse(t *testing.T) {
	info, ok := M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="/20230502/xthms/2000kb/hls/key.key",IV=0x10c27a9e3fa363dfe4c44b59b67304b3
`))
	if !ok {
		t.Fatal()
	}

	if info.Version != 3 {
		t.Fatal()
	}
	if info.TargetDuration != 6 {
		t.Fatal()
	}

	if info.MediaSequence != 0 {
		t.Fatal()
	}

	var hasKey = false

	for _, part := range info.PartList {
		if part.Key != nil {
			key := part.Key
			if key.Method != "AES-128" || key.URI != "/20230502/xthms/2000kb/hls/key.key" || key.IV != "0x10c27a9e3fa363dfe4c44b59b67304b3" {
				t.Fatal()
			}
			hasKey = true
		}
	}
	if hasKey == false {
		t.Fatal()
	}
}

func TestM3U8Parse2(t *testing.T) {
	_, ok := M3U8Parse([]byte(`#EXTM3U`))
	if !ok {
		t.Fatal()
	}

	_, ok = M3U8Parse([]byte(`#EXTM3U8`))
	if ok {
		t.Fatal()
	}

	_, ok = M3U8Parse(nil)
	if ok {
		t.Fatal()
	}
}

func TestM3U8Parse3(t *testing.T) {
	info, ok := M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=1280000,RESOLUTION=640x480
master_640x480.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2560000,RESOLUTION=1280x720
master_1280x720.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5120000,RESOLUTION=1920x1080
master_1920x1080.m3u8`))
	if ok == false || len(info.PartList) != 3 {
		t.Fatal()
	}

	var playList []M3U8Playlist
	for _, one := range info.PartList {
		if one.Is_EXT_X_DISCONTINUITY || one.Is_EXT_X_ENDLIST || one.Key != nil || one.Segment != nil || one.Playlist == nil {
			t.Fatal()
		}
		playList = append(playList, *one.Playlist)
	}

	ok = reflect.DeepEqual(playList, []M3U8Playlist{
		{
			URI: "master_640x480.m3u8",
			Resolution: M3U8Resolution{
				Width:  640,
				Height: 480,
			},
			Bandwidth: 1280000,
		},
		{
			URI: "master_1280x720.m3u8",
			Resolution: M3U8Resolution{
				Width:  1280,
				Height: 720,
			},
			Bandwidth: 2560000,
		},
		{
			URI: "master_1920x1080.m3u8",
			Resolution: M3U8Resolution{
				Width:  1920,
				Height: 1080,
			},
			Bandwidth: 5120000,
		},
	})

	if !ok {
		t.Fatal()
	}
}

func TestM3U8Parse4(t *testing.T) {
	info, ok := M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-TARGETDURATION:8
#EXT-X-DISCONTINUITY
#EXTINF:4,
0000000.ts
#EXTINF:4.24,
0000001.ts
#EXTINF:4.08,
0000002.ts
#EXT-X-ENDLIST
`))
	if !ok || len(info.PartList) != 5 {
		t.Fatal()
	}

	p0 := info.PartList[0]
	if p0.Is_EXT_X_DISCONTINUITY == false || p0.Is_EXT_X_ENDLIST || p0.Key != nil || p0.Playlist != nil || p0.Segment != nil {
		t.Fatal()
	}

	p2 := info.PartList[2]
	if p2.Is_EXT_X_DISCONTINUITY || p2.Is_EXT_X_ENDLIST || p2.Playlist != nil || p2.Key != nil {
		t.Fatal()
	}
	if p2.Segment.Title != "" || p2.Segment.URI != "0000001.ts" {
		t.Fatal()
	}
	sec := p2.Segment.Duration
	secStr := strconv.FormatFloat(sec, 'f', 2, 64)
	if secStr != "4.24" {
		t.Fatal(sec, secStr)
	}

	p4 := info.PartList[4]
	if p4.Is_EXT_X_ENDLIST == false || p4.Is_EXT_X_DISCONTINUITY || p4.Key != nil || p4.Segment != nil || p4.Playlist != nil {
		t.Fatal()
	}
}

func TestM3U8Parse5(t *testing.T) {
	info, ok := M3U8Parse([]byte(`#EXTM3U
#EXT-X-STREAM-INF:PROGRAM-ID=1,BANDWIDTH=800000,RESOLUTION=1920x1080
2000k/hls/mixed.m3u8`))
	if ok == false || len(info.PartList) != 1 {
		t.Fatal()
	}
	p0 := info.PartList[0]
	if p0.Is_EXT_X_ENDLIST || p0.Is_EXT_X_DISCONTINUITY || p0.Key != nil || p0.Segment != nil || p0.Playlist == nil {
		t.Fatal()
	}
	playlist := p0.Playlist
	ok = reflect.DeepEqual(playlist, &M3U8Playlist{
		URI: "2000k/hls/mixed.m3u8",
		Resolution: M3U8Resolution{
			Width:  1920,
			Height: 1080,
		},
		Bandwidth: 800000,
	})
	if ok == false {
		t.Fatal()
	}
}

func TestM3U8Parse6(t *testing.T) {
	info, ok := M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:4
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="/20240916/yOPVFSK2/2000kb/hls/key.key"
#EXTINF:2,
/20240916/yOPVFSK2/2000kb/hls/qa8prnvU.ts
#EXTINF:2,
/20240916/yOPVFSK2/2000kb/hls/ZSROhpt2.ts
#EXTINF:2,
/20240916/yOPVFSK2/2000kb/hls/oXwblJIJ.ts
#EXT-X-DISCONTINUITY
#EXT-X-KEY:METHOD=NONE
#EXTINF:4.44,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/VqKz3x8J.ts
#EXTINF:3,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/hu7W1VcP.ts
#EXTINF:3,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/eIKABbW9.ts
#EXTINF:1.76,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/vyliJqOQ.ts
#EXTINF:2.8,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/X8lm3Te1.ts
#EXTINF:3,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/6dRPUkHF.ts
#EXTINF:1,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/wDuqI6ER.ts
#EXT-X-DISCONTINUITY
#EXT-X-KEY:METHOD=AES-128,URI="/20240916/yOPVFSK2/2000kb/hls/key.key"
#EXTINF:2,
/20240916/yOPVFSK2/2000kb/hls/fXWY6Ld2.ts
#EXTINF:2,
/20240916/yOPVFSK2/2000kb/hls/WrhoJ2Lt.ts
#EXTINF:2,
/20240916/yOPVFSK2/2000kb/hls/qwaK0jsx.ts
#EXTINF:2,
/20240916/yOPVFSK2/2000kb/hls/ydRRH9Oa.ts
#EXTINF:0.36,
/20240916/yOPVFSK2/2000kb/hls/xQPVryWB.ts
#EXT-X-DISCONTINUITY
#EXT-X-KEY:METHOD=NONE
#EXTINF:4.44,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/VqKz3x8J.ts
#EXTINF:3,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/hu7W1VcP.ts
#EXTINF:3,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/eIKABbW9.ts
#EXTINF:1.76,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/vyliJqOQ.ts
#EXTINF:2.8,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/X8lm3Te1.ts
#EXTINF:3,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/6dRPUkHF.ts
#EXTINF:1,
https://bobo.example.com/20240816/mU723LBo/2000kb/hls/wDuqI6ER.ts
#EXT-X-ENDLIST`))
	if ok == false || len(info.PartList) != 30 {
		t.Fatal(len(info.PartList))
	}

	p0 := info.PartList[0].Key
	if p0 == nil || p0.Method != EncryptMethod_AES128 || p0.URI != `/20240916/yOPVFSK2/2000kb/hls/key.key` {
		t.Fatal()
	}

	p5 := info.PartList[5].Key
	if p5 == nil || p5.Method != EncryptMethod_NONE {
		t.Fatal()
	}

	p14 := info.PartList[14].Key
	if p14 == nil || p14.Method != EncryptMethod_AES128 || p14.URI != `/20240916/yOPVFSK2/2000kb/hls/key.key` {
		t.Fatal()
	}

	p21 := info.PartList[5].Key
	if p21 == nil || p21.Method != EncryptMethod_NONE {
		t.Fatal()
	}
}
