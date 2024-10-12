package m3u8d

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/orestonce/m3u8d/mformat"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SkipTimeSecEnd = 99 * 60 * 60

type SkipTsUnit struct {
	Start uint32 // 包含
	End   uint32 // 包含
}

type SkipTsInfo struct {
	HttpCodeList      []int
	SkipByIdxList     []SkipTsUnit
	IfHttpCodeMergeTs bool
	SkipByTimeSecList []SkipTsUnit
}

func ParseSkipTsExpr(expr string) (info SkipTsInfo, errMsg string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return info, ""
	}
	list := strings.Split(expr, ",")
	singleRe := regexp.MustCompile(`^([0-9]+)$`)
	betweenRe := regexp.MustCompile(`^([0-9]+) *- *([0-9]+)$`)
	httpCodeRe := regexp.MustCompile(`^http.code *= *([0-9]+)$`)
	betweenTimeRe := regexp.MustCompile(`^(!?time) *: *(\d{2}:\d{2}:\d{2}) *- *(\d{2}:\d{2}:\d{2})$`)

	for _, one := range list {
		one = strings.TrimSpace(one)
		var groups []string
		var ok = false

		if groups = singleRe.FindStringSubmatch(one); len(groups) > 0 {
			i, err := strconv.Atoi(groups[1])
			if err == nil || i > 0 {
				ok = true
				info.SkipByIdxList = skipListAddUnit(info.SkipByIdxList, SkipTsUnit{
					Start: uint32(i),
					End:   uint32(i),
				})
			}
		} else if groups = betweenRe.FindStringSubmatch(one); len(groups) > 0 {
			i1, err1 := strconv.Atoi(groups[1])
			i2, err2 := strconv.Atoi(groups[2])
			if err1 == nil && err2 == nil && i1 > 0 && i2 > 0 && i1 <= i2 {
				ok = true
				info.SkipByIdxList = skipListAddUnit(info.SkipByIdxList, SkipTsUnit{
					Start: uint32(i1),
					End:   uint32(i2),
				})
			}
		} else if groups = httpCodeRe.FindStringSubmatch(one); len(groups) > 0 {
			i, err := strconv.Atoi(groups[1])
			if err == nil && i > 0 {
				ok = true
				if isInIntSlice(i, info.HttpCodeList) == false {
					info.HttpCodeList = append(info.HttpCodeList, i)
				}
			}
		} else if one == `if-http.code-merge_ts` {
			info.IfHttpCodeMergeTs = true
			ok = true
		} else if groups = betweenTimeRe.FindStringSubmatch(one); len(groups) > 0 {
			startSec, err1 := getTimeSecFromStr(groups[2])
			endSec, err2 := getTimeSecFromStr(groups[3])
			if err1 == nil && err2 == nil && startSec < endSec {
				if groups[1] == "time" {
					ok = true
					info.SkipByTimeSecList = skipListAddUnit(info.SkipByTimeSecList, SkipTsUnit{
						Start: startSec,
						End:   endSec,
					})
				} else if groups[1] == "!time" {
					ok = true
					info.SkipByTimeSecList = skipListAddUnit(info.SkipByTimeSecList, SkipTsUnit{
						Start: 0,
						End:   startSec,
					})
					info.SkipByTimeSecList = skipListAddUnit(info.SkipByTimeSecList, SkipTsUnit{
						Start: endSec,
						End:   SkipTimeSecEnd,
					})
				}
			}
		}
		if ok == false {
			return info, "parse expr part invalid " + strconv.Quote(one)
		}
	}
	sort.Slice(info.SkipByIdxList, func(i, j int) bool {
		a, b := info.SkipByIdxList[i], info.SkipByIdxList[j]
		return a.Start < b.Start
	})
	sort.Ints(info.HttpCodeList)
	return info, ""
}

func getTimeSecFromStr(str string) (sec uint32, err error) {
	var h, m, s uint32

	_, err = fmt.Sscanf(str, `%d:%d:%d`, &h, &m, &s)
	if err != nil {
		return 0, err
	}
	if m >= 60 || s >= 60 {
		return 0, errors.New("invalid str " + strconv.Quote(str))
	}
	sec = h*60*60 + m*60 + s
	return sec, nil
}

