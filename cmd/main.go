package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/orestonce/m3u8d"
	"github.com/orestonce/m3u8d/m3u8dcpp"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var rootCmd = &cobra.Command{
	Use: "m3u8d",
}

var downloadCmd = &cobra.Command{
	Use: "download",
	Run: func(cmd *cobra.Command, args []string) {
		if gRunReq.M3u8Url == "" {
			cmd.Help()
			return
		}
		downloadFromCmd(gRunReq)
	},
}

func downloadFromCmd(req m3u8d.StartDownload_Req) {
	req.ProgressBarShow = true
	errMsg := m3u8dcpp.StartDownload(req)
	if errMsg != "" {
		fmt.Println(errMsg)
		return
	}
	resp := m3u8dcpp.WaitDownloadFinish()
	fmt.Println() // 有进度条,所以需要换行
	if resp.ErrMsg != "" {
		fmt.Println(resp.ErrMsg)
		return
	}
	if resp.IsSkipped {
		fmt.Println("已经下载过了: " + resp.SaveFileTo)
		return
	}
	if resp.SaveFileTo == "" {
		fmt.Println("下载成功.")
		return
	}
	fmt.Println("下载成功, 保存路径", resp.SaveFileTo)
}

var curlCmd = &cobra.Command{
	Use: "curl",
	Run: func(cmd *cobra.Command, args []string) {
		resp1 := m3u8d.ParseCurl(args)
		if resp1.ErrMsg != "" {
			fmt.Println(resp1.ErrMsg)
			return
		}
		downloadFromCmd(resp1.DownloadReq)
	},
}

var gRunReq m3u8d.StartDownload_Req

var gMergeReq struct {
	InputTsDir           string
	OutputMp4Name        string
	UseFirstTsMTime      bool
	SkipBadResolutionFps bool
}

var mergeCmd = &cobra.Command{
	Use: "merge",
	Run: func(cmd *cobra.Command, args []string) {
		if gMergeReq.InputTsDir == "" {
			var err error
			gMergeReq.InputTsDir, err = os.Getwd()
			if err != nil {
				log.Fatalln("获取当前目录失败")
				return
			}
		}
		fList, err := ioutil.ReadDir(gMergeReq.InputTsDir)
		if err != nil {
			log.Fatalln("读取目录失败", err)
			return
		}
		var tsFileList []string
		for _, f := range fList {
			if f.Mode().IsRegular() && strings.HasSuffix(strings.ToLower(f.Name()), ".ts") {
				tsFileList = append(tsFileList, filepath.Join(gMergeReq.InputTsDir, f.Name()))
			}
		}
		sort.Strings(tsFileList) // 按照字典顺序排序
		if len(tsFileList) == 0 {
			log.Fatalln("目录下不存在ts文件", gMergeReq.InputTsDir)
			return
		}

		status := &m3u8d.SpeedStatus{
			IsRunning:       true,
			ProgressBarShow: true,
		}

		if len(tsFileList) == 0 {
			log.Fatalln("目录下不存在ts文件", gMergeReq.InputTsDir)
			return
		}
		if gMergeReq.OutputMp4Name == "" {
			gMergeReq.OutputMp4Name = filepath.Join(gMergeReq.InputTsDir, "all.mp4")
		}
		if gMergeReq.SkipBadResolutionFps {
			tsFileList, err = m3u8d.AnalyzeTs(status, tsFileList, gMergeReq.OutputMp4Name, context.Background())
			if err != nil {
				log.Fatalln("分析ts失败", err)
				return
			}
		}

		if len(tsFileList) == 0 {
			log.Fatalln("目录下不存在ts文件2", gMergeReq.InputTsDir)
			return
		}

		status.SetProgressBarTitle("合并ts")
		status.SpeedResetTotalBlockCount(len(tsFileList))
		err = m3u8d.MergeTsFileListToSingleMp4(m3u8d.MergeTsFileListToSingleMp4_Req{
			TsFileList: tsFileList,
			OutputMp4:  gMergeReq.OutputMp4Name,
			Ctx:        context.Background(),
			Status:     status,
		})
		if err != nil {
			log.Fatalln("合并失败", err)
			return
		}
		if gMergeReq.UseFirstTsMTime {
			err = m3u8d.UpdateMp4Time(tsFileList[0], gMergeReq.OutputMp4Name)
			if err != nil {
				log.Fatalln("更新mp4文件时间失败", err)
				return
			}
		}
		log.Println("合并成功", gMergeReq.OutputMp4Name)
	},
}

