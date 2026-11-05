# Bundled CU Viewer

The Go extension embeds the complete production page from
[cu-viewer-static](../../cu-viewer-static). It needs no Node.js, npm or Python
to build or run after those files are supplied. There is no Release resolver,
downloader, resource archive, cache, daily update check or `viewer update`
command. Old user caches are neither used nor automatically removed.

## Responsibilities

- `assets.go` checks that the embedded entry page and licenses are available.
- `source.go` validates the local source using its contents, not its extension.
- `viewer.go` owns preparation, the local HTTP session, response delivery and shutdown.
- `browser.go` opens the system browser using a fixed platform launcher.

`Prepare` checks the local source, bundled resources and loopback port before
authentication or analysis. `Show` receives the complete original response,
checks that the source has not changed, verifies server readiness, opens the
browser and waits for cancellation. `Close` releases the listener, connections
and source handle. Existing CLI output formats and the analysis flow are unchanged.

## Serving contract

Each session binds `127.0.0.1` on a dynamic port and uses an unpredictable path.
Only embedded files, the selected source and `session.json` are served; no
directory listings, working-directory files, Go source, credentials or profiles
are exposed. Source byte ranges and HEAD requests remain supported.

The page-relative session JSON contains `source` (a same-session source URL),
`name` and `result` (the original CU JSON, including unknown fields and exact
numbers). The browser runs React; the Go process only provides files and data.

Host/Origin checks, GET/HEAD-only handling, CSP, correct JS/Worker/WASM MIME,
`no-store`, `nosniff`, `no-referrer` and same-origin resource policy are retained.
The command remains in the foreground until Ctrl+C; closing a tab is not a
shutdown signal. Viewing makes no additional Azure calls or external resource
requests.

Missing resources fail explicitly before analysis or browser startup. Failure
after analysis preserves completed results and never triggers a second analysis.
Source/result data and capability URLs are not part of the compiled payload.

## Refresh and validation

Use the reviewed producer revision and manual procedure in
[the static directory](../../cu-viewer-static/README.md). Keep all fonts,
codecs, workers, lazy JS/CSS chunks and licenses together. Updating the page
requires rebuilding the extension, not clearing or updating a user cache.
Use trusted build inputs; runtime ZIP/hash-manifest validation no longer applies.

From the extension directory:

```powershell name="Validate the local Viewer"
go test .\internal\viewer .\internal\cmd .\internal\exterrors .\cu-viewer-static
go vet .\internal\viewer .\internal\cmd .\internal\exterrors .\cu-viewer-static
go build .
```

An opt-in bounded browser fixture uses the compiled static page and saved local
data without authentication or analysis:

```powershell name="Serve a saved browser fixture"
$env:CU_VIEWER_TEST_SOURCE = 'C:\path\sample.pdf'
$env:CU_VIEWER_TEST_RESULT = 'C:\path\sample.result.json'
go test .\internal\viewer -run '^TestStaticViewerBrowserFixture$' -v -count=1
```

With `CU_VIEWER_TEST_STOP` set to a nonexistent local file, the fixture logs
its ready URL and remains available for browser automation until that file is
created, with a two-minute limit. Without it, HTTP/session assertions complete
and the fixture shuts down immediately. This is test tooling, not a new CLI
command, runtime downloader or installed-extension replacement.
