# Embedded CU Viewer

This package hosts the production browser page built from
[changjian-wang/cu-viewer](https://github.com/changjian-wang/cu-viewer) at commit
`3c480f62d1ba69f06b107b32567eaf1fc48959c6`.

The checked-in `resources/viewer/viewer.zip` and `manifest.json` are exported
build artifacts. Their schema, file sizes, SHA-256 digests, and archive entries
are verified before use. The archive includes the Viewer license and all
third-party notices, PDF.js resources, Workers, and HEIF WebAssembly resources.
It is embedded in the Go executable and is never extracted to a user directory
or downloaded at runtime.

## Updating resources

Use a reviewed Viewer source revision and its existing build dependencies.
With sibling `cu-viewer` and `azure-dev` checkouts, run from `cu-viewer`:

```powershell name="Build and export Go Viewer resources"
npm run export:cli -- ..\azure-dev\cli\azd\extensions\azure.ai.content-understanding\internal\viewer\resources\viewer
```

The shared exporter builds the production CLI page and deterministically writes
the archive and manifest, retaining licenses. Node.js and Python are needed only
for this explicit update, not for Go builds or installed use. The exporter
rejects unknown or modified destination resources rather than overwriting them.
Commit both generated files and update the source revision above together.

The Go host supplies the existing page-relative `session.json` contract:
`source` is a same-session source URL, `name` is its display name, and `result`
is the complete original CU response. This package does not analyze, authenticate,
upload, or change the shared frontend or Python package.
