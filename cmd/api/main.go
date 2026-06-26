package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/boxify/api-go/internal/config"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	httptransport "github.com/boxify/api-go/internal/transport/http"
)

func main() {
	cfg := config.Load()
	xlog.Configure(xlog.Config{
		Env:   cfg.App.Env,
		Level: slog.LevelInfo,
		Color: true,
	})

	ctx := context.Background()
	svcCtx, err := svc.New(ctx, cfg)
	if err != nil {
		slog.Error("初始化服务上下文失败", "错误", err)
		os.Exit(1)
	}

	defer func() {
		if err := svcCtx.Close(ctx); err != nil {
			slog.Error("关闭服务上下文失败", "错误", err)
		}
	}()

	router := httptransport.NewRouter(httptransport.Dependencies{
		Svc: svcCtx,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("API 服务启动中", "地址", cfg.HTTPAddr())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("API 服务异常停止", "错误", err)
		os.Exit(1)
	}
}
