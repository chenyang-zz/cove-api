# Cove Monorepo Instructions

This workspace is the single Git repository for the Cove project. Product code is grouped under `packages/`:

- `packages/app/`: Cove clients and frontend code.
- `packages/server/`: Cove backend services and the OpenAPI contract.

Before working, read [.codex/rules/architecture.md](.codex/rules/architecture.md). Also read the rules for every area touched by the task:

- App work: [.codex/rules/frontend.md](.codex/rules/frontend.md) and `packages/app/AGENTS.md`.
- Server work: [.codex/rules/backend.md](.codex/rules/backend.md) and `packages/server/AGENTS.md`.
- End-to-end test work: [.codex/rules/e2e-testing.md](.codex/rules/e2e-testing.md) plus the rules for every exercised surface.
- Cross-package work: all applicable files above.

## Workspace Rules

- Run Git commands and shared Make targets from the monorepo root. The root `Makefile` delegates package-native commands to `packages/app/` and `packages/server/`.
- Keep package concerns explicit in commit scope and validation output. A cross-package feature may be committed atomically when the app and server changes implement one contract.
- Preserve existing uncommitted changes across all packages.
- For API changes, update the server contract first, then update app consumers against `packages/server/docs/openapi.json`.
- When a completed requirement adds a new HTTP endpoint, add a preset request to the appropriate existing module collection in the Postman `boxify-go` workspace after regenerating the OpenAPI contract. The request must include the method, `{{base_url}}` URL, path/query variables, representative body and content type when applicable, and the correct authentication mode. Reuse existing collections and variables instead of creating duplicates. These presets are for manual invocation; do not add test scripts or Runner workflows unless explicitly requested. The requirement is not complete until the request is visible in Postman; if Postman is unavailable, report it as an incomplete validation item.
- Do not hard-code environment-specific service URLs in source code.
- A backend requirement is not complete until its real scenario has passed against a local real database after applying the real migration path; fake repositories and `httptest` alone are insufficient completion evidence.
- Cross-frontend/backend E2E acceptance must use an isolated local database managed by the workspace OrbStack workflow. Never point it at a remote, production, staging, or developer-owned database.
- Report validation results separately for app and server changes, followed by cross-package E2E results when applicable.
- Release tags (`vX.Y.Z`) must be created only after the release commit is on `main`. Never tag or push a production version from `dev` or a feature branch. Order: merge to `main` → tag that commit → push the tag. Local pre-push and CD both enforce this.
