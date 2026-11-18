# 贡献指南

谢谢你愿意参与。动手之前请读 [AGENTS.md](AGENTS.md)：里面是内核的硬规则，对人和对 AI 助手都适用。

## 许可

提交到本仓库的贡献按 [AGPL-3.0](LICENSE) 授权，和项目其他部分一样；提交拉取请求即表示你有权这样授权你提交的内容。
项目名称和图标的使用规则见 [TRADEMARKS.md](TRADEMARKS.md)。

## 开发环境

- Go 1.26+，Node 24+。
- `git config core.hooksPath .githooks` 启用提交前检查。

## 提交之前

```bash
gofmt -l .                 # 应该没有输出
go vet ./...
go test ./...
go run ./tools/hygiene
go run ./tools/i18n        # 多语言有没有漏
cd web && npm run build
```

## 用 AI 编码助手

项目规范只写在 [AGENTS.md](AGENTS.md) 一处。OpenAI Codex 和 DeepSeek Harness 直接读它，Claude Code 通过 [CLAUDE.md](CLAUDE.md) 引入它。
不要为某个工具另写一份规则；个人的本地补充放在 `AGENTS.local.md`、`CLAUDE.local.md` 或 `AGENTS.override.md`，它们不进仓库。

## 版本与发布

- 版本号遵循语义化版本，从 `v0.1.0` 开始。
- 发布前在 [CHANGELOG.md](CHANGELOG.md) 顶部加一节，并把 `web/package.json`、`deploy/` 里的镜像标签和文档里提到的当前版本一起改掉。
- 打一个 `vX.Y.Z` 标签，[发布流程](.github/workflows/release.yml)会构建、签名并推送镜像。
  第一次发布前用 `go run ./tools/relsign gen` 生成一对发布密钥：公钥放进仓库变量 `HOSTOWER_UPDATE_KEY`，私钥放进机密 `HOSTOWER_RELEASE_KEY`。

## 几条约定

- **改判级规则先改测试表**（`internal/kernel/risk/testdata/table.txt`），再改代码。
- **安全内核不接受扩展点**：判级取最高、L3、审批状态机、审计写入不加开关和钩子。
- **仓库里不放真实环境的任何信息**：示例只用保留网段和 `example.com`。
- 一个改动只做一件事；说明里写清楚“为什么”，而不只是“改了什么”。
- 安全问题不要开公开 issue，见 [SECURITY.md](SECURITY.md)。
