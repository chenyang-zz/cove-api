# Boxify API — Go

Boxify 是一个 AI 助手平台的后端服务，提供基于 LLM 的对话、RAG 知识检索、文档处理、记忆管理、Agent 编排、MCP 集成及实时事件流等功能。

## 技术栈

| 层 | 技术 |
|---|---|
| **语言** | Go 1.25 |
| **Web 框架** | Gin |
| **数据库** | PostgreSQL (pgx + GORM) |
| **搜索** | Elasticsearch 8.x |
| **图数据库** | Neo4j 5.x |
| **消息队列** | Redis + asynq |
| **LLM SDK** | Anthropic / OpenAI |
| **认证** | JWT |
| **对象存储** | 腾讯云 COS (本地文件回退) |
| **可观测性** | slog + OpenTelemetry |
| **代码生成** | 自定义 codegen |
| **API 文档** | OpenAPI 3.0 |

## 架构概览

```
cmd/
├── api         # HTTP API 服务器
├── worker      # 后台任务工作进程
├── scheduler   # 定时任务调度器
├── migration   # 数据库迁移
└── codegen     # 代码生成工具
```

### 分层架构

```
transport/http/   →  Gin 路由、中间件、请求/响应 DTO
    ↓
logic/            →  业务逻辑编排
    ↓
repository/       →  数据访问层 (GORM / Neo4j)
    ↓
domain/           →  领域类型与事件
```

各层通过 `internal/svc/` 中的服务上下文（ServiceContext）完成依赖注入。

### 内部包结构

```
internal/
├── config/          # 配置加载
├── core/            # 核心业务逻辑
│   ├── agent/       # Agent 编排
│   ├── id/          # ID 生成
│   ├── jsonx/       # JSON 解析工具
│   ├── llm/         # LLM 抽象层
│   ├── mcp/         # MCP 协议集成
│   ├── memory/      # 记忆管理
│   ├── prompt/      # 通用提示词模板渲染
│   ├── rag/         # RAG 引擎
│   │   ├── chunker/       # 文档分块
│   │   ├── classifier/    # LLM 内容分类/打标签
│   │   ├── documentparse/ # 文档解析
│   │   ├── imagecompress/ # 图片压缩
│   │   ├── imagedescribe/ # LLM 图片描述
│   │   ├── prompt/        # RAG 提示词模板 (embed)
│   │   ├── search/        # 混合检索
│   │   └── webcrawl/      # 网页爬取 (含 SSRF 防护)
│   ├── security/    # 安全
│   └── valuex/      # 通用值转换工具
├── domain/          # 领域类型
├── infrastructure/  # 基础设施适配器
├── logic/           # 业务逻辑层
├── models/          # GORM 模型
├── repository/      # 数据访问层
├── transport/http/  # HTTP 传输层
└── worker/tasks/    # 异步任务处理器
```

## 快速开始

### 前置条件

- Go 1.25+
- Docker & Docker Compose
- (可选) Go 工具链

### 1. 启动依赖服务

```bash
docker compose -f deployments/docker-compose.yml up -d
```

启动以下服务：

| 服务 | 端口 |
|---|---|
| PostgreSQL | 5432 |
| Elasticsearch | 9200 |
| Neo4j | 7474 (HTTP), 7687 (Bolt) |
| Redis | 6379 |

### 2. 配置

复制配置模板并按需修改：

```bash
cp configs/config.yml.example configs/config.yml
```

主要配置项：

- **数据库**: PostgreSQL 连接串
- **Redis**: 会话与队列
- **Elasticsearch**: 向量搜索
- **LLM**: 模型提供者与 API Key
- **JWT**: 签名密钥
- **存储**: COS 或本地文件

### 3. 运行数据库迁移

```bash
make migration
# 或
go run ./cmd/migration
```

### 4. 启动服务

```bash
# 启动 API 服务器 (端口 8000)
make api

# 启动后台工作进程 (另开终端)
make worker

# (可选) 启动定时调度器
go run ./cmd/scheduler
```

## Makefile 命令

| 命令 | 说明 |
|---|---|
| `make api` | 启动 HTTP API 服务器 |
| `make worker` | 启动后台工作进程 |
| `make migration` | 运行数据库迁移 |
| `make gen-route` | 从注解生成路由代码 |
| `make gen-repository MODEL=xxx LABEL=yyy` | 为模型生成类型安全的仓库 |
| `make gen-docs` | 生成 OpenAPI 规范 |

## 代码生成

项目内置代码生成工具 (`cmd/codegen`)，通过扫描 Go 源文件中的注解自动生成：

- **路由注册** — `make gen-route`
- **处理器与逻辑层** — 与路由生成联动
- **Repository** — `make gen-repository MODEL=Conversation LABEL=会话`
- **OpenAPI 规范** — `make gen-docs`

## API 路由

所有路由挂载在 `/api` 路径下，主要领域：

| 领域 | 说明 |
|---|---|
| `/api/health` | 健康检查 (公开) |
| `/api/auth` | 注册/登录 (公开) |
| `/api/models` | 模型配置 |
| `/api/chat` | 对话/流式聊天 |
| `/api/conversations` | 会话管理 |
| `/api/documents` | 文档管理 |
| `/api/knowledge-bases` | 知识库 |
| `/api/agents` | Agent 配置 |
| `/api/agent-personas` | Agent 角色 |
| `/api/mcp-servers` | MCP 服务器管理 |

认证相关路由通过 JWT 中间件保护。

## 异步任务

基于 asynq (Redis) 的任务队列：

| 任务类型 | 说明 |
|---|---|
| `parse:document` | 文档解析与分块 |
| `parse:image` | 图片内容解析 |
| `memory:extract` | 记忆提取 |
| `memory:consolidate` | 记忆整合 (每日定时) |
| `research:run` | 研究任务 |

## RAG 管道

文档处理流程：

1. **爬取** — 网页爬取 (`webcrawl/`)：获取页面 HTML，含重试、重定向追踪、URL 安全校验（防 SSRF）
2. **解析** — 文档解析 (`documentparse/`)：提取结构化内容
3. **图片描述** — 图片理解 (`imagedescribe/`)：通过 LLM 生成图片描述、OCR 文字、物体与场景标签
4. **压缩** — 图片预处理 (`imagecompress/`)：降低图片体积以适配模型输入
5. **分块** — 基于 token 的分层分块 (`chunker/`, child/parent)
6. **嵌入** — 通过 LLM 提供者生成向量
7. **索引** — 写入 Elasticsearch
8. **检索** — 混合搜索 (`search/`, 向量 + 关键词)
9. **重排序** — 结果排序优化
10. **分类** — LLM 自动打标签 (`classifier/`)：为文档/内容生成分类标签，失败时降级不阻断主流程
11. **引用** — 生成带引用的回答

> 所有提示词模板统一通过 `prompt/` 包渲染，业务侧模板嵌入在 `rag/prompt/` 中。

## 项目背景

该项目从 Gin HTTP 框架边界出发，保持 Gin 不出现在 app、domain、repository、infrastructure 层。第一阶段聚焦 API 核心：认证、模型配置、Chat SSE、Agent 工具编排、RAG 边界、记忆提取边界、异步任务与部署存储服务。
