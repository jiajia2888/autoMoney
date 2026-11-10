---
name: media-crawler
description: 抓取抖音/小红书/快手/B站/微博/贴吧/知乎的公开内容与素材，落到本机供后续改写与出片使用。当用户说「抓一批素材」「爬一下这个词」「找点XX的视频」「搜XX的笔记」，或需要为出片准备原始素材时使用。不用于文案改写、配音、分镜设计、渲染出片或事实核验。
---

# 素材抓取

从平台按关键词抓公开内容，落到本机磁盘。**这一步只负责把素材拿到手** ——
它不判断素材好坏、不改写、不选音色。抓到之后交给 `spoken-script-rewriter`
改写、`speech-and-asr` 转写与配音、`video-director-runtime` 出片。

## 先体检

```bash
python <本skill目录>/bin/run-crawl.py doctor
```

它一次报清四件事，全绿才算能跑：

| 检查 | 缺了会怎样 |
|---|---|
| 运行时根 / 项目根 | 后面所有命令都找不到人 |
| 解释器 + 爬虫依赖 | 跑到一半才 `ModuleNotFoundError` |
| `main.py --help` 能起来 | 包都在但 CLI 起不来 —— 两者不是一回事 |
| 登录态 / 浏览器 / 可写 | 首跑的扫码、CDP 拉起的 Chrome、落盘目录 |

**`--help` 那条不能省。** 「包能 import」和「CLI 能跑」是两回事，
media-crawler 打包时就漏过一个 `httpx`：文件全在、import 检查全过，
第一条真命令才炸。

## 登录态：只在失效时才叫用户

**不要一上来就找用户。** 爬虫开场自己会探一次登录态
（`media_platform/*/core.py` 里的 `if not await client.pong(...)`）：
探通了**整个登录流程根本不进** —— 哪怕 `--lt` 写的是 `qrcode` 也不会弹任何东西。
只有探不通才需要用户出面。登录态过期是几周量级。

`cookie + localStorage` 落在持久 profile 目录
`browser_data/cdp_<platform>_user_data_dir/`，`--lt cookie` 每次复用。
**绝不要清 `browser_data/`** —— 那等于让用户重登一次；真要重登用 `reset-login`（定点清）。

### 实测结论：清凭据后抖音给什么，**不稳定**（2026-09-20 两次不一样）

拿 `reset-login` 清掉凭据、保留设备指纹（= 真用户「登录态过期」）后重跑：

| 场景 | 抖音的反应 |
|---|---|
| 登录态有效 | 直接可用（无需任何人工） |
| 凭据过期、设备熟悉（`reset-login` 之后） | **上午实测：滑块验证**（「请按住左边按钮拖动完成上方拼图」）；**12:09 实测：直接给了二维码** ← 同一套操作，两次结果不同 |
| 整份 profile 换成空的（陌生设备） | **短信验证**（爬虫没有喂短信码的通路） |

所以**别假定一定会给码，也别假定一定会给滑块** —— 它自己会变。
对用法没影响：浮层弹什么就在窗口里处理什么，只是**必须有人在电脑前**。

真到要重登的地步，顺序是下面这样：

1. 确认爬虫没在跑（它自己检查 9222 端口，占了会拒绝）。
2. 跑 `run-crawl.py login-qrcode`（见下节）：它起一个可见的 Chrome 窗口、
   注入 `stealth.min.js`，并等浮层里的二维码渲染出来（约 10~18 秒自动弹）。
3. 窗口里出现二维码就用手机扫；出现滑块就拖；又冒出短信验证说明这条路
   不通 —— 回滚登录态备份，或换网络/设备再试。

### 扫码这条路：能跑通，但**换不换发持久登录态看运气**（2026-09-20 两次实测相反）

同一天两次「清凭据 → 扫码」，结果**完全相反**，所以别拿单次结果当定论：

| | 13:48 那次 | **15:04 那次（成功）** |
|---|---|---|
| `sessionid` | expires 当天 18:03（游客会话） | expires **11-19**，`persistent=1` |
| `passport_assist_user` | session cookie（关浏览器即丢） | expires **2027-10-25**，`persistent=1` |
| 随后真抓取 | **0 条** | **14 条**（素材落盘 32MB、台账 success） |

