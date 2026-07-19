WORKSPACE_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
APP_DIR := $(WORKSPACE_ROOT)/packages/app
APP_FRONTEND_DIR := $(APP_DIR)/frontend
APP_MOBILE_DIR := $(APP_DIR)/mobile
SERVER_DIR := $(WORKSPACE_ROOT)/packages/server

.PHONY: api worker gateway migration gen-route gen-repository gen-docs gen-prompt docs install-hooks
.PHONY: app-dev app-build app-package app-run app-build-server app-run-server app-build-docker app-run-docker
.PHONY: app-go-test app-frontend-build app-frontend-test app-mobile-lint app-mobile-typecheck app-mobile-test app-mobile-e2e-profile-password app-mobile-e2e-chat-persistence app-mobile-e2e-native-lifecycle app-mobile-e2e-knowledge-upload
.PHONY: server-db-smoke e2e-up e2e-app-backend e2e-smoke e2e e2e-logs e2e-down
.PHONY: help

# Server targets
api:
	cd "$(SERVER_DIR)" && go run ./cmd/api

worker:
	cd "$(SERVER_DIR)" && go run ./cmd/worker

gateway:
	cd "$(SERVER_DIR)" && go run ./cmd/gateway

migration:
	cd "$(SERVER_DIR)" && go run ./cmd/migration

gen-route:
	cd "$(SERVER_DIR)" && go run ./cmd/codegen route $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

gen-repository:
	@if [ -z "$(MODEL)" ]; then echo "MODEL is required, e.g. make gen-repository MODEL=Conversation LABEL=会话"; exit 1; fi
	cd "$(SERVER_DIR)" && go run ./cmd/codegen repository -model $(MODEL) $(if $(LABEL),-label $(LABEL),) $(if $(SCOPE),-scope $(SCOPE),) $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

gen-docs:
	cd "$(SERVER_DIR)" && go run ./cmd/codegen docs $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

gen-prompt:
	cd "$(SERVER_DIR)" && go run ./cmd/codegen prompt $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

# docs: 验证 route / repository / OpenAPI 生成产物与源码同步，适合 CI 与 pre-commit。
docs:
	cd "$(SERVER_DIR)" && go run ./cmd/codegen route --check --format json
	cd "$(SERVER_DIR)" && go run ./cmd/codegen repository --list-models --format json
	cd "$(SERVER_DIR)" && go run ./cmd/codegen docs --check --format json
	cd "$(SERVER_DIR)" && go run ./cmd/codegen prompt --check --format json

# install-hooks: 通过 core.hooksPath 让钩子随 Cove monorepo 生效。
install-hooks:
	git -C "$(WORKSPACE_ROOT)" config core.hooksPath .githooks
	@echo "Cove git hooks 已激活：hooksPath → .githooks"
	@echo "现在每次 push 前都会自动运行本地 CI 校验，避免流水线失败。"
	@echo "发版规则：vX.Y.Z 必须先合入 main 再打 tag 推送（pre-push + CD 均强制）。"
	@echo "跳过校验：GIT_PUSH_VERIFY=false git push（不能跳过发版 tag 的 main 检查）"

# App targets delegate to the existing Taskfile and package scripts.
app-dev:
	cd "$(APP_DIR)" && task dev

app-build:
	cd "$(APP_DIR)" && task build

app-package:
	cd "$(APP_DIR)" && task package

app-run:
	cd "$(APP_DIR)" && task run

app-build-server:
	cd "$(APP_DIR)" && task build:server

app-run-server:
	cd "$(APP_DIR)" && task run:server

app-build-docker:
	cd "$(APP_DIR)" && task build:docker

app-run-docker:
	cd "$(APP_DIR)" && task run:docker

app-go-test:
	cd "$(APP_DIR)" && go test ./...

app-frontend-build:
	cd "$(APP_FRONTEND_DIR)" && pnpm build

app-frontend-test:
	cd "$(APP_FRONTEND_DIR)" && pnpm test

app-mobile-lint:
	cd "$(APP_MOBILE_DIR)" && pnpm lint

app-mobile-typecheck:
	cd "$(APP_MOBILE_DIR)" && pnpm typecheck

app-mobile-test:
	cd "$(APP_MOBILE_DIR)" && pnpm test

app-mobile-e2e-profile-password:
	cd "$(APP_MOBILE_DIR)" && pnpm e2e:ios:profile-password

app-mobile-e2e-chat-persistence:
	cd "$(APP_MOBILE_DIR)" && pnpm e2e:ios:chat-persistence

app-mobile-e2e-native-lifecycle:
	cd "$(APP_MOBILE_DIR)" && pnpm e2e:ios:native-lifecycle

app-mobile-e2e-knowledge-upload:
	cd "$(APP_MOBILE_DIR)" && pnpm e2e:ios:knowledge-upload

# Cross-package E2E lifecycle
server-db-smoke:
	bash "$(WORKSPACE_ROOT)/e2e/scripts/e2e.sh" server-db-smoke

e2e-up:
	bash "$(WORKSPACE_ROOT)/e2e/scripts/e2e.sh" up

e2e-app-backend:
	bash "$(WORKSPACE_ROOT)/e2e/scripts/e2e.sh" app-backend

e2e-smoke:
	bash "$(WORKSPACE_ROOT)/e2e/scripts/e2e.sh" smoke

e2e: e2e-smoke

e2e-logs:
	bash "$(WORKSPACE_ROOT)/e2e/scripts/e2e.sh" logs

e2e-down:
	bash "$(WORKSPACE_ROOT)/e2e/scripts/e2e.sh" down

help:
	@echo "Server: api worker gateway migration gen-route gen-repository gen-docs gen-prompt docs install-hooks"
	@echo "App: app-dev app-build app-package app-run app-build-server app-run-server app-build-docker app-run-docker"
	@echo "App checks: app-go-test app-frontend-build app-frontend-test app-mobile-lint app-mobile-typecheck app-mobile-test app-mobile-e2e-profile-password app-mobile-e2e-chat-persistence app-mobile-e2e-native-lifecycle app-mobile-e2e-knowledge-upload"
	@echo "Real database: server-db-smoke"
	@echo "E2E: e2e-up e2e-app-backend e2e-smoke e2e e2e-logs e2e-down"
