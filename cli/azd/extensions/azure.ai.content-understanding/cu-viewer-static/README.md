# Bundled CU Viewer files

This directory contains the complete production browser page used by the Go
extension. It is not an npm consumer, a separate Go module, or a runtime cache.
Only the page, assets and licenses are embedded; this README and Go source are
not served to the browser.

## Current source

- Repository: cu-viewer.
- Source commit: `2d764a52146c0bcf9e6caeac2ea2da8dbe282394` (clean producer worktree).
- Frontend package version: `0.0.0` (local preview source, not a published npm dependency).
- Build command: `npm run build:viewer -- --configLoader runner`.
- Imported on: 2026-11-10.
- Payload: 226 plain files, including the root licenses.

This local preview includes the header-free shell, pointer-transparent temporary
previews, shared typography/colors, independent object expansion and the optional
azd field-order bridge. Fieldless results default to nonempty Markdown without
an empty Fields tab. Object previews show their value summary and the union
bounds of current-page descendant sources. Preview field names use regular
weight, Legend switches use compact visuals with unchanged hit areas, and
Fields Settings has a Confidence switch plus full-width highlighted actions
including Reset Field Order. This is a local development build, not an official
extension release.

Paragraphs now provides collapsible page groups, search, cyclic navigation and
page expansion settings. Info replaces Metadata with a read-only hierarchy of
returned metadata, usage, infos and warnings, including group expansion settings
and compact token-count summaries.

The complete page and assets were imported together. Third-party notice and
license text retains the source revision's canonical LF content; codec and
worker binaries are copied byte-for-byte.

The Go host keys saved field orders only by the full analyzer ID. Preference
keys do not contain endpoints; legacy endpoint-prefixed keys migrate while
preserving customized order and unrelated settings.

## Manual updates

1. Select a reviewed cu-viewer source revision and build its production page.
2. Copy the complete contents of `dist-app` here, including all JS/CSS chunks,
   PDF workers/fonts/CMaps/ICC/WASM and HEIF workers/decoder files.
3. Copy that source revision's `LICENSE` as `LICENSE.txt`, and its
   `THIRD-PARTY-NOTICES.txt` unchanged.
4. Preserve `embed.go` and this README. Replace the generated `assets` subtree
   as a complete set rather than leaving obsolete hashed chunks behind.
5. Update the source revision above, run the extension's tests and rebuild it.

Node/npm are needed only for the producer's frontend build. The extension
needs neither Node, npm nor Python to build or run after these files are supplied.
There is no downloader, ZIP extraction, resource cache or automatic update.

The extension's [Git ignore rules](../.gitignore) explicitly include this static
directory's `assets/pdfjs/build` runtime files. Other build outputs remain
ignored. Include `assets/pdfjs/build/pdf.worker.mjs` with normal `git add`
when updating the static page; no forced add is needed.

The extension's [Git attributes](../.gitattributes) preserve the compiled
payload byte-for-byte, including vendor notices. The Go wrapper and this README
remain normal LF text files with standard whitespace checks.
