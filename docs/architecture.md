# 本地多账号监控架构

参考 KnowledgeSearch：Gin + GORM、configs/dao/web/base/util 分层，补充 service 业务层；原生 HTML/JS 前端。Go 模块为 automoney。

## 数据流

HxTechPages → web → service/monitor → SQLite（本地默认）或 MySQL。
service/monitor → service/benchmark/PythonCrawler → scripts/douyin_collector.py → 上游 DouYinClient。

采集器复用 vendor/media-crawler/media_platform/douyin/client.py 的分页接口与签名逻辑。通过包路径加载避免导入整个 core/store 采集流水线；没有改动上游文件。Python 工作目录固定到上游源码根目录，签名 JS 才能被正确找到。

## 数据与调度

- BenchmarkAccount：主页、显示名称、检查间隔、点赞门槛、每次扫描上限、提醒开关、基线和检查状态。
- Video：按 account_id + platform_id 唯一；保存原始整数点赞及互动指标、首次/最近看到的时间。历史页面样本使用 likes_exact=false 和单独来源标记。
- Notification：新视频、首次过门槛及内部基线标记；event_key 唯一约束实现重复检查去重。baseline 不作为提醒展示。
- CrawlRun：任务状态、扫描数量、收录数量、提醒数量、是否完整及停止原因。

服务存活时每 15 秒检查到期账号，按 next_check_at 顺序串行执行；每次检查结束后按该账号分钟间隔安排下次检查。多个账号共享一个专用浏览器 profile，任务全局串行，避免登录态锁冲突。并发手动请求返回 409，可等待当前任务结束后重试。服务重启把遗留 queued/running 任务标记为 interrupted，后续重新检查到期账号。

macOS 可选通过 `make service-install` 安装当前用户 LaunchAgent（com.automoney.local），RunAtLoad / KeepAlive 自动启动和重启；make run 仍可用于前台开发，两者不同时启动。浏览器网页关闭不影响 Go 进程内定时任务；Go 服务或电脑关闭则停止检查。没有对关闭期间的每个时间点补采承诺。

## 提醒口径

首次成功采集建立本次扫描范围的基线，不把历史视频作为新视频推送；高于门槛的历史作品建立内部已知标记，不在下次刷新时集中报爆款。
后续首次发现且发布时间不早于基线时间的视频发送上新提醒；首次记录到高于点赞门槛且未发过该门槛提醒的作品发送达标提醒。同一视频/事件/门槛只提醒一次。修改门槛后可能产生新门槛事件；初次扫描范围以外的老视频，未来首次扫描到时可能产生首次达标提醒。

判定采用原始整数 likes > threshold，10000 不入选而 10001 入选。采集适配器排除已识别图文内容，缺少有效原始点赞数则失败，不当作零。样本导入不触发提醒、不建立实时基线、不自动启用账号。

## 范围与失败处理

每次按抖音主页接口顺序扫描，保留置顶作品并去重；最多 1–500 条视频、最多 35 页、每页间隔 3 秒。达到上限标记 partial，扫描范围外的旧视频点赞不会更新。不会因为一条作品点赞低就停止扫描。
取消或超时不会把不完整响应写入视频和提醒；正常完成的有限范围扫描可写入但标明 complete=false。接口无作品列表、空结果、业务失败、重复游标、缺失点赞等均明确记录错误，不将“0 条”冒充成功。账号/视频/提醒更新在同一事务中完成。

## 登录与本地边界

只监听 127.0.0.1 或 ::1；校验 Host，拒绝跨源写入。本地版没有用户鉴权，不能直接对公网暴露。
Python 通过本机官方 Chrome 使用 .local/douyin-profile，登录任务打开可见窗口，普通检查 headless 静默运行；首次需用户登录，之后持久化。不会读取默认 Chrome 配置或导出其 cookie。登录等待上限 10 分钟；任务总超时 12 分钟，可从检查记录停止任务。

### 手动 Cookie 配置

GET /api/v1/collector/connection 只返回 mode/configured/updated_at；PUT 保存 Cookie + 同一浏览器 User-Agent，或 mode=browser 清除手动配置。两条路由注册在同一受本机 Host/origin/body-size 中间件保护的 Router 上。凭据原子替换到默认 .local/douyin-credentials.json、权限 0600；不回显、不写请求日志、不放入命令行参数或数据库。网页只保留本次填写的输入，关闭后清空，不从后端取回密钥。

