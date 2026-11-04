# Runtime CU Viewer

The Go extension retrieves the compiled production page from the public
[cu-viewer releases](https://github.com/changjian-wang/cu-viewer/releases).
It does not embed a frontend copy or require Node.js, Python, npm, or gh at
runtime. Downloading resources never uploads the source document, CU result or
Azure credentials.

## Preparation and updates

`--view` validates the local source and prepares Viewer resources before
authentication or analysis. A verified compatible cache is reused, with an
update-notification check at most once every 24 hours. On first use, or after detecting corrupt/incompatible cached
resources, the command explains what it is preparing on stderr, downloads,
verifies and caches a compatible release, then starts the local viewing flow.

Updates are explicit:

```powershell name="Prepare or update the runtime Viewer"
azd ai cu viewer update
azd ai cu viewer update --output json
```

The update command does not analyze or authenticate against Azure. Its version,
protocol and cache location are the result output; preparation progress goes to
stderr. A failed update leaves the previous active cache intact.

Ordinary `--view` checks release metadata at most once every 24 hours, using a
three-second timeout. A newer compatible version produces a diagnostic notice
with the current version, available version, and `azd ai cu viewer update`
command. Known update notices continue on later invocations without another
network request until an update is applied or check information is refreshed.
The check never downloads a new archive or changes the active Viewer.

Check failures produce a warning and keep a valid cached Viewer usable offline;
failed checks are also limited to once per 24 hours when their metadata can be
saved. A check record that cannot be read or written produces an explicit warning,
not a failure of cached viewing. A successful first download or manual update
records a fresh check, so immediate subsequent uses do not query releases again.
Progress and notices remain on the injected diagnostic writer, not result JSON,
including with `--output none` or `--no-prompt`.

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
only versioned resource archives/manifests, an atomic active-version record,
an update-check record, and a cross-process download/check lock, not documents or analysis responses. Existing
anchored directory/ACL/symlink checks and atomic file publication are reused.
Concurrent first uses share a single download, and concurrent cached uses share
a daily update check. The active pointer changes only
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

The first public resource prerelease is
[v0.1.0-preview.1](https://github.com/changjian-wang/cu-viewer/releases/tag/v0.1.0-preview.1),
from Viewer source `9b1667703502ff6dd3035c644a7b4377fb6ac054`, with 225 production
files, schema 2 and protocol 1. Its public ZIP and manifest downloads were
independently verified against the built candidate. It uses the committed UI;
the detached single-toolbar preview is not included.

Source push, resource publication and local extension replacement have separate
authorization boundaries. The initial local rollout authorized publication and
CU-only replacement after backup; azd and the developer extension were unchanged.
Local HTTP fixtures continue to cover offline/integrity/error paths. Actual
public first-use still requires GitHub availability and anonymous API quota;
HTTP errors must abort before analysis, not trigger reanalysis or unverified
cache population.

The real installed-host rollout completed after an anonymous API limit reset:
public cold download/cache validation, one successful single-page invoice
analysis with 31 fields, original saved-JSON/session equality and original-PDF
byte equality, then real browser field selection, source highlight, 150% zoom
and original Result view. Explicit resource update also succeeded without
changing the active resource pointer or repeating analysis. Only the CU
extension was replaced; azd and the developer extension retained their hashes.

The foreground command's redirected console files had not flushed during the
live session, so saved output and HTTP/browser data were checked independently;
this does not claim that a redirected stdout stream had already completed.
Local session URLs, Azure credentials and analysis results are not published
as part of the Viewer resources or these source notes.
