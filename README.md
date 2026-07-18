# Cove

Cove 是一个统一管理桌面端、Web、移动端与后端服务的 monorepo。客户端（Wails 桌面、React/Web、Expose 移动端）通过 HTTP + JWT + SSE 与 Go 后端通信，后端提供认证、实时聊天、RAG 知识库与 MCP 工具集成。

项目托管在 [`chenyang-zz/cove`](https://github.com/chenyang-zz/cove)。原前端仓库 [`chenyang-zz/cove-legacy`](https://github.com/chenyang-zz/cove-legacy) 的完整提交历史已导入 `packages/app/`。

## 特性

- **多端客户端**：Wails v2 桌面端、React 19 + Vite Web 端、Expo SDK 57 React Native 移动端
- **实时聊天**：SSE 流式响应，支持思考链、工具调用、Token 增量渲染
- **AI Agent**：内置 ReAct 循环，支持 MCP 外部工具、RAG 检索增强、知识库
- **认证与会籍**：JWT + Refresh Token 轮换，SecureStore / LocalStorage 会话持久化
- **RAG 知识库**：文档/URL 导入、异步 Worker 处理、Elasticsearch 混合检索（向量 + BM25）
- **数据隔离**：PostgreSQL 多租户隔离，Redis 实时消息与异步队列，Neo4j 长期记忆图
- **E2E 验证**：基于 OrbStack 的本地真实依赖测试 + iOS Simulator 原生交互验证

## 目录结构

```text
cove/
├── packages/
│   ├── app/              # 用户端应用
│   │   ├── frontend/     # React 19 + Vite（Web / Wails 内嵌）
│   │   ├── mobile/       # Expo SDK 57 React Native
│   │   ├── internal/     # Wails 桌面壳、Go 服务绑定
│   │   └── build/        # 桌面端构建产物与脚本
│   └── server/           # Go 后端
│       ├── cmd/          # api / worker / scheduler / migration / codegen
│       ├── internal/     # transport / logic / domain / core / repository / infrastructure
│       └── docs/         # OpenAPI 契约与生成文档
├── e2e/                  # 跨前后端本地真实依赖测试编排
├── .githooks/            # pre-push 本地 CI 校验
├── .codex/rules/         # 架构与开发边界规则
└── Makefile              # 统一开发入口
```

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 桌面端 | Wails v2, Go, TypeScript |
| Web / 内嵌 | React 19, Vite 8, Vitest, Playwright |
| 移动端 | Expo SDK 57, React Native 0.86, Expo Router, SecureStore |
| 后端 | Go, Gin, GORM, asynq |
| 持久化 | PostgreSQL, Redis, Elasticsearch, Neo4j |
| 传输 | HTTP JSON, JWT, Server-Sent Events |
| 测试 | Go testing, Vitest, Playwright, iOS Simulator (Computer Use) |

## 快速开始

### 环境要求

- Go 1.22+
- Node.js 20+ 与 pnpm 10+
- OrbStack（本地依赖运行时，替代 Docker Desktop）
- Xcode（仅移动端 / iOS Simulator 验证）
- Task（app 包构建编排）

### 安装

```bash
# 克隆
git clone https://github.com/chenyang-zz/cove.git
cd cove

# 前端依赖
pnpm --dir packages/app/frontend install
pnpm --dir packages/app/mobile install

# 激活 git hooks（push 前自动跑本地 CI）
make install-hooks
```

### 常用命令

```bash
# 查看所有可用目标
make help

# --- 后端 ---
make api             # 启动 API 服务
make worker          # 启动后台 Worker
make gateway         # 启动 Gateway
make migration       # 执行数据库迁移
make docs            # 校验 route / repository / OpenAPI / prompt 生成产物同步

# --- 前端 ---
make app-frontend-test      # Web 端测试
make app-mobile-lint        # 移动端 lint
make app-mobile-typecheck   # 移动端类型检查
make app-mobile-test        # 移动端测试

# --- 真实依赖 / E2E ---
make server-db-smoke        # 后端真实数据库冒烟（PostgreSQL + Redis + ES）
make e2e-smoke              # 辅助 React/Vite 浏览器冒烟
make e2e-up                 # 启动可丢弃本地依赖栈
make e2e-down               # 清理本地依赖栈
```

前端和后端保留各自的依赖清单与构建方式。Git 命令与共享 Make 目标从仓库根目录运行；直接的 Go、pnpm 或 Task 命令从对应 package 目录运行。

## 开发指南

### 后端开发

后端代码位于 `packages/server/`，基于 go-zero 风格的分层架构：

- `cmd/` 定义进程入口（api / worker / scheduler / migration / codegen）
- `internal/transport/http/` 负责 Gin 路由、中间件、响应封装
- `internal/logic/` 编排认证、聊天、会话、文档、模型配置等用例
- `internal/domain/flow/` 承载聊天 Agent 等工作流
- `internal/core/` 提供 LLM、RAG、MCP、记忆等可复用引擎
- `internal/repository/` 定义持久化接口与各存储实现
- `internal/infrastructure/` 适配 PostgreSQL / Redis / ES / Neo4j / 对象存储 / LLM 提供者

依赖方向必须由外向内：core 包不得引用 HTTP handler 或具体数据库适配器。`internal/svc/ServiceContext` 负责组装所有基础设施客户端。

API 契约以 `packages/server/docs/openapi.json` 为准；新增或修改接口后运行 `make docs` 校验生成产物同步。

### 前端开发

Web 端位于 `packages/app/frontend/`：

- 会话存储在 localStorage，key 为 `cove.auth.session.v1`
- 通过 `VITE_API_BASE_URL` 覆盖 API 地址，开发默认 `http://localhost:8000`
- `features/auth/api.ts` 处理 JSON envelope、Bearer Token、401 时最多一次 refresh 重试
- `features/chat/api.ts` 处理会话请求与 `/api/chat/stream` 的 SSE 增量解析

移动端位于 `packages/app/mobile/`（当前主力前端产品）：

- 基于 Expo Router / native stack，受保护的 `Stack.Protected` 认证边界
- `AuthProvider` 管理认证状态，页面不应持有会话全局状态
- `core/session.ts` 使用 Expo SecureStore 持久化 Token（key 同 `cove.auth.session.v1`）
- 通过 `EXPO_PUBLIC_API_BASE_URL` 覆盖 API 地址；`EXPO_ALLOW_INSECURE_HTTP=true` 为开发专用

两个客户端的 UI、导航、存储、传输实现各自独立，不得共享浏览器专属实现。修改服务端契约时，须同时审计 `frontend/src/features/**/api.ts` 与 `mobile/src/core/*.ts`。

### 端到端验证

Cove 强调基于真实依赖的端到端验证，不使用 mock 数据库或 `httptest` 充当完成证明：

- **后端真实数据库冒烟**：`make server-db-smoke` 在 OrbStack 中启动 PostgreSQL、Redis、Elasticsearch，运行真实迁移与 API，验证会话/消息持久化、跨用户隔离、密码轮换
- **辅助浏览器冒烟**：`make e2e-smoke` 通过 Playwright 验证 React/Vite 端会话持久化与 Token 刷新
- **原生 App 验证**：通过 iOS Simulator + Computer Use 完成，遵循 `.agents/skills/ios-simulator/SKILL.md`，并由 `e2e/REAL_DATABASE_COVERAGE.md` 记录增量覆盖

E2E 环境始终使用独立的 Compose project、空闲本地端口与合成用户，默认端口 PostgreSQL `55432`、Redis `56379`、Elasticsearch `59200`，可通过 `E2E_*_PORT` 覆盖。

## 文档

| 文档 | 说明 |
| --- | --- |
| [.codex/rules/architecture.md](.codex/rules/architecture.md) | 架构与开发边界 |
| [.codex/rules/frontend.md](.codex/rules/frontend.md) | 前端开发规则 |
| [.codex/rules/backend.md](.codex/rules/backend.md) | 后端开发规则 |
| [.codex/rules/e2e-testing.md](.codex/rules/e2e-testing.md) | E2E 测试规则 |
| [e2e/README.md](e2e/README.md) | E2E 测试编排说明 |
| [e2e/REAL_DATABASE_COVERAGE.md](e2e/REAL_DATABASE_COVERAGE.md) | 真实数据库覆盖台账 |
| [packages/server/docs/openapi.json](packages/server/docs/openapi.json) | 后端 OpenAPI 契约 |
| [CONTRIBUTING.md](CONTRIBUTING.md) | 贡献指南 |
| [SECURITY.md](SECURITY.md) | 安全披露流程 |
| [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) | 行为准则 |

## 贡献

欢迎提交 Issue 与 Pull Request。请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 了解分支策略与提交规范，并确认 pre-push 校验（`make install-hooks`）通过后再推送。

## 许可证

[LICENSE](LICENSE)
