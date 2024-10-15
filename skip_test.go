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

	checkCase := func(rawExpr string, resultExpect ...uint32) {
		list := info.GetTsList()
		skipInfo, errMsg := ParseSkipTsExpr(rawExpr)
		if errMsg != "" {
			t.Fatal(errMsg, rawExpr)
		}
		after, _ := skipApplyFilter(list, skipInfo)
		if len(after) != len(resultExpect) {
			t.Fatal(rawExpr, len(after), len(resultExpect))
		}
		for idx, ts := range after {
			if ts.Idx != resultExpect[idx] {
				t.Fatal(rawExpr)
			}
		}
	}

	checkCase("", 1, 2, 3, 4, 5)
	checkCase("2-4", 1, 5)
	checkCase("3, 1", 2, 4, 5)
	checkCase("!time:00:00:00-00:00:20", 1, 2, 3, 4, 5)
	checkCase("!time:00:00:05-00:00:10", 1, 2, 3) // 测试“按时间保留”, 会保留边界ts
	checkCase("!time:00:00:06-00:00:10", 2, 3)    // 测试“按时间保留”, 会保留边界ts
	checkCase("time:00:00:01-00:00:12", 1, 4, 5)  // 测试“按时间跳过”, 会保留边界ts
}
