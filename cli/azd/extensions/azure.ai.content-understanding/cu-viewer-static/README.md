# Bundled CU Viewer files

This directory contains the complete production browser page used by the Go
extension. It is not an npm consumer, a separate Go module, or a runtime cache.
Only the page, assets and licenses are embedded; this README and Go source are
not served to the browser.

## Current source

- Repository: cu-viewer.
- Source commit: `7da11868c9e6a583e89e591e3acc20ac955ac0f1` on `feature/cu-analyze-view`.
- Frontend package version: `0.0.0` (local preview source, not a published npm dependency).
- Build command: `npm run build:viewer`, using Node.js 24.22.1 and the committed npm lockfile in an isolated build.
- Imported on: 2026-11-19.
- Payload: 229 plain files, including the root licenses.
- Payload index SHA-256: `880446b11f2ea757a53e94754ec992f5cc07804ef54c739b71c8fa8b4d6a9349`.
- Index format: UTF-8 entries sorted by relative POSIX path, each containing the path, NUL, lowercase file SHA-256 and LF. Only served payload files are included, not this README or Go source.

This local preview includes the header-free shell, pointer-transparent temporary
field/source previews, shared typography/colors, independent object expansion and the optional
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

Result uses one paged JSON tree for all input sizes, retaining node expansion,
collection pages and scroll across Tab/content changes in the same response.
Replacing source/result/labels resets these in-memory choices. Span hover/focus
previews use Fields-style pointed popups showing only their own content's raw
Markdown value, without offset, length or encoding labels. They use
codePoint/utf16/utf8 offsets (codePoint when omitted), retain explicit invalid-range
and missing-content text, stay inside the viewport, and dismiss on Escape or scroll.
Adjacent span fragments use one newline without an extra blank line; line breaks
inside each original fragment remain unchanged.
The full-string modal searches the complete displayed JSON literal, counts and
cycles through case-insensitive literal matches, and supports Enter/Shift+Enter
and local Ctrl/Command+F. Cancellable sparse indexing and visible-only merged
highlights preserve virtualization and syntax colors; Copy still uses the
original value. These Result changes do not alter PDF/labels/Fields/Legend.

When the analyze command opens `--view` for a PDF, the host discovers an
adjacent `<PDF>.labels.json` and passes it alongside the current analysis result.
The existing Fields tab shows Field Name, Label Value and Extracted Value,
including side-by-side array tables by default and whole-array status filtering.
The per-array Side by side / Stacked controls temporarily change the layout;
refreshing or replacing inputs restores side by side without writing preferences.

Comparison fields support reordering, and mixed matching and differing object
summaries use orange. Paired monetary amount and currency sources use a shared
interaction frame and, when either has low confidence, one shared red outline.
The outline is hidden while that amount is hovered or selected. Info shows
analyzer ID, API version and creation time first when returned.
Fields with `method: "generate"` show a purple sparkle and purple field name
in both regular and comparison views without changing value or source colors.

Paired array tables share bidirectional horizontal scrolling and restore one
column position within the available range after tab switches or collapse.
Side-by-side tables also synchronize vertical scrolling to align rows; stacked
tables retain independent vertical scrolling. Other array groups remain
independent. The fixed `#` corner stays visible above scrolling column headers
and row numbers.

The palette restores the pre-MUI cu-viewer values: `#ECF3FC` for default
scalar/object values, `#DEE9FA` for arrays, `#DCFBD7` for matches, `#FFDADD` for
differences/extras and `#F8C0C8` for label-only values. Actual extracted values
can carry the exact old `#B43C4B99` outline at 60% opacity, explicitly chosen by
the user despite its lower contrast; it no longer claims the previous 3:1
outline guarantee. Both-missing slots are transparent, never receive confidence
outlines and do not change statistics. Zero, false, empty strings and explicit
null remain valid values.

The low-confidence Legend example shows a value with its matching fill plus
the red border, not an empty white state. PDF Fields interaction strokes restore
`#8063B5`/`#5B21B6` and leave 3 CSS px around confidence edges, including their
antialiasing. Raw source geometry and hit targets are unchanged. Flyout headings
and arrows use the old purple `#E5DAFF` with `#4C1D95` heading text, independent
of value state. Every tooltip value is plain text, including both comparison
sides, summaries, cells, references and missing placeholders: no value background
or confidence inset border. Label stays above extraction; matching fills remain
only in Fields and the PDF source. Ordinary single-value previews restore their
historical adaptive width, 12.5px text and spacing rather than comparison-box
layout. Low-score badges retain `#FFDADD`/`#9B1C1C`, and returned confidence
stays visible when outlines are disabled. No UI dependency is added.

Confidence source strokes are painted after every opaque source fill so
pixel-snapped polygons cannot hide their edges. Normal color edges keep their
previous order; the confidence layer is pointer-transparent and retains the
old 60% color, 1.5 CSS px width, source points and original hit targets.

Unlabeled PDF Fields share the restored scalar/array blue value boxes, purple
preview headings/arrows, independent confidence outlines and purple frames.
Rows are 28px and value boxes 24px, including object and array summaries. The
Legend shows only the two blues and a blue value with the red confidence outline;
it never implies matching or adds comparison statistics. Low-score badges remain
truthful when outlines are hidden. The optional Confidence column, single array
table and stored layout/order remain available. Images and non-Fields details
retain their existing visuals.

Only PDF Fields with no labels input use compact per-color checkbox rows;
the 123 confidence example is retained. Label reference/comparison sessions,
other contents in a labeled session and image Legends keep their previous
layout. This boundary uses labels input presence, not comparability, and
reuses existing row/control CSS without adding settings or changing behavior.

Unsaved comparison layouts use a 45% Source / 55% results split; stored layouts
remain authoritative, and ordinary/reference defaults are unchanged.

Comparison search accepts readable names and parent paths, including literal
slashes and tildes. Both array summaries preview their own grounded descendants;
missing sources are explicitly identified rather than borrowed from the other
side. Paired tables retain shared columns and synchronized horizontal scrolling.

Label-only missing regions use the label's own PDF dimensions; extraction pixels
and label inches are not mixed. Optional session warnings explain unreadable or
invalid adjacent labels while keeping the current analysis viewable. Label
comparison does not expose analyzer-field-order editing. The Go host and these
complete assets must be updated together; input documents and results are not
part of this payload.

Label source frames additionally merge adjacent word/punctuation fragments and
use a thin, outward-offset decoration at small zoom. This does not change raw
source polygons, hit targets or PDF text.
The same comparison field's extracted region does not block the label's own
fragments from merging. Different fields still block unsafe unions; label frames
never borrow the extraction's coordinates.

The complete page and assets were imported together. License and third-party
notice files retain the producer files' bytes and line endings; generated
assets, codecs and workers are copied byte-for-byte.

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