**15:04 那次的完整链路 —— 这就是「跑通」该有的样子**：

1. `run-crawl.py reset-login --yes` 清凭据（保留设备指纹）
2. `run-crawl.py login-qrcode --keep-open --wait 1200` → 用户扫码 → 24 秒内登录成功
3. **落地前先验**：读磁盘 Cookies 库，看 `is_persistent` 和 `expires`（脚本见下）
4. 是 `persistent=1` + 几周后的 `expires` → 直接 `run-crawl.py --lt cookie` 抓即可，
   **不需要保留浏览器** —— 新起的 Chrome 从磁盘就能读回登录态

⇒ 「扫码登得进去、成果带不走」**不是必然** —— 上一轮那条绝对结论只对了一半。
⇒ 真正的判据是 **`is_persistent`**，不是 `expires` 有没有值：
  `0` + 当天几小时的 `expires` = 游客会话，白扫；
  `1` + 几周几月后的 `expires` = 真换发了长期登录态，可用。
⇒ `passport_assist_user` / `d_ticket` 在**运行期**一定存在（所以扫码脚本读得到），
  但 `persistent=0` 时 Chrome 一关就不落盘 ⇒「关掉浏览器再查库看不到它」**不代表当时没登上**。

**扫码后必做的验证**（只读库，不起浏览器）：

```bash
python - <<'PY'
import sqlite3, datetime
p = r"<profile>\Default\Network\Cookies"
c = sqlite3.connect("file:%s?mode=ro" % p.replace("\\", "/"), uri=True)
w = lambda x: "-" if not x else (datetime.datetime(1601,1,1) + datetime.timedelta(microseconds=x)).strftime("%m-%d %H:%M")
for n in ("sessionid", "passport_assist_user", "d_ticket", "has_biz_token"):
    r = c.execute("select creation_utc, expires_utc, is_persistent from cookies "
                  "where host_key like '%douyin%' and name=?", (n,)).fetchone()
    print(n, f"created={w(r[0])} expires={w(r[1])} persistent={r[2]}" if r else "—— 不存在")
PY
```

（时间戳是 **UTC**，+8 才是本地 —— Chrome 存的是 WebKit 纪元微秒。）

**兜底路径（本次没验到，因为用不上）**：万一第 3 步看到的是 `persistent=0` 的游客态，
说明成果只活在浏览器内存里，那就**别关窗口**：

```bash
run-crawl.py login-qrcode --keep-open --wait 1200                    # 登录后窗口留着
CDP_CONNECT_EXISTING=true run-crawl.py --lt cookie --type search …   # 爬虫连上去复用
```

`CDP_CONNECT_EXISTING` 默认关（`config/base_config.py`），打开后经
`cdp_browser.py:108` 连 9222、走 `contexts[0]` 复用已存在的 context
（项目自己也有对应处理：`cdp_browser.py:490`「连的是别人的浏览器，跳过进程清理」）。
**这条链尚未真跑通过** —— 只在没换发出持久态时才需要它。

**注意 `run-crawl.py` 没有 `--connect-existing` 这个开关**：它的 `_clean_env()` 是
`dict(os.environ)` 起底、只剔 `PYTHONPATH`，所以只能像上面那样用**环境变量**传进去
（写成 `--connect-existing` 会被当透传参数甩给爬虫主程序，报 typer 参数错）。

### 要深挖浮层就上探针：`bin/probe-login.py`

抖音改版导致选择器失效时（本 skill 2026-09-20 就被改版坑了两回），别猜 —— 直接 dump：

```bash
# Windows：
<项目根>/.venv/Scripts/python.exe <本包>/bin/probe-login.py <项目根>
# macOS / Linux：
<项目根>/.venv/bin/python3 <本包>/bin/probe-login.py <项目根>
```

它起一个可见窗口、等浮层渲染出来，然后把 `HasUserLogin`、全部 cookie 名、
浮层里的 `img`/`canvas` 节点（含自然尺寸）、以及各候选选择器的命中数
写进 `<项目根>/.workbuddy/tmp/probe-login-dom.json`。约 20 秒，**只读**，
不点不填不清。`pong()` 那条错判据（游客 sessionid）就是靠它钉死的。

