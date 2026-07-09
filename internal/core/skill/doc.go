// Package skill 提供业务无关的 Markdown skill 文档解析能力。
//
// 本包只理解 SKILL.md 一类文档的通用结构：YAML front matter 元数据和 Markdown 正文。
// 调用方负责决定模板来源、业务字段映射、持久化方式和运行环境要求检查。
package skill
