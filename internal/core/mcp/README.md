# Package mcp — MCP 工具发现、缓存与会话管理

`internal/core/mcp` 提供与 MCP（Model Context Protocol）服务发现、工具缓存和工具调用相关的通用核心能力。该包供业务层（如 agent、chat 编排）复用，本身不承载任何业务过滤或路由逻辑。

## 核心概念

```
┌──────────────┐     ┌──────────────┐     ┌─────────────────┐
│ ToolClient   │     │ Service      │     │ OpenedTools     │
│ (ListTools)  │────▶│ (缓存+编排)   │────▶│ (lease + 调用)  │
└──────────────┘     └──────┬───────┘     └─────────────────┘
                            │
                     ┌──────▼───────┐
                     │ ToolCache    │
                     │ (指纹+TTL)   │
                     └──────────────┘
```

### 数据流

1. **发现**：`Service.BuildToolList` / `RefreshToolList` 通过 `ToolClient` 拉取远端工具列表，写入 `ToolCache`。
2. **缓存**：每条缓存以 server ID 为 key，`Fingerprint(server)` 检测配置变更，`TTL`（默认 5 分钟）控制过期。
3. **调用**：`Service.OpenTools` 返回 `OpenedTools` lease；有效缓存直接复用，延迟到首次 `CallTool` 才建立连接。

## 主要类型

### `Service`

包入口。组合 `ToolClient`、`SessionOpener`、`ToolCache` 三类依赖：

```go
mcpService := mcp.NewService(
    mcp.WithClient(customClient),       // 可选，默认 SDKToolClient
    mcp.WithSessionOpener(customOpener),// 可选，默认复用 client
    mcp.WithCache(customCache),         // 可选，默认 MemoryToolCache
    mcp.WithTTL(10 * time.Minute),
    mcp.WithDiscoverTimeout(5 * time.Second), // 可选，发现路径超时
    mcp.WithFailCooldown(30 * time.Second),    // 可选，失败冷却
)
// 生产默认：mcp.NewService()
// 业务启动时也可从 configs.mcp 解析后注入 WithTTL / WithDiscoverTimeout / WithFailCooldown。
```

| 方法 | 说明 |
|------|------|
| `BuildToolList(ctx, server)` | 返回有效缓存或同步刷新远端工具列表；遵守发现超时与失败冷却 |
| `RefreshToolList(ctx, server)` | 跳过缓存与冷却，强制刷新并更新运行时缓存（仍受发现超时约束） |
| `OpenTools(ctx, server)` | 返回 `OpenedTools` lease（延迟连接）；失败时可 stale-if-error |
| `CacheStatus(ctx, server)` | 查询当前缓存是否有效及过期时间 |

### `ToolInfo` / `ToolMeta`

`ToolInfo` 是完整的工具元数据（名称、描述、Schema、注解等），`ToolMeta` 是仅包含 `Name` + `Description` 的轻量子集，用于仅需名称/描述的场景。

### `Content` / `CallResult`

`Content` 标准化了 MCP 工具返回的内容片段（text / image / audio / resource），`Raw` 保留原始 JSON 供调用方处理扩展类型。`CallResult` 封装一次工具调用的全部输出，`IsError` 区分业务错误与协议错误。

### `ServerConfig`

描述一个 MCP 服务的连接与配置信息，包含传输协议（SSE / StreamableHTTP）、URL、认证类型与配置。

### `CacheEntry` / `CacheStatus`

`CacheEntry` 保存一组工具列表及其指纹和过期时间；`CacheStatus` 是 `CacheStatus` 方法的返回值，描述当前缓存状态。

## 接口

### `ToolClient`

不复用 session 的工具发现接口。适合轻量场景（如仅获取工具目录）。

```go
type ToolClient interface {
    ListTools(ctx context.Context, server ServerConfig) ([]ToolInfo, error)
}
```

### `ToolSession`

一次可复用的 MCP 连接。同一个 session 的调用由 `OpenedTools` 串行化。

```go
type ToolSession interface {
    ListTools(ctx context.Context) ([]ToolInfo, error)
    CallTool(ctx context.Context, name string, input map[string]any) (*CallResult, error)
    Close() error
}
```

### `SessionOpener`

创建指定 MCP server 的可复用连接。

```go
type SessionOpener interface {
    OpenSession(ctx context.Context, server ServerConfig) (ToolSession, error)
}
```

