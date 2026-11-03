# Runtime CU Viewer

The Go extension retrieves the compiled production page from the public
[cu-viewer releases](https://github.com/changjian-wang/cu-viewer/releases).
It does not embed a frontend copy or require Node.js, Python, npm, or gh at
runtime. Downloading resources never uploads the source document, CU result or
Azure credentials.

## Preparation and updates

`--view` validates the local source and prepares Viewer resources before
authentication or analysis. A verified compatible cache is used directly without
network requests. On first use, or after detecting corrupt/incompatible cached
resources, the command explains what it is preparing on stderr, downloads,
verifies and caches a compatible release, then starts the local viewing flow.

Updates are explicit:

```powershell name="Prepare or update the runtime Viewer"
azd ai cu viewer update
azd ai cu viewer update --output json
```

The update command does not analyze or authenticate against Azure. Its version,
protocol and cache location are the result output; preparation progress goes to
stderr. A failed update leaves the previous active cache intact. Ordinary
`--view` does not silently check for or switch to newer releases.

## Release contract

The configured source is the public `changjian-wang/cu-viewer` GitHub repository.
The resolver inspects the latest 100 public release entries, skips drafts and
releases without both `manifest.json` and `viewer.zip`, and considers semantic
version tags (normally `v<version>`) in descending order. Compatible prereleases
are accepted by this preview CLI. The first valid protocol-1 manifest is used;
an incompatible protocol is skipped rather than downloading its archive.

The manifest uses schema version 2 and declares `viewerVersion`,
`protocolVersion: 1`, archive byte count/SHA-256 and every file's path, size and
SHA-256. The Viewer version must match the release tag. The original HTML,
Workers, PDF.js/HEIF assets and licenses must all be included. HTTPS and a fixed
release source provide transport/source trust; hash checks verify integrity,
not an additional publisher signature.

Downloads are bounded (1 MiB manifest, 32 MiB archive, 128 MiB expanded content,
4096 files), cancellable and time-limited. Paths, entry types, counts, sizes and
hashes are checked before activation. Unsafe or incomplete archives are never
served. There is no embedded-asset fallback.

## Cache and local serving

Resources live under `os.UserCacheDir()` in
`azure-ai-content-understanding\viewer\protocol-1`. The private cache contains
only versioned resource archives/manifests, an atomic active-version record and
a cross-process download lock, not documents or analysis responses. Existing
anchored directory/ACL/symlink checks and atomic file publication are reused.
Concurrent first uses share a single download. The active pointer changes only
after a complete valid snapshot is stored; previously cached versions are not
deleted by an unsuccessful update.

Validated resources are decoded into memory and served on loopback with the
existing CSP and page-relative `session.json` contract: `source` is a same-session
URL, `name` its display name, and `result` the complete original CU response.
No archive is extracted into the document directory.

If preparation fails, stderr explains that no analysis was started; the command
returns nonzero without opening a browser. If analysis completes but the Viewer
then fails to open, the command explicitly says analysis completed, retains
existing stdout/saved files, and does not analyze again.

## Publisher and validation boundary

Build a versioned resource pair from the reviewed Viewer source, not this Go
repository:

```bash name="Create a local Viewer release candidate"
npm run package:release -- --viewer-version 0.1.0-preview.1
```

The pair is generated directly under `dist-viewer` in cu-viewer and is not
checked into the Go extension. A future release must publish both files on the
matching version tag after source review. Go, Python and future C# hosts use the
same Release assets/protocol rather than bundling language-specific frontend
copies; TypeScript hosts may consume the npm component package instead.

Publishing tools require Node.js, Bash 4+, zip/unzip, jq and GNU coreutils/findutils,
not Python or PowerShell. CI builds the resource pair on Linux; Windows maintainers
can use Git Bash with those tools installed. Its container bytes/digest may differ
from the former exporters; decoded payloads and schema 2/protocol 1 remain compatible.
Use the generated manifest's digest and never replace an already published version.

The Viewer repository no longer ships a Python host package/API. Each native CLI
owns download/cache/local serving and lifecycle in its own repository. Removing that
package does not implement or migrate the Python/C# CLI hosts.

Source commit/push is authorized separately from resource publication and
installed-extension replacement. No GitHub Release was published, and the
repository currently has none. Therefore a cold
public-source request correctly reports that no compatible release is available
until the first resource release is published. Success/offline/error flows are
verified using local HTTP release fixtures, including an actual 225-file build;
that is not a claim that public downloads are already operational.
