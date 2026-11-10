# 内容生产导演（成片）

把一句话题或一堆素材做成能发的短视频：抓素材、口播稿改写、分镜设计、配音与配时，
再对着**真实成片**逐句抽帧验收。交付边界是「**候选片 + 全部可验证项**」，
最后那一眼人耳复核留给人。

## 类型

Agent 型（单个 AI 专家）

## 功能

- **素材抓取** —— 内置 `media-crawler`：按关键词从抖音 / 小红书 / 快手 / B站 / 微博 /
  贴吧 / 知乎抓公开内容与素材，落在本机 `data/archived_videos/runs/`。只负责把素材拿到手。
- **口播稿改写** —— 内置 `spoken-script-rewriter`：把整理文案或原始 ASR 洗成可直接朗读的成稿，
  去机器腔、合并碎片、保留主稿声音，且不编造事实。多篇素材以第一篇为主稿合成一篇。
- **分镜设计** —— 每个镜头一个信息点，附带一套可验收的画面约束（隐藏未讲到的模块、
  动作必须在下一拍前完成、状态改变要有可辨认的中间态、退场不得半透明残留）。
- **配音与转写** —— 内置 `speech-and-asr`：配音**直连平台** `tw.hzxaitoy.com`
  （`X-API-Key`，按字扣点）。平台现挂两套 TTS 后端 —— 硅基流动 CosyVoice2
  （3 点/百字，8 音色）与**豆包语音合成 2.0** `seed-tts-2.0`（5 点/百字，4 音色）；
  **模型与音色必须配套**，先跑 `speech.py models` / `voices` 看清楚再合成。
  **默认模型是豆包**（不写 `--model` 就是它），合成时**顺带给出词级时码**，
  **默认写进 `voice.words.json`**（**不额外花钱**，`--no-timeline` 可关）；
  CosyVoice2 给不了时码。
  转写默认走**平台** `/asr/transcribe`（快、带标点、1 点/次）；
  但那个接口不给时间轴，**出片要的句级 SRT 只能用 `--via local` 在本机跑** faster-whisper
  （慢、不扣点、不联网）。另有 `timeline` 命令走平台 `/asr/submit`
  （豆包录音识别 2.0，1 点/次）对**已托管音频**取词级时码，用于分句校准。
  ⚠️ 豆包合成的音频句间不停顿，本机分句会偏粗（实测 37 字切成 1 句），
  出片前务必核对 `source.srt` 的分块 —— 有 `voice.words.json` 就拿它当切句真值
  （详见该 skill 的 SKILL.md）。
- **成片验收** —— 按真实视频抽帧与连续播放检查，不拿脚本声明或类型检查当观感通过。

三条链路都不经过本机 8080 产品服务 —— 那条链路已退役。

## 使用前需要配一次

只差两样，都在 `~/.workbuddy/content-production.json`：

```json
{
  "runtime_root": "D:\\content-production-runtime",
  "platform_base_url": "https://tw.hzxaitoy.com/api/scheduler",
  "platform_api_key": "sk_...",
  "asr_model": "mobiuslabsgmbh/faster-whisper-large-v3-turbo",
  "asr_model_dir": ""
}
```

1. **平台 KEY**（配音与线上转写用）—— 见下面「怎么拿到平台 KEY」。
   只存在这台机器上，**不随专家包分发**。
2. **出片运行时 + ASR 依赖**（出片与转写用）—— 跑一次体检就知道缺什么：

   ```bash
   python <包内>/skills/speech-and-asr/bin/speech.py doctor
   python <包内>/skills/video-director-runtime/bin/run-director.py doctor
   ```

   也可以用 `CP_PLATFORM_API_KEY` / `CP_ASR_MODEL_DIR` / `CONTENT_PRODUCTION_RUNTIME`
   环境变量代替配置项。

两条互不依赖：KEY 没配只挡配音，不影响转写与出片。

