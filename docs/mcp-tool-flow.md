# MCP 工具注册、兜底等待与缓存流转

本文描述 MCP（Model Context Protocol）工具从用户配置、服务端发现、缓存管理到 Chat
编排器全量复用的数据流，以及远端不可用时的兜底与等待策略。

---

## 1. 整体数据流概览

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  用户配置层   │     │  服务发现层   │     │  编排消费层   │
│              │     │              │     │              │
│ MCP Server   │────▶│  core/mcp    │────▶│ orchestrator │
│ Tool Config  │     │  Service     │     │  Run()       │
└──────────────┘     └──────────────┘     └──────────────┘
       │                    │                    │
       ▼                    ▼                    ▼
  mcp_servers          MemoryToolCache      OpenedTools
  tool_configs         (TTL 5 min)          (lazy session)
```

三层职责分离：
- **用户配置层**：用户添加 MCP Server、切换工具开关，落库 `mcp_servers` / `tool_configs`。
- **服务发现层**：`core/mcp.Service` 负责缓存命中判断、远端拉取、连接复用。
- **编排消费层**：`chat.Orchestrator` 在每次对话中把 MCP 工具注册到 agent registry。

---

## 2. 用户配置层 —— 数据落库与稳定 Key 生成

### 2.1 MCP Server 注册

用户在 `mcp_servers` 表存储远端服务地址与加密后的认证信息：

| 字段 | 用途 |
|------|------|
| `id` | UUID 主键 |
| `transport` | `sse` / `streamable_http` |
| `url` | 远端端点 |
| `auth_type` | `none` / `bearer` / `api_key` |
| `auth_config` | Fernet 加密的 token / key |
| `enabled` | server 级启停开关 |
| `tools_cache` | 上次同步下来的 `[{name, description}]` 快照 |

`tools_cache` 是兜底的核心数据：即使远端临时不可用，前端仍可从这份快照展示工具列表。

### 2.2 Tool Key 生成

每个归属于 MCP Server 的工具会被分配一个全局唯一、稳定的 key，由
`domain/mcp.ToolKey(serverID, rawName)` 生成：

```
mcp_{serverUUID(无横杠)}_{sanitizedName}_{sha256前4字节hex}
```

关键性质：
- **包含完整 server UUID** → 不同 server 的同名工具不会冲突。
- **server 重命名不影响 key** → key 不携带 server name。
- **长度控制在 64 字符以内** → 超过 17 字符的远端名称会被截断，由 hash 兜底唯一性。

### 2.3 工具启停

用户对工具的启用/禁用记录在 `tool_configs` 表：

| 字段 | 用途 |
|------|------|
| `tool_key` | 与 `ToolKey()` 返回值对应 |
| `tool_type` | `builtin` / `mcp` |
| `enabled` | 用户显式配置的状态 |

**默认行为**：未持久化配置的内置工具默认启用，只覆盖用户显式保存过的状态。

---

## 3. 服务发现层 —— `core/mcp.Service`

### 3.1 核心接口

```go
type ToolCache interface {
    Get(ctx, key) (CacheEntry, bool, error)
    Set(ctx, key, entry) error
    Valid(server, entry) bool   // fingerprint + 未过期
}

type ToolSession interface {
    ListTools(ctx) ([]ToolInfo, error)
    CallTool(ctx, name, input) (*CallResult, error)
    Close() error
}

type SessionOpener interface {
    OpenSession(ctx, server) (ToolSession, error)
}
```

`Service` 通过组合以上三个接口完成解耦，便于测试替换与运行时策略切换。

### 3.2 工具列表缓存流

`BuildToolList()` 是主入口，实现 **缓存优先 + 同步刷新**：

```
BuildToolList(ctx, server)
    │
    ├─ cache.Get(server.ID)
    │       │
    │       ├─ 命中 && Valid(fingerprint 匹配 && 未过期)
    │       │       └─ 返回克隆的 tools        ◀── 快速路径
    │       │
    │       └─ 未命中 / fingerprint 变化 / 过期
    │               └─ RefreshToolList()
    │                       │
    │                       ├─ client.ListTools()    ◀── 临时 session，拉完即关闭
    │                       ├─ cache.Set(tools, ttl)
    │                       └─ 返回 tools
    │
    └─ 返回 []ToolInfo
