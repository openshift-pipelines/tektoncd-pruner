---
name: filing-issues
description: >-
  File a well-crafted GitHub issue for tekton-pruner. Use when the user wants
  to report a bug, request a feature, propose a design change, or track a task.
  Searches for duplicates, asks clarifying questions, and creates the issue
  using the gh CLI following Tekton community conventions.
license: Apache-2.0
compatibility: Requires gh CLI with authentication
metadata:
  project: tekton-pruner
allowed-tools: Bash(gh issue:*) Bash(gh repo:*) Read
---

# Filing GitHub Issues

A good issue gives a reader everything they need to understand the problem
without prescribing a solution.

## Process

### 1. Identify the repository

Default repo: `tektoncd/pruner`. Confirm access:

```bash
gh repo view tektoncd/pruner
```

### 2. Search for duplicates

Before writing anything, search with at least two different query terms:

```bash
gh issue list --repo tektoncd/pruner --state all --search "<key terms>"
```

If a related issue exists, present it to the user. Do not file duplicates
without explicit go-ahead.

### 3. Ask clarifying questions

Identify what a stranger would need to understand the problem. Focus on:

- **Component**: controller, webhook, ConfigMap validation, metrics, e2e tests?
- **Trigger**: which ConfigMap field, annotation, resource type (PipelineRun/TaskRun)?
- **Observed vs expected**: what actually happens vs what should happen?
- **Version**: output of `kubectl get cm -n tekton-pipelines tekton-pruner-default-spec -o yaml`
  and `kubectl -n tekton-pipelines get deployment tekton-pruner-controller -o jsonpath='{.spec.template.spec.containers[0].image}'`
- **Reproduction**: is a minimal ConfigMap snippet reproducible?

Ask all questions in one message. Wait for answers.

### 4. Write the issue

**Title**: concise, scannable. Lead with the affected component.

Good: `pruner: TTL-based deletion fires on completed PipelineRuns with keep-history set`
Poor: `Issue with pruner not working`

**Body sections** (include only what applies):

- **What happens**: current behavior, error messages, logs
- **What should happen**: expected behavior
- **How to reproduce**: numbered steps from a clean state, include ConfigMap YAML
- **Context**: why it matters, who is affected

**What to leave out**: do not propose a fix in the issue body.

### 5. File the issue

After the user approves the draft:

```bash
gh issue create \
  --repo tektoncd/pruner \
  --title "<title>" \
  --body "$(cat <<'EOF'
<body>
EOF
)"
```

Return the issue URL.

## Constraints

- **Never file without user approval.** Present draft first.
- **Never propose a solution** in the issue body.
- **Never invent facts.** Ask if version or reproduction steps are missing.
- Match the issue template in `.github/ISSUE_TEMPLATE/`:
  `bug-report.md` for bugs, `feature-request.md` for features,
  `free-form.md` for anything else.
