---
name: commit-message
description: >-
  Generate a conventional commit message for tekton-pruner. Use when the user
  asks to "create a commit", "generate commit message", "commit changes",
  "make a commit", mentions "conventional commits", or discusses commit message
  formatting. Provides guided workflow for creating properly formatted commit
  messages with line length validation and required trailers.
version: 0.1.0
license: Apache-2.0
metadata:
  project: tekton-pruner
allowed-tools: Bash(git diff:*) Bash(git status:*) Bash(git log:*) Bash(git config:*) Read
---

# Conventional Commit Message Creation

Create properly formatted conventional commit messages following
project standards with line length validation and required trailers.

## Purpose

Generate commit messages that:

- Follow conventional commits format (`type(scope): description`)
- Use component names from changed file paths as scope
- Respect line length limits (50 for subject, 72 for body)
- Include required trailers (`Signed-off-by`, `Assisted-by`)
- Follow [Tekton commit conventions](https://github.com/tektoncd/community/blob/main/standards.md#commit-messages)
  (imperative mood, line length limits; conventional commit prefixes
  are project practice, not in the written standard)

## Quick Workflow

1. Analyze changes: `git status` and `git diff --cached`
2. Determine type and scope (see tables below)
3. Generate message with proper format
4. Add required trailers
5. **Never commit without explicit user confirmation**

## Format

```
<type>(<scope>): <description>

<optional body — explain what and why, not how>

<trailers>
```

## Type Selection

| Type | When to use |
|------|-------------|
| `feat` | New functionality |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `refactor` | Code restructure, no behavior change |
| `test` | Adding or updating tests |
| `chore` | Maintenance, dependency updates |
| `build` | Build system or tooling changes |
| `ci` | CI/CD workflow changes |
| `perf` | Performance improvement |
| `style` | Formatting, linting fixes |
| `revert` | Reverting a previous commit |

See `references/commit-types.md` for detailed examples.

## Scope Rules

Derive scope from the changed file paths:

| Path pattern | Scope |
|-------------|-------|
| `pkg/reconciler/pipelinerun/` | `pipelinerun` |
| `pkg/reconciler/taskrun/` | `taskrun` |
| `pkg/reconciler/tektonpruner/` | `controller` |
| `pkg/reconciler/namespaceprunerconfig/` | `controller` |
| `pkg/config/` | `config` |
| `pkg/webhook/` | `webhook` |
| `pkg/metrics/` | `metrics` |
| `cmd/controller/` | `controller` |
| `cmd/webhook/` | `webhook` |
| `config/` | `config` |
| `docs/` | scope = doc topic |
| `.github/workflows/` | `.github/workflows` |
| `.tekton/` | `.tekton` |
| `Makefile` | `Makefile` |
| `go.mod` / `go.sum` / `vendor/` | `deps` |
| `hack/` | `hack` |
| `skills/` | `skills` |

If changes span multiple scopes, use the most significant one.
To link work to a GitHub issue, add `Fixes #NNN` or `Closes #NNN`
in the commit body (not the subject scope).

## Line Length

- **Subject**: target 50 characters, hard limit 72
- **Body**: wrap at 72 characters per line

## Required Trailers

Every commit must include:

1. **`Signed-off-by`** — for DCO compliance

   Detection priority:
   - `$GIT_AUTHOR_NAME` / `$GIT_AUTHOR_EMAIL` environment variables
   - `git config user.name` / `git config user.email`
   - Ask user (last resort)

2. **`Assisted-by`** — when AI assists with the commit

   Format: `Assisted-by: <Model Name> (via <Tool Name>)`

   **Do not use `Co-Authored-By` for AI attribution.** The Tekton
   community adopted `Assisted-by` to distinguish AI assistance from
   human co-authorship. `Co-Authored-By` implies ownership and
   accountability that AI tools cannot bear. Some AI coding tools
   default to injecting `Co-Authored-By` — override or remove it.

### Trailer Format

```
Signed-off-by: Full Name <email@example.com>
Assisted-by: Claude Sonnet 4.6 (via Claude Code)
```

- Blank line before trailers
- No blank lines between trailers
- No trailing blank lines

## User Confirmation

**CRITICAL RULE**: Always display the full commit message and wait for
explicit user approval before executing `git commit`.

## Commit Execution

Use heredoc format for multi-line messages:

```bash
git commit -m "$(cat <<'EOF'
<type>(<scope>): <description>

<body>

Signed-off-by: Name <email>
Assisted-by: Model (via Tool)
EOF
)"
```

**Never use**: `--no-verify`, `--no-gpg-sign`, or `--amend`
(unless the user explicitly requests it).

## Examples

### Feature with component scope

```
feat(config): add enforcedConfigLevel validation

Validate that enforcedConfigLevel is one of the three allowed values
(global, namespace, resource) during ConfigMap admission. Previously
invalid values were silently accepted.

Signed-off-by: Jane Developer <jane@example.com>
Assisted-by: Claude Sonnet 4.6 (via Claude Code)
```

### Bug fix closing an issue

```
fix(controller): prevent duplicate TTL annotations on PipelineRuns

The reconciler was setting TTL annotations on every reconcile loop
even when the annotation already existed with the correct value.
This caused unnecessary API writes and noisy audit logs.

Fixes #42

Signed-off-by: John Developer <john@example.com>
Assisted-by: Claude Sonnet 4.6 (via Claude Code)
```

### Documentation update

```
docs(tutorials): add resource-group selector examples

Signed-off-by: Jane Developer <jane@example.com>
```

### Breaking change

```
feat(config)!: rename historyLimit to retentionLimit

BREAKING CHANGE: The ConfigMap field `historyLimit` is now
`retentionLimit`. Update your tekton-pruner-default-spec ConfigMap
before upgrading.

Signed-off-by: John Developer <john@example.com>
Assisted-by: Claude Sonnet 4.6 (via Claude Code)
```

## Auto-Detection Summary

When generating commit messages:

1. Run `git status` (without `-uall` flag)
2. Run `git diff --cached` for staged changes
3. Identify primary component from staged file paths
4. If scope unclear, ask user
5. If user mentions a GitHub issue, add `Fixes #NNN` to body
6. Analyze staged files to determine commit type
7. Generate scope and description
8. Detect author info from env vars or git config
9. Ensure subject line is ≤50 characters (max 72)
10. Wrap body text at 72 characters per line
11. Add required trailers (`Signed-off-by` and `Assisted-by`)
12. **Display message and ask for user confirmation**
13. Only commit after receiving confirmation

## Additional Resources

- **`references/commit-types.md`** — complete type reference with examples
- [Tekton community conventions](https://github.com/tektoncd/community/blob/main/standards.md#commit-messages)
