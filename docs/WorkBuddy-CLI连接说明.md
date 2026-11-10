# 本机 WorkBuddy CLI 连接说明

核对日期：2026-11-10。以下根据本机实际文件与命令帮助整理，路径属于当前这台 Mac。

## 1. 当前是怎么启动的

终端里的 `workbuddy` 指向：

```text
/Users/fuzhoujinsanjuyuanzhou/.local/bin/workbuddy
```

这是一个 Shell 启动脚本，内容为：

```sh
#!/bin/sh
exec "/Users/fuzhoujinsanjuyuanzhou/.nvm/versions/node/v22.19.0/bin/node" "/Applications/WorkBuddy.app/Contents/Resources/app.asar.unpacked/cli/dist/codebuddy-lite-wb.mjs" "$@"
```

实际调用链：

```text
终端输入 workbuddy
  → ~/.local/bin/workbuddy
  → 本机 Node.js v22.19.0 路径
  → WorkBuddy.app 内置 codebuddy-lite-wb.mjs
  → CLI 根据选定模型调用对应服务
```

已实测 `workbuddy --version` 返回 `2.165.0`。`workbuddy --help` 的程序名显示 `codebuddy|cbc`，与其内置 CLI 入口一致。

本机还有独立的 `codebuddy` 命令，位于 `~/.nvm/versions/node/v22.19.0/bin/codebuddy`。本文的操作统一使用 `workbuddy`，对应上面的应用内置入口。

## 2. 模型连接在哪里配置

本机 WorkBuddy 的自定义模型配置位于：

```text
~/.workbuddy/models.json
```

当前文件中存在以下配置，且 `workbuddy --help` 列出了对应的 `custom-local:` 模型 ID：

| CLI 模型 ID | 配置的服务地址 |
| --- | --- |
| `custom-local:gpt-6.1-sol` | `https://mdac.hzxaitoy.com/codex-relay/v1` |
| `custom-local:MiniMax-M3` | `https://api.minimaxi.com/v1` |
| `custom-local:MiniMax-M2.5` | `https://api.minimaxi.com/v1/chat/completions` |

选择 `custom-local:gpt-6.1-sol` 时，配置指向的链路是：

```text
WorkBuddy CLI
  → models.json 中的自定义模型地址和 apiKey
  → https://mdac.hzxaitoy.com/codex-relay/v1
  → 该中转服务实际提供的模型服务
```

这里确认的是本机的配置地址和可选模型名。本轮没有调用模型，也没有验证中转服务上游实际使用的模型或当前可用性。默认启动时实际选择哪个模型，应以 CLI 会话中的模型显示为准。

真实 `apiKey` 保留在本机配置中，本文不包含它。WorkBuddy 其他用户设置位于 `~/.workbuddy/settings.json`；内置代码支持通过 `WORKBUDDY_CONFIG_DIR` 指定 WorkBuddy 配置目录。复用当前环境时无需另设该变量。

## 3. 在工程中使用

进入工程目录后启动交互会话：

```sh
cd /Users/fuzhoujinsanjuyuanzhou/Desktop/git/autoMoney
workbuddy
```

明确选择已配置的模型：

```sh
workbuddy --model custom-local:gpt-6.1-sol
```

单次文本问答，结束后退出；示例禁用内置工具，避免问答过程中修改文件：

```sh
workbuddy --model custom-local:gpt-6.1-sol \
  --tools "" \
  --print "请简要介绍你的能力。"
```

需要供脚本读取时，可添加 JSON 输出选项：

```sh
workbuddy --model custom-local:gpt-6.1-sol \
  --tools "" \
  --print --output-format json "请回答：连接测试。"
```

继续最近会话或选择历史会话：

```sh
workbuddy --continue
workbuddy --resume
```

以上选项已从本机 `--help` 核实；单次模型请求示例本轮未执行。开发时让 CLI 先阅读工程 `AGENTS.md`，遵守与 Codex、Claude Code、Cursor 共用的协作约定。

## 4. 与 Codex、知识库的关系

`custom-local:gpt-6.1-sol` 是 WorkBuddy 的模型选项。它的地址名称含 `codex-relay`，但这本身不代表 WorkBuddy 已连接 Codex 桌面会话，也不代表它会读取 Codex 当前聊天。

前面提供的 `yfzks_…` Key 用于「林佳AI知识库」的搜索接口：

```text
POST https://knowledge.operai.com.cn/api/v1/public/knowledge/search
Authorization: Bearer <知识库搜索 Key>
```

它与 `models.json` 中模型服务的 `apiKey` 用途不同。知识库的 Codex 技能和使用方式见 [Codex 外部知识库](knowledge-base.md)。本次没有把该知识库 Key 配到 WorkBuddy，也没有更改其 MCP 设置。

## 5. 排查与迁移

先运行以下只读命令确认入口：

```sh
command -v workbuddy
workbuddy --version
workbuddy --help
```

- 找不到命令：检查 `~/.local/bin` 是否在当前终端的 `PATH` 中。
- 提示 Node 不存在：启动脚本写死了 `v22.19.0` 的安装路径；更新或清理 Node 后，需要核对并修正脚本路径。
- 提示 `.mjs` 不存在：检查 WorkBuddy 是否仍安装在 `/Applications/WorkBuddy.app`，以及应用更新后 CLI 文件位置是否变化。
- 模型选项不存在：核对当前 WorkBuddy 的自定义模型配置和 `--help` 中列出的模型 ID。
- 模型请求认证失败或网络失败：分别检查所选服务的 Key、地址和网络；`--version` 成功仅证明本地 CLI 可启动。

迁移到另一台电脑时，需要安装 WorkBuddy 和 Node，再按新机器的实际位置设置启动入口，并通过对应服务配置凭据。仅复制这份 Markdown 不能恢复连接。

## 6. 本次验证范围

已验证：启动脚本真实存在、目标 Node 和内置 CLI 可以启动、版本为 `2.165.0`、帮助中有本文使用的选项与模型 ID，以及脱敏后的模型连接配置。

未执行：模型推理请求、重新登录、配置修改、后台 HTTP 服务启动、MCP 连接测试或知识库写入。
