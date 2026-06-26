package middleware

import (
	"context"
	"strings"

	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const UserIDKey = "user_id"

type TokenVerifier interface {
	VerifyAccessToken(ctx context.Context, token string) (uuid.UUID, error)
}

func Auth(verifier TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			response.FromError(c, xerr.Unauthorized("请先登录"))
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
		if token == "" {
			response.FromError(c, xerr.Unauthorized("请先登录"))
			c.Abort()
			return
		}
		userID := uuid.Nil
		if verifier != nil {
			var err error
			userID, err = verifier.VerifyAccessToken(c.Request.Context(), token)
			if err != nil {
				response.FromError(c, err)
				c.Abort()
				return
			}
		} else if token == "dev-token" {
			userID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
		}
		if userID == uuid.Nil {
			response.FromError(c, xerr.InvalidToken())
			c.Abort()
			return
		}
		c.Set(UserIDKey, userID)
		ctx := util.WithUserID(c.Request.Context(), userID)
		ctx = xlog.WithUserID(ctx, userID.String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
