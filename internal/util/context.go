package util

import (
	"context"

	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type userIDContextKey struct{}

// WithUserID 将已认证用户 ID 写入标准 context，供 logic/service 读取。
func WithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	if ctx == nil || userID == uuid.Nil {
		return ctx
	}
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

// UserIDFromContext 从标准 context 中读取已认证用户 ID，缺失时返回未登录错误。
func UserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	if ctx == nil {
		return uuid.Nil, xerr.Unauthorized("请先登录")
	}
	userID, ok := ctx.Value(userIDContextKey{}).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return uuid.Nil, xerr.Unauthorized("请先登录")
	}
	return userID, nil
}
