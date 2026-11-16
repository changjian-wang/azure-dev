# Bundled CU Viewer files

This directory contains the complete production browser page used by the Go
extension. It is not an npm consumer, a separate Go module, or a runtime cache.
Only the page, assets and licenses are embedded; this README and Go source are
not served to the browser.

## Current source

- Repository: cu-viewer.
- Source commit: `f18dd733290c6f255352149f05525339c361703c` on `feature/cu-analyze-view`.
- Frontend package version: `0.0.0` (local preview source, not a published npm dependency).
- Build command: `npm run build:viewer`, using Node.js 22.23.5 and the existing dependencies; manifests and lockfile are unchanged.
- Imported on: 2026-11-16.
- Payload: 229 plain files, including the root licenses.
- Payload index SHA-256: `98d03550dd4cf4a47900f113631b8b47325ab2b5f75182a20dfa7aeb2ff11f37`.
- Index format: UTF-8 entries sorted by relative POSIX path, each containing the path, NUL, lowercase file SHA-256 and LF. Only served payload files are included, not this README or Go source.

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

Label sessions use the existing Fields tab, not a separate Label tab. The
`label show` host discovers the adjacent saved result automatically, with `view`
retained as an alias. Comparable inputs show Field Name, Label Value and Extracted
Value, including side-by-side array tables by default and whole-array status
filtering. Labels without comparable results remain a two-column reference view.
The per-array Stacked / Side by side controls temporarily change the layout;
refreshing or replacing inputs restores side by side without writing preferences.

Paired array tables share bidirectional horizontal scrolling and restore one
column position within the available range after tab switches or collapse.
Side-by-side tables also synchronize vertical scrolling to align rows; stacked
tables retain independent vertical scrolling. Other array groups remain
independent. The fixed `#` corner stays visible above scrolling column headers
and row numbers.

The label-session palette uses [MUI colors](https://mui.com/material-ui/customization/color/#color-palette):
blue 50/100 for default scalar/object and array values, green 100 for matches,
red 100 for differences/extras and red 200 for label-only values. Actual extracted
values can carry an independent red 800 outline at 80% opacity. Both-missing slots
are transparent, never receive confidence outlines and do not change statistics.
Zero, false, empty strings and explicit null remain valid values.

The low-confidence Legend example shows a value with its matching fill plus
the red border, not an empty white state. Label-only interaction strokes use
deep purple 600/800 and leave 3 CSS px around confidence edges, including their
antialiasing. Raw source geometry and hit targets are unchanged. Flyout banners
and arrows use the status color, with label above extraction; returned confidence
metadata stays visible when outlines are disabled. No MUI dependency is added.

Ordinary results without labels retain the pre-comparison confidence fills,
violet preview headings, four-color Legend, optional Confidence column and
single array table. Label-only styles do not leak into that mode. Unsaved
comparison layouts use a 45% Source / 55% results split; stored layouts remain
authoritative, and ordinary/reference defaults are unchanged.

Comparison search accepts readable names and parent paths, including literal
slashes and tildes. Both array summaries preview their own grounded descendants;
missing sources are explicitly identified rather than borrowed from the other
side. Paired tables retain shared columns and synchronized horizontal scrolling.

Label-only missing regions use the label's own PDF dimensions; extraction pixels
and label inches are not mixed. Optional session warnings explain invalid-result
fallbacks. Label sessions never use analyzer-preference RPCs. The Go host and
these complete assets must be updated together; input documents and results are
not part of this payload.

Label source frames additionally merge adjacent word/punctuation fragments and
use a thin, outward-offset decoration at small zoom. This does not change raw
source polygons, hit targets, PDF text or existing Fields frame styling.
The same comparison field's extracted region does not block the label's own
fragments from merging. Different fields still block unsafe unions; label frames
never borrow the extraction's coordinates.

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
