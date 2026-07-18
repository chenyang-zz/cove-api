package services

import (
	"github.com/chenyang-zz/cove/internal/domain"
	"github.com/chenyang-zz/cove/internal/platform"
)

type AppInfoService struct {
	name    string
	version string
}

func NewAppInfoService(name, version string) *AppInfoService {
	return &AppInfoService{
		name:    name,
		version: version,
	}
}

func (s *AppInfoService) GetAppInfo() (domain.AppInfo, error) {
	system := platform.CurrentSystemInfo()

	return domain.AppInfo{
		Name:     s.name,
		Version:  s.version,
		Platform: system.Platform,
		Arch:     system.Arch,
	}, nil
}
