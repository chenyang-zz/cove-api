package svc

import (
	"net/http"
	"testing"
	"time"

	corechannel "github.com/boxify/api-go/internal/core/channel"
)

// TestNewChannelRegistryRegistersOfficialProviders 验证 ServiceContext 组合根完整注册三种官方渠道 Provider。
func TestNewChannelRegistryRegistersOfficialProviders(t *testing.T) {
	registry, err := newChannelRegistry(&http.Client{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("newChannelRegistry() error = %v", err)
	}

	descriptors := registry.Descriptors()
	wantProviders := []corechannel.ProviderName{
		corechannel.ProviderFeishu,
		corechannel.ProviderTelegram,
		corechannel.ProviderWebhook,
	}
	if len(descriptors) != len(wantProviders) {
		t.Fatalf("newChannelRegistry() provider count = %d, want %d", len(descriptors), len(wantProviders))
	}
	for index, wantProvider := range wantProviders {
		if descriptors[index].Name != wantProvider {
			t.Errorf("newChannelRegistry() descriptor[%d].Name = %q, want %q", index, descriptors[index].Name, wantProvider)
		}
		if _, ok := registry.Get(wantProvider); !ok {
			t.Errorf("newChannelRegistry() missing provider %q", wantProvider)
		}
	}
}
