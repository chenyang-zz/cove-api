package platform

import "runtime"

type SystemInfo struct {
	Platform string
	Arch     string
}

func CurrentSystemInfo() SystemInfo {
	return SystemInfo{
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
}
