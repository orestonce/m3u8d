package mformat

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestAesDecrypt(t *testing.T) {
	encInfo := TsKeyInfo{
		Method:     EncryptMethod_AES128,
		KeyContent: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6},
		Iv:         make([]byte, 16),
	}
	binary.BigEndian.PutUint64(encInfo.Iv[8:], 1)
	before := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 4}
	after, err := AesDecrypt(before, encInfo)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(after, []byte{69, 46, 52, 180, 68, 205, 99, 220, 193, 44, 116, 174, 96, 196, 199, 87, 214, 77, 67, 5, 37, 8, 139, 146, 229, 120, 164, 76, 107, 0, 204, 0}) == false {
		t.Fatal(after)
	}
}

func TestM3U8File_GetTsList(t *testing.T) {
	info, ok := M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-TARGETDURATION:8
#EXT-X-KEY:METHOD=AES-128,URI="/20230502/xthms/2000kb/hls/key.key",IV=0x10c27a9e3fa363dfe4c44b59b67304b3
#EXT-X-DISCONTINUITY
#EXTINF:4,
0000000.ts
#EXTINF:4.24,
0000001.ts
#EXTINF:4.08,
0000002.ts
#EXT-X-ENDLIST
`))
	if !ok || len(info.PartList) != 6 {
		t.Fatal()
	}

	list := info.GetTsList()
	if len(list) != 3 {
		t.Fatal()
	}
	if info.ContainsMediaSegment() == false {
		t.Fatal()
	}
	for _, one := range list {
		if one.Idx_EXT_X_DISCONTINUITY != 0 {
			t.Fatal(one.Idx_EXT_X_DISCONTINUITY, one.Seq)
		}
		switch one.Seq {
		case 0:
			if one.URI != `0000000.ts` || one.Name != "00001.ts" {
				t.Fatal(one.URI, one.Name)
			}
		case 1:
			if one.URI != `0000001.ts` || one.Name != "00002.ts" {
				t.Fatal(one.URI, one.Name)
			}
		case 2:
			if one.URI != `0000002.ts` || one.Name != "00003.ts" {
				t.Fatal(one.URI, one.Name)
			}
		}
	}
}

func TestM3U8File_LookupHDPlaylist(t *testing.T) {
	info, ok := M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=1280000,RESOLUTION=640x480
master_640x480.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2560000,RESOLUTION=1280x720
master_1280x720.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5120000,RESOLUTION=1920x1080
master_1920x1080.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5120000,RESOLUTION=2560x1440
master_2560X1440.m3u8
`))
	if ok == false || len(info.PartList) != 4 {
		t.Fatal()
	}

	is := info.IsNestedPlaylists()
	if is == false {
		t.Fatal()
	}

	playlist := info.LookupHDPlaylist()
	if playlist == nil || playlist.URI != "master_2560X1440.m3u8" {
		t.Fatal(playlist.URI)
	}
}
