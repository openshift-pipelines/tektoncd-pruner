# Tekton Pruner Official Release Cheat Sheet

These steps provide a no-frills guide to performing an official release
of Tekton Pruner. Releases are now largely automated via
[Pipelines-as-Code](https://pipelinesascode.com) (PAC) on the `oci-ci-cd`
cluster.

## How releases work

### Initial releases (e.g. v0.4.0)

1. Create a release branch named `release-v<major>.<minor>.x` (e.g.
   `release-v0.4.x`) from the desired commit on `main`.
2. PAC automatically detects the branch creation and triggers the release
   pipeline defined in `.tekton/release.yaml`.
3. The version is derived from the branch name: `release-v0.4.x` → `v0.4.0`.
4. Monitor the PipelineRun on the
   [Tekton Dashboard](https://tekton.infra.tekton.dev/#/namespaces/releases-pruner/pipelineruns).

### Patch releases (e.g. v0.3.6)

Patch releases happen in two ways:

- **Automatically**: A weekly cron (Thursday 10:00 UTC) in
  `.github/workflows/patch-release.yaml` scans all `release-v*` branches.
  If new commits exist since the last tag, it triggers a patch release via
  PAC incoming webhook.
- **Manually**: Run the "Patch Release" workflow from GitHub Actions
  (`workflow_dispatch`) with the branch and version as inputs.

Both methods trigger the release pipeline defined in `.tekton/release-patch.yaml`
on the `oci-ci-cd` cluster.

## Post-release steps

Once the release pipeline completes successfully:

1. Check the PipelineRun results on the
   [Tekton Dashboard](https://tekton.infra.tekton.dev/#/namespaces/releases-pruner/pipelineruns):

    ```
    📝 Results

    NAME                    VALUE
    ∙ commit-sha            ff6d7abebde12460aecd061ab0f6fd21053ba8a7
    ∙ release-file           https://infra.tekton.dev/tekton-releases/pruner/previous/v0.3.6/release.yaml
    ∙ release-file-no-tag    https://infra.tekton.dev/tekton-releases/pruner/previous/v0.3.6/release.notags.yaml
    ```

1. Create a GitHub release announcement:

    1. Find the Rekor UUID for the release

    ```bash
    PRUNER_VERSION=v0.3.6  # Set to the released version
    RELEASE_FILE=https://infra.tekton.dev/tekton-releases/pruner/previous/${PRUNER_VERSION}/release.yaml
    CONTROLLER_IMAGE_SHA=$(curl -L $RELEASE_FILE | sed -n 's/"//g;s/.*ghcr\.io.*controller.*@//p;')
    REKOR_UUID=$(rekor-cli search --sha $CONTROLLER_IMAGE_SHA | grep -v Found | head -1)
    echo -e "CONTROLLER_IMAGE_SHA: ${CONTROLLER_IMAGE_SHA}\nREKOR_UUID: ${REKOR_UUID}"
    ```

    1. Execute the Draft Release Pipeline.

        [Setup a context to connect to the dogfooding cluster](#setup-dogfooding-context) if you haven't already.

        ```bash
        WORKSPACE_TEMPLATE=$(mktemp /tmp/workspace-template.XXXXXX.yaml)
        cat <<'EOF' > $WORKSPACE_TEMPLATE
        spec:
         accessModes:
         - ReadWriteOnce
         resources:
           requests:
             storage: 1Gi
        EOF

        POD_TEMPLATE=$(mktemp /tmp/pod-template.XXXXXX.yaml)
        cat <<'EOF' > $POD_TEMPLATE
        securityContext:
          fsGroup: 65532
          runAsUser: 65532
          runAsNonRoot: true
        EOF
        ```

        ```bash
        PRUNER_RELEASE_GIT_SHA=<commit-sha-from-results>
        PRUNER_OLD_VERSION=v0.3.5  # Previous version

        tkn pipeline start \
          --workspace name=shared,volumeClaimTemplateFile="${WORKSPACE_TEMPLATE}" \
          --workspace name=credentials,secret=oci-release-secret \
          --pod-template "${POD_TEMPLATE}" \
          -p package="tektoncd/pruner" \
          -p git-revision="$PRUNER_RELEASE_GIT_SHA" \
          -p release-tag="${PRUNER_VERSION}" \
          -p previous-release-tag="${PRUNER_OLD_VERSION}" \
          -p release-name="Tekton Pruner" \
          -p repo-name="pruner" \
          -p bucket="tekton-releases" \
          -p rekor-uuid="$REKOR_UUID" \
          release-draft-oci
        ```

    1. On successful completion, visit the logged URL and review the release notes.
       Manually add upgrade and deprecation notices, verify the commit list, then
       publish the GitHub release.

1. Edit `releases.md` on the `main` branch, add an entry for the release.
   - In case of a patch release, replace the latest release with the new one,
     including links to docs and examples. Append the new release to the list
     of patch releases as well.
   - In case of a minor or major release, add a new entry for the
     release, including links to docs and example
   - Check if any release is EOL, if so move it to the "End of Life Releases"
     section

1. If the release introduces a new minimum version of Kubernetes required,
   edit `README.md` on `main` branch and add the new requirement in the
   "Required Kubernetes Version" section.

1. Push & make PR for updated `releases.md` and `README.md`.

1. Test release that you just made against your own cluster:

    ```bash
    # Test latest
    kubectl apply --filename https://infra.tekton.dev/tekton-releases/pruner/latest/release.yaml
    ```

    ```bash
    # Test backport
    kubectl apply --filename https://infra.tekton.dev/tekton-releases/pruner/previous/v0.3.6/release.yaml
    ```

1. For major releases, update the [website sync configuration](https://github.com/tektoncd/website/blob/main/sync/config/pruner.yaml)
   to include the new release.

1. Announce the release in Slack channels #general, #announcements and #pruner.

Congratulations, you're done!

## Manual release (fallback)

If the automated pipeline is unavailable, you can trigger a release manually
using `tkn`:

```bash
WORKSPACE_TEMPLATE=$(mktemp /tmp/workspace-template.XXXXXX.yaml)
cat <<'EOF' > $WORKSPACE_TEMPLATE
spec:
 accessModes:
 - ReadWriteOnce
 resources:
   requests:
     storage: 1Gi
EOF

tkn --context dogfooding pipeline start pruner-release \
  --param package=github.com/tektoncd/pruner \
  --param repoName="pruner" \
  --param gitRevision="<COMMIT_SHA>" \
  --param imageRegistry=ghcr.io \
  --param imageRegistryPath=tektoncd/pruner \
  --param imageRegistryRegions="" \
  --param imageRegistryUser=tekton-robot \
  --param serviceAccountImagesPath=credentials \
  --param versionTag="<VERSION>" \
  --param releaseBucket=tekton-releases \
  --param koExtraArgs="" \
  --workspace name=release-secret,secret=oci-release-secret \
  --workspace name=release-images-secret,secret=ghcr-creds \
  --workspace name=workarea,volumeClaimTemplateFile="${WORKSPACE_TEMPLATE}" \
  --tasks-timeout 2h \
  --pipeline-timeout 3h
```

## Setup dogfooding context

1. Configure `kubectl` to connect to
   [the dogfooding cluster](https://github.com/tektoncd/plumbing/blob/main/docs/dogfooding.md):

   The dogfooding cluster is currently an OKE cluster in oracle cloud. we need the Oracle Cloud CLI client. Install oracle cloud cli (https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm) 

    ```bash
    oci ce cluster create-kubeconfig --cluster-id <CLUSTER-OCID> --file $HOME/.kube/config --region <CLUSTER-REGION> --token-version 2.0.0  --kube-endpoint PUBLIC_ENDPOINT
    ```

1. Give [the context](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/)
   a short memorable name such as `dogfooding`:

   ```bash
   kubectl config rename-context $(kubectl config current-context) dogfooding
   ```

1. **Important: Switch `kubectl` back to your own cluster by default.**

    ```bash
    kubectl config use-context my-dev-cluster
    ```

## Cherry-picking commits for patch releases

The easiest way to cherry-pick a commit into a release branch is to use the "cherrypicker" plugin (see https://prow.tekton.dev/plugins for documentation).
To use the plugin, comment "/cherry-pick <branch-to-cherry-pick-onto>" on the pull request containing the commits that need to be cherry-picked.
Make sure this command is on its own line, and use one comment per branch that you're cherry-picking onto.
Automation will create a pull request cherry-picking the commits into the named branch, e.g. `release-v0.3.x`.

The cherrypicker plugin isn't able to resolve merge conflicts. If there are merge conflicts, you'll have to manually cherry-pick following these steps:
1. Fetch the branch you're backporting to and check it out:
```sh
git fetch upstream <branchname>
git checkout upstream/<branchname>
```
1. (Optional) Rename the local branch to make it easier to work with:
```sh
git switch -c <new-name-for-local-branch>
```
1. Find the 40-character commit hash to cherry-pick. Note: automation creates a new commit when merging contributors' commits into main.
You'll need to use the hash of the commit created by tekton-robot.

1. [Cherry-pick](https://git-scm.com/docs/git-cherry-pick) the commit onto the branch:
```sh
git cherry-pick <commit-hash>
```
1. Resolve any merge conflicts.
1. Finish the cherry-pick:
```sh
git add <changed-files>
git cherry-pick --continue
```
1. Push your changes to your fork and open a pull request against the upstream branch.
