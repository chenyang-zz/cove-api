package mapper

import (
	"testing"

	corecontext "github.com/boxify/api-go/internal/core/context"
	"github.com/boxify/api-go/internal/models"
)

// TestAgentConfigToContextPolicyUsesDefaultsForLegacyRows 验证旧记录缺少上下文字段时会使用 32K 默认策略。
func TestAgentConfigToContextPolicyUsesDefaultsForLegacyRows(t *testing.T) {
	policy := AgentConfigToContextPolicy(&models.AgentConfig{})
	if policy.WindowTokens != corecontext.DefaultWindowTokens || !policy.Enabled {
		t.Fatalf("AgentConfigToContextPolicy(legacy) = %#v, want enabled default policy", policy)
	}
}

// TestAgentConfigToContextPolicyMapsPersistedColumns 验证数据库独立列会完整映射为 core 策略。
func TestAgentConfigToContextPolicyMapsPersistedColumns(t *testing.T) {
	row := &models.AgentConfig{
		ContextEnabled: false, ContextWindowTokens: 8192, ContextOutputReserveTokens: 1024,
		ContextSafetyMarginTokens: 256, ContextTriggerRatio: 0.9, ContextTargetRatio: 0.7,
		ContextKeepRecentTokens: 2048, ContextSummaryMaxTokens: 512,
	}
	policy := AgentConfigToContextPolicy(row)
	if policy.Enabled || policy.WindowTokens != 8192 || policy.SummaryMaxTokens != 512 {
		t.Fatalf("AgentConfigToContextPolicy() = %#v, want persisted values", policy)
	}
}
