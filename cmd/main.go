package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"m3u8d"
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
	m3u8d.SetShowProgressBar()
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

func init() {
	downloadCmd.Flags().StringVarP(&gRunReq.M3u8Url, "M3u8Url", "u", "", "M3u8Url")
	downloadCmd.Flags().BoolVarP(&gRunReq.Insecure, "Insecure", "", false, "是否允许不安全的请求")
	downloadCmd.Flags().StringVarP(&gRunReq.SaveDir, "SaveDir", "d", "", "文件保存路径(默认为当前路径)")
	downloadCmd.Flags().StringVarP(&gRunReq.FileName, "FileName", "f", "", "文件名")
	downloadCmd.Flags().IntVarP(&gRunReq.SkipTsCountFromHead, "SkipTsCountFromHead", "", 0, "跳过前面几个ts")
	downloadCmd.Flags().StringVarP(&gRunReq.SetProxy, "SetProxy", "", "", "代理设置, http://127.0.0.1:8080 socks5://127.0.0.1:1089")
	rootCmd.AddCommand(downloadCmd)
	curlCmd.DisableFlagParsing = true
	rootCmd.AddCommand(curlCmd)
}

func main() {
	rootCmd.Execute()
}
