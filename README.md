# Cove API — Go

<p align="center">
  <b>Cove 是一个 AI 助手平台后端</b><br/>
  对话、RAG、Agent、记忆、MCP——全部整合在一个 Go 代码库中。
</p>

<p align="center">
  <!-- Badges -->
  <img src="https://img.shields.io/github/go-mod/go-version/chenyang-zz/cove-api?logo=go&logoColor=white&style=flat" alt="Go version" />
  <img src="https://img.shields.io/github/v/release/chenyang-zz/cove-api?style=flat&color=blue" alt="Release" />
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat" alt="License" />
</p>

<p align="center">
  <a href="#%E6%A0%87%E5%BF%97">特性</a> ·
  <a href="#%E6%8A%80%E6%9C%AF%E6%A0%88">技术栈</a> ·
  <a href="#%E5%BF%AB%E9%80%9F%E5%BC%80%E5%A7%8B">快速开始</a> ·
  <a href="#%E6%9E%B6%E6%9E%84">架构</a> ·
  <a href="#rag-%E7%AE%A1%E7%BA%BF">RAG 管线</a> ·
  <a href="#%E9%85%8D%E7%BD%AE">配置</a> ·
  <a href="#%E5%BC%80%E5%8F%91">开发</a> ·
  <a href="https://github.com/chenyang-zz/cove-api/blob/main/docs/architecture.md">文档</a>
</p>

---

## 特性

- **对话** — 基于 SSE 的流式聊天，多轮上下文管理
- **RAG 引擎** — 完整的检索增强生成：抓取、解析、分块、嵌入、检索、排序
- **Agent 编排** — 支持人设与记忆的工具调用 Agent
- **记忆** — 长期记忆的提取、合并与召回
- **MCP 集成** — 通过 Model Context Protocol 连接外部工具
- **实时推送** — 基于 Redis 的事件流
- **文档处理** — 多格式解析：TXT、Markdown、HTML、DOCX、PDF
- **内容分类** — LLM 驱动的自动标签，支持优雅降级

## 技术栈

| 层 | 技术 |
|---|---|
| **语言** | Go 1.25 |
| **HTTP** | Gin（仅传输层，不侵入 domain） |
| **数据库** | PostgreSQL (pgx + GORM) |
| **搜索** | Elasticsearch 8.x（向量 + BM25 混合） |
| **图数据库** | Neo4j 5.x |
| **队列** | Redis + asynq |
| **LLM** | Anthropic / OpenAI |
| **认证** | JWT |
| **存储** | 腾讯云 COS（本地 fallback） |
| **可观测性** | slog + OpenTelemetry |

## 快速开始

### 前置条件

- Go 1.25+
- Docker & Docker Compose

### 1. 启动依赖服务

```bash
docker compose -f deployments/docker-compose.yml up -d
```

| 服务 | 端口 |
|---|---|
| PostgreSQL | 5432 |
| Elasticsearch | 9200 |
| Neo4j | 7474 (HTTP), 7687 (Bolt) |
| Redis | 6379 |

### 2. 配置

```bash
cp configs/config.yml.example configs/config.yml
# 编辑 configs/config.yml，填入 LLM 密钥和连接串
```

### 3. 数据库迁移

```bash
make migration
```

### 4. 运行

```bash
make api       # API 服务 :8000
make worker    # 后台 worker（另开终端）
make scheduler # Cron 任务（可选）
```

## 架构

```
transport/http/    →  Gin 路由、中间件、请求/响应 DTO
    ↓
logic/             →  跨 repository 与 domain 的业务编排
    ↓
repository/        →  数据访问（GORM / Neo4j / Elasticsearch）
    ↓
domain/            →  领域类型、事件、接口
```

横切关注点（LLM、记忆、RAG、MCP、安全）位于 `internal/core/`，通过单一的 `ServiceContext` 注入 — 参见 `internal/svc/context.go`。

### 核心包

```
internal/core/
├── agent/          # Agent 编排与工具调度
├── llm/            # LLM Provider 抽象
├── rag/            # 检索增强生成引擎
│   ├── chunker/        # tiktoken 感知的 parent/child 分块
│   ├── classifier/     # LLM 内容分类
│   ├── documentparse/  # 多格式文本提取
│   ├── imagecompress/  # 模型输入预处理
│   ├── imagedescribe/  # 视觉模型结构化描述
│   ├── prompt/         # RAG 提示词模板（嵌入产物）
│   ├── search/         # 向量 + BM25 混合检索
│   └── webcrawl/       # 网页抓取，含 SSRF 防护
├── memory/         # 长期记忆提取与合并
├── mcp/            # Model Context Protocol 集成
├── prompt/         # 模板渲染（文件系统、记忆、向后兼容 fallback）
└── security/       # JWT、加解密、密钥管理
```