### 框架自带的扫码登录：实现是齐的，路是断的

先纠正一个容易搞错的印象 —— **爬虫框架本身完整实现了扫码登录，而且默认就走它**：
`--lt` 三选一 `qrcode`（**默认值**，见 `config/base_config.py:28`）/ `phone` / `cookie`；
`media_platform/douyin/login.py` 的 `login_by_qrcode` 用的选择器
`xpath=//div[@id='animate_qrcode_container']//img` 跟我们摸索出来的一致；
框架甚至还带**自动过滑块**（`check_page_display_slider` + `move_slider`，
图像识别缺口 + 模拟轨迹，最多重试 20 次）—— 上游自己在这段 docstring 里承认
「滑块验证准确率不太好…建议不要用抖音登录，或改用 cookie 登录」。

但这条路有三个断点，所以实际走不通：

| # | 位置 | 症状 |
|---|---|---|
| A | `client.py:223` `pong()` | 判据含 `sessionid`，游客态也判「已登录」→ **整个登录流程被跳过** |
| B | `login.py:115` `popup_login_dialog()` | 兜底 `//p[text()='登录']` 已过时 → **超时 exit 1** |
| C | `login.py:96` `check_login_state()` | 同一条 `sessionid` 判据 → **没人扫码也判「登录成功」** |

另外框架出码走 `utils.show_qrcode()` —— 内部是 `PIL.Image.show()`，弹一个**独立图片窗口**
（不是让你扫网页里那张），而且由 `run_in_executor` fire-and-forget 调用，弹不出来静默失败；
抠不到码时直接 `sys.exit()`。

所以在登录态真失效时**不要指望 `--lt qrcode` 能走完**（它会 exit 1）。
改用 `login-qrcode.py` 绕开 B、C：它自己连 CDP、直接把登录浮层逼出来。
谁点的登录按钮不重要 —— 只要在**同一个 profile** 里登录成功就行。

```bash
run-crawl.py login-qrcode                            # 默认守候 900 秒
run-crawl.py login-qrcode --wait 1200
run-crawl.py login-qrcode --keep-open --wait 1200    # 登录后保留窗口（兜底用，见上节）
```

登录成功的判据（每 3 秒检一次）：**只认「登录票」** ——
cookie 里出现 `passport_assist_user` / `d_ticket` / `LOGIN_STATUS=1`，
或本地 `localStorage.HasUserLogin == "1"`；成功会落一张 `login-ok.png` 截图。

⚠️ **绝不要拿 `sessionid` 当登录判据**（2026-09-20 实测踩到）：抖音**游客态**
也会下发 `sessionid`/`sessionid_ss`/`sid_guard`/`sid_tt`/`uid_tt` 一整套，
看着像登录、实际是空的。带这条错判据的旧版脚本在**没人扫码**的情况下
45 秒就报「登录成功」，浏览器一关什么都没留下。

守候超时**不等于失败**，只说明这段时间没人扫码 —— 登录态保持登出，
用 `_loginstate-backup-*` 回滚即可。

抠码也踩过两坑：**别用泛化选择器**（`img[src^='data:image']` 在页面上命中 2 张，
第一张是抖音 logo），**也别急着对容器截图**（容器先出现、二维码后画好，
中间那段容器里显示的正是 logo 占位图）。脚本现在等的是
`#animate_qrcode_container` 里那个 `<img>` 插进来，再解它的 src。
另外真二维码是**调色板 PNG，只有 ~800 字节**，别按体积去判断像不像二维码。

### 登录态半失效：用 `reset-login` 定点重置

**这是破坏性操作，会清掉凭据。动手前先备份：**

```bash
# 备份（只这几处就够回滚，约 1MB，秒级完成）
cp <profile>/Default/Network/Cookies   <备份目录>/Default/Network/
cp <profile>/Local\ State              <备份目录>/          # cookie 解密密钥，必须一起备
cp <profile>/Default/Preferences       <备份目录>/Default/
cp -r <profile>/Default/Local\ Storage <备份目录>/Default/  # 含 HasUserLogin

run-crawl.py reset-login --dry-run   # 先看会清掉什么（不动手）
run-crawl.py reset-login --yes       # 真清：凭据 cookie + HasUserLogin 标记
```