Python 收到的是配置文件路径。文件不存在时走原扫码 profile；配置损坏时明确报错，不悄悄回退。有效格式的手动配置使用原 DouYinClient，CookieSigningContext 仅将 Cookie 中的 msToken 提供给原客户端的 localStorage 读取位置，签名仍使用上游 Node.js 实现，作品响应仍来自真实接口；无需启动浏览器。扫码与手动模式共用分页/校验/数量限制。手动凭据不承诺永不过期或必定通过访问校验；实际检查成功才建立基线。未接入第三方数据服务 API Token。

## 配置与迁移

默认值 → AUTOMONEY_CONFIG JSON → AUTOMONEY_ADDR / AUTOMONEY_MYSQL_DSN / AUTOMONEY_PAGES_DIR / AUTOMONEY_SQLITE_PATH / AUTOMONEY_PYTHON / AUTOMONEY_COLLECTOR_SCRIPT / AUTOMONEY_PROFILE_DIR / AUTOMONEY_CREDENTIALS_FILE。
相对路径均按进程工作目录；Makefile 先进入后端目录。默认 SQLite ../.local/monitor.db；配置 MySQL DSN 后使用 MySQL。
make migrate 显式执行 GORM 模型迁移；普通启动只检查所需表存在，不执行迁移。当前为本地首版，没有生产版本化 SQL 回滚方案。

## 后续服务器版本

需先验证服务器浏览器/登录态可持续使用，再补后台守护、鉴权、提醒渠道和迁移管理。当前不包含微信/钉钉推送；视频下载仅用于用户选定作品的本地口播提取。上游非商业学习许可证保留在原目录；商业接入需确认其授权要求。

## 过万发现日期与本地口播提取

Video.first_over_10k_at 只在首次精确采集到 likes > 10000 时写入，此后点赞回落或再次上涨不改原记录；此前缺少精确低位记录时 already_over_10k=true，表示实际过万时刻未知。历史样本无日期，显式迁移不为旧数据补造日期。GET /api/v1/videos 支持 over_10k、sort=over_date、over_date_from / over_date_to，日期按 Asia/Shanghai 转成 UTC 半开区间；过万记录保留当前点赞回落的历史命中，未知日期排最后。

web/transcript → service/transcript → scripts/extract_copy.py → scripts/download_douyin.py → 专家DouYinClient详情及媒体候选 → FFmpeg音轨转换 → 本机whisper.cpp。VideoTranscript 保存任务状态、发布文案、口播正文与 SRT；每台本地服务串行执行一个提取任务。GET/POST /api/v1/videos/:id/transcript 读取或发起，POST .../cancel 停止，GET .../download?format=txt|srt 下载成功结果。仅接受已收录的固定抖音视频 URL，不提供任意 URL 或本地路径 HTTP 入口。脚本的 --input 仅供本机开发验证。

下载凭据复用手动credentials_file，传给专家签名客户端，不把凭据放入命令行或回显上游错误；没有手动配置时，通过 headless 官方 Chrome 读取同一个专用 profile 的 Cookie 与 User-Agent，不访问默认 Chrome profile，不将 Cookie 存入数据库。读取后关闭浏览器，再执行下载；profile 占用时报可重试错误，不自动要求扫码。临时媒体随任务清理，正文保留在 SQLite；单条 150MB / 30 分钟，超过限制明确报错。任务总时限 35 分钟，取消终止子进程组；重启遗留任务标记 interrupted。成功结果复用，不以标题替代未识别的口播。单条真实抖音视频下载现已通过，见2026-11-10接入记录。

## 2026-11-09 故障复验

截图 Failed to fetch 可通过文案 API / healthz 连接失败复现，原因是 9010 服务未运行；新增断连中文提示与重新读取按钮。另有独立下载失败：专用 profile 可读到 session Cookie，但 yt-dlp 的未签名详情接口仍被拒绝；在同一 profile 打开真实视频，页面标题为“验证码中间页”，不能把持有 session 当成平台认证通过。当时真实下载/口播提取未验收，2026-11-10已改专家下载链路。后台无可见认证窗口不代表绕过平台验证。


## 视频号登录与平台入口（2026-11-09）

前端每次加载默认 wx，TAB 切 dy 展示原有工作台。视频号工作区独立展示登录和待接入进度，不将抖音账号、样本或统计显示成视频号数据。切换不更改后台抖音调度开关。已接入视频号作者搜索、作品采集和平台路由调度（见下节）。

web/wechat → service/wechat → channels.weixin.qq.com/promote/api/web。POST /api/v1/wechat/login 生成本地 PNG 二维码和随机任务 ID；POST .../login/poll 在请求体携带 ID，每 2 秒查询一次微信状态，完整原样传回 challenge data。状态 0 等待、5+acctStatus=1 手机待确认、1+acctStatus=1 取得 Cookie 后核验 selfAccountInfo.username、4 或本地 5 分钟超时为过期；其他状态不当成功。只有 Cookie 和有效身份都存在才原子保存会话。只保留单个当前任务，刷新替代旧任务，关闭弹窗 POST .../login/cancel 清理；重启前未完成任务不恢复。

