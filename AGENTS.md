<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **cove-api** (9736 symbols, 45165 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/cove-api/context` | Codebase overview, check index freshness |
| `gitnexus://repo/cove-api/clusters` | All functional areas |
| `gitnexus://repo/cove-api/processes` | All execution flows |
| `gitnexus://repo/cove-api/process/{name}` | Step-by-step execution trace |

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

## Testing

- When adding or modifying test functions, you must place a Chinese comment immediately above each test function explaining what the test verifies.

## Generated Code

- For generated code, key steps must include Chinese comments explaining what they do.

## Go Style

- Prefer using pointers wherever pointers can reasonably be used.

## Core Package Template

Use `internal/core/rag/search` as the implementation template for new packages under `internal/core`.

- Split package responsibilities into focused files: `types.go` for interfaces/request/result types, `options.go` for defaults and functional options, one main implementation file for behavior, and `*_test.go` for package-level behavior tests.
- Keep core packages business-neutral. Define small dependency interfaces inside the core package, accept dependencies through constructors, and let callers provide business filters, decoders, or adapters.
- Constructors should return pointers, initialize complete defaults first, then apply functional options. Options should ignore invalid zero values when preserving defaults is safer.
- Separate package-level options from per-call request options when both long-lived configuration and request-specific overrides exist.
- Prefer generic result/source types when the core behavior is reusable but callers need typed metadata.
- Keep exported methods small and orchestration-focused. Move query building, normalization, filtering, sorting, decoding, and fallback behavior into private helper functions.
- Key implementation steps must include Chinese comments explaining what they do, especially non-obvious algorithms, fallback behavior, score normalization, filtering, and external-service query construction.
- Tests should use local fakes for dependencies, cover defaults/options, dependency errors, request overrides, helper edge cases, fallback paths, and result shaping. Every test function must include a Chinese comment immediately above the function explaining what the test verifies.

## Go Documentation and Function Style

These constraints apply to new or modified packages under `internal/core`.

- Package comments should exist for non-trivial packages and start with `Package <name> ...`. Keep one package comment per package, usually near the main package file.
- Exported top-level types, functions, methods, constants, and variables must have Go doc comments. Non-trivial unexported types and functions should also have comments when behavior is not obvious.
- Go doc comments should be Chinese complete sentences, start with the declared identifier or a natural article plus the identifier, and end with punctuation. 
  Body comments for key implementation steps should remain Chinese when explaining non-obvious logic, as required by the existing generated-code rule.
- Function and method doc comments must describe caller-visible behavior: what the function returns or does, important errors, side effects, concurrency guarantees, nil/zero-value behavior, fallback behavior, and special cases. Do not document private implementation algorithms in exported doc comments; put algorithm notes inside the function body.
- Boolean-returning functions should use “reports whether” in doc comments. Multi-result functions should document what each result means when it is not obvious.
- Function names should be short, idiomatic, and read well with the package name. Avoid redundant package words, avoid `Get` prefixes for simple getters, use `New` or `New<Type>` for constructors, and use MixedCaps with standard initialisms.
- One-method interfaces should usually use the `-er` naming pattern, such as `Reader`, `Writer`, `Searcher`, or `Decoder`. Define interfaces at the consumer side and keep them as small as the behavior actually needed.
- Functions should accept `context.Context` as the first parameter when they perform request-scoped work, I/O, external calls, cancellation-aware work, or potentially long-running operations.
- Keep exported functions orchestration-focused and easy to scan. Move parsing, validation, query construction, normalization, filtering, sorting, decoding, and fallback details into private helpers when the main function becomes dense.
- Prefer explicit error returns over panic for normal failures. Error strings should be lower-case and should not end with punctuation unless the message includes a proper noun or acronym.
- Avoid mutable package globals for core behavior. Inject dependencies through constructors or options so tests can use local fakes without global mutation.
- Tests for core functions must have useful failure messages that include the function or behavior under test, the actual result, and the expected result. For large values, prefer diffs over dumping unreadable structs.
