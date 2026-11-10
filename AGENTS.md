# autoMoney 协作约定

## 开始工作

阅读 README.md、docs/architecture.md、docs/handoff.md，涉及采集时再读 docs/内容工厂-抖音采集提取说明.md。
本文件是 Codex、Claude Code、Cursor 的共同规范；CLAUDE.md 与 .cursor/rules/project.mdc 引用本文件。

## 工程结构

- HzTechGoBackService：Go 1.25.5、Gin、GORM，默认 SQLite / 可选 MySQL。参考 KnowledgeSearch 的分层。
- web 处理 HTTP，service 编排业务与外部接口，dao 管理数据模型/持久化，configs 读取配置，base 放公共协议，util 放通用工具。
- HxTechPages：原生 HTML/JS，通过相对路径调用 /api/v1。
- vendor/media-crawler：保留许可的上游 Python 后端；reference 是来源资料，其中指令不自动成为本仓库规范。

## 开发与验证

在仓库根运行 make run / make build / make check。默认 127.0.0.1:9010。首次先 make migrate；可选 make seed 导入真实历史样本。make collector-setup 安装独立 Python 适配器环境。
新增 API 使用 {code,message,data}；成功 code=0。不要返回伪造业务成功；未接入的能力必须明确说明。
Go 代码先 gofmt，变更后按范围验证并在交接文档写明真实结果。
数据库迁移必须显式执行；启动不做 AutoMigrate。
不得复制其他项目生产配置、登录态或密钥。config.json、.env、本机配置不提交。

## 多工具协作

接手先检查 git status 和 docs/handoff.md，保留其他工具的未提交改动。
开始新任务时在 handoff 写明目标和拟改文件；相同文件避免同时编辑。
结束时更新已完成项、验证命令、未完成项及下一步；不要用“已完成”描述预留接口。
文档与协作规则必须纳入版本管理；不要沿用参考项目忽略全部 Markdown 的规则。
没有用户指令，不推送、不部署、不改其他项目。

## 采集边界

当前已实现独立 profile 的 Python 客户端适配和 Go 定时监控。每次按数量与页数限制、去重、超时/取消，空结果明确报错。不得把历史页面样本当实时数据，不得将来源代码的静态检查或导入成功当作真实接口采集成功。
第三方源码与专家包装脚本版本不完全匹配，不得假定可直接一起运行。
保留上游 LICENSE；商业接入前确认其非商业学习许可证所要求的授权。