GET /api/v1/wechat/connection 返回是否本地保存、身份白名单和最后核验时间，不代表当前会话一定有效；POST .../connection/verify 实时身份核验，明确失效码 -330/-333/-334 或 HTTP 401/403 清除已保存会话，普通网络/未知业务失败保留并明确报错；DELETE .../connection 删除本地会话并取消待登录任务。全部使用既有本机 Host/origin/body-size 保护，响应 no-store；任务 ID 走请求体，不把扫码 token 或 Cookie 放入日志 URL。二维码是微信 token 构成的官方手机登录 URL 的 PNG，禁止第三方二维码服务。

会话独立存储为 AES-256-GCM，路径 wechat_session_file（默认 ../.local/wechat-session.enc），随机密钥同路径加 .key，两者 0600，临时文件原子替换；Cookie 不进数据库或 GET 响应。不能抵御已取得当前操作系统用户权限的访问者。不会读取抖音或参考项目的会话。HTTP 仅访问固定微信官网、禁止重定向、限制响应大小、检查 HTTP/顶层 error/errCode/嵌套 baseResp 和必要身份字段；接受 201。

配置增加 AUTOMONEY_WECHAT_SESSION_FILE、AUTOMONEY_WECHAT_DNS_SERVER / wechat_dns_server。后端默认读取工作目录 config.json（若存在），AUTOMONEY_CONFIG 优先。专用 DNS 可选，必须 IP:53，启用后微信 HTTP transport 使用指定解析器直连并保留 TLS 证书核验；默认为空，使用系统网络。本机系统 DNS 返回 198.18 段虚拟地址且 TLS 失败，独立 DNS 实测返回官方地址且 HTTPS 可达，故仅本机忽略的 config.json 启用 223.5.5.5:53。未修改系统网络、hosts 或其他项目。


### 微信 Cookie 超长有效期兼容修复（2026-11-09）

真实已确认登录响应 HTTP 201、status=1/acctStatus=1，并下发 promotewebsessionid（Path=/、Secure、Max-Age=253402300000、Expires 年份 10056）。Go 标准 cookiejar 用 time.Duration(MaxAge)*time.Second 计算期限时溢出，导致新 Cookie 立刻被判过期；原测试短期 Cookie 没覆盖此响应差异。现在用 sessionJar 包装标准 CookieJar，仅将超过 400 天的正 MaxAge 收窄到 400 天后交给标准实现，不修改调用方 Cookie，不改变删除、域名、路径、Secure 或 TLS 规则。400 天为本地上限，不是平台会话有效期承诺。

用户此次已扫码确认的响应经修正后的 CookieJar 读取后，真实微信 profile 核验通过、会话加密落盘；临时响应头和恢复测试已清理，未输出原始凭据。回归覆盖完整确认登录链路与同样超长有效期，以及 Secure/路径/域名隔离和 MaxAge<0 删除。


手动 Cookie 校验兼容 Chromium 原始请求中的无名 Cookie（无等号的 token 片段）：保留原始 HTTP 头，只从有名称的字段提取登录与签名参数；无名片段不能代替 sessionid。重复命名字段、非法名称及换行仍拒绝。


## 视频号对标监控（2026-11-09）

POST /wechat/authors/search 使用已加密保存的独立投手会话，以表单调用 searchAccount；返回身份/昵称/头像/简介白名单。POST /accounts?platform=wx 使用所选 finder_id 建档，固定 threshold=10000，后续 PATCH 保持此门槛。GET accounts/videos/overview/notifications/runs 和 notifications/read 支持 platform 参数；账号子查询隔离平台数据，无参数保留旧客户端兼容行为。

benchmark.PlatformCrawler 将 wx 请求交给 service/wechat.Crawler，dy 交给原 PythonCrawler；共用一个 Monitor 串行队列。微信固定官网表单 getFinderObject，请求时间为 Unix 秒，游标原样传递。favCount 映射内部统一门槛字段 Video.likes（大拇指），新增 nullable hearts 保存 likeCount，favorites 不作微信收藏数。nonce 只用于按需 getObjectShortLink；POST /wechat/videos/:id/share 只接受已存视频号作品，结果仅为官方分享页。缺计数/作者不符/空页/游标异常不写入业务数据。

