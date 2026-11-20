# Bundled CU Viewer files

This directory contains the complete production browser page used by the Go
extension. It is not an npm consumer, a separate Go module, or a runtime cache.
Only the page, assets and licenses are embedded; this README and Go source are
not served to the browser.

## Current source

- Repository: cu-viewer.
- Source commit: `9f9a1c75579293d85af4e3dcc986affb480939ad` on `feature/cu-analyze-view`.
- Frontend package version: `0.0.0` (local preview source, not a published npm dependency).
- Build command: `npm run build:viewer`, using Node.js 24.22.1 and the committed npm lockfile in an isolated build.
- Imported on: 2026-11-20.
- Payload: 229 plain files, including the root licenses.
- Payload index SHA-256: `fe0b393821a993a4a144107ebf4c6771ef2205775d128f39b5173fc844c9dbbb`.
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
and compact token-count summaries. Message values wrap after semicolons and
keep the separators and original JSON.

Result uses one paged JSON tree for all input sizes, retaining node expansion,
collection pages and scroll across Tab/content changes in the same response.
Replacing source/result/labels resets these in-memory choices. The Result footer
provides search across collapsed nodes and collection pages, plus bulk expand
and collapse controls. Span hover/focus
previews use Fields-style pointed popups showing only their own content's raw
Markdown value, without offset, length or encoding labels. They use
codePoint/utf16/utf8 offsets (codePoint when omitted), retain explicit invalid-range
and missing-content text, stay inside the viewport, and dismiss on Escape or scroll.
The popup prefers the right of the span and places itself above or below the
expanded entry when needed, clear of offset and length. Adjacent span fragments
use one newline without an extra blank line; original line breaks remain unchanged.
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
summaries use pastel orange `#FBE2C1`, shared with their Legend swatch. Up and Down
follow the same order as the footer navigation and focus the value cell with a
rounded blue outline; Alt+Up/Down still reorders siblings. Paired monetary amount
and currency sources use a shared
interaction frame and, when either has low confidence, one shared red outline.
The outline is hidden while that amount is hovered or selected. Info shows
analyzer ID, API version and creation time first when returned.
Returned values with no direct equivalent in the field spans or the same
content's Markdown use a pale blue `#CBEAF1` marker and value fill without
recoloring the field name. This is a review hint, not an analyzer `generate`
marker or a confidence judgment. Missing values use gray italic `Not labelled`
on the label side and `Not returned` on the extraction side without changing
comparison status or confidence. Value hover hints use short lines for
confidence and source review, with a comparison status only in comparison mode.

Paired array tables share bidirectional horizontal scrolling and restore one
column position within the available range after tab switches or collapse.
Both layouts show draggable horizontal scrollbars when the columns overflow
and reserve no scrollbar space when the tables fit.
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
of value state. In comparison previews, returned extraction values use the same
fill as their Fields values, including object summaries and cells. Unlabeled
PDF previews, label values, and missing placeholders stay plain, and preview
values do not receive confidence outlines.
Single-value previews retain their adaptive width, 12.5px text and spacing.
Low-score badges retain `#FFDADD`/`#9B1C1C`, and returned confidence stays visible
when outlines are disabled. No UI dependency is added.

Confidence source strokes are painted after every opaque source fill so
pixel-snapped polygons cannot hide their edges. Normal color edges keep their
previous order; the confidence layer is pointer-transparent and retains the
old 60% color, 1.5 CSS px width, source points and original hit targets.

Unlabeled PDF Fields use green `#DCFBD7` for ordinary scalar and array values
without implying a label match. The optional Confidence column, single array
table and stored layout/order remain available. Images and non-Fields details
retain their existing visuals. The PDF Fields Legend shares one layout across
value-only, reference and comparison modes. Value-only uses one combined
green swatch; comparison explains matches, differences and partial matches,
without default or label-only swatches. Low-confidence outlines remain visible
even with both source-fill switches off; reference mode has no extraction
confidence control. Continuous multi-line source fragments share one interaction
frame without expanding hit targets.

Unsaved comparison layouts use a 45% Source / 55% results split; stored layouts
remain authoritative, and ordinary/reference defaults are unchanged.

Comparison search accepts readable names and parent paths, including literal
slashes and tildes. Both array summaries preview their own grounded descendants;
missing sources are explicitly identified rather than borrowed from the other
side. Paired tables retain shared columns and synchronized horizontal scrolling.

Label-only missing regions use the label's own PDF dimensions; extraction pixels
and label inches are not mixed. Optional session warnings explain unreadable or
invalid adjacent labels while keeping the current analysis viewable. Label
comparison saves field order to the same analyzer preferences as extracted-only
Fields. It restores order from the active field tree after label validation and
preserves the ordering of fields not present in the current content. The Go host
and these complete assets must be updated together; input documents and results
are not part of this payload.

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