var getTsVideoInfoCmd = &cobra.Command{
	Use: "getTsVideoInfo",
	Run: func(cmd *cobra.Command, args []string) {
		for _, tp := range args {
			data, _ := json.Marshal(m3u8d.GetTsVideoInfo(tp))
			fmt.Println(tp, string(data))
		}
	},
}

func init() {
	downloadCmd.Flags().StringVarP(&gRunReq.M3u8Url, "M3u8Url", "u", "", "M3u8Url")
	downloadCmd.Flags().BoolVarP(&gRunReq.Insecure, "Insecure", "", false, "是否允许不安全的请求")
	downloadCmd.Flags().StringVarP(&gRunReq.SaveDir, "SaveDir", "d", "", "文件保存路径(默认为当前路径)")
	downloadCmd.Flags().StringVarP(&gRunReq.FileName, "FileName", "f", "", "文件名")
	downloadCmd.Flags().StringVarP(&gRunReq.SkipTsExpr, "SkipTsExpr", "", "", "跳过ts信息，ts编号从1开始，例如: 1,92-100 表示跳过第1号ts、跳过92到100号ts")
	downloadCmd.Flags().StringVarP(&gRunReq.SetProxy, "SetProxy", "", "", "代理设置, http://127.0.0.1:8080 socks5://127.0.0.1:1089")
	downloadCmd.Flags().BoolVarP(&gRunReq.SkipRemoveTs, "SkipRemoveTs", "", false, "不删除下载的ts文件")
	downloadCmd.Flags().IntVarP(&gRunReq.ThreadCount, "ThreadCount", "", 8, "下载线程数")
	downloadCmd.Flags().BoolVarP(&gRunReq.SkipMergeTs, "SkipMergeTs", "", false, "不合并ts为mp4")
	downloadCmd.Flags().BoolVarP(&gRunReq.DebugLog, "DebugLog", "", false, "调试日志")
	downloadCmd.Flags().StringVarP(&gRunReq.TsTempDir, "TsTempDir", "", "", "临时ts文件目录")
	downloadCmd.Flags().BoolVarP(&gRunReq.UseServerSideTime, "UseServerSideTime", "", false, "使用服务端提供的文件时间")
	rootCmd.AddCommand(downloadCmd)
	curlCmd.DisableFlagParsing = true
	rootCmd.AddCommand(curlCmd)
	mergeCmd.Flags().StringVarP(&gMergeReq.InputTsDir, "InputTsDir", "", "", "存放ts文件的目录(默认为当前工作目录)")
	mergeCmd.Flags().StringVarP(&gMergeReq.OutputMp4Name, "OutputMp4Name", "", "", "输出mp4文件名(默认为输入ts文件的目录下的all.mp4)")
	mergeCmd.Flags().BoolVarP(&gMergeReq.UseFirstTsMTime, "UseFirstTsMTime", "", false, "使用第一个ts文件的修改时间作为输出mp4文件的创建时间")
	mergeCmd.Flags().BoolVarP(&gMergeReq.SkipBadResolutionFps, "SkipBadResolutionFps", "", true, "跳过分辨率、fps异常的ts文件")
	rootCmd.AddCommand(mergeCmd)
	rootCmd.AddCommand(getTsVideoInfoCmd)
	rootCmd.Version = m3u8d.GetVersion()
}

func main() {
	rootCmd.Execute()
}