账号增加 history_cursor/history_end/history_complete，经显式 migrate 执行；启动只校验字段。首次历史扫描固定 endTs，分批有界采集（最多500条/100页、每页5条、页间1秒），未完时保存游标，启用账号30秒后自动续扫。历史批次全部使用基线去重，不产生旧视频上新或达标提醒；baseline_at 只在首次成功批次建立且后续不移动。完成历史后定时任务从最新作品开始检查，仍有单轮上限。

Video.first_over_10k_at 继续采用观测日期，AlreadyOver10k 区分首次已高位和有低位观察的跨越；后台保存低位作品支持跨越判断，前端 wx 默认 min_likes=10000。cohort=historical/new 按发布时刻与账号 baseline_at 比较，published_from/to 按北京时间日期过滤；over_date_from/to 为首次发现过万日期，二者含义独立。视频号口播转写已与本地下载串接；元宝单条视频下载代码及入口见后文，真实下载验收见handoff。

### 视频号高位计数与分享入口

原始 favCount/likeCount=100002 时前端保守显示10万+，注明接口高位值、非精确数量；未取得微信正式特殊值定义。数据库原整数及严格过万判定保持不变。分享 API 返回官方短链接、本地PNG二维码和可选 direct_url；展开请求不带登录Cookie，仅接受官方 channels.weixin.qq.com/finder-preview/pages/sph 重定向，不自动跟随其他域名。弹窗支持扫码、复制、电脑打开；浏览器代理错误与接口获取成功分别呈现，不承诺分享页播放。

视频列表GET /videos增加all=true，保留全部筛选与排序、取消默认24条分页，返回all/page_size；仅查询本地已收录记录。前端底部显示全部/分页显示切换，平台切换恢复分页，采用请求序号避免旧查询覆盖新结果。

## 抖音专家媒体下载接入（2026-11-10）

scripts/download_douyin.py复用现有本项目Cookie/专用profile及保留的签名客户端，核对详情作品ID、排除图集，按上游media.py提取候选。只接受固定抖音视频URL和HTTPS抖音媒体域名，每次跳转检查域名，CDN不携带Cookie；最多3个候选、150MB、30分钟及180秒超时。临时文件核对响应类型/长度并经ffprobe验视频流后原子发布，不覆盖已有文件。提供单条CLI；extract_copy替换此前yt-dlp下载步骤，后续FFmpeg/Whisper保持。真实一条MP4下载及完整解码通过，页面批量下载未接入，本地Range播放已接入，不能把单条下载验收当批量功能完成。

## 抖音网页视频下载（2026-11-10）

web/download → service/download → scripts/download_douyin.py，复用专家链路和项目凭据。GET/POST /api/v1/videos/:id/media 读取/发起，GET .../media/file 下载已完成MP4附件（支持HTTP Range）。只接受已收录的抖音数字ID和固定官网视频URL；单进程单任务，重复请求复用，其他作品忙时409，后台4分钟超时并随服务停止。状态保存在内存；重启后通过ffprobe重新识别本机已完成文件，不自动恢复中断任务。文件位于SQLite目录旁downloads/douyin，常规为.local/downloads/douyin；不覆盖现有文件。前端轮询状态、失败可重试，关闭弹窗继续，成功显示保存MP4；尚无批量下载；已支持页面内播放器。

## 元宝视频号媒体适配（2026-11-10）

service/yuanbao封装独立登录、分享解析和媒体保存；Go启动项目scripts/yuanbao_login.py，在配套Playwright Chromium的无头新context中加载官方微信登录页，不读取默认profile。GET /yuanbao/login/qr仅在活动登录期间提供官方iframe的PNG（no-store），结束后清理。starting表示页面加载，waiting及qr_ready仅在PNG就绪后返回，前端直接展示二维码。只有取得非匿名hy_user/hy_token才原子保存0600凭据，GET /yuanbao/connection返回configured/state/saved_at，POST /yuanbao/login启动或复用等待，POST /yuanbao/login/cancel取消。configured仅表示本机凭据存在，接口有效性以解析结果为准，错误和会话状态分开。

下载仍使用/videos/:id/media状态和附件路由。service/download按已收录账号平台分派：dy保留原脚本；wx要求非空exportId/nonce，调用项目wechat.Share，再通过yuanbao.Download解析。元宝POST使用独立元宝Cookie；微信预览与CDN无Cookie/Authorization，所有跳转显式受限。预览地址须官方HTTPS、路径/参数有效；跨服务opaque exportId实测会重发，不能直接用字符串相等判错作品；以项目投手从已收录exportId/nonce即时取得分享链接，再强制核对元宝author/desc及预览description/createtime与本地作者昵称、完整文案、精确发布秒一致，任一缺失或不符均拒绝；仅mediaType=4普通视频。媒体限定腾讯域名HTTPS，150MB/30分钟，临时文件检查长度/类型、ffprobe视频流/时长及ffmpeg完整解码后硬链接原子发布，不覆盖。wx使用downloads/wechat/<本地数据库ID>-<exportId的SHA256>.mp4，避免将含斜杠的exportId拼路径。复用现有单任务与4分钟超时；重启只重新核对已存在文件，不恢复未完成任务。未新增数据库字段，无迁移。