```

**Fingerprint 决定缓存有效性**：

```go
func Fingerprint(server) string {
    // sha256(ID + Transport + URL + AuthType + sorted(AuthConfig))
}
```

任何影响远端工具集合的配置变更（URL、传输方式、认证信息）都会使 fingerprint 变化，
立即失效旧缓存，防止配置更新后仍展示过期工具列表。

### 3.3 `Valid()` 判定

```go
func (c *MemoryToolCache) Valid(server, entry) bool {
    return entry.Fingerprint == Fingerprint(server) &&
           now().Before(entry.ExpiresAt)
}
```

TTL 默认 5 分钟（`DefaultTTL`），过期后下一次调用触发远端刷新。

### 3.4 发现超时与失败冷却

对话链路中 `toolRegistry` 会同步调用 `OpenTools`。若无独立超时，网络 hang 常会落到操作系统级
约 60s 连接超时；且失败不缓存时，下一轮对话仍会再等一轮。

核心层默认保护：

| 机制 | 默认 | 作用 |
|------|------|------|
| `DiscoverTimeout` | 5s | Connect + ListTools 上界 |
| `FailCooldown` | 30s | 失败后跳过远端探测 |
| stale-if-error | — | 有指纹匹配的 runtime 工具列表时降级复用（可已过 TTL） |

```
OpenTools / BuildToolList
    │
    ├─ valid cache → 立即返回
    ├─ fail cooldown?
    │     ├─ stale tools → lazy / 列表降级返回（不打远端）
    │     └─ 无 stale → 立即 error
    └─ discover(ctx, DiscoverTimeout)
          ├─ 成功 → Set cache、清冷却
          └─ 失败 → 记冷却；有 stale 则降级，否则 error
```

`RefreshToolList`（手动同步）**忽略冷却**强制探测，但仍受 `DiscoverTimeout` 约束。
`CallTool` 不受发现超时限制。

### 3.5 延迟连接 `OpenTools()`

`OpenTools()` 在缓存命中时实现 **lazy session**：

```
OpenTools(ctx, server)
    │
    ├─ cache.Get(server.ID)
    │       │
    │       ├─ 命中 && Valid
    │       │       └─ newOpenedTools(tools, opener, session=nil)  ◀── 延迟建立连接
    │       │
    │       └─ 未命中 / 过期
    │               ├─ fail cooldown + stale? → lazy stale tools
    │               ├─ opener.OpenSession(discoverCtx)   ◀── DiscoverTimeout
    │               ├─ session.ListTools(discoverCtx)
    │               ├─ 失败 → 记冷却；可 stale-if-error
    │               ├─ cache.Set(...)
    │               └─ newOpenedTools(tools, opener, session)
    │
    └─ 返回 *OpenedTools
```

**为什么要延迟？** Chat 编排器可能注册了几十种工具，但单轮对话只会调用其中几个。
如果每次注册都建立 TCP 连接，会造成大量空闲连接浪费。`OpenedTools.CallTool()`
在首次调用时才真正建立 session：

```go
func (o *OpenedTools) CallTool(ctx, name, input) {
    if o.session == nil {                    // 首次调用
        session, err := o.opener.OpenSession(ctx, o.server)
        o.session = session
    }
    return o.session.CallTool(ctx, name, input)
}
```

### 3.6 并发安全

- `OpenedTools.CallTool()` 使用 `sync.Mutex` 串行化，保证同一 session 的请求顺序。
- `MemoryToolCache` 使用 `sync.RWMutex`，读多写少场景友好。
- 失败冷却 map 由 `Service` 内部 mutex 保护。
- `cloneTools()` 每次返回深拷贝，防止调用方修改污染缓存条目。

---

## 4. 兜底等待策略 —— 远端不可用时的降级路径

### 4.1 三层降级

当 `liveMCPDefinitions()` 失败时，`loadMCPToolGroup()` 执行分级降级：

```
liveMCPDefinitions()       尝试实时拉取
    │
    ├─ 成功
    │       └─ CacheFresh + 工具定义
    │
    └─ 失败
            │
            ├─ mcpSnapshotGroup()
            │       │
            │       ├─ ToolsCache 非空  → CacheStale  (旧快照可用)
            │       └─ ToolsCache 空    → CacheEmpty  (无任何定义)
            │
            └─ 前端根据 state 展示不同的 UI 提示
