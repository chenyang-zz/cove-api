package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

const (
	toolKeyPrefix          = "mcp_"
	toolKeySeparator       = "__"
	toolKeyNameMaxLength   = 17
	toolKeyHashBytesLength = 4
)

// ToolKey 返回稳定、合法且不超过 64 字符的 MCP 工具 key。
//
// key 同时包含完整 server UUID 和远端工具名摘要，因此 server 重命名不会改变 key，
// 不同 server 的同名工具也不会冲突。空远端名称会使用 tool 作为可读片段。
func ToolKey(serverID uuid.UUID, rawName string) string {
	serverPart := strings.ReplaceAll(serverID.String(), "-", "")
	namePart := sanitizeToolName(rawName)
	if len(namePart) > toolKeyNameMaxLength {
		namePart = namePart[:toolKeyNameMaxLength]
	}
	digest := sha256.Sum256([]byte(serverID.String() + "\x00" + rawName))
	hashPart := hex.EncodeToString(digest[:toolKeyHashBytesLength])
	return toolKeyPrefix + serverPart + toolKeySeparator + namePart + "_" + hashPart
}

// ParseToolKey 解析 MCP 工具 key 中的 server UUID。
//
// 第二个返回值 reports whether key 具有合法的 MCP key 格式；它不校验远端工具名摘要。
func ParseToolKey(key string) (uuid.UUID, bool) {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, toolKeyPrefix) {
		return uuid.Nil, false
	}
	rest := strings.TrimPrefix(key, toolKeyPrefix)
	parts := strings.SplitN(rest, toolKeySeparator, 2)
	if len(parts) != 2 || len(parts[0]) != 32 || parts[1] == "" {
		return uuid.Nil, false
	}
	serverID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, false
	}
	return serverID, true
}

// sanitizeToolName 将远端工具名转换为合法的 key 片段，
func sanitizeToolName(value string) string {
	var out strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch {
		case char >= 'a' && char <= 'z':
			out.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			out.WriteRune(char)
		case char >= '0' && char <= '9':
			out.WriteRune(char)
		case char == '_' || char == '-':
			out.WriteRune(char)
		default:
			out.WriteByte('_')
		}
	}
	name := strings.Trim(out.String(), "_")
	if name == "" {
		return "tool"
	}
	return name
}