**别用 `cp -r` 整份 profile 当备份** —— 450MB 里塞满小文件，实测好几分钟还跟 Chrome 抢文件；
上面四件套 1MB 秒级，且回滚只需这四件。

它清的是两组东西：**登录凭据 cookie** + **本地 `HasUserLogin` 标记**。
但**光清不够** —— 见下节：抖音给游客也发 `sessionid`，`pong()` 照样认账。

**前提：爬虫不能在跑**（脚本自己检查 9222 端口，占着就拒绝执行）。
清完之后**必须接一次 `login-qrcode` 让用户真登录**，否则抓取就会变成
「0 条 + exit 0」的静默失败（2026-09-20 实测正是如此）。而且扫码
**必须有人在电脑前** —— 实测浮层可能给二维码、也可能给滑块。

### 「0 条 + exit 0」怎么判：先看有没有 `Begin login`

`pong()` 有两条判据（`media_platform/douyin/client.py:223`）：

```python
if local_storage.get("HasUserLogin", "") == "1":           # 判据 1
    return True
return cookie_dict.get("LOGIN_STATUS") == "1" or bool(     # 判据 2 ← 坏的那条
    cookie_dict.get("sessionid") or cookie_dict.get("sessionid_ss"))
```

**判据 2 是坏的** —— 抖音**未登录（游客）也会下发整套**
`sessionid`/`sessionid_ss`/`sid_guard`/`sid_tt`/`uid_tt`
（2026-09-20 探针实测：`HasUserLogin = null` 而 cookie 里有 `sessionid`）。
于是 `pong()` 返回 True → 不进登录流程 → 带着游客 cookie 去搜索 →
抖音返回空 → **0 条、exit 0、零报错**。

**真登录的标志**是那几张「登录票」：`passport_assist_user` / `d_ticket`
（游客态一个都没有）。

判据只有一条：**日志里有没有 `Begin login …`**

| 日志 | 含义 | 该做什么 |
|---|---|---|
| 没有 `Begin login`、0 条 | 登录态是**游客**（`pong()` 被游客 sessionid 骗了，本地标记在不在都一样） | 让用户真登录：`run-crawl.py login-qrcode`；或回滚 `_loginstate-backup-*` |
| 有 `Begin login`、最后 0 条 | 卡在验证上（滑块/短信，或选择器过时 exit 1） | 看登录那几行；多半要人坐到电脑前 |
| 有 `Begin login`、验证过了但 0 条 | 风控，不是登录问题 | 降频、换词再试 |

**不起浏览器就能判断登录态真假** —— 只读 sqlite，查有没有「登录票」：

```bash
python - <<'PY'
import sqlite3
p = r"<profile>\Default\Network\Cookies"
c = sqlite3.connect("file:%s?mode=ro" % p.replace("\\", "/"), uri=True)
print(c.execute(
    "select name from cookies where host_key like '%douyin%' "
    "and name in ('passport_assist_user','d_ticket')").fetchall())
PY
```

拿回空列表 = 游客态（登录态是假的，抓到 0 条是必然）；拿到 `passport_assist_user`
或 `d_ticket` = 真登录。

**别把「exit 0」当成功。** 唯一可信的账是 run 目录里的文件数
（`[archive]` 那行会打出来）—— 这也是为什么收尾要再看一眼目录。

## 跑一次抓取

```bash
run-crawl.py --platform dy --lt cookie --type search \
  --keywords "关键词1,关键词2" --save_data_option sqlite \
  --crawler_max_notes_count 15 --get_comment true --headless false
```