> **平台**：Windows 与 macOS / Linux 都能跑，差别只在**工具链从哪来**。
> Windows 用摆好的那套二进制（`D:\content-production-runtime\runtime\`）；
> POSIX 上不需要那一层，用 `brew install node pnpm ffmpeg python@3.11`
> （Debian/Ubuntu 换成 `apt install nodejs npm ffmpeg python3-venv`）装的就够 ——
> 启动器先看运行时目录、找不到就落回系统 PATH。
>
> 两样东西**必须按平台各做一遍、不能互相拷**：
> ① `node_modules`（里面是平台专属的 `@remotion/compositor-<平台>`）；
> ② 渲染浏览器（目录名 `mac-arm64` / `win64` 不同）。
> `pnpm install --frozen-lockfile` 会按当前平台自动挑对的那份 —— 锁文件里七种平台都在。
>
> 无论哪个平台，**转写默认走云端**（见「转写」一节）。`--via local` 要本机有
> `faster-whisper`：Windows 那份随包解释器自带，POSIX 上自己 `pip install faster-whisper`。

### 怎么拿到平台 KEY（没有账号的从这里开始）

平台是**扬帆者｜全域内容工厂**：<https://tw.hzxaitoy.com>

**第 1 步 · 注册账号**（已有账号跳到第 2 步）

打开 <https://tw.hzxaitoy.com/toolpages/login.html?tab=register> ，
用**邮箱 + 验证码**注册（密码至少 8 位，验证码发到邮箱、10 分钟内有效）。
注册后要建一个**工作空间** —— 填个名称，再起一个标识
（小写字母/数字/中划线，2–31 位，**全平台唯一**）。

> 新注册送点数，够把整条链路跑通一遍。
> 也可以先点首页的「游客进入」浏览模板与示例流程（0 点数、不保存内容），
> 生成/保存/下载/发布时才需要注册。

**第 2 步 · 生成 KEY**

登录后进**工作空间后台**，左侧菜单「**系统设定 → 外部接入**」→ 新建一把 KEY。
创建时**勾上要用的能力**：

| 能力 | 勾了才能用 |
|---|---|
| `tts` | 配音合成（`speech.py say`） |
| `asr` | 线上转写（`speech.py asr` 的**默认**那条路） |

**两个都勾上。** 转写默认就走云端，`asr` 没勾的话那一步会直接 `403` 报错
（**不会**自动改成本机）—— `--via local` 是**显式**选项，不是兜底。

生成后把 `sk_...` 填进配置文件的 `platform_api_key`。

> **搞错了怎么办**：脚本的报错里带了解法 ——
> `401` = KEY 无效或过期；`403` = 这把 KEY **没有对应能力**（回后台把能力补勾）。
> 所以「配音报 403」不是 KEY 错了，是没勾 `tts`。

## 出片运行时怎么装起来（接收方照着跑）

**专家包里不含运行时，但含出片 Skill 的规则层。** 这三件事要分开看：

| | 体积 | 在包里吗 |
|---|---|---|
| 出片 Skill 规则层（`SKILL.md` / `references` / `scripts` / `assets` / `profiles`） | **约 600 KB** | ✅ **在** —— `skills/video-director-runtime/vendor/xiaobai-video-director/` |
| 渲染依赖（`node_modules` 658 MB ＋ 270 MB 浏览器） | **约 900 MB** | ❌ 打包器硬排除 `node_modules` 与隐藏目录（POSIX 上由 `pnpm install` 按平台重建） |
| 工具链（node / pnpm / ffmpeg / CPython） | **1.6 G**（仅 Windows 形态） | ❌ **没有任何脚本会下**；POSIX 上用系统的，Windows 上从装好的机器拷 `runtime/` |

规则层进包，是为了让「导入了专家包、但盘上还没有那份 Skill」的机器上**至少读得到
规则**（`run-director.py docs` 会自动读包内那份，并在结尾标明来源）。但**渲染仍然
跑不起来** —— 依赖要接收方自建。**下面两套按平台选一套**：

**Windows**

```bash
# 0. 把运行时骨架放到 D:\content-production-runtime（或配置里的 runtime_root）
#    需要 runtime/{node,pnpm,ffmpeg,python} + skills/xiaobai-video-director/
#    包内 vendor/xiaobai-video-director/ 可以直接当这里的 skills/xiaobai-video-director/ 用

# 1. 重建 JS 依赖 —— 约 2 分钟 / 244 包 / 241 MB，需联网
cd <runtime_root>/skills/xiaobai-video-director
pnpm install --frozen-lockfile

# 2. 下渲染用的浏览器 —— 约 270 MB，需联网（pnpm install 不会顺带下它）
python <包内>/skills/video-director-runtime/bin/run-director.py bootstrap

# 3. 体检，全绿就能出片
python <包内>/skills/video-director-runtime/bin/run-director.py doctor
```

**macOS / Linux**（没有「运行时骨架」这一层）

```bash
# 0. 装工具链
brew install node pnpm ffmpeg python@3.11      # Debian/Ubuntu：apt install nodejs npm ffmpeg python3-venv