func maxUint32(a uint32, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func minUint32(a uint32, b uint32) uint32 {
	if a > b {
		return b
	}
	return a
}

func skipListAddUnit(skipList []SkipTsUnit, unit SkipTsUnit) (after []SkipTsUnit) {
	for idx, one := range skipList {
		// 交集的开始索引
		jStart := maxUint32(one.Start, unit.Start)
		// 交集的结束索引
		jEnd := minUint32(one.End, unit.End)
		// 有交集, 或者正好拼接为一个大区间10-20,21-30 => 10-30
		if jStart <= jEnd || jStart == jEnd-1 {
			unit.Start = minUint32(one.Start, unit.Start)
			unit.End = maxUint32(one.End, unit.End)
			var pre, post []SkipTsUnit // 前面部分，后面部分
			pre = skipList[:idx]
			if len(skipList) > idx+1 {
				post = skipList[idx+1:]
			}
			skipList = append(pre, post...)
			return skipListAddUnit(skipList, unit)
		}
	}
	// 都无交集
	skipList = append(skipList, unit)
	return skipList
}

func isSkipByTsTime(beginSec float64, endSec float64, list []SkipTsUnit) bool {
	for _, unit := range list {
		newBegin := math.Max(float64(unit.Start), beginSec)
		newEnd := math.Min(float64(unit.End), endSec)

		if newEnd > newBegin {
			return true
		}
	}
	return false
}

func skipApplyFilter(list []mformat.TsInfo, skipInfo SkipTsInfo) (after []mformat.TsInfo, skipList []mformat.TsInfo) {
	var hasEmptyExtinf bool
	for _, ts := range list {
		if ts.TimeSec < 1e-5 {
			hasEmptyExtinf = true
		}
	}
	isSkipByTsIndex := func(idx uint32) bool {
		for _, unit := range skipInfo.SkipByIdxList {
			if unit.Start <= idx && idx <= unit.End {
				return true
			}
		}
		return false
	}

	var timeBegin float64
	var timeEnd float64

	for idx, ts := range list {
		if idx > 0 {
			timeBegin += list[idx-1].TimeSec
		}
		timeEnd += ts.TimeSec

		if isSkipByTsIndex(uint32(idx) + 1) {
			skipList = append(skipList, ts)
			continue
		}
		if hasEmptyExtinf == false && isSkipByTsTime(timeBegin, timeEnd, skipInfo.SkipByTimeSecList) {
			skipList = append(skipList, ts)
			continue
		}
		after = append(after, ts)
	}
	return after, skipList
}

type removeSkipListResp struct {
	mergeTsList         []mformat.TsInfo
	skipByHttpCodeCount int
	skipLogFileName     string
	skipLogContent      []byte
}

func (this *DownloadEnv) removeSkipList(tsSaveDir string, list []mformat.TsInfo) (resp removeSkipListResp, err error) {
	if len(list) == 0 {
		return resp, errors.New("ts list is empty")
	}

	var inputVideoInfo *TsVideoInfo
	var skipByHttpCodeBuffer bytes.Buffer
	var skipByResolutionFpsBuffer bytes.Buffer

	this.status.SpeedResetBytes()
	this.status.SpeedResetTotalBlockCount(len(list))

	for _, one := range list {
		if this.GetIsCancel() {
			return resp, errors.New("用户取消")
		}
		this.status.SpeedAdd1Block(time.Now(), 0)
		if one.SkipByHttpCode {
			resp.skipByHttpCodeCount++
			if skipByHttpCodeBuffer.Len() == 0 {
				skipByHttpCodeBuffer.WriteString("skipByHttpCode\n")
			}
			fmt.Fprintf(&skipByHttpCodeBuffer, "filename=%v,url=%v，http.code=%v\n", one.Name, one.Url, one.HttpCode)
			continue
		}
		vInfo := GetTsVideoInfo(filepath.Join(tsSaveDir, one.Name))
		if inputVideoInfo == nil {
			inputVideoInfo = &vInfo
		}
		if vInfo.Fps == inputVideoInfo.Fps && vInfo.Width == inputVideoInfo.Width && vInfo.Height == inputVideoInfo.Height {
			resp.mergeTsList = append(resp.mergeTsList, one)
		} else {
			if skipByResolutionFpsBuffer.Len() == 0 {
				skipByResolutionFpsBuffer.WriteString("skipByResolutionFps\n")
			}
			fmt.Fprintf(&skipByResolutionFpsBuffer, "filename=%v,url=%v,resolution=%vx%v,fps=%v\n", one.Name, one.Url, vInfo.Width, vInfo.Height, vInfo.Fps)
		}
	}
	if skipByHttpCodeBuffer.Len() > 0 || skipByResolutionFpsBuffer.Len() > 0 {
		resp.skipLogFileName = filepath.Join(tsSaveDir, logFileName)
		resp.skipLogContent = append(skipByHttpCodeBuffer.Bytes(), skipByResolutionFpsBuffer.Bytes()...)
		err = os.WriteFile(resp.skipLogFileName, resp.skipLogContent, 0666)
		if err != nil {
			return resp, err
		}
	}

	// 写入ffmpeg合并命令
	err = this.writeFfmpegCmd(tsSaveDir, resp.mergeTsList)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

func AnalyzeTs(status *SpeedStatus, tsFileList []string, OutputMp4Name string, ctx context.Context) (mergeList []string, err error) {
	var skipByResolutionFpsBuffer bytes.Buffer
	status.SetProgressBarTitle("分析ts文件")
	status.SpeedResetTotalBlockCount(len(tsFileList))

	var inputVideoInfo *TsVideoInfo

	for _, one := range tsFileList {
		if IsContextCancel(ctx) {
			return nil, errors.New("用户取消")
		}
		vInfo := GetTsVideoInfo(one)
		if inputVideoInfo == nil {
			inputVideoInfo = &vInfo
		}
		if vInfo.Fps == inputVideoInfo.Fps && vInfo.Width == inputVideoInfo.Width && vInfo.Height == inputVideoInfo.Height {
			mergeList = append(mergeList, one)
		} else {
			if skipByResolutionFpsBuffer.Len() == 0 {
				skipByResolutionFpsBuffer.WriteString("skipByResolutionFps\n")
			}
			fmt.Fprintf(&skipByResolutionFpsBuffer, "filename=%v,resolution=%vx%v,fps=%v\n", one, vInfo.Width, vInfo.Height, vInfo.Fps)
		}
		status.SpeedAdd1Block(time.Now(), 0)
	}

	if skipByResolutionFpsBuffer.Len() > 0 {
		skipName := OutputMp4Name + "_skip.txt"
		err = os.WriteFile(skipName, skipByResolutionFpsBuffer.Bytes(), 0666)
		if err != nil {
			return nil, err
		}
	}
	return mergeList, nil
}