| 参数 | 取值 |
|---|---|
| `--platform` | `xhs` / `dy` / `ks` / `bili` / `weibo` / `tieba` / `zhihu` |
| `--lt` | `qrcode` / `phone` / `cookie` |
| `--type` | `search` / `detail` / `creator` |
| `--save_data_option` | `sqlite`（默认）/ `csv` / `json` / `jsonl` / `excel` / `db` / `postgres` / `mongodb` |
| `--sort_type` / `publish_time` | **只对抖音有效**：0 综合 / 1 最多点赞 / 2 最新；0 不限 / 1 一天 / 7 一周 / 180 半年 |

`--keywords` 多个词用英文逗号分隔。起量前先小抓一轮（`--crawler_max_notes_count 5`）
确认登录态和风控都通，再放大。

## 产物在哪

| 内容 | 位置 |
|---|---|
| 元数据（账号、笔记、评论） | `database/sqlite_tables.db`（首跑自动建表） |
| 素材本体 | `data/archived_videos/runs/<run_folder>/<platform>/<id>.mp4` |
| 封面 | 同目录 `<id>.cover.jpg`（统一转成 jpg、最长边 4096） |
| 台账 | 同库的 `crawl_run` / `crawl_run_keyword` / `crawl_run_item` / `content_index` |

`run_folder` 形如 `20260917_174909_005a643b`（时间戳 + run_id 前 8 位）。
**跨回合回来收结果时，认这个目录** —— 不要靠进程句柄，跨回合之后它已经没了。

### 素材落盘不是自动的：它挂在两个环境变量上

`tools/video_archive.py` 的 `is_active()` 只认这两条**同时**成立：

- `CRAWL_RUN_ID` 有值
- `CRAWL_ARCHIVE_VIDEOS=1`

其余三个含义：`CRAWL_RUN_FOLDER` 决定目录名（不设就退化成 `<platform>/<日期>/`）、
`CRAWL_TARGET_NEW_COUNT` 决定「抓够几条新的就停」、`MEDIACRAWLER_VIDEO_ROOT` 改根目录。

**这组变量原本由已退役的 8080 层设置**（`api/services/crawler_manager.py`），
客户端下线后没人设 → `is_active()` 恒假 → **素材一条都不落，只有元数据进库，
命令还照样 exit 0**。`run-crawl.py` 现在替它做了：生成 run_id、建 `crawl_run` 台账行、
把这组变量灌进子进程。所以**用它跑，不要自己直接调 `main.py`**。

顺序不能颠倒：**台账行必须先建**。`crawl_run_keyword.run_id` 有外键指向 `crawl_run.id`，
行不存在时归档写关键词行会 `FOREIGN KEY constraint failed`，整轮直接崩（实测踩过）。
`run-crawl.py` 会在起爬之前先调 `bin/crawl-run-row.py create`，失败会明确告诉你
「素材仍会落盘，但这轮的条数账不完整」。

### 归档还要 ffprobe

下载完的文件会先过 `ffprobe` 确认真有视频流，否则删掉不算数。
`run-crawl.py` 会把 `<runtime-root>/runtime/ffmpeg` 塞进 PATH，
`doctor` 里那行 `ffprobe` 就是查这个。缺了它，坏文件也会进库。

## 长任务：起了就撒手

抓取可能跑几十分钟（翻页 + 限速）。后台起来之后**直接结束回合**，
过一阵回来读 `data/archived_videos/runs/` 里最新的 `run_folder`。

- 后台任务**不会被回合边界杀掉**（已实测：15 分钟探针跨回合连续 333 行零断点）。
- 单次上限 3600 秒（项目 `.env` 的 `MEDIACRAWLER_CRAWL_TIMEOUT_SECONDS`）。
- 但**别指望「进程还在」等于「还在抓」**：写盘可能早就停了。
  判断依据只有一个 —— run 目录里的文件数有没有在涨。

**`--target-new` 是在翻页之间检查的，所以一定会超出。** 实测 `--target-new 3`
在抖音搜索上抓了 14 条才停：第一页整页都会落完，然后才在页边界发现目标已达
（`crawl_run_keyword.stop_reason = target_reached`）。按这个量级估预算，
别把 3 当成「只会下 3 个文件」—— 抖音单条能到 112 MB。

## 回归基线（2026-09-17 实测）

拿这条对一次，能立刻看出归档链路有没有变：

