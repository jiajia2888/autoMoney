# Duix-Avatar 替代 HeyGen：换装图与口播批量生成可行性

调研开始：2026-11-09；完成：2026-11-10（Asia/Shanghai）。仅阅读官方仓库、文档和许可；未下载模型、未拉取 Docker 镜像、未安装运行、未测试生成画质与速度，也未发送付费任务。用户业务为合作 IP 代切片，计划从真实视频取得人物参考，每种视频换一套衣服/背景图，再生成数字人口播。

本轮核查官方仓库为 [duixcom/Duix-Avatar](https://github.com/duixcom/Duix-Avatar)，固定到提交 **1328feb5871448c8fa3d0e45b3bbc87e7c1d458a**（提交日期 2026-04-21），避免后续主分支变化导致许可引用歧义。旧 README 链接中的 Duix.Avatar 仓库名会重定向至该仓库。

## 可直接用于决策的结论

**可以把 Duix-Avatar 列为本地口播视频合成的候选，但目前不能把它当作“换装静态图直接输入、完整等效 HeyGen”的替代。**

1. 官方主流程是 **一段人物视频 + 目标音频 → 非实时口播成片**。README_zh 开头称提交约 10 秒视频；上传组件选择视频并校验时长至少 8 秒；视频合成 API 参数也是 `video_url` 和 `audio_url`。没有从本轮公开一手资料核实照片直接输入的正式支持。
2. Duix 不替上游生成新衣服或新背景。若坚持每次只生成一张换装图，仍需另配“图片变自然动作视频”的步骤，或选已证实照片驱动的其他模型。把图片转成静态 MP4 即使后端能接收，也不会自动获得自然身体动作；本轮没有验证该技巧是否能运行。
3. 它确有本地提交/查询 API 和文件导出、等待队列，可自行接工作流，并非只能做实时交互 SDK。
4. 官方部署为 Windows / Ubuntu 22.04 + NVIDIA GPU；推荐 RTX 4070、32GB 内存。当前 Mac 可担当中控，但不能将其说成官方支持的 Mac 本地 GPU 推理方案。
5. **商用许可是当前关键未决项**。README 的免费商用说明、根 LICENSE 和单列模型社区许可存在明显差异，需厂商书面确认当前部署镜像和模型适用的授权，不能笼统称“免费无条件商用开源”。

## 输入方式与“每换一套衣服是否重训”

[README_zh](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/README_zh.md) 的首段为：

> 仅需提交一段10秒左右的视频，即可快速完成数字人形象和声音克隆，输入文案或上传音频即可驱动数字人口型，自动生成口播播报视频。

[上传组件 ModalBoxUpload.vue](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/src/renderer/src/components/model-create/ModalBoxUpload.vue) 调用 `Client.file.selectVideo()`、读取视频信息，并拒绝 `videoInfo.duration < 8`。

[官方常见问题](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/doc/%E5%B8%B8%E8%A7%81%E9%97%AE%E9%A2%98.md) 明确：“用于创建模特的视频必须有声音，且是人在说话，程序需要用这个声音来做声音克隆”。所以静态图片在当前客户端建模流程中不能直接替代有声底视频。

但不能反过来说“每个衣服都需要耗时训练一遍人物模型”：

- [model.js](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/src/main/service/model.js) 的 `addModel` 将输入转 H.264、分离音频、调用 `trainVoice`，并将 `videoPath/audioPath/voiceId` 登记到数据库。
- [voice.js](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/src/main/service/voice.js) 的 `train` 调用音频预处理接口取得参考音频与参考文本；这里没有显示一个完整人物面部深度训练任务。
- [video.js](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/src/main/service/video.js) 渲染时把人物底视频路径与目标音频交给合成接口；已有目标音频时可跳过 TTS。

准确表述为：**每套外观需有可用的相应视频底片/模板；是否存在服务器端预处理缓存、重建耗时和复用收益，本轮没有运行验证。** 把它说成每套 look 必须训练专属人脸模型，证据不足。

## 本地 API、离线导出与批量能力

[README 英文版的 Open APIs 节](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/README.md) 明确开放本地服务：

- 视频提交：`http://127.0.0.1:8383/easy/submit`，参数 `audio_url`、`video_url`、唯一 `code` 及选项。
- 进度查询：`http://127.0.0.1:8383/easy/query?code=...`。
- TTS 为独立服务；已有配音可以不采用其声音克隆链路。

`video.js` 有 waiting/pending/success/failed 状态、自动取下一等待任务、查询结果路径，以及 `exportVideo` 将成片文件复制到目标路径。README 明确该项目用于数字人克隆与**非实时视频合成**；实时交互指向另一商业平台，不应混为同一能力。

这些源码可以支持“具备封装为自有异步 worker 的基础”这一判断，但本轮没有证明并发稳定性、长视频能力、吞吐量、中文口型画质或 API 在实际镜像中的兼容性。

服务端 GPU 推理部署依据是 [docker-compose-lite.yml](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/deploy/docker-compose-lite.yml) 中的 `guiji2025/duix.avatar` 镜像与 `/code/app_local.py`。本轮公开主分支树未发现与该 Docker 内部推理程序对应的 Python 实现，且未拉取镜像审查。因此不能仅从 Electron 客户端代码断言其 OpenCV 解码支持图片、每个模型权重的来源或 GPU 算法实现；镜像 tag 也未在本次资料中固定到 digest。

## 硬件与运行条件

依[官方 README](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/README.md)：

- Windows 10 19042.1526 及以上；另有官方验证过的 Ubuntu 22.04 桌面部署。
- 必须有 NVIDIA 显卡与驱动，Docker 使用 NVIDIA runtime。
- 推荐 i5-13400F、32GB 内存、RTX 4070；Ubuntu 节将 32GB 或以上标为必要。
- Windows 文档列 C 盘 100GB 以上空间、D 盘数据 30GB 以上；完整服务初始下载约 70GB。可按实际镜像/目录配置重新评估空间，不能把下载量当成固定长期占用。
- 新部署说明覆盖 50 系列 GPU，称在 5090 测试通过，但本轮没有实测任何型号。

README 中“8G 显存可用”的链接是社区视频教程，不将其升格成官方统一最低要求，也不据此承诺生产速度。

## 商用许可：必须区分 README、代码与模型

### README 与根 LICENSE 的门槛不同

README 表格声称“支持全球免费商用（用户量超过 10 万或年营收达 1000 万美元以上的企业需签署商业许可协议）”。

但同一提交的[根 LICENSE](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/LICENSE) 是自定义 **DUIX.COM COMMUNITY LICENSE AGREEMENT**，不是 MIT/Apache。其第 2 条写的是 **greater than 1 thousand Monthly Active Users** 时需申请商业许可。该 MAU 指使用集成其材料的产品/服务的用户，不应擅自解释为短视频播放量，也不能简单等同生成条数。

第 1.b 节还有协议随附、`Built with DUIX.COM` 标识、用户协议说明及 Notice 要求；第 5.c 节含对案例、示例、应用或实现的使用授权条款。具体适用应按计划提供的内部/客户服务确认。

### 模型 PDF 另有商用书面授权要求

官方仓库单列模型协议：

- [英文模型许可 PDF](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/Duix.Avatar%20model%20community%20Licensing%20Agreement.pdf)
- [中文模型许可 PDF](https://github.com/duixcom/Duix-Avatar/blob/1328feb5871448c8fa3d0e45b3bbc87e7c1d458a/Duix.Avatar%20model%20community%20Licensing%20Agreement_zh.pdf)

本轮已读取中英文 PDF 文本。英文第 2 页、中文版第 2 页均含商业书面授权要求；中文版第 1 页定义商业用途。标题适用对象是 **Duix.Avatar:0.0.7_sdk_slim Model**；第 2 节首先限定非商业用途，第 **2.2** 条明确：如果模型或其派生用于商业，应提前登记并取得许可方**书面授权**。商业用途定义包含直接或间接经营、推广、创收/营利。第 8 节列商业授权申请方式及材料。

该 PDF 与 README“免费商用”表述明显不同。由于部署 compose 使用未固定版本的 Docker tag，尚不能确认当前镜像是否使用该 PDF 指定的模型，以及厂商是否另发了替代许可；不能自行挑选最宽松的一段解释。实际接入前应取得覆盖**当前镜像、模型版本、商业代制作成片及客户交付**的明确授权与报价。不能从公开文本算出其商业许可费用为零。

## 针对用户流程的选择建议

- **保留原人物视频外观，只改口播**：Duix 的输入形态更贴近，可作为本地合成候选；有目标配音时可用精简视频合成链路，减少对其 TTS 的依赖。
- **每条从一张换装图生成新形象口播**：Duix 不能仅凭已核实资料直接承接。建议先用明确支持图片+音频的模型做对照，或在 Duix 前增加能保持身份、衣服与背景的动作视频生成步骤。
- **固定一套新服装/场景批量出几十条**：先制作并验收对应视频底片，之后复用模板合成新音频，是较合理的 Duix 评估路径。

真正应比较的成本是：`许可费用 + GPU折旧/租用 + 运维 + 换装图/底片生成 + 废片返工 + 质检`，不能只把离线推理的边际 API 账单当成全部成本。本报告未测最终效果，不承诺能在画质、动作自然度、速度上等效 HeyGen。
