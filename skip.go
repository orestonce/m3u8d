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
	"strconv"
	"strings"
	"time"
)

const SkipTimeSecEnd = 99 * 60 * 60

type SkipTsUnit struct {
	Start      uint32 // 包含
	End        uint32 // 包含
	OriginExpr string // 原始表达式
}

type SkipTsInfo struct {
	HttpCodeList      []int
	SkipByIdxList     []SkipTsUnit
	IfHttpCodeMergeTs bool
	SkipByTimeSecList []SkipTsUnit
	KeepByTimeSecList []SkipTsUnit
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
				info.SkipByIdxList = append(info.SkipByIdxList, SkipTsUnit{
					Start:      uint32(i),
					End:        uint32(i),
					OriginExpr: one,
				})
			}
		} else if groups = betweenRe.FindStringSubmatch(one); len(groups) > 0 {
			i1, err1 := strconv.Atoi(groups[1])
			i2, err2 := strconv.Atoi(groups[2])
			if err1 == nil && err2 == nil && i1 > 0 && i2 > 0 && i1 <= i2 {
				ok = true
				info.SkipByIdxList = append(info.SkipByIdxList, SkipTsUnit{
					Start:      uint32(i1),
					End:        uint32(i2),
					OriginExpr: one,
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
					info.SkipByTimeSecList = append(info.SkipByTimeSecList, SkipTsUnit{
						Start:      startSec,
						End:        endSec,
						OriginExpr: one,
					})
				} else if groups[1] == "!time" {
					ok = true
					info.KeepByTimeSecList = append(info.KeepByTimeSecList, SkipTsUnit{
						Start:      startSec,
						End:        endSec,
						OriginExpr: one,
					})
				}
			}
		}
		if ok == false {
			return info, "parse expr part invalid " + strconv.Quote(one)
		}
	}
	return info, ""
}

func (this *SkipTsUnit) IsCoverageFull(begin float64, end float64) bool {
	return float64(this.Start) <= begin && float64(this.End) >= end
}

func (this *SkipTsUnit) HasIntersect(begin float64, end float64) bool {
	newBegin := math.Max(begin, float64(this.Start))
	newEnd := math.Min(end, float64(this.End))
	return newBegin <= newEnd
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

type skipFilterRecord struct {
	ts     mformat.TsInfo
	reason string
}

// skipApplyFilter
//
//	策略:
//		确认每个ts文件都解析到了持续时常。
//			如果存在没有未解析到时间的ts文件，则不应用"按时间保留"、"按时间跳过"的规则
//			否则
//				1. 应用"按时间保留"规则, 把不需要保留的都剔除
//				2. 应用"按时间跳过"规则, 把需要跳过的都剔除
//		应用按照编号跳过的规则
func skipApplyFilter(list []mformat.TsInfo, skipInfo SkipTsInfo) (after []mformat.TsInfo, skipList []skipFilterRecord) {
	timeRange, ok := calculateTsTimeRange(list)

	if ok {
		//应用"按时间保留"规则, 把不需要保留的都剔除
		if len(skipInfo.KeepByTimeSecList) > 0 {
			var keepIdxList = make([]bool, len(list)) // 每个是否保留

			for _, rule := range skipInfo.KeepByTimeSecList {
				for idx, tsTime := range timeRange {
					if keepIdxList[idx] {
						continue
					}
					//规则只要覆盖到了ts, 就要保留
					if rule.HasIntersect(tsTime.begin, tsTime.end) {
						keepIdxList[idx] = true
					}
				}
			}
			var newList []mformat.TsInfo
			for idx, keep := range keepIdxList {
				if keep == false {
					skipList = append(skipList, skipFilterRecord{
						ts:     list[idx],
						reason: "不在!time指明的时间范围内",
					})
				} else {
					newList = append(newList, list[idx])
				}
			}
			list = newList
		}

		//应用"按时间跳过"规则, 把需要跳过的都剔除
		if len(skipInfo.SkipByTimeSecList) > 0 {
			var newList []mformat.TsInfo

			for idx, tsTime := range timeRange {
				var match = false
				for _, rule := range skipInfo.SkipByTimeSecList {
					if rule.IsCoverageFull(tsTime.begin, tsTime.end) {
						skipList = append(skipList, skipFilterRecord{
							ts:     list[idx],
							reason: "匹配表达式" + rule.OriginExpr,
						})
						match = true
						break
					}
				}
				if match == false {
					newList = append(newList, list[idx])
				}
			}
			list = newList
		}
	}

	if len(skipInfo.SkipByIdxList) > 0 {
		var newList []mformat.TsInfo
		for _, ts := range list {
			var match = false
			for _, rule := range skipInfo.SkipByIdxList {
				if rule.Start <= ts.Idx && ts.Idx <= rule.End {
					skipList = append(skipList, skipFilterRecord{
						ts:     ts,
						reason: "匹配表达式" + rule.OriginExpr,
					})
					match = true
					break
				}
			}
			if match == false {
				newList = append(newList, ts)
			}
		}
		list = newList
	}
	return list, skipList
}

type tsTimeRangeUnit struct {
	begin float64
	end   float64
}

func calculateTsTimeRange(list []mformat.TsInfo) (timeRangeList []tsTimeRangeUnit, ok bool) {
	var beginTime float64
	for _, ts := range list {
		//有未解析出持续时长的ts片段
		if ts.TimeSec < 1e-5 {
			return nil, false
		}
		endTime := beginTime + ts.TimeSec
		timeRangeList = append(timeRangeList, tsTimeRangeUnit{
			begin: beginTime,
			end:   endTime,
		})
		beginTime = endTime
	}
	return timeRangeList, true
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