# 1. 建 skill 目录 —— 规则层直接拷包内那份（package.json 与 pnpm-lock.yaml 都在里面）
mkdir -p ~/.content-production-runtime/skills
cp -r <包内>/skills/video-director-runtime/vendor/xiaobai-video-director \
      ~/.content-production-runtime/skills/

# 2. 重建 JS 依赖 —— 会按当前平台装上 compositor 与浏览器，需联网
cd ~/.content-production-runtime/skills/xiaobai-video-director
pnpm install --frozen-lockfile

# 3. 下渲染用的浏览器 —— 需联网（目录名会是 mac-arm64 / mac-x64 / linux64）
python3 <包内>/skills/video-director-runtime/bin/run-director.py bootstrap

# 4. 体检，全绿就能出片
python3 <包内>/skills/video-director-runtime/bin/run-director.py doctor
```

> **Linux / WSL 还要多两样**（上游 README 的原话）：浏览器所需的系统库，以及
> **中文字体**（`Noto Sans CJK SC`）—— 缺字体成片里会是方框。macOS 自带
> `PingFang SC`，不用管。Windows 用 `Microsoft YaHei`。

**哪些是自动下载的**：只有「重建 JS 依赖」与「下浏览器」两步（从公网拉，合计约
511 MB）。工具链那一步**没有任何脚本会替你下** —— `node` / `pnpm` / `ffmpeg` /
`CPython` 要么从装好的机器拷（Windows），要么用包管理器装（POSIX）。

第 2 步**不能省**：Remotion 的浏览器是懒下载的，只在第一次渲染时才拉 —— 不提前下，
接收方会在第一次出片时突然卡住 270 MB 的下载。`bootstrap` 也不会重复下载，
已经在了就直接报路径和体积。

`doctor` 的返回码是接收方的红绿灯：**0** 运行时完整；**1** 规则在包内、渲染依赖
还没建起来（按上面那一套建完再重跑）；**2** 连规则都找不到。**只要不是 0，都不要
继续出片** —— 尤其**不要换个渲染器凑一条片子**，那出来的必然是静态拼贴的 PPT。
规则层进包后，`score-check` 这一步（纯 Python、不碰 `node_modules`）**在没建
运行时之前就能跑**，可以用来离线验 `score.json` 写得对不对。

解释器方面：Windows 那份自包含 CPython 3.11 要另配（`pillow` / `python-dotenv` /
`httpx[socks]==0.28.1` 三项）；POSIX 上没有随包解释器，用系统 `python3`，同样要有
这三样（`python3 -m pip install httpx pillow python-dotenv`）。两种都由
`run-director.py doctor` 逐项查。


## 使用示例

- 把这篇素材改成能直接念的口播稿
- 给这篇稿子配音，出一版竖屏视频
- 检查这条成片：音画同步、字幕和节奏哪里有问题

## 边界

- **渲染由会话驱动，但不在会话的进程里**。渲染器是盘上的
  `xiaobai-video-director`（Windows 默认 `D:\content-production-runtime`，
  POSIX 默认 `~/.content-production-runtime`，都可用配置里的 `runtime_root` 改），
  `video-director-runtime` 只负责找得到它、跑得起来它、不假装它跑通了。
  成品只认 `final-*.mp4`。
- **终审是人工闸门**。渲染器的 `score-release` 要求逐句观察、20 条历史经验复核，
  以及「声音可闻」—— 最后那项要求真听过，而 Agent 听不到声音。所以交付边界是
  「候选片 + 全部可验证项」，最后一眼留给人。
- **配音要花钱，且不可盲目重试**。平台配音是同步的、没有幂等键：提交前先报价、
  先拿用户同意；请求超时后自动重试就是重复扣点。
  **按平台真实价目表算**（`quote` 会读 `/models?kind=tts` 的
  `points_per_100_chars`）：默认的豆包 `seed-tts-2.0` **5 点/百字**、
  CosyVoice2 **3 点/百字**。以 `say` 响应里带回的真实 `cost_points` 为准。
  （脚本里那个 `ceil(字数/4.4)` 是**未校准的兜底公式，偏高约 10 倍**，
  只在平台没报价时才用 —— 别拿它当实价。）
- **不做事实核验**。事实存疑时输出 `terms_to_verify` 交人确认，不自行补全。

## 头像

`avatars/expert.png`（512×512 PNG，321 KB）。想换的话：PNG 或 JPG，512×512，≤500 KB。

## 重新注册

改完 `plugin.json` 或 Agent MD 后要重新注册才会生效：

```bash
python3 <expert-manager>/scripts/validate_expert.py <expert-dir>
python3 <expert-manager>/scripts/register_expert.py <expert-dir>
```
