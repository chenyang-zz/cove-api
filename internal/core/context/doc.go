// Package context 提供业务无关的模型上下文计数、滚动摘要和确定性裁剪能力。
//
// 包内的 Manager 不读取数据库或静态配置。调用方通过 Policy 提供预算，通过 Store
// 持久化摘要状态，并通过 Summarizer 接入任意模型。由于包名与标准库 context 相同，
// 本包内部统一把标准库导入别名写为 stdcontext。
package context
