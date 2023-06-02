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
