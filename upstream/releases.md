# Tekton Pruner Releases

## Release Frequency

Tekton Pruner follows the Tekton community
[release policy](https://github.com/tektoncd/community/blob/main/releases.md)
as follows:

- Versions are numbered according to semantic versioning: `vX.Y.Z`
- A new release is produced on a monthly basis, when enough content is available
- Four releases a year are chosen for
  [long term support (LTS)](https://github.com/tektoncd/community/blob/main/releases.md#support-policy).
  All remaining releases are supported for approximately 1 month (until the
  next release is produced)
  - LTS releases take place in January, April, July and October every year
  - Releases usually happen towards the middle of the month, but the exact date
    may vary, depending on week-ends and readiness

## Release Process

Tekton Pruner releases are made of YAML manifests and container images.
Manifests are published to cloud object-storage as well as
[GitHub](https://github.com/tektoncd/pruner/releases). Container images are
signed by [Sigstore](https://sigstore.dev) via
[Tekton Chains](https://github.com/tektoncd/chains); signatures can be verified
through the
[public key](https://github.com/tektoncd/chains/blob/main/tekton.pub) hosted by
the Tekton Chains project.

Further documentation available:

- The Tekton Pruner [release process](./tekton/release-cheat-sheet.md)
- [Installing Tekton Pruner](./docs/tutorials/getting-started.md)
- Standard for
  [release notes](https://github.com/tektoncd/community/blob/main/standards.md#release-notes)

## Releases

### v0.3

- **Latest Release**: [v0.3.5](https://github.com/tektoncd/pruner/releases/tag/v0.3.5)
  (2025-12-23)
  ([docs](https://github.com/tektoncd/pruner/tree/v0.3.5/docs),
  [tutorials](https://github.com/tektoncd/pruner/tree/v0.3.5/docs/tutorials/README.md))
- **Initial Release**: [v0.3.0](https://github.com/tektoncd/pruner/releases/tag/v0.3.0)
  (2025-11-07)
- **Patch Releases**: [v0.3.0](https://github.com/tektoncd/pruner/releases/tag/v0.3.0),
  [v0.3.1](https://github.com/tektoncd/pruner/releases/tag/v0.3.1),
  [v0.3.3](https://github.com/tektoncd/pruner/releases/tag/v0.3.3),
  [v0.3.4](https://github.com/tektoncd/pruner/releases/tag/v0.3.4),
  [v0.3.5](https://github.com/tektoncd/pruner/releases/tag/v0.3.5)

### Required Kubernetes Version

- Starting from the v0.3.x release of Pruner: **Kubernetes version 1.27 or later**

## Older Releases

Older releases including pre-releases are available on
[GitHub](https://github.com/tektoncd/pruner/releases).


