## m3u8d 一款m3u8下载工具
* 提供windows图形界面(Qt), mac\linux命令行, linux支持arm和386 
* 使用gomedia转换、合并ts格式为mp4
* windows自带GUI界面的版本下载: [m3u8d_qt_v1.4_windows_amd64.exe](https://github.com/orestonce/m3u8d/releases/download/v1.4/m3u8d_qt_v1.4_windows_amd64.exe):
    ![](m3u8d-qt/screenshot.png)
* 全部版本下载, 包括windows图形界面/linux命令行/mac命令行: https://github.com/orestonce/m3u8d/releases    

## 实现说明
* download.go 大部分抄自 https://github.com/llychao/m3u8-downloader
* 使用 https://github.com/yapingcat/gomedia 代替ffmpeg进行格式转换
* 支持跳过前面几个ts文件(一般是广告, 嘿嘿)
* 程序会在下载保存目录创建:
    * downloading/ 目录, 用于存放正在下载的分段ts视频, 按照m3u8的url进行划分
    * m3u8d_cache.cdb 文件, 用于存放以前的下载历史, 用于防止重复下载文件
* 重复下载文件的判定和跳过    
    * 将M3u8Url+SkipTsCountFromHead进行hash, 得到文件下载id
    * 将文件下载id/文件大小/文件内容hash 储存在 m3u8_cache.cdb里面, 下载前搜索下载目录
    如果发现某个文件大小/文件内容hash和以前的记录相相等,则认为这个文件是以前下载的文件, 跳过
    此次下载.
## TODO:
  * [x] 如果不是m3u8样子的URL，自动下载html下来、搜索其中的m3u8链接进行下载
  * [x] windows、linux、mac都支持转换、合并ts格式为mp4
  * [x] 充分测试后，使用 https://github.com/yapingcat/gomedia 代替ffmpeg进行格式转换
  * [x] 支持嵌套m3u8的url
  * [x] 支持设置代理
  * [ ] 支持从curl命令解析出需要的header、auth-basic、cookie等信息，正如 https://github.com/cxjava/m3u8-downloader 一样
## 二次开发操作手册:
* 如果只开发命令行版本, 则只需要修改*.go文件, 然后编译 cmd/main.go 即可
* 如果涉及到Qt界面打包, 则需要运行 export/main.go 将 *.go导出为Qt界面需要的
`m3u8-qt/m3u8.h`,`m3u8-qt/m3u8.cpp`, `m3u8-qt/m3u8-impl.a`. 然后使用QtCreator进行打包
## 发布协议
* m3u8-qt/ 目录采用 [GPL协议 v3](m3u8d-qt/LICENSE) 发布
* 除 m3u8-qt/ 以外的代码, 采用[MIT协议](LICENSE)发布 
