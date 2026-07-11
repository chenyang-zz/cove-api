# 阿里云 ACR + GHCR CD 部署指南

生产 compose 只部署 API 与 worker。GitHub Actions 对每个服务构建一次，并将同一提交 SHA 的镜像同时推送到阿里云容器镜像服务（ACR）和私有 GitHub Container Registry（GHCR）；服务器只从 ACR 拉取。

推送 `main` 或手动触发 CD 后，CI 通过的提交一定会以完整 SHA 构建、双推送并部署。符合 Conventional Commits 的 `feat`、`fix`、`perf` 或破坏性变更会额外自动创建 GitHub Release、Git tag 和版本镜像标签；`chore`、`ci`、`docs` 等提交仍部署，但不创建新版本。

## 1. 阿里云 ACR 准备

在 ACR 创建一个命名空间，例如 `cove`，并创建 `cove-api` 和 `cove-worker` 两个私有仓库。GitHub Actions 使用有推送权限的账号或访问凭证；部署服务器使用独立的只读拉取账号或访问凭证。

ACR 镜像地址格式为：

```text
<ALIYUN_REGISTRY>/<ALIYUN_NAMESPACE>/cove-api:<git-sha>
<ALIYUN_REGISTRY>/<ALIYUN_NAMESPACE>/cove-worker:<git-sha>
```

`ALIYUN_REGISTRY` 使用 ACR 实例实际提供的 Docker 登录域名，不包含 `https://`。个人版与企业版均支持该格式。

## 2. GitHub 配置

在仓库 Variables 中配置：

| 名称 | 示例 | 说明 |
| --- | --- | --- |
| `ALIYUN_REGISTRY` | `registry.cn-hangzhou.aliyuncs.com` | ACR Docker 登录域名和可选端口，不含协议。 |
| `ALIYUN_NAMESPACE` | `cove` | ACR 命名空间。 |

在 `production` Environment Secrets 中配置：

| 名称 | 说明 |
| --- | --- |
| `ALIYUN_USERNAME` / `ALIYUN_PASSWORD` | GitHub Actions 使用的 ACR 推送账号或访问凭证。 |
| `DEPLOY_ALIYUN_USERNAME` / `DEPLOY_ALIYUN_PASSWORD` | 服务器使用的 ACR 只读拉取账号或访问凭证。 |
| `DEPLOY_HOST` / `DEPLOY_USER` / `DEPLOY_PORT` | 服务器 SSH 地址、用户和端口。 |
| `DEPLOY_SSH_KEY` | 部署私钥。 |
| `DEPLOY_SSH_FINGERPRINT` | SSH action 实际协商到的主机公钥 SHA256 指纹。当前 action 优先使用 ECDSA，应填写诊断步骤输出的 ECDSA 指纹。 |
| `DEPLOY_COMPOSE_DIR` | 服务器部署目录；未设置时为 `/opt/cove`。 |

工作流使用 `GITHUB_TOKEN` 推送 GHCR，不需要额外的 GitHub 镜像 secret。首次发布后，在 GitHub Packages 页面确认 `cove-api`、`cove-worker` 两个包保持私有，并关联到 `chenyang-zz/cove-api` 仓库。

镜像始终生成不可变的完整提交 SHA tag；从 `main` 发布时额外生成 `main` tag。自动发版时额外生成 `0.1.0`、`0.1`、`0` 等版本别名。部署始终使用完整 SHA，任一 ACR 或 GHCR 推送失败都会阻止部署。

## 3. 自动版本规则

版本从仓库中最新的 `vX.Y.Z` Git tag 计算；没有版本 tag 时，首个符合发版规则的提交创建 `v0.1.0`。

| 提交格式 | 版本变化 |
| --- | --- |
| `feat:` 或 `feat(scope):` | minor，例如 `v0.1.0` → `v0.2.0`。 |
| `fix:`、`perf:`、`revert:` | patch，例如 `v0.1.0` → `v0.1.1`。 |
| `type!:` 或正文包含 `BREAKING CHANGE:` | major，例如 `v0.1.0` → `v1.0.0`。 |
| `chore:`、`ci:`、`docs:` 等 | 不创建版本，但仍构建、同步并部署 SHA 镜像。 |

自动创建的 Git tag 不会再次触发 CD，避免重复构建和部署。

## 4. 服务器初始化与验收

在 `DEPLOY_COMPOSE_DIR` 中准备：

```text
deployments/docker-compose.production.yml
configs/config.production.yml
```

PostgreSQL、Redis、Elasticsearch 和 Neo4j 不由本生产 compose 管理；它们必须已在外部 Docker 网络 `boxify_default` 中运行。生产 compose 会将 API/worker 加入该网络，`configs/config.production.yml` 应使用这些容器在该网络中的实际 DNS 名称。本服务器当前使用 `boxify-postgresql-1`、`redis-server`、`elasticsearch-server`、`neo4j-server`。

首次部署前确认网络和服务别名存在：

```bash
docker network inspect boxify_default
```

首次部署前，以服务器的 ACR 只读账号验证：

```bash
docker login <ALIYUN_REGISTRY>
docker pull <ALIYUN_REGISTRY>/<ALIYUN_NAMESPACE>/cove-api:<full-git-sha>
docker pull <ALIYUN_REGISTRY>/<ALIYUN_NAMESPACE>/cove-worker:<full-git-sha>

export ALIYUN_REGISTRY=<ALIYUN_REGISTRY>
export ALIYUN_NAMESPACE=<ALIYUN_NAMESPACE>
export IMAGE_TAG=<full-git-sha>
docker compose -f deployments/docker-compose.production.yml config
```

推送 `main`、推送语义版本 tag 或手动运行 CD 后，确认日志出现 `Deployment completed from Alibaba Cloud: <sha>`，并在服务器执行：

```bash
docker compose -f deployments/docker-compose.production.yml ps
curl -fsS http://127.0.0.1:8000/api/health
```

部署会先预拉取两个候选镜像，依次替换 worker 和 API，并分别等待 Docker health 变为 `healthy`，每项最多等待 5 分钟。首次切换时会从两个当前运行容器回填 `.deploy/image-tag`；二者必须使用同一完整 SHA，否则部署会安全终止。

健康检查失败时，工作流会输出容器状态与最近 200 行日志，并回滚到 `.deploy/image-tag` 指向的版本。可在服务器手动复核：

```bash
docker compose -f deployments/docker-compose.production.yml ps
docker inspect deployments-api-1 --format '{{json .State}}'
docker inspect deployments-worker-1 --format '{{json .State}}'
docker compose -f deployments/docker-compose.production.yml logs --tail 200 api worker
```