```

`fallbackCacheState()` 根据 `tools_cache` 是否有数据决定 `CacheStale` 还是
`CacheEmpty`，并把错误信息通过 `cacheErr` 字段返回前端用于提示。

### 4.2 配置页加载 —— `mcpToolGroups()`

HTTP 接口 `GET /tool-configs` 的核心逻辑：

```
ListToolConfigs(userID)
    │
    ├─ builtinToolResponses()                  ◀── 内置工具，始终可用
    │
    ├─ toolEnabledMap(tool_configs)            ◀── 用户显式配置
    │
    └─ mcpToolGroups(servers, enabledByKey)
            │
            ├─ server.Enabled == false
            │       └─ mcpSnapshotGroup(CacheDisabled)   ◀── 仅展示快照
            │
            └─ server.Enabled == true
                    │
                    ├─ 并发拉取 (semaphore=4)
                    │       ├─ loadMCPToolGroup()        ◀── 见 4.1
                    │       └─ ctx.Done()
                    │               └─ 超时兜底快照
                    │
                    └─ 排序后返回
```

并发控制：最多 4 个 MCP 远端同时刷新（`mcpRefreshConcurrencyMax = 4`），
防止用户配置大量 server 时打满连接数。`ctx.Done()` 场景下也会降级到快照，
避免单个慢 server 阻塞整体接口。

### 4.3 缓存状态映射

前端根据 `CacheState` 字段展示不同提示：

| State | 含义 | 数据来源 |
|-------|------|---------|
| `fresh` | 实时或缓存命中成功 | 远端拉取 / 内存缓存 |
| `stale` | 远端刷新失败，展示快照 | `tools_cache` |
| `disabled` | server 被禁用 | `tools_cache` |
| `empty` | 无可用数据 | — |

---

## 5. 编排消费层 —— Chat 会话中的注册与调用

### 5.1 `Orchestrator.toolRegistry()`

每次 Chat 请求都会重新构建工具 registry：

```
toolRegistry(ctx, userID)
    │
    ├─ 1. 构建内置工具 registry (system / knowledge)
    │
    ├─ 2. 应用用户启停配置
    │       enabled[key] = row.Enabled (只覆盖用户显式配置过的)
    │
    ├─ 3. 过滤 registry：只保留 enabled 的工具
    │
    └─ 4. MCP servers（有限并行发现 + 串行注册）
            │
            ├─ 收集 enabled servers + ServerConfig.decrypt
            │
            ├─ Phase 1 并行发现
            │     mcpCtx = WithTimeout(ctx, assembleBudget=8s)
            │     concurrency semaphore = 4
            │     go OpenTools(mcpCtx, serverConfig) per server
            │           ├─ 缓存命中 → lazy session
            │           └─ 缓存失效 → DiscoverTimeout 内建连+ListTools
            │
            └─ Phase 2 串行注册（稳定顺序，无并发写 registry）
                  ├─ OpenTools 失败 → warn + skip
                  ├─ Definitions + 工具级启停过滤
                  ├─ NewTool → filtered.Register
                  └─ Register 硬错误 → closeAll + return error
```

**组装预算**：多 server 发现墙钟与并发度由 `configs.mcp` 控制（默认
`assemble_budget=8s`、`assemble_concurrency=4`）；单 server 仍受
`tools_cache_ttl` / `discover_timeout` / `fail_cooldown` 约束。可在
`configs/config.yml` 或环境变量 `MCP_*` 中覆盖。

**连接管理**：所有成功的 `OpenedTools` lease 在 Wait 后统一收集，通过 `closeAll`
闭包在 `generate()` 结束时关闭（defer）；Register 失败路径也会释放本轮全部 lease。

### 5.2 MCP 工具调用的完整路径

```
Agent 决策调用 "mcp_xxx_search"
    │
    ▼
Runner.Invoke(ctx, name, input)
    │
    ├─ registry.Lookup(name)
    │
    ▼
coretool.FuncTool.Execute(ctx, input)
    │
    ▼
opened.CallTool(ctx, rawName, input)       ◀── 首次调用时 lazy OpenSession
    │
    ├─ opener.OpenSession(ctx, serverConfig)
    │       ├─ authHTTPClient(server)       ◀── 注入 bearer/api_key headers
    │       ├─ transport = SSE | StreamableHTTP
    │       └─ client.Connect(ctx, transport)
    │
    ├─ session.CallTool(ctx, name, args)
    │       └─ 序列化 Content → []Content{Type, Text, Data, Raw}
    │
    ▼
