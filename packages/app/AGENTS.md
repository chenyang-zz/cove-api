<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **cove** (474551 symbols, 686596 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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

## API Documentation and Environment Configuration

- Shared Make targets live in the Cove workspace root `Makefile`. Run App wrappers such as `make app-build` and `make app-mobile-test` from `/Users/sheepzhao/WorkSpace/agent/boxify/cove`; keep `Taskfile.yml` and the package scripts as the implementation sources of truth.
- The Figma design source for this project is `[Cove](https://www.figma.com/design/wks3wwXIDCjdsVS6jPQqS6/Cove?node-id=0-1&m=dev&t=5gPGykVr97PD5dac-1)`. Use this file as the design reference for Cove UI work.
- The OpenAPI contract is located at `/Users/sheepzhao/WorkSpace/agent/boxify/cove/packages/server/docs/openapi.json`. Before adding or changing an API call, use it to verify the path, request body, response shape, and authentication requirements.
- The frontend API base URL is supplied exclusively through `VITE_API_BASE_URL`; it defaults to `http://localhost:8000` when unset.
- Configure environment-specific values in uncommitted files such as `frontend/.env.development.local` and `frontend/.env.production.local`. Do not hard-code environment URLs in application code.
- If the `openapi.json` path changes, update this instruction at the same time.
- When an API behavior is not documented in the OpenAPI contract, such as streaming response event types, inspect the service implementation at `/Users/sheepzhao/WorkSpace/agent/boxify/cove/packages/server` and treat it as the source of truth.

## iOS UI Debugging

- Treat iOS as the primary mobile platform: design, implement, and validate mobile-facing changes against iOS first.
- When debugging iOS visual or interaction changes, run the app in an iOS Simulator by default. Use a physical device only for device-specific verification after simulator validation.