| 项 | 值 |
|---|---|
| 命令 | `--platform dy --lt cookie --type search --keywords "短视频运营" --save_data_option sqlite --crawler_max_notes_count 8 --target-new 3 --headless false` |
| 耗时 | 1 分 53 秒（含 4 次视频下载） |
| 素材 | `data/archived_videos/runs/20260917_174909_005a643b/dy/`，14 条 mp4 + 14 张 cover.jpg，28 个文件 / 276 MB |
| 文件形态 | h264 **1080×1920** 竖屏，44.5 s / 39.3 s（ffprobe 实读） |
| 台账 | `crawl_run.status=success, new_count=14`；`content_index` 50 → 64 |
| 退出码 | 0（**但退出码不是判据，文件数才是**） |

**反面对照**：同一台机器、同一个关键词，归档变量没设那次（`crawl-1`）
—— 14 条元数据全进了库，素材目录一个文件都没有，命令照样 exit 0。
这就是为什么「只跑 `main.py`」和「跑 `run-crawl.py`」不是一回事。

## 环境前提（不由本 skill 负责，缺了会报错）

- **必须在项目根目录运行。** 登录态目录是 `os.getcwd()` 拼出来的
  （`media_platform/*/core.py`）。换个 cwd 跑，cookie 落到别处 ——
  表现为「刚扫完码又要扫」。`run-crawl.py` 已经把 cwd 钉死在项目根，
  所以**用它跑，不要自己 `cd` 到别处直接调 `main.py`**。
- **解释器不写死，按「真 import 全过」挑。** 顺序是 `--python` →
  `$CP_CRAWLER_PYTHON` → 配置里的 `crawler_python` → **项目 `.venv`** →
  `<runtime-root>/runtime/python`（POSIX 上再落回 PATH 里的 `python3`）。
  `.venv` 排前面是因为**它是这套爬虫的已知可用环境**，版本组合由它定；运行时那份
  python 是给「没有 .venv 的机器」的兜底。
  实测踩过：运行时里按 PyPI 最新装的 `aiomysql 0.3.2` / `pandas 3.0` 直接 import 就炸
  —— `aiomysql 0.3.2` 要的 `pymysql.converters.escape_dict` 已被 `pymysql 1.2` 删掉。
  **要装就照 `.venv` 的版本钉**（`aiomysql==0.2.0` 这类）。
- **解释器不能带宿主的 `PYTHONPATH`。** 宿主注入了
  `D:\ipDesign\.runtime\python-packages`（cp312），和这里的 3.11 混在一起会炸
  一堆 `numpy._core` 报错。`run-crawl.py` 会清掉它 —— 手动跑别的脚本时自己
  `env -u PYTHONPATH`。
- **`node` 要在 PATH 里**（pyexecjs 算个别平台的签名参数用）。`run-crawl.py`
  会把随包 node 塞到 PATH 前面；POSIX 上用系统那个（`brew install node`）。
- **`ffprobe` 要在 PATH 里**（归档器校验下载到的视频）。本层会把
  `<runtime-root>/runtime/ffmpeg` 塞进去；POSIX 上随包那份不存在，用系统的
  （`brew install ffmpeg`），`run-crawl.py doctor` 会去 PATH 上找。
- 浏览器走 CDP，用**本机已装的 Chrome/Edge**，不下 playwright 自带浏览器。
  反爬决定了它是有头的，这是设计如此不是缺陷。
- **爬虫依赖里有两个反直觉的**：`redis`（`proxy/base_proxy.py` 构造时就按
  `config.CACHE_TYPE_REDIS` 建缓存客户端）和 `aiomysql`（`var.py` 模块级 import，
  还拿 `aiomysql.Pool` 做类型注解）。它们不是「8080 才要的」。

## 完成时要说什么

- **报 run 目录的绝对路径**，那是跨回合唯一的账本。
- 报抓到多少条、落在哪张表 / 哪个子目录。
- **不要把「命令 exit 0」当成「抓到了」**：登录态过期、风控拦截都可能
  带着 0 条结果正常退出。要去看 run 目录里有没有真东西。
- 失败就说在哪一步、上游原话是什么，不要用「已检查」糊过去。
