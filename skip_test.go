package m3u8d

import (
	"github.com/orestonce/m3u8d/mformat"
	"reflect"
	"testing"
)

func TestParseSkipTsExpr(t *testing.T) {
	info, errMsg := ParseSkipTsExpr("12-17, 1, 19 - 121, 6, 122-125, 100-1000, http.code=403, http.code=404")
	if errMsg != "" {
		panic(errMsg)
	}
	ok1 := reflect.DeepEqual(info.SkipByIdxList, []SkipTsUnit{
		{
			Start:      12,
			End:        17,
			OriginExpr: "12-17",
		},
		{
			Start:      1,
			End:        1,
			OriginExpr: "1",
		},
		{
			Start:      19,
			End:        121,
			OriginExpr: "19 - 121",
		},
		{
			Start:      6,
			End:        6,
			OriginExpr: "6",
		},
		{
			Start:      122,
			End:        125,
			OriginExpr: "122-125",
		},
		{
			Start:      100,
			End:        1000,
			OriginExpr: "100-1000",
		},
	})
	if ok1 == false {
		t.Fatal(info.SkipByIdxList)
	}

	ok2 := reflect.DeepEqual(info.HttpCodeList, []int{403, 404})
	if ok2 == false {
		t.Fatal()
	}
}

func TestParseSkipTsExpr2(t *testing.T) {
	list, errMsg := ParseSkipTsExpr("1,90-100,102-122,900-1000000000")
	if errMsg != "" {
		panic(errMsg)
	}
	ok := reflect.DeepEqual(list.SkipByIdxList, []SkipTsUnit{
		{
			Start:      1,
			End:        1,
			OriginExpr: "1",
		},
		{
			Start:      90,
			End:        100,
			OriginExpr: "90-100",
		},
		{
			Start:      102,
			End:        122,
			OriginExpr: "102-122",
		},
		{
			Start:      900,
			End:        1000000000,
			OriginExpr: "900-1000000000",
		},
	})
	if ok == false {
		t.Fatal()
	}
}

func TestParseSkipTsExpr3(t *testing.T) {
	info, errMsg := ParseSkipTsExpr("12-17, time:01:23:45-12:34:56,time:00:00:00-00:00:43")
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	ok := reflect.DeepEqual(info.SkipByTimeSecList, []SkipTsUnit{
		{
			Start:      (1 * 60 * 60) + (23 * 60) + 45,
			End:        (12 * 60 * 60) + (34 * 60) + 56,
			OriginExpr: "time:01:23:45-12:34:56",
		},
		{
			Start:      0,
			End:        43,
			OriginExpr: "time:00:00:00-00:00:43",
		},
	})
	if ok == false {
		t.Fatal()
	}
}

func TestParseSkipTsExpr4(t *testing.T) {
	info, errMsg := ParseSkipTsExpr("!time:01:23:45-12:34:56")
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	ok := reflect.DeepEqual(info.KeepByTimeSecList, []SkipTsUnit{
		{
			Start:      (1 * 60 * 60) + (23 * 60) + 45,
			End:        (12 * 60 * 60) + (34 * 60) + 56,
			OriginExpr: "!time:01:23:45-12:34:56",
		},
	})
	if ok == false {
		t.Fatal()
	}
}

func TestParseSkipTsExpr5(t *testing.T) {
	info, ok := mformat.M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="key.key"
#EXTINF:5.867,0-5.867秒
1.ts
#EXTINF:2.933,5.867-8.8秒
2.ts
#EXTINF:2.933,8.8-11.733秒
3.ts
#EXTINF:2.927,11.733-14.66秒
4.ts
#EXTINF:2.582,14.66-17.242秒
5.ts
#EXT-X-ENDLIST`))
	if ok == false {
		t.Fatal()
	}

	checkCase(info, "", 1, 2, 3, 4, 5)
	checkCase(info, "!tag:2-4", 1, 5)
	checkCase(info, "3, 1", 2, 4, 5)
	checkCase(info, "!time:00:00:00-00:00:20", 1, 2, 3, 4, 5)
	checkCase(info, "!time:00:00:05-00:00:10", 1, 2, 3) // 测试“按时间保留”, 会保留边界ts
	checkCase(info, "!time:00:00:06-00:00:10", 2, 3)    // 测试“按时间保留”, 会保留边界ts
	checkCase(info, "time:00:00:01-00:00:12", 1, 4, 5)  // 测试“按时间跳过”, 会保留边界ts
}

func TestParseSkipTsExpr6(t *testing.T) {
	info, ok := mformat.M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-KEY:METHOD=AES-128,URI="key.key"
# TS001=00:00.00-00:09.59
# TS002=00:10.00-00:19.59
# TS003=00:20.00-00:29.59
#EXTINF:600
1.ts
#EXTINF:600
2.ts
#EXTINF:600
3.ts
#EXT-X-ENDLIST`))
	if ok == false {
		t.Fatal()
	}

	checkCase(info, "!time:00:00:05-00:00:10", 1)
}

func checkCase(info mformat.M3U8File, rawExpr string, resultExpect ...uint32) {
	list := info.GetTsList()
	skipInfo, errMsg := ParseSkipTsExpr(rawExpr)
	if errMsg != "" {
		panic(errMsg + "\n" + rawExpr)
	}
	after, _ := skipApplyFilter(list, skipInfo)
	if len(after) != len(resultExpect) {
		panic(rawExpr)
	}
	for idx, ts := range after {
		if ts.Idx != resultExpect[idx] {
			panic(rawExpr)
		}
	}
}

func TestM3U8Parse7(t *testing.T) {
	info, ok := mformat.M3U8Parse([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-KEY:METHOD=AES-128,URI="../key",IV=0x00000000000000000000000000000000
#EXTINF:2.166667,
https://example.io/bk1
#EXTINF:10.416667,
https://example.io/bk2
#EXTINF:10.416667,`))
	if ok == false {
		t.Fatal()
	}
	list := info.GetTsList()
	//data, _ := json.Marshal(list)
	//fmt.Println(string(data))

	errMsg := UpdateMediaKeyContent(`https://www.example.com/1080P.m3u8`, list, func(urlStr string) (content []byte, err error) {
		return []byte(`ba9ab15653b9fa216d921dd43a08e280`), nil
	})
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	//data, _ = json.MarshalIndent(list, "", "\t")
	//fmt.Println(string(data))

	errMsg = UpdateMediaKeyContent(`https://www.example.com/1080P.m3u8`, list, func(urlStr string) (content []byte, err error) {
		return []byte(`ba9ab15653b9fa13`), nil
	})
	if errMsg != "" {
		t.Fatal(errMsg)
	}
}
