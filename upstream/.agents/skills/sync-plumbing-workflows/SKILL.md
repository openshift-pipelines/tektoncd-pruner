---
name: sync-plumbing-workflows
description: >-
  Sync reusable workflow digests from tektoncd/plumbing into this repo's
  .github/workflows. Use when updating pinned SHA digests for workflows
  sourced from tektoncd/plumbing (e.g. _chatops_retest.yml,
  _cherry-pick-command.yaml), or when checking whether local workflow pins
  are behind the current main branch of the plumbing repo.
license: Apache-2.0
compatibility: Requires git, gh CLI, sed (GNU or BSD), and internet access to github.com
metadata:
  project: tekton-pruner
allowed-tools: Bash(git:*) Bash(gh api:*) Bash(grep:*) Bash(sed:*) Read Glob
---

# Sync Plumbing Workflow Digests

tekton-pruner references reusable workflows from
[tektoncd/plumbing](https://github.com/tektoncd/plumbing). These are pinned to
SHA digests for supply-chain security. This skill resolves the latest commit
SHA on `main` for each referenced plumbing workflow and updates the pins in
`.github/workflows/`.

## Background

Workflow references look like:

```yaml
uses: tektoncd/plumbing/.github/workflows/_chatops_retest.yml@1cf2292a30268252a734b413b8a44f15c04d1de8 # main
uses: tektoncd/plumbing/.github/workflows/_cherry-pick-command.yaml@9f8e1781d5cc431e5c95190a9fc14dfba1cec391 # main
```

The SHA after `@` must be the full 40-character commit hash. The `# main`
comment documents the intended branch.

## Process

### 1. Find all plumbing workflow references in this repo

```bash
grep -rn "tektoncd/plumbing/.github/workflows" .github/workflows/
```

Expected output lists each file and the current pinned SHA.

### 2. Resolve the latest commit SHA on plumbing main

Use the GitHub API (no clone needed):

```bash
gh api repos/tektoncd/plumbing/commits/main --jq '.sha'
```

This returns the HEAD SHA of `main`. All reusable workflows in plumbing are
versioned together — there is no per-file SHA; the commit SHA covers the whole
repo tree.

To get the SHA for a specific workflow path (confirms the file still exists
at that commit):

```bash
# Check that the workflow file exists on main
gh api "repos/tektoncd/plumbing/contents/.github/workflows/_chatops_retest.yml?ref=main" --jq '.sha'

gh api "repos/tektoncd/plumbing/contents/.github/workflows/_cherry-pick-command.yaml?ref=main" --jq '.sha'
```

> **Note:** The `.sha` from the contents API is the blob SHA (not the commit
> SHA). Use the commit SHA from step 2 for the `uses:` pin — that is what
> GitHub Actions validates.

### 3. Compare current pins to latest

```bash
LATEST=$(gh api repos/tektoncd/plumbing/commits/main --jq '.sha')
echo "Latest plumbing main: $LATEST"
echo ""
echo "Current pins in this repo:"
grep -rh "tektoncd/plumbing" .github/workflows/ | grep -oE '@[0-9a-f]{40}' | sort -u
```

If the current pin already matches `$LATEST`, no update is needed.

### 4. Update all plumbing workflow pins

Replace all occurrences of the old SHA with the new one across every workflow
file:

```bash
OLD=$(grep -rh "tektoncd/plumbing" .github/workflows/ | grep -oE '@[0-9a-f]{40}' | sort -u | head -1)
NEW=$(gh api repos/tektoncd/plumbing/commits/main --jq '.sha')

echo "Updating $OLD → $NEW"

# Dry run first
grep -rln "tektoncd/plumbing" .github/workflows/

# Apply (portable across macOS and Linux)
for f in $(grep -rln "tektoncd/plumbing" .github/workflows/); do
  sed -i.bak "s|tektoncd/plumbing/.github/workflows/\(.*\)@${OLD}|tektoncd/plumbing/.github/workflows/\1@${NEW}|g" "$f"
  rm -f "${f}.bak"
  echo "Updated $f"
done
```

### 5. Verify the changes

```bash
grep -rn "tektoncd/plumbing" .github/workflows/
```

All occurrences should now show the new SHA, with `# main` comment preserved.

### 6. Commit the update

Use a conventional commit message:

```bash
git add .github/workflows/
git commit -m "chore: update tektoncd/plumbing workflow pins to $(gh api repos/tektoncd/plumbing/commits/main --jq '.sha[0:12]')"
```

Open a PR following normal contribution process.

## Checking Other Pinned Actions

Other pinned actions in `.github/workflows/` (e.g. `actions/checkout`,
`actions/setup-go`, `golangci/golangci-lint-action`) are managed separately.
This skill covers **only `tektoncd/plumbing` references**.

To audit all pinned actions in the repo at once:

```bash
grep -rh "uses:" .github/workflows/ | grep -v "^#" | sort -u
```

For a full dependency update including non-plumbing actions, use Dependabot or
the [zizmor](https://github.com/zizmorcore/zizmor) security scanner already
configured in `.github/workflows/zizmor.yaml`.

## References

- [tektoncd/plumbing](https://github.com/tektoncd/plumbing) — shared workflow sources
- [GitHub Actions: reusable workflows](https://docs.github.com/en/actions/sharing-automations/reusing-workflows)
- [OpenSSF Scorecard: pinned dependencies](https://github.com/ossf/scorecard/blob/main/docs/checks.md#pinned-dependencies)
