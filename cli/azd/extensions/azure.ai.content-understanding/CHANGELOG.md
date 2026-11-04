# Release History

## 0.1.0-preview (Unreleased)

- Notify about newer compatible Viewer releases during `--view`, with checks
  limited to once every 24 hours. Resource upgrades remain manual.
- View one analyzed local PDF or supported image in the browser Viewer
  with `azd ai cu analyze --view`, without changing existing result output.
- Initial preview of Azure Content Understanding commands under `azd ai cu`,
  using Azure Developer CLI sign-in and environments.
- Analyze documents and media from local files or URLs, including batch
  processing and saved results. Create, manage, validate, and test custom
  analyzers.
- Save global CU defaults, configure resource model mappings, check setup
  readiness, and generate an azd/Bicep project for resource provisioning and
  model setup.
