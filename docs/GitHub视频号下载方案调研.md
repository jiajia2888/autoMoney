# GitHub 视频号下载方案调研

调研时间：2026-11-10（Asia/Shanghai）。范围：GitHub 公开仓库 README、当前源码、LICENSE 和 GitHub API 元数据。以下“支持”表示作者文档及源码具备该路径，不等于本机真实下载验收。没有执行克隆仓库程序、安装根证书、改系统代理或读取其他项目登录态。

## 结论

确实有多种视频号下载实现，但它们解决的关键问题是**取得可用媒体地址，必要时同时取得 decodeKey**。我们当前易起量投手接口只取得作品信息和分享链接；35 条目标的视频号预览响应没有媒体。下载器、批量队列和 ISAAC64 算法不能单独补出缺失的媒体来源。

当前 Mac 优先研究 **QinGeneral/wx-downloader 的元宝解析路线**：独立登录腾讯元宝后，把现有分享链接交给元宝解析得到新的 preview token/eid，再取 h264VideoInfo.videoUrl。它无需修改系统代理、安装根证书或操作微信客户端，最容易与现有 Python 适配器、Go 任务状态和本机 MP4 附件路由结合。需要新的元宝扫码登录，现有视频号投手会话不是元宝会话。商业接入还受 MIT + Commons Clause 限制。

另一条成熟路线是 **微信 PC 客户端播放 → 本地代理取得详情/媒体/密钥 → 下载解密**。ltaoo 和 res-downloader 明确支持 macOS；但普通用法需要系统信任的抓包证书和代理，必须单独评估网络影响，不能在用户睡觉时自动安装。nobiyou 可借鉴队列与纯 Go 解密实现，其当前产品说明主要面向 Windows。

## 候选比较

