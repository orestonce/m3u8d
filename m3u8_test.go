package m3u8d

import (
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

func TestM3u8Parse(t *testing.T) {
	info := M3u8Parse(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="/20230502/xthms/2000kb/hls/key.key"
`)
	part := info.GetPart("#EXT-X-KEY")
	if part.KeyValue["METHOD"] != EncryptMethod_AES128 {
		panic("method")
	}
	if part.KeyValue["URI"] != "/20230502/xthms/2000kb/hls/key.key" {
		panic("uri")
	}
}

func TestGetFileNameFromUrl(t *testing.T) {
	{
		part := M3u8Parse(`#EXT-X-KEY:IV=0x10c27a9e3fa363dfe4c44b59b67304b3`).GetPart("#EXT-X-KEY")
		iv, err := hex.DecodeString(strings.TrimPrefix(part.KeyValue["IV"], "0x"))
		checkErr(err)
		if len(iv) != 16 {
			panic("iv " + strconv.Quote(string(iv)))
		}
	}
	{
		part := M3u8Parse(`#EXT-X-KEY:nothing`).GetPart("#EXT-X-KEY")
		iv, err := hex.DecodeString(strings.TrimPrefix(part.KeyValue["IV"], "0x"))
		checkErr(err)
		if len(iv) != 0 {
			panic("iv " + strconv.Quote(string(iv)))
		}
	}
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func TestM3u8Parse2(t *testing.T) {
	seq1 := parseBeginSeq([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0`))
	if seq1 != 0 {
		panic(seq1)
	}
	seq2 := parseBeginSeq([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:2`))
	if seq2 != 2 {
		panic(seq2)
	}

}

func TestM3u8Parse3(t *testing.T) {
	beginSeq := parseBeginSeq([]byte(m3u8Sample3))
	list, errMsg := getTsList(beginSeq, `https://www.example.com`, m3u8Sample3)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	after := skipApplyFilter(list, nil, true)
	if len(after) != 11 {
		t.Fatal()
	}
	if len(list) != 35 {
		t.Fatal()
	}
}

const m3u8Sample3 = `#EXTM3U
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
#EXTINF:6.12,
#EXT-X-DISCONTINUITY
#EXTINF:4.12,
/video/adjump/time/17137143014620000000.ts
#EXTINF:3,
/video/adjump/time/17137143014620000001.ts
#EXTINF:2.6,
/video/adjump/time/17137143014620000002.ts
#EXTINF:2.72,
/video/adjump/time/17137143014620000003.ts
#EXTINF:3,
/video/adjump/time/17137143014630000004.ts
#EXTINF:1.8,
/video/adjump/time/17137143014630000005.ts
#EXT-X-DISCONTINUITY
#EXTINF:3.36,
0000165.ts
#EXTINF:4,
0000478.ts
#EXTINF:4,
0000479.ts
#EXT-X-DISCONTINUITY
#EXTINF:4.12,
/video/adjump/time/17137143014620000000.ts
#EXTINF:3,
/video/adjump/time/17137143014620000001.ts
#EXTINF:2.6,
/video/adjump/time/17137143014620000002.ts
#EXTINF:2.72,
/video/adjump/time/17137143014620000003.ts
#EXTINF:3,
/video/adjump/time/17137143014630000004.ts
#EXTINF:1.8,
/video/adjump/time/17137143014630000005.ts
#EXT-X-DISCONTINUITY
#EXTINF:4,
0000480.ts
#EXT-X-DISCONTINUITY
#EXTINF:4.12,
/video/adjump/time/17137143014620000000.ts
#EXTINF:3,
/video/adjump/time/17137143014620000001.ts
#EXTINF:2.6,
/video/adjump/time/17137143014620000002.ts
#EXTINF:2.72,
/video/adjump/time/17137143014620000003.ts
#EXTINF:3,
/video/adjump/time/17137143014630000004.ts
#EXTINF:1.8,
/video/adjump/time/17137143014630000005.ts
#EXT-X-DISCONTINUITY
#EXTINF:4,
0000870.ts
#EXT-X-DISCONTINUITY
#EXTINF:4.12,
/video/adjump/time/17137143014620000000.ts
#EXTINF:3,
/video/adjump/time/17137143014620000001.ts
#EXTINF:2.6,
/video/adjump/time/17137143014620000002.ts
#EXTINF:2.72,
/video/adjump/time/17137143014620000003.ts
#EXTINF:3,
/video/adjump/time/17137143014630000004.ts
#EXTINF:1.8,
/video/adjump/time/17137143014630000005.ts
#EXT-X-DISCONTINUITY
#EXTINF:4,
0001281.ts
#EXTINF:1.72,
0001282.ts
#EXTINF:2.96,
0001414.ts
#EXT-X-ENDLIST
`
