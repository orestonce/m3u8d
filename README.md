
## m3u8视频下载工具
* 没有ffmpeg依赖, 不需要单独配置任何环境
* 提供windows图形界面(Qt), mac、linux命令行, linux支持arm、386、mipsle 
* 程序会自动将下载的ts文件合并转换格式为mp4
* ![](m3u8d-qt/screenshot.png)
* [全部版本下载](https://github.com/orestonce/m3u8d/releases ), 包括windows图形界面/linux命令行/mac命令行/mac图形化界面   
* 命令行使用教程
  * 普通下载命令: `./m3u8d download -u https://example.com/index.m3u8`
  * curl模式： `./m3u8d curl 'https://example.com/index.m3u8' -H 'cookie: CONSENT=YES'`
  * 合并某个目录下的ts文件为 mp4: `./m3u8d merge --InputTsDir /root/save --OutputMp4Name save.mp4` 
## 实现说明
* download.go 大部分抄自 [llychao/m3u8-downloader](https://github.com/llychao/m3u8-downloader)
* 使用[gomedia](https://github.com/yapingcat/gomedia) 代替ffmpeg进行格式转换
* 支持跳过ts文件
* 程序会在下载保存目录创建:
    * downloading/ 目录, 用于存放正在下载的分段ts视频, 按照m3u8的url进行划分
    * m3u8d_config.json 文件, 用于存放Qt ui的的界面上的配置信息, 只有Windows/Macos的Qt版本会创建此文件
* **curl模式** 可以赋予使用者任意设置下载请求的Header信息的能力，方便解决只有一个m3u8的链接时无法下载视频的尴尬局面
  * 例子1, 你需要下载的视频是要登陆后观看的，Cookie信息里存放了登陆状态
  * 例子2, 网站开发者验证了Referer信息、Authority信息、Origin信息、User-Agent信息、各种特定的Header信息
  * 以windows下的chrome为例，找到对应的m3u8请求记录，然后右键选择 "Copy - Copy as cURL(bash)", 
    然后打开 windows-qt版本的 m3u8d, 点击 "curl 模式"，将复制出来的请求粘贴上去即可
* 已有功能列表
  * 如果不是m3u8样子的URL，自动下载html下来、搜索其中的m3u8链接进行下载
  * 支持下载aes加密的m3u8, 支持单个m3u8文件内不同ts文件使用不同的加密策略
  * 内部使用多线程下载ts文件
  * windows、linux、mac都支持转换、合并ts格式为mp4
  * 充分测试后，使用 [gomedia](https://github.com/yapingcat/gomedia) 代替ffmpeg进行格式转换
  * 增加openwrt路由器的mipsle二进制
  * 支持从curl命令解析出需要的信息，正如 [cxjava/m3u8-downloader](https://github.com/cxjava/m3u8-downloader) 一样
  * 显示下载速度、合并ts的速度
  * 提供macos的图形化界面
  * 支持嵌套m3u8的url
  * ts文件合并优化
    * ts文件列表中的媒体文件可能分辨率、fps不一致，例如第一个文件分辨率为1920x1080, 第二个文件为800x600，直接合并第一第二个文件则会造成合并的mp4无法播放
    * 目前的处理方案是，分析需要合并的ts文件中的第一个文件的分辨率、fps，若后续的ts文件的分辨率、fps与第一个不同则不合并后续的ts文件
  * 支持设置代理: http/socks5
    * http代理解释: 要访问的真实url是http协议, 使用代理服务器可见的GET/POST/HEAD...形式; 如果要访问的真实url是https协议, 使用代理服务器不可见的CONNECT形式
  * 跳过ts的表达式使用英文逗号','隔开, 编写规则:
    * ts列表文件名从1开始编号，例如第一个ts文件的编号就是1，第十个ts的编号就是10
    * 想要跳过编号为10的ts: 10
    * 想要跳过编号为23到199的ts: 23-199
    * 想要跳过下载ts时，服务器返回http状态码为403,404的ts: http.code=403, http.code=404
    * 使用服务器的http状态码跳过ts可能造成判断错误，所以默认情况不会合并下载的ts、不会删除下载的ts。 
      * 如果要让http状态码跳过的ts也能被自动合并: if-http.code-merge_ts
    * 如果需要记录被跳过http下载、合并的日志, 则需要标明 with-skip_log
    
## TODO:
  * [ ] 多线程修改为自适应模式，在下载过程中动态调整线程池大小，以便达到最快的下载速度
  * [ ] 支持多国语言
  * [ ] 支持从一个txt里读取下载列表，批量下载
## 二次开发操作手册:
* 如果只开发命令行版本, 则只需要修改*.go文件, 然后编译 cmd/main.go 即可
* 如果涉及到Qt界面打包, 则需要运行 export/main.go 将 *.go导出为Qt界面需要的
`m3u8-qt/m3u8.h`,`m3u8-qt/m3u8.cpp`, `m3u8-qt/m3u8-impl.a`. 然后使用QtCreator进行打包
## 发布协议
* m3u8d-qt/ 目录采用 [GPL协议 v3](m3u8d-qt/LICENSE) 发布
* 除 m3u8d-qt/ 以外的代码, 采用[MIT协议](LICENSE)发布 
## 开发支持
 * 本项目由 jetbrains 开源开发许可证-社区版([Licenses for Open Source Development](https://jb.gg/OpenSourceSupport)) 提供goland开发支持
 * 感谢 [gomedia](https://github.com/yapingcat/gomedia) 开发者提供的ts转mp4逻辑
 * 感谢 [cobra](https://github.com/spf13/cobra) 提供的命令行解析支持
 * 感谢 [setft](https://github.com/xiaoqidun/setft) 开发者提供的更新文件创建时间的逻辑
 
----------------------------------
## 关于为什么使用 gomedia 替代 ffmpeg
### 引入ffmpeg很麻烦, 原因列表:
1. ffmpeg开源协议是 GPL的,具有传染性, 这个项目的主要逻辑就不能使用 MIT 开源了
2. 如果使用cgo调用的形式引入ffmpeg
    * 最终二进制体积特别大
    * 编译mac/linux/路由器 版本的时候必然要依赖对应的跨平台编译器, 编译难度提升
3. 如果使用内嵌 静态编译的ffmpeg二进制, 使用的时候释放到 临时目录再调用命令行
    * 最终二进制体积会更大, 可以看[以前的v1.1版本](https://github.com/orestonce/m3u8d/releases/tag/v1.1) , 每个最终二进制都比现在大25MB左右
    * 没找到mipsle路由器版本的静态编译的ffmpeg
4. 如果直接调用ffmpeg命令, 用户则必须首先安装ffmpeg到操作系统, 难用
### 引入MIT协议的gomedia解决ts转换成mp4好处
1. gomedia是纯go代码, 跨平台编译容易
2. 本项目也可以使用MIT协议进行开源, 无需限定为GPL/LGPL
3. 最终二进制体积特别小, linux/mac 版本的命令行版本才 5-7MB, windows由于有静态编译进来的qt界面, 现在体积有26MB
4. 用户无需预先安装ffmpeg, 降低用户的使用难度