### `ToolCache`

运行时工具缓存接口。默认实现 `MemoryToolCache` 使用内存 map，可替换为分布式缓存。

```go
type ToolCache interface {
    Get(ctx context.Context, key string) (CacheEntry, bool, error)
    Set(ctx context.Context, key string, entry CacheEntry) error
    Valid(server ServerConfig, entry CacheEntry) bool
}
```

## 默认实现

### `SDKToolClient`

基于官方 `github.com/modelcontextprotocol/go-sdk` 的 `ToolClient` + `SessionOpener` 实现。

- 传输协议：`TransportSSE`（SSE）/ `TransportStreamableHTTP`（默认）
- 认证：`AuthNone` / `AuthBearer`（Header）/ `AuthAPIKey`（Header 或 Query）
- 未传 `http.Client` 时使用带 TCP/TLS 建连超时的默认客户端（**不设**整体 `Client.Timeout`，避免截断长 `CallTool`）

### `MemoryToolCache`

基于 `sync.RWMutex` 的内存缓存实现。`Valid` 同时校验指纹（`Fingerprint(server)`）和过期时间。`Stale` 忽略 TTL，仅校验指纹与工具非空，供发现失败时的降级复用。

### `OpenedTools`

工具调用 lease。核心行为：

- **延迟连接**：缓存命中时 session 为 `nil`，首次 `CallTool` 才通过 `SessionOpener` 建立
- **串行调用**：内部 `sync.Mutex` 保证同一 session 的并发安全
- **幂等关闭**：`Close` 可重复调用，未建 session 时仅标记状态

## 缓存机制

缓存键为 `server.ID.String()`。失效条件：

1. **指纹变更**：`Fingerprint(server)` 基于 ID、transport、URL、auth 配置计算 SHA256，任一配置变化会触发重新拉取
2. **TTL 过期**：默认 5 分钟，可通过 `Options.TTL` 调整

```go
// 缓存命中判断
Valid(server, entry) = (entry.Fingerprint == Fingerprint(server)) && (now < entry.ExpiresAt)
// 失败降级判断（忽略 TTL）
Stale(server, entry) = (entry.Fingerprint == Fingerprint(server)) && len(entry.Tools) > 0
```

## 发现超时与失败冷却

对话组装等同步路径不能依赖操作系统级 ~60s 连接超时。`Service` 在核心层统一保护：

| 机制 | 默认 | 行为 |
|------|------|------|
| `DiscoverTimeout` | 5s | `OpenSession` + `ListTools` / `ListTools` 临时 session 的上界 |
| `FailCooldown` | 30s | 发现失败后跳过远端探测；指纹变更或冷却到期后重试 |
| stale-if-error | — | 失败或冷却时，若存在指纹匹配的 runtime tools（可已过期），仍返回 lazy lease |

- **CallTool** 不受 `DiscoverTimeout` 约束，继续使用调用方 `context`
- **RefreshToolList** 忽略失败冷却（用户显式同步），但仍受 `DiscoverTimeout` 限制
- 成功发现后清除该 server 的失败冷却

## 安全传输

`SDKToolClient` 按 `ServerConfig.AuthType` 处理：

| AuthType | 方式 |
|----------|------|
| `AuthBearer` | `Authorization: Bearer <token>` |
| `AuthAPIKey` + `placement=header` | 自定义 Header（默认 `X-Api-Key`） |
| `AuthAPIKey` + `placement=query` | URL query 参数（默认 `key`） |

## 错误约定

- 远端 `IsError=true` 属于业务错误（模型可观察纠正），不等于调用失败
- transport/protocol 错误通过返回值 `error` 传递
- `OpenedTools.CallTool` 在 lease 关闭后返回 `fmt.Errorf("opened tools is closed")`

## 文件结构

| 文件 | 内容 |
|------|------|
| `doc.go` | 包注释 |
| `types.go` | 核心类型定义与辅助函数 |
| `options.go` | `Options` / `Option` 与 `With*` 函数式选项 |
| `service.go` | `Service` 编排逻辑、接口定义与 `NewService` |
| `cache.go` | `ToolCache` 接口 + `MemoryToolCache` |
| `opened_tools.go` | `OpenedTools` lease 实现 |
| `sdk_client.go` | SDK 实现与认证/传输适配 |
