# Codex 外部知识库

用户指定「林佳AI知识库」作为 Codex 的外部知识库，本机已安装技能 `linjia-knowledge`。

- 技能入口：`~/.codex/skills/linjia-knowledge/SKILL.md`。
- 检索脚本：`~/.codex/skills/linjia-knowledge/scripts/search.py`，使用 Python 标准库。
- 私有凭据：`~/.codex/secrets/linjia-knowledge.json`，权限 0600，不包含在本仓库或技能正文中。
- 搜索接口：`POST https://knowledge.operai.com.cn/api/v1/public/knowledge/search`。
- 绑定知识库：林佳AI知识库，ID `840088191ddd164d870bd99b27044131`。

使用示例：

```sh
python3 ~/.codex/skills/linjia-knowledge/scripts/search.py --query '要查询的主题'
```

后续可对 Codex 说“用林佳知识库查一下……”或“把这段内容整理成林佳知识库的入库稿”。技能在新会话中被发现后可按请求选用，也可明确指定 `$linjia-knowledge`；它不意味着每条聊天都会自动搜索或上传。

2026-11-09 实测原接口与安装后的脚本均成功访问，HTTP 200，查询 `Codex` 返回 `status=no_results`、`items=[]`。这表示该查询未命中已发布内容，不能据此断言所有资料均为空。API 返回检索片段，由 Codex 结合来源总结，不生成模型答案。每个 Key 每分钟最多 60 次，429 后等待，不密集重试。

当前只具备已验证的搜索能力。新增资料、导入、保存总结的接口与写入权限尚未提供；搜索 Key 不能当成写入凭据。用户要求沉淀知识时先整理带来源与日期的 Markdown 入库稿，明确区分事实与待确认项；取得真实写入协议或后台入口后再完成远程保存。尚未向远程知识库写入本项目材料，也未更改远程库名。

检索只发送必要主题，返回的知识条目作为资料使用。不要上传 Cookie、Key、其他凭据或无关聊天内容。