网络复用配置wechat_dns_server的独立解析策略；登录浏览器通过临时127.0.0.1 CONNECT通道使用同一DNS，仅允许腾讯域名443端口，登录结束关闭。通道透传TLS，不解密、不装证书、不改系统代理，保留TLS验证；元宝/微信接口错误不回显签名地址或token。本轮仅依据公开协议独立编写适配代码，未将上游程序作为运行依赖。真实下载验收状态记录在handoff。

## 视频发现紧凑列表（2026-11-10）

前端renderVideos保留原筛选、分页/显示全部、日期分组和事件委托，改为每作品一行的CSS Grid；桌面行高60px，小屏分行适配。缩略图失败使用小占位，不再生成大海报；精确/历史样本、高位10万+口径保留，完整标题与统计提示通过title提供。视频号新增本地发布文案弹窗，来自当前记录title，明确与视频口播分开；不新增接口或采集。抖音口播、双平台下载及分享仍调用原流程。

## 下载复用、首帧与转写

transcript.Service.Prepare调用download.Ensure：启动或等待同一视频下载，成功后只把验证过的项目文件路径传给PythonExtractor.ExtractFile（--input）；下载失败则任务失败，不用标题冒充口播。转写取消不取消共享下载，完成结果持久化复用。无新增表或迁移。

GET /api/v1/media?ids=批量返回最多500个已收录视频状态；缓存成功状态每次核对文件存在性、常规文件及大小。GET /videos/:id/media/play提供inline MP4和HTTP Range；/poster用FFmpeg首帧生成缓存JPEG。路径由数据库视频身份生成，不接受用户文件路径。前端已下载页签沿用当前平台/筛选后取交集，封面及播放按钮打开本地播放器。

认证/监控异常在当前平台显示持续提示，已知认证错误使连接按钮失去绿色；网络异常保留实际原因，不能证明认证已过期。没有外部通知或主动实时失效推送。

## 作者批量操作与登录状态更新（2026-11-10）

前端把多个昵称拆分去重（最多20个、单项80字），依次调用现有作者搜索API，每组5条，可独立翻页至最多50页/失败重试；作者按username去重，多选、全选均排除已添加账号。勾选后依次POST已有accounts?platform=wx，统一间隔/固定过万门槛/500条上限并启用自动监控；由现有Go调度串行建档，不并行发起check。部分失败保留失败项选择与原因，再试不重发已成功项；关闭弹窗停止后续派发，已经发出的请求会完成。无后端/迁移变化。

微信poll成功原先只刷新身份而不关弹窗，旧账号错误又能覆盖更晚的核验。现在成功即关闭、停止轮询、清理当前平台操作错误并刷新连接和异常面板；last_checked_at早于connection.verified_at的历史认证错误不覆盖新状态，网络错误保留真实原因。

## 逐帧切片（2026-11-10）

web/frames → service/frames → download.Ensure → FFmpeg。GET/POST /videos/:id/frames查状态/启动，POST /cancel取消，GET /images/:frame取图，GET /archive下载ZIP；仅已收录视频，沿用Host/origin限制。单进程串行切片，独立取消不取消共享下载。FFmpeg -fps_mode passthrough保留VFR原帧、不补帧，-xerror使解码错误失败；50001帧哨兵和1GiB图片预算检测超限，不静默截断。临时目录内完成图片、ZIP和manifest后重命名发布，服务重启复用manifest；强杀时临时目录清理和历史产物回收尚未接入。前端用弹窗epoch隔离过期请求，每页48张，避免一次渲染数千图片。

## 切片抽样更新（2026-11-10）

替代此前逐帧默认行为：GET/POST frames及images/archive均通过mode/count/interval参数选择同一缓存，缺省count=30，数量范围1–300，interval范围0.1–3600秒。数量模式ffprobe计数后按源帧序列均匀选取（含首尾），不足则取全部实际帧；间隔模式select基于上一选中帧时间，保留原帧、不插值。缓存路径包含视频ID和规范化参数，旧逐帧目录保留但不复用为抽样结果。最多1000张、1GiB图片，上限为失败哨兵，不静默截断。
