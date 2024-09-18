package m3u8d

import (
	"reflect"
	"testing"
)

func TestParseSkipTsExpr(t *testing.T) {
	info, errMsg := ParseSkipTsExpr("12-17, 1, 6, 19 - 121 , 122-125, 100-1000, http.code=403, http.code=404")
	if errMsg != "" {
		panic(errMsg)
	}
	ok1 := reflect.DeepEqual(info.SkipByIdxList, []SkipTsUnit{
		{
			Start: 1,
			End:   1,
		},
		{
			Start: 6,
			End:   6,
		},
		{
			Start: 12,
			End:   17,
		},
		{
			Start: 19,
			End:   1000,
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
			Start: 1,
			End:   1,
		},
		{
			Start: 90,
			End:   100,
		},
		{
			Start: 102,
			End:   122,
		},
		{
			Start: 900,
			End:   1000000000,
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
			Start: (1 * 60 * 60) + (23 * 60) + 45,
			End:   (12 * 60 * 60) + (34 * 60) + 56,
		},
		{
			Start: 0,
			End:   43,
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
	ok := reflect.DeepEqual(info.SkipByTimeSecList, []SkipTsUnit{
		{
			Start: 0,
			End:   (1 * 60 * 60) + (23 * 60) + 45,
		},
		{
			Start: (12 * 60 * 60) + (34 * 60) + 56,
			End:   SkipTimeSecEnd,
		},
	})
	if ok == false {
		t.Fatal()
	}
}
