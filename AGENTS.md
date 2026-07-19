# Cove Monorepo Instructions

This workspace is the single Git repository for the Cove project. Product code is grouped under `packages/`:

- `packages/app/`: Cove clients and frontend code.
- `packages/server/`: Cove backend services and the OpenAPI contract.

Before working, read [.codex/rules/architecture.md](.codex/rules/architecture.md), [.codex/rules/worktree.md](.codex/rules/worktree.md), and [.codex/rules/git-workflow.md](.codex/rules/git-workflow.md). Also read the rules for every area touched by the task:

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

## Isolated Feature Development

- A request to implement a new product feature is explicit authorization to create one Codex project task in a managed worktree for that feature. If the request starts in a Local task, keep the Local task as the coordinator and perform implementation in the worktree task.
- Do not edit product code for a new feature in the primary checkout. Read-only exploration, reviews, commits of already completed work, documentation-only work, and explicitly requested changes to the current checkout may remain Local.
- Start feature worktrees from the requested existing branch, or from `dev` when none is specified. Do not copy the primary checkout's uncommitted changes into a feature worktree unless the user explicitly places those changes in scope.
- The worktree task must follow [.codex/rules/worktree.md](.codex/rules/worktree.md), including setup, runtime isolation, validation, commit handoff, and cleanup requirements.

## E2E Agent Routing

- For requests that add, change, run, diagnose, or review Cove E2E tests, follow the staged multi-agent workflow in `.codex/rules/e2e-testing.md`.
- Delegate repository discovery and test inventory to `e2e-explorer`.
- Delegate deterministic test execution, first-failure evidence confirmation, and explicitly authorized test-local fixture or harness fixes to `e2e-runner`.
- Escalate repeated failures, flaky behavior, unexplained cross-layer mismatches, and every product-code, shared-lifecycle, or App/Server contract fix to `e2e-debugger`.
- Delegate final coverage and evidence review to `e2e-reviewer` whenever the task changes product code, tests, fixtures, harnesses, or lifecycle configuration. A pure execution-only task with no file changes may omit this stage.
- Keep the root agent responsible for scope, sequencing, user communication, write-conflict avoidance, and the final App/Server/cross-boundary report.
- Do not spawn parallel write-heavy agents against overlapping files. Exploration and independent read-only validation may run in parallel when useful.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **cove** (15089 symbols, 33478 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/cove/context` | Codebase overview, check index freshness |
| `gitnexus://repo/cove/clusters` | All functional areas |
| `gitnexus://repo/cove/processes` | All execution flows |
| `gitnexus://repo/cove/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