| 项目 | 媒体来源及输入 | macOS 与运行依赖 | 批量及接入方式 | 许可与本轮判断 |
| --- | --- | --- | --- | --- |
| [QinGeneral/wx-downloader](https://github.com/QinGeneral/wx-downloader) | 元宝登录 + weixin.qq.com/sph 分享链接；解析新预览 token/eid，再取普通 MP4 | Python≥3.10、httpx、Playwright/Chrome；独立浏览器登录；无需微信 PC/代理证书 | 多链接/JSON、--no-login、--json；stdout JSON、失败继续；最适合独立子进程适配 | MIT + Commons Clause；推荐优先验证一条，尚未真实成功 |
| [ltaoo/wx_channels_download](https://github.com/ltaoo/wx_channels_download) | ① 微信客户端页面代理/注入，返回媒体及 decodeKey；② 元宝 Cookie + 分享链接；无元宝 Cookie 时可回退客户端 | README 支持 macOS/Windows；主程序首次安装证书及代理；TUN 也改变路由；元宝解析代码本身不依赖证书 | 已有 HTTP/MCP、作者列表、详情、下载资源、解密及任务；不宜整包嵌入我们后端 | MIT + Commons Clause；可作为独立配套工具/协议参考，须先解决登录或客户端连通 |
| [putyy/res-downloader](https://github.com/putyy/res-downloader) | 本地代理抓包、筛媒体；视频号详情提取 media.url+urlToken 和 decodeKey | Go+Wails；README 支持 Windows/macOS/Linux；证书及系统代理8899 | 适合捕获已打开内容后下载；不能仅靠我们现有 exportId 自动补齐35条 | LICENSE Apache-2.0，但 README 又写禁止商业用途；商业复用需作者澄清矛盾 |
| [nobiyou/wx_channel](https://github.com/nobiyou/wx_channel) | 微信页面注入、SunnyNet 捕获媒体/密钥；Web控制台队列 | 当前 README 平台标注 Windows、入口 wx_channel.exe、首次安装 SunnyRoot；源码部分模块有 darwin 路径，不能据此认定全产品支持 Mac | POST /api/queue 接收 videos[]，有 videoId/videoUrl/decryptKey 等；Go 流式解密可供算法研究 | MIT；队列和解密可借鉴，当前不作为本机首选运行工具 |
| [jiamuAi/jiamu-wechat-channels-downloader-skill](https://github.com/jiamuAi/jiamu-wechat-channels-downloader-skill) | TikHub 搜索/详情API，以 export_id 换 media.url/urlToken/decodeKey；本地解密服务 | Node + 本地Playwright/WASM解密；转写可接DashScope，但我们已有本地Whisper | 单条脚本可传 --no-transcribe，只下载；需付费供应商凭据，不能假设与现有投手 exportId 通用 | 主仓库MIT，解密外部依赖许可须另核对；作为有预算/API条件后的备选 |

## 维护状态快照

GitHub API 的 pushed_at 只说明仓库推送时间，不代表该视频号路径在该日成功下载；release 与源码 HEAD 也可能不同。

| 项目 | 审阅 commit | pushed_at（UTC） | latest release API |
| --- | --- | --- | --- |
| putyy/res-downloader | f8ac4d9c85baf28322106e40ea3fdea9867e2f74 | 2026-11-05 12:46:40 | 3.1.3，2025-12-30 |
| ltaoo/wx_channels_download | c7d15d7e055a62090d6a98694cc986812f8d86f4 | 2026-11-09 13:38:56 | v261106，2026-11-06 |
| nobiyou/wx_channel | 182ef7bb40c607a1a8ebd13149747ad996dd6130 | 2026-11-05 15:32:31 | v5.7.14，2026-11-05 |
| QinGeneral/wx-downloader | 95bfc87a3c8c125998499b06b086df02ebb57b43 | 2026-11-04 15:57:02 | releases/latest 返回404；README/PyPI版号0.1.2 |
| jiamuAi/jiamu-wechat-channels-downloader-skill | eeb4f6b4f63efc30bf0f8d6c0bd365f0c5c6cbf7 | 2026-08-04 12:47:36 | releases/latest 返回404 |

元数据一手来源为各仓库 `https://api.github.com/repos/<owner>/<repo>` 与 `/releases/latest`。只读浅克隆保存在忽略目录 `.local/research/`，不纳入应用运行依赖。

## 优先方案：元宝解析

审阅 [QinGeneral client.py](https://github.com/QinGeneral/wx-downloader/blob/95bfc87a3c8c125998499b06b086df02ebb57b43/src/wx_channels_cli/client.py) 与 [auth.py](https://github.com/QinGeneral/wx-downloader/blob/95bfc87a3c8c125998499b06b086df02ebb57b43/src/wx_channels_cli/auth.py)：

1. 使用独立 Playwright context 打开腾讯元宝，用户扫码/手机号验证后等 `hy_user` 与 `hy_token`；不读取日常浏览器 profile。原工具保存独立 auth.json（0600），不应读取或复制其他项目的现成凭据。
2. `POST https://yuanbao.tencent.com/api/weixin/get_parse_result`，请求体 `{"type":"video_channel_url","url":"https://weixin.qq.com/sph/...","scene":1}`。元宝 Cookie 仅发送给该元宝端点，附浏览器UA/Origin/Referer/x-source及必要头。
3. 只有业务code=0且data.playable_url为官方channels.weixin.qq.com、包含token和eid，才进入下阶段。
4. `POST https://channels.weixin.qq.com/finder-preview/api/feed/get_feed_info?...`，请求体 `{"baseReq":{"generalToken":"<token>"},"exportId":"<eid>"}`，Referer使用上一步的新预览链接；不携带元宝Cookie。
5. 检查errCode和嵌套data.errMsg，要求feedInfo.mediaType=4，然后优先h264VideoInfo.videoUrl、通用videoUrl、再h265VideoInfo.videoUrl；不把HTTP200或errCode0本身当成功。
6. 下载不带元宝Cookie。原工具只校验长度与MP4头，不做ffprobe；接到我们项目必须继续做现有的ffprobe/尺寸时间/原子落盘/媒体域名及重定向限制。原工具的任意http/https媒体地址和自动跟随重定向不能直接暴露成我们本地HTTP任意URL入口。

[download.py](https://github.com/QinGeneral/wx-downloader/blob/95bfc87a3c8c125998499b06b086df02ebb57b43/src/wx_channels_cli/download.py) 有.part清理、不覆盖、记录去重与有限重试；但没有解密步骤。该路线假设预览接口直接提供可用MP4，是否对我们的35条有效，需要真实元宝登录后检验。

上游 [README](https://github.com/QinGeneral/wx-downloader/blob/95bfc87a3c8c125998499b06b086df02ebb57b43/README.md) 声称曾下载一条8,155,895字节/73.45秒H264+AAC；这是作者测试，不能当本机验收。[tests/test_cli.py](https://github.com/QinGeneral/wx-downloader/blob/95bfc87a3c8c125998499b06b086df02ebb57b43/tests/test_cli.py) 主要通过httpx.MockTransport验证请求契约、批量继续、Cookie隔离和错误处理。

**本机实际探测**：主任务从已有35条清单选1条真实分享链接，不带Cookie请求元宝解析端点，返回HTTP401 Unauthorized，未取得playable_url、未下载文件。这证明该端点不能按本次探测匿名使用；不能直接宣布35条已可下载。用户醒来后需独立元宝扫码，再先验收1条。

## 客户端方案的实际边界

ltaoo [README](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/README.md) 的普通运行步骤是管理员启动、自动安装证书、打开微信PC视频页面、注入下载按钮。[证书文档](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/docs/guide/certificate.md) 明确用于拦截并修改响应；[代理文档](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/docs/config/proxy.md) 的system=false只是不自动设置系统代理，不代表无需流量转发；TUN会创建虚拟网卡并改路由。

[ltaoo v260823说明](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/docs/releases/260823.md) 写“未配置元宝Cookie也支持解析分享链接”；[client.go](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/pkg/scraper/wxchannels/client.go) 的实际逻辑是元宝凭据缺失时回退 `FetchChannelsFeedProfile`，仍需可调用的客户端WebSocket。不能解读成匿名HTTP下载API。

其 [routes.go](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/internal/adapter/wxchannels/routes.go) 有 `/api/channels/parse_sph`、`/api/channels/feed/profile`、`/api/channels/decrypt` 等；[MCP tools.go](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/internal/mcpserver/tools.go) 返回 `download_url`、`decode_key` 和 `requires_decryption`。可做独立本机伴随服务适配，但不能把其管理API原样暴露给我们网页。原 [yuanbao.go](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/pkg/scraper/wxchannels/yuanbao.go) 有打印generalToken的日志，复用协议时必须去掉这类敏感日志。

res-downloader 的 [README](https://github.com/putyy/res-downloader/blob/f8ac4d9c85baf28322106e40ea3fdea9867e2f74/README.md) 明确依赖代理抓包。其 [plugin.qq.com.go](https://github.com/putyy/res-downloader/blob/f8ac4d9c85baf28322106e40ea3fdea9867e2f74/core/plugins/plugin.qq.com.go) 从捕获详情提取 `firstMedia.url`、`urlToken`、`decodeKey`。有源码不代表它能从我们只有exportId的数据库主动解析所有作品。

nobiyou 的 [队列API](https://github.com/nobiyou/wx_channel/blob/182ef7bb40c607a1a8ebd13149747ad996dd6130/internal/handlers/console_api.go) `POST /api/queue` 要求 `videos[]`，其 [VideoInfo](https://github.com/nobiyou/wx_channel/blob/182ef7bb40c607a1a8ebd13149747ad996dd6130/internal/services/queue_service.go) 包含 videoId/title/author/videoUrl/decryptKey 等。这个API接收**已经解析好的媒体**，不能拿sph分享链接填videoUrl期待自动解析。其 [MIT LICENSE](https://github.com/nobiyou/wx_channel/blob/182ef7bb40c607a1a8ebd13149747ad996dd6130/LICENSE) 与 [README](https://github.com/nobiyou/wx_channel/blob/182ef7bb40c607a1a8ebd13149747ad996dd6130/README.md) 均已核对。

## 解密与付费接口备选

视频号部分媒体使用ISAAC64密钥流对前131072字节做XOR；这不是所有预览MP4都必需的步骤。nobiyou [decrypt_reader.go](https://github.com/nobiyou/wx_channel/blob/182ef7bb40c607a1a8ebd13149747ad996dd6130/internal/utils/decrypt_reader.go) 是纯Go流式实现，支持偏移，依赖同仓库util ISAAC64。ltaoo [decrypt.go](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/pkg/scraper/wxchannels/decrypt.go) 注明来自Hanson/WechatSphDecrypt；复用时需沿源码追溯原许可。

可独立验证的内容包括：固定种子向量、同文件分块读取一致性、跨131072边界、Range偏移、XOR往返。但自造明文加密再解密只能证明算法自洽，不能证明当前微信加密流兼容，更不能代替真实播放验收。decodeKey必须字符串传输避免JavaScript精度丢失，不能由exportId推算。

jiamu [download-video.mjs](https://github.com/jiamuAi/jiamu-wechat-channels-downloader-skill/blob/eeb4f6b4f63efc30bf0f8d6c0bd365f0c5c6cbf7/scripts/download-video.mjs) 调 `POST /api/v1/wechat_channels/v2/fetch_video_detail`，发送 `{export_id}` 到TikHub，再取 `objects[0].objectDesc.media[0]` 中url/urlToken/decodeKey；本地解密后可转写。它实际依赖外部付费数据供应商和额外解密服务，README所谓“无风控风险”不作为我们的结论。主项目已有Whisper，可只参考下载/解密契约，避免云ASR上传。未注册账户、未调用付费接口、未发送凭据，无法确认现有投手exportId可直接用于该API。

## 与 autoMoney 的结合建议

1. 保留现有视频号投手登录、作者搜索、过万筛选和双监控。新增媒体解析适配器独立于监控采集器，输入本机已收录视频ID，从数据库取得官方分享链接，禁止任意URL下载。
2. 首选独立元宝登录流程；会话只用于腾讯元宝解析，前端不回显。取得新媒体后，复用受限下载、ffprobe、任务状态及附件保存。元宝解析失败需显示真实错误，不能显示已连接/下载成功。
3. 同一目标必须匹配作者/标题/媒体类型及关联标识；签名媒体地址短期使用，不输出到普通日志，不以旧缓存地址无限重试。
4. 先真实验收一条MP4：大小>0、ffprobe视频流和时长、完整解码、电脑播放。成功后再对现有35条目标做限速串行队列、去重、失败明细、取消/重试及磁盘限制，按成功/失败数量交付。
5. 如果元宝新token也返回“此内容暂时无法播放”，再考虑用户明确同意后的微信客户端捕获，或有预算且可正常授权的供应商API。不会把当前未验证的媒体接口预先上成成功按钮。

## 许可来源

- [QinGeneral LICENSE](https://github.com/QinGeneral/wx-downloader/blob/95bfc87a3c8c125998499b06b086df02ebb57b43/LICENSE)：MIT + Commons Clause，明确不授予Sell；该定义含主要价值来自软件功能的收费产品/服务。
- [ltaoo LICENSE](https://github.com/ltaoo/wx_channels_download/blob/c7d15d7e055a62090d6a98694cc986812f8d86f4/LICENSE)：同类附加销售限制，不应标为无条件MIT。
- [res-downloader LICENSE](https://github.com/putyy/res-downloader/blob/f8ac4d9c85baf28322106e40ea3fdea9867e2f74/LICENSE)：Apache-2.0；与README非商业文字矛盾，需澄清。
- [nobiyou LICENSE](https://github.com/nobiyou/wx_channel/blob/182ef7bb40c607a1a8ebd13149747ad996dd6130/LICENSE)：MIT。
- [jiamu LICENSE](https://github.com/jiamuAi/jiamu-wechat-channels-downloader-skill/blob/eeb4f6b4f63efc30bf0f8d6c0bd365f0c5c6cbf7/LICENSE)：MIT；不自动覆盖其外部解密服务及付费API。

本轮仅做公开源码及请求契约研究；没有把上游自测、静态检查、README宣称或仓库星数当作我们真实下载成功。用户当前无需立即操作；独立元宝登录与首条文件验收是下一步真实门槛。
