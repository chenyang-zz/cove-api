# 贡献指南

感谢你对 Cove 的兴趣！无论你是在修 bug、加功能、改进文档还是分享想法，这份指南都能帮助你快速上手。

## 行为准则

参与本项目即表示你已阅读并同意我们的 [行为准则](CODE_OF_CONDUCT.md)。请保持友善、尊重、专业。

## 如何开始

1. Fork 本仓库并 clone 到本地
2. 创建特性分支：`git checkout -b feat/your-feature`
3. 开发与测试
4. 提交并推送到你的 fork
5. 开启 Pull Request 到 `main` 分支

## 开发环境搭建

Git 与 `make` 命令从 Cove monorepo 根目录运行；直接 Go、pnpm 和 Task 命令从对应 package 目录运行。

```bash
# 1. 启动依赖服务
docker compose -f packages/server/deployments/docker-compose.yml up -d

# 2. 复制并编辑配置
cp packages/server/configs/config.yml.example packages/server/configs/config.yml

# 3. 运行迁移
make migration

# 4. 启动服务（另开终端启动 worker）
make api
make worker
```

依赖服务端口：PostgreSQL 5432、Elasticsearch 9200、Neo4j 7474/7687、Redis 6379。

## 分支命名

| 前缀 | 用途 | 示例 |
|---|---|---|
| `feat/` | 新功能 | `feat/memory-search` |
| `fix/` | 修复 bug | `fix/sse-reconnect` |
| `refactor/` | 重构 | `refactor/llm-provider` |
| `docs/` | 文档 | `docs/contributing` |
| `chore/` | 工具/依赖 | `chore/go-1.25` |
| `test/` | 测试 | `test/rag-pipeline` |

## 提交规范

本项目遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范，并使用中文描述：

```
<图标> <类型>(<范围>): <简短描述>

<详细描述（可选）>
```

类型与图标：

| 类型 | 图标 | 说明 |
|---|---|---|
| `feat` | ✨ | 新功能 |
| `fix` | 🐛 | 修复 bug |
| `refactor` | ♻️ | 重构（不改变外部行为） |
| `docs` | 📝 | 文档更新 |
| `test` | ✅ | 测试相关 |
| `chore` | 🔧 | 构建/工具链/依赖 |
| `perf` | ⚡ | 性能优化 |

示例：

```
✨ feat(agent): 添加 ReAct 文本推理双路径

- 实现基于文本的 ReAct 循环，作为 function calling 的 fallback
- 新增 Thought/Action/Observation 解析器
- 添加 max_iterations 保护防止无限循环
```

## 发版（Release Tag）

生产发版由语义化版本 tag 驱动（触发 CD），格式固定为 `vX.Y.Z`（例如 `v1.2.3`）。

**硬性规则：必须先合并到 `main`，再打 tag 发布。** 不允许从 `dev` 或特性分支直接推送发版 tag。

正确流程：

```bash
# 1. 变更已通过 PR 合入 main
git checkout main
git pull origin main

# 2. 在 main 上的目标提交打 tag
git tag vX.Y.Z

# 3. 推送 tag（触发 CD）
git push origin vX.Y.Z
```

校验位置：

| 层级 | 行为 |
|---|---|
| 本地 `.githooks/pre-push` | 推送 `vX.Y.Z` 时检查 tag 提交是否已在 `main` 历史中；`GIT_PUSH_VERIFY=false` **不能**跳过此规则 |
| GitHub Actions CD `release-guard` | 远端再次校验；未合入 `main` 的 tag 直接失败，不构建、不部署 |

本地紧急绕过（仅调试用，CD 仍会拒绝）：`GIT_PUSH_ALLOW_OFF_MAIN_TAG=true git push origin vX.Y.Z`。

## Pull Request 清单

在开启 PR 前，请确认：

- [ ] 代码通过 `go vet ./...` 与 `gofmt` 检查
- [ ] 新增功能附带单元测试（测试函数上方附中文注释说明验证点）
- [ ] 后端测试通过：在 `packages/server/` 运行 `go test ./...`
- [ ] 受影响的 Web、桌面端和移动端检查通过
- [ ] 代码生成产物已提交（如 `make gen-route`、`make gen-repository`、`make gen-docs` 的输出）
- [ ] PR 描述清楚说明变更动机、方案与影响范围
- [ ] 关联相关 Issue（如 `Closes #123`）

## 代码风格

- **Go 风格**：遵循标准 Go 约定，优先使用指针；导出的标识符须有中文 doc 注释
- **测试注释**：每个测试函数上方须有中文注释说明验证点
- **Generated code**：生成的代码关键步骤须有中文注释解释逻辑
- **包设计**：新 `internal/core/` 包参照 `internal/core/rag/search` 模板，拆出 `types.go`、`options.go`、实现文件与测试文件

## 报告 Bug / 请求功能

请使用 [Issue 模板](.github/ISSUE_TEMPLATE) 提交。Bug 报告请尽量包含：

- 复现步骤
- 期望行为与实际行为
- 环境信息（Go 版本、操作系统）
- 相关日志或截图

## 安全漏洞

请查阅 [安全政策](SECURITY.md) 了解如何负责任地披露安全问题。

---

如有疑问，欢迎在 Discussion 中提问或直接联系维护者。
