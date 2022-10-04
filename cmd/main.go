package main

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"m3u8d"
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

func downloadFromCmd(req m3u8d.RunDownload_Req) {
	req.ProgressBarShow = true
	resp := m3u8d.RunDownload(req)
	fmt.Println() // 有进度条,所以需要换行
	if resp.ErrMsg != "" {
		fmt.Println(resp.ErrMsg)
		return
	}
	if resp.IsSkipped {
		fmt.Println("已经下载过了: " + resp.SaveFileTo)
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

var gRunReq m3u8d.RunDownload_Req

var gMergeReq struct {
	InputTsDir    string
	OutputMp4Name string
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
		if len(tsFileList) == 0 {
			log.Fatalln("目录下不存在ts文件", gMergeReq.InputTsDir)
			return
		}
		sort.Strings(tsFileList) // 按照字典顺序排序
		if gMergeReq.OutputMp4Name == "" {
			gMergeReq.OutputMp4Name = filepath.Join(gMergeReq.InputTsDir, "all.mp4")
		}
		err = m3u8d.MergeTsFileListToSingleMp4(m3u8d.MergeTsFileListToSingleMp4_Req{
			TsFileList: tsFileList,
			OutputMp4:  gMergeReq.OutputMp4Name,
			Ctx:        context.Background(),
		})
		if err != nil {
			log.Fatalln("合并失败", err)
			return
		}
		log.Println("合并成功", gMergeReq.OutputMp4Name)
	},
}

func init() {
	downloadCmd.Flags().StringVarP(&gRunReq.M3u8Url, "M3u8Url", "u", "", "M3u8Url")
	downloadCmd.Flags().BoolVarP(&gRunReq.Insecure, "Insecure", "", false, "是否允许不安全的请求")
	downloadCmd.Flags().StringVarP(&gRunReq.SaveDir, "SaveDir", "d", "", "文件保存路径(默认为当前路径)")
	downloadCmd.Flags().StringVarP(&gRunReq.FileName, "FileName", "f", "", "文件名")
	downloadCmd.Flags().BoolVarP(&gRunReq.RemoteName, "RemoteFileName", "F", false, "尝试自动获取文件名")
	downloadCmd.Flags().IntVarP(&gRunReq.SkipTsCountFromHead, "SkipTsCountFromHead", "", 0, "跳过前面几个ts")
	downloadCmd.Flags().StringVarP(&gRunReq.SetProxy, "SetProxy", "", "", "代理设置, http://127.0.0.1:8080 socks5://127.0.0.1:1089")
	downloadCmd.Flags().BoolVarP(&gRunReq.SkipRemoveTs, "SkipRemoveTs", "", false, "不删除下载的ts文件")
	rootCmd.AddCommand(downloadCmd)
	curlCmd.DisableFlagParsing = true
	rootCmd.AddCommand(curlCmd)
	mergeCmd.Flags().StringVarP(&gMergeReq.InputTsDir, "InputTsDir", "", "", "存放ts文件的目录(默认为当前工作目录)")
	mergeCmd.Flags().StringVarP(&gMergeReq.OutputMp4Name, "OutputMp4Name", "", "", "输出mp4文件名(默认为输入ts文件的目录下的all.mp4)")
	rootCmd.AddCommand(mergeCmd)
}

func main() {
	rootCmd.Execute()
}
