package svc

import (
	"net/http"

	corechannel "github.com/boxify/api-go/internal/core/channel"
	"github.com/boxify/api-go/internal/infrastructure/channel/feishu"
	"github.com/boxify/api-go/internal/infrastructure/channel/telegram"
	"github.com/boxify/api-go/internal/infrastructure/channel/webhook"
)

// newChannelRegistry 在 ServiceContext 组合根注册 Cove 官方渠道 Provider。
// client 仅用于用户配置的 Webhook 回调；Telegram 保持独立的官方 API 超时边界。
func newChannelRegistry(client *http.Client) (*corechannel.Registry, error) {
	return corechannel.NewRegistry(corechannel.WithProviders(
		telegram.New(nil),
		feishu.New(),
		webhook.New(client),
	))
}
