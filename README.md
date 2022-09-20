
## m3u8视频下载工具
* 没有ffmpeg依赖, 不需要单独配置任何环境
* 提供windows图形界面(Qt), mac、linux命令行, linux支持arm、386、mipsle 
* 程序会自动将下载的ts文件合并转换格式为mp4
* ![](m3u8d-qt/screenshot.png)
* [全部版本下载](https://github.com/orestonce/m3u8d/releases ), 包括windows图形界面/linux命令行/mac命令行/mac图形化界面   
* 命令行使用教程
  * 普通下载命令 `./m3u8d download -u https://example.com/index.m3u8`
  * curl模式： `./m3u8d curl 'https://example.com/index.m3u8' -H 'cookie: CONSENT=YES'`
## 实现说明
* download.go 大部分抄自 [llychao/m3u8-downloader](https://github.com/llychao/m3u8-downloader)
* 使用[gomedia](https://github.com/yapingcat/gomedia) 代替ffmpeg进行格式转换
* 支持跳过前面几个ts文件(一般是广告, 嘿嘿)
* 程序会在下载保存目录创建:
    * downloading/ 目录, 用于存放正在下载的分段ts视频, 按照m3u8的url进行划分
    * m3u8d_cache.cdb 文件, 用于存放以前的下载历史, 用于防止重复下载文件
* 重复下载文件的判定和跳过    
    * 将M3u8Url+SkipTsCountFromHead进行hash, 得到文件下载id
    * 将文件下载id/文件大小/文件内容hash 储存在 m3u8_cache.cdb里面, 下载前搜索下载目录
    如果发现某个文件大小/文件内容hash和以前的记录相等,则认为这个文件是以前下载的文件, 跳过此次下载.
* 已有功能列表
  * 如果不是m3u8样子的URL，自动下载html下来、搜索其中的m3u8链接进行下载
  * windows、linux、mac都支持转换、合并ts格式为mp4
  * 充分测试后，使用 [gomedia](https://github.com/yapingcat/gomedia) 代替ffmpeg进行格式转换
  * 支持嵌套m3u8的url
  * 增加openwrt路由器的mipsle二进制
  * 支持从curl命令解析出需要的信息，正如 [cxjava/m3u8-downloader](https://github.com/cxjava/m3u8-downloader) 一样
  * 显示下载速度、合并ts的速度
  * 提供macos的图形化界面
  * 支持下载aes加密的m3u8
  * 内部使用多线程下载ts文件
  * 支持设置代理: http/socks5
    * http代理解释: 要访问的真实url是http协议, 使用代理服务器可见的GET/POST/HEAD...形式; 如果要访问的真实url是https协议, 使用代理服务器不可见的CONNECT形式 
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
