package m3u8d

import (
	"fmt"
	"strconv"
	"testing"
)

func TestFindUrlInStr(t *testing.T) {
	urlStr := FindUrlInStr(" https://www.baidu.com/url1.jsp?sxjs=1&s=2&io=%%112 xs")
	if urlStr != "https://www.baidu.com/url1.jsp?sxjs=1&s=2&io=%%112" {
		fmt.Println(strconv.Quote(urlStr))
		t.Fail()
	}
}

func TestSkip(t *testing.T) {
	skip := isSkipByTsTime(10, 20, []SkipByTimeUnit{
		{
			StartSec: 0,
			EndSec:   6,
		},
	})
	if skip {
		t.Fatal()
	}

	skip = isSkipByTsTime(10, 20, []SkipByTimeUnit{
		{
			StartSec: 6,
			EndSec:   11,
		},
	})
	if skip == false {
		t.Fatal()
	}

	skip = isSkipByTsTime(10, 20, []SkipByTimeUnit{
		{
			StartSec: 12,
			EndSec:   17,
		},
	})
	if skip == false {
		t.Fatal()
	}

	skip = isSkipByTsTime(10, 20, []SkipByTimeUnit{
		{
			StartSec: 18,
			EndSec:   28,
		},
	})
	if skip == false {
		t.Fatal()
	}

	skip = isSkipByTsTime(10, 20, []SkipByTimeUnit{
		{
			StartSec: 21,
			EndSec:   28,
		},
	})
	if skip {
		t.Fatal()
	}
}