CallResult → coretool.Output{Text, Parts, Metadata}
    │
    ├─ IsError=true
    │       └─ NewTool 返回 (output, error)
    │               └─ Runner.handleError(output, error)
    │                       ├─ ErrorAsOutput=false → 直接返回 error
    │                       └─ ErrorAsOutput=true  → 保留原始 Parts/Metadata
    │                                                Text 规范为 "tool invocation failed:\n..."
    │                                                Metadata["error"] 写入错误信息
    │                                                ◀── 供模型观察纠正
    │
    └─ StructuredContent → metadata["structured_content"]
```

**错误处理策略**：

`Runner.handleError()` 现在接收完整的 `Output` 与 `err` 两个参数。当工具同时返回
部分结果和错误时（例如 transport 层超时但已收到部分 streaming 响应），会保留原始
`Parts`、`Metadata`，仅将 `Text` 规范为失败 observation 文本。这避免了错误路径下
丢失已返回的内容信息。

### 5.3 认证处理

`SDKToolClient` 根据 `AuthType` 自动适配传输层：

- `none`：直接使用默认 HTTP client。
- `bearer`：注入 `Authorization: Bearer <token>` header。
- `api_key` header 模式：注入指定 header（默认 `X-Api-Key`）。
- `api_key` query 模式：把 key 写入 URL query param（`?key=xxx`）。

所有认证敏感字段在数据库中以 Fernet 加密存储，仅在 `ServerConfig()` 转换时解密。

---

## 6. 数据流全景时序

```
配置阶段:
  用户 ──▶ mcp_servers (加密 auth_config, tools_cache)
  用户 ──▶ tool_configs (tool_key, enabled)

HTTP 查询阶段 (GET /tool-configs):
  ListToolConfigsLogic
      ├─ ToolConfigRepo.List()          ──▶ enabledByKey
      ├─ MCPServerRepo.List()           ──▶ servers[]
      └─ mcpToolGroups()
              ├─ snapshot (server.Enabled=false)
              └─ live refresh (Enabled=true, semaphore=4)
                      ├─ BuildToolList ─▶ cache ─▶ 远端 ListTools
                      └─ fallback ─▶ tools_cache snapshot

Chat 编排阶段 (Orchestrator.generate):
  toolRegistry()
      ├─ builtin tools ─▶ enabled map
      ├─ MCP Phase1 并行 OpenTools(mcpCtx, budget=8s, concurrency=4)
      ├─ MCP Phase2 串行 Definitions + Register
      └─ defer closeAll()

Chat 调用阶段 (Agent → MCP):
  Runner.Invoke(ctx, name, input)
      ├─ registry.Lookup(name)
      ├─ FuncTool.Execute → opened.CallTool()
      │       ├─ [lazy] OpenSession ─▶ auth ─▶ transport ─▶ Connect
      │       └─ session.CallTool ─▶ CallResult → coretool.Output
      ├─ IsError=true → (output, error)
      │       └─ Runner.handleError(output, error)
      │               ├─ ErrorAsOutput=false → 直接返回 error
      │               └─ ErrorAsOutput=true  → 保留 Parts/Metadata，Text 规范为失败 observation
      └─ 正常 → 返回 Output
```

---

## 7. 设计要点总结

| 关注点 | 实现策略 |
|--------|---------|
| **稳定性** | ToolKey 包含 server UUID 与 hash，重命名 / 同名工具不冲突 |
| **可用性** | 三层降级：实时 → 快照(stale) → 空(empty)，远端抖动不影响前端展示 |
| **性能** | 内存缓存 TTL 5 分钟；发现超时 5s + 失败冷却 30s；对话组装有限并行（concurrency=4）+ 墙钟预算 8s |
| **安全** | auth_config Fernet 加密存储，仅在内存中解密使用 |
| **一致性** | Fingerprint 包含全部连接配置，任何变更立即失效缓存 |
| **资源释放** | OpenedTools.Close() 幂等，编排器 defer closeAll() 兜底 |
| **可测试** | Service 通过 Options 注入 Client / SessionOpener / Cache |
| **错误处理** | Runner.handleError 接收完整 Output+err，保留部分结果避免内容丢失 |
