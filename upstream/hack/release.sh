#!/usr/bin/env bash

set -e

RELEASE_VERSION="$1"

DOCKER_CMD=${DOCKER_CMD:-docker}

BINARIES="gh"

info() {
  echo "INFO: $@"
}

err() {
  echo "ERROR: $@"
}

getReleaseVersion() {
  [[ -z ${RELEASE_VERSION} ]] && {
      read -r -e -p "Enter a target release (i.e: v0.1.2): " RELEASE_VERSION
      [[ -z ${RELEASE_VERSION} ]] && {
        echo "no target release"
        exit 1
      }
    }
    [[ ${RELEASE_VERSION} =~ v[0-9]+\.[0-9]*\.[0-9]+ ]] || {
      echo "invalid version provided, need to match v\d+\.\d+\.\d+"
      exit 1
    }
}

buildImageAndGenerateReleaseYaml() {
    RELEASE_YAML_FILE=$1
  	info Creating  Release Yaml
  	echo "------------------------------------------"

    echo "# Generated for Release $RELEASE_VERSION" > $RELEASE_YAML_FILE
  	for file in config/*.yaml; do
  	  echo >> $RELEASE_YAML_FILE
      cat "$file" >> $RELEASE_YAML_FILE
    done
    info "Update Release version to $RELEASE_VERSION in $RELEASE_YAML_FILE"
    sed -i. "s/version: devel/version: \"$RELEASE_VERSION\"/g" $RELEASE_YAML_FILE
    echo "------------------------------------------"
}

createNewPreRelease() {
  ASSETS=$1
  echo "Creating New Pre-Release from $ASSETS"
  REPO=$(gh repo view --json nameWithOwner --jq '.nameWithOwner')

  TAG=$RELEASE_VERSION
  # Check if the release exists
  EXISTING_RELEASE=$(gh release view "$TAG" --repo "$REPO" --json id --jq '.id' 2>/dev/null || echo "")
  echo "Checking for existing release..."
  # If release exists, delete it
  if [ -n "$EXISTING_RELEASE" ]; then
      echo "Existing release found. Deleting..."
      gh release delete "$TAG" --repo "$REPO" --yes
  fi

  # Ensure the tag is deleted
  EXISTING_TAG=$(gh api repos/$REPO/git/refs/tags/$TAG --jq '.ref' 2>/dev/null || echo "")
  echo "EXISTING_TAG: $EXISTING_TAG"
  if [ -n "$EXISTING_TAG" ]; then
      gh api --method DELETE "/repos/$REPO/git/refs/tags/$TAG" || echo "Failed to delete tag (may not exist)"
  fi

  # Create new release
  echo "Creating new release: $TAG"
  gh release create "$TAG" \
      --repo "$REPO" \
      --title "$TITLE" \
      --notes "$NOTES"

  # Upload assets
  for ASSET in "${ASSETS[@]}"; do
      if [ -f "$ASSET" ]; then
          echo "Uploading asset: $ASSET"
          set -x
          gh release upload "$TAG" "$ASSET" --repo "$REPO"
          set +x
      else
          echo "Warning: Asset not found - $ASSET"
      fi
  done

  echo "✅ Release $TAG created successfully!"

}

createNewBranchAndPush() {
  git checkout -b release-${RELEASE_VERSION}
  git push origin release-${RELEASE_VERSION}
}

main() {

  # Check if all required command exists
  for b in ${BINARIES};do
      type -p ${b} >/dev/null || { echo "'${b}' need to be avail"; exit 1 ;}
  done

  # Ask the release version to build images
  getReleaseVersion
  RELEASE_YAML_FILE=release-${RELEASE_VERSION}.yaml

  buildImageAndGenerateReleaseYaml $RELEASE_YAML_FILE
  createNewPreRelease $RELEASE_YAML_FILE
#  createNewBranchAndPush

  echo "************************************************************"
  echo    Release Created successfully
  echo "************************************************************"
}

main $@