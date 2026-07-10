.PHONY: api worker migration gen-route gen-repository gen-docs gen-prompt docs install-hooks

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

migration:
	go run ./cmd/migration

gen-route:
	go run ./cmd/codegen route $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

gen-repository:
	@if [ -z "$(MODEL)" ]; then echo "MODEL is required, e.g. make gen-repository MODEL=Conversation LABEL=会话"; exit 1; fi
	go run ./cmd/codegen repository -model $(MODEL) $(if $(LABEL),-label $(LABEL),) $(if $(SCOPE),-scope $(SCOPE),) $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

gen-docs:
	go run ./cmd/codegen docs $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

gen-prompt:
	go run ./cmd/codegen prompt $(if $(DRY_RUN),--dry-run,) $(if $(CHECK),--check,) $(if $(VERBOSE),--verbose,) $(if $(FORMAT),--format $(FORMAT),)

# docs: 验证 route / repository / OpenAPI 生成产物与源码同步，适合 CI 与 pre-commit
docs:
	go run ./cmd/codegen route --check --format json
	go run ./cmd/codegen repository --list-models --format json
	go run ./cmd/codegen docs --check --format json
	go run ./cmd/codegen prompt --check --format json

# install-hooks: 安装 git hooks 到本地 .git/hooks
install-hooks:
	cp .githooks/pre-push .git/hooks/pre-push
	chmod +x .git/hooks/pre-push
	@echo "git hooks 安装完成。运行 'make install-hooks' 后即可在推送前自动校验。"