## RAG 管线

Cove 的 11 步入库流水线将原始来源转换为可检索的知识：

```
Source
  │
  ▼
1. Crawl       ──── webcrawl/     抓取，含重试、重定向跟踪、SSRF 防护
  │
  ▼
2. Parse       ──── documentparse/ 从 TXT/MD/HTML/DOCX/PDF 提取文本
  │
  ▼
3. Describe    ──── imagedescribe/ 视觉模型生成描述、OCR、物体、场景
  │
  ▼
4. Compress    ──── imagecompress/ 缩放与重编码，适配模型输入
  │
  ▼
5. Chunk       ──── chunker/       基于 tiktoken 的 parent/child 分块
  │
  ▼
6. Embed       ──── (provider)     通过 LLM Provider 生成稠密向量
  │
  ▼
7. Index       ──── Elasticsearch  Bulk upsert 写入 chunk 索引
  │
  ▼
8. Search      ──── search/        向量 + BM25 混合召回
  │
  ▼
9. Rerank      ──── search/        分数归一化与重排序
  │
  ▼
10. Classify   ──── classifier/    LLM 自动标签（非阻塞）
  │
  ▼
11. Answer     ──── agent/         引用参考，生成回答
```

所有提示词模板位于 `internal/core/rag/prompt/`，由 `internal/core/prompt/` 统一渲染。

## 配置

`configs/config.yml` 关键配置节：

```yaml
database:
  postgres: "postgres://user:pass@localhost:5432/cove"

redis:
  addr: "localhost:6379"

elasticsearch:
  addresses: ["http://localhost:9200"]

rag:
  chunk_index: "cove_chunks"
  embedding_dim: 1024

llm:
  provider: "anthropic"   # 或 "openai"
  api_key: "${LLM_API_KEY}"

jwt:
  secret: "${JWT_SECRET}"

storage:
  driver: "cos"           # 或 "local"
```

## 开发

### 代码生成

Cove 内置代码生成器（`cmd/codegen/`），扫描 Go 注解自动生成：

| 命令 | 产物 |
|---|---|
| `make gen-route` | Gin 路由注册 |
| `make gen-repository MODEL=User LABEL=用户` | 类型安全仓储 |
| `make gen-docs` | OpenAPI 3.0 规范 |

### API 路由

所有路由挂载在 `/api/` 下：

| 路径 | 说明 |
|---|---|
| `/api/health` | 健康检查（公开） |
| `/api/auth` | 注册 / 登录 |
| `/api/models` | 模型配置 |
| `/api/chat` | 流式对话 |
| `/api/conversations` | 会话管理 |
| `/api/documents` | 文档 CRUD |
| `/api/knowledge-bases` | 知识库管理 |
| `/api/agents` | Agent 配置 |
| `/api/mcp-servers` | MCP 服务集成 |

已认证路由受 JWT 中间件保护。

### 异步任务

基于 asynq + Redis 驱动：

| 任务 | 说明 |
|---|---|
| `parse:document` | 文档解析与分块 |
| `parse:image` | 图片内容提取 |
| `memory:extract` | 记忆提取 |
| `memory:consolidate` | 每日记忆合并 |
| `research:run` | 研究任务执行 |

## 项目结构

```
.
├── cmd/                # 入口
│   ├── api/            # HTTP 服务
│   ├── worker/         # 后台处理器
│   ├── scheduler/      # Cron 调度器
│   ├── migration/      # 数据库迁移
│   └── codegen/        # 代码生成工具
├── configs/            # 配置模板
├── deployments/        # Docker Compose
├── db/                 # 迁移脚本 & 查询
├── docs/               # 架构文档 & OpenAPI
├── internal/
│   ├── config/         # 配置加载
│   ├── core/           # 核心业务能力
│   ├── domain/         # 领域类型 & 事件
│   ├── infrastructure/ # 外部适配器
│   ├── logic/          # 业务逻辑层
│   ├── models/         # GORM 模型
│   ├── repository/     # 数据访问
│   ├── svc/            # ServiceContext (DI)
│   └── transport/http/ # HTTP 传输层
├── Makefile
└── README.md
```

---

<p align="center">
  Built with Go · LLM-powered · 欢迎贡献
</p>
