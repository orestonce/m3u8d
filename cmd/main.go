package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"m3u8d"
)

var rootCmd = cobra.Command{
	Use: "m3u8下载工具",
	Run: func(cmd *cobra.Command, args []string) {
		m3u8d.SetShowProgressBar()
		resp := m3u8d.RunDownload(gRunReq)
		fmt.Println()	// 有进度条,所以需要换行
		if resp.ErrMsg != "" {
			fmt.Println(resp.ErrMsg)
			return
		}
		if resp.IsSkipped {
			fmt.Println("已经下载过了: " + resp.SaveFileTo)
			return
		}
		fmt.Println("下载成功, 保存路径", resp.SaveFileTo)
	},
}

var gRunReq m3u8d.RunDownload_Req

func init() {
	rootCmd.Flags().StringVarP(&gRunReq.M3u8Url, "M3u8Url", "u", "", "M3u8Url")
	rootCmd.Flags().StringVarP(&gRunReq.HostType, "HostType", "", "apiv1", "设置getHost的方式(apiv1: `http(s):// + url.Host + filepath.Dir(url.Path)`; apiv2: `http(s)://+ u.Host`")
	rootCmd.Flags().BoolVarP(&gRunReq.Insecure, "Insecure", "", false, "是否允许不安全的请求")
	rootCmd.Flags().StringVarP(&gRunReq.SaveDir, "SaveDir", "d", "", "文件保存路径(默认为当前路径)")
	rootCmd.Flags().StringVarP(&gRunReq.FileName, "FileName", "f", "", "文件名")
	rootCmd.Flags().IntVarP(&gRunReq.SkipTsCountFromHead, "SkipTsCountFromHead", "", 0, "跳过前面几个ts")
}

func main() {
	rootCmd.Execute()
}
