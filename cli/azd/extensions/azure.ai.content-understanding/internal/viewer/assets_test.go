// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	viewerstatic "azure.ai.content-understanding/cu-viewer-static"
	"azure.ai.content-understanding/internal/exterrors"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                        {Data: []byte(`<div id="root"></div><script src="./assets/main.js"></script>`)},
		"assets/main.js":                    {Data: []byte("viewer")},
		"LICENSE.txt":                       {Data: []byte("MIT")},
		"THIRD-PARTY-NOTICES.txt":           {Data: []byte("notices")},
		"assets/pdfjs/build/pdf.worker.mjs": {Data: []byte("worker")},
		"assets/heif/heic_decoder_bg.wasm":  {Data: []byte{0, 'a', 's', 'm'}},
	}
}

func TestBundledPageContainsItsResourcesAndCodecs(t *testing.T) {
	require.NoError(t, validateAssets(viewerstatic.Files))
	body, err := viewerstatic.Files.ReadFile("index.html")
	require.NoError(t, err)
	assert.Contains(t, string(body), `<div id="root"></div>`)
	references := regexp.MustCompile(`(?:src|href)="\./([^"]+)"`).FindAllStringSubmatch(string(body), -1)
	require.NotEmpty(t, references)
	for _, reference := range references {
		content, err := viewerstatic.Files.ReadFile(reference[1])
		require.NoError(t, err, reference[1])
		assert.NotEmpty(t, content, reference[1])
	}
	for _, name := range []string{
		"assets/pdfjs/build/pdf.worker.mjs", "assets/heif/heif.worker.js",
		"assets/heif/heic_decoder.js", "assets/heif/heic_decoder_bg.wasm",
	} {
		content, err := viewerstatic.Files.ReadFile(name)
		require.NoError(t, err, name)
		assert.NotEmpty(t, content, name)
		if strings.HasSuffix(name, ".wasm") {
			assert.True(t, bytes.HasPrefix(content, []byte{0, 'a', 's', 'm'}))
		}
	}
	for _, pattern := range []string{
		"assets/pdfjs/cmaps/*", "assets/pdfjs/standard_fonts/*",
		"assets/pdfjs/wasm/*", "assets/pdfjs/iccs/*",
	} {
		files, err := fs.Glob(viewerstatic.Files, pattern)
		require.NoError(t, err)
		assert.NotEmpty(t, files, pattern)
	}
	for _, name := range []string{"embed.go", "README.md", "viewer.zip", "manifest.json"} {
		_, err := viewerstatic.Files.Open(name)
		assert.ErrorIs(t, err, fs.ErrNotExist, name)
	}
}

func TestPrepareRejectsMissingOrEmptyStaticResources(t *testing.T) {
	for _, name := range []string{"index.html", "LICENSE.txt", "THIRD-PARTY-NOTICES.txt"} {
		for _, state := range []string{"missing", "empty"} {
			t.Run(name+"/"+state, func(t *testing.T) {
				assets := testAssets()
				if state == "empty" {
					assets[name].Data = nil
				} else {
					delete(assets, name)
				}
				input := localInput(t, "invoice.pdf", []byte("%PDF-1.7\nsource bytes"))
				session, err := prepare(t.Context(), input, io.Discard, assets)
				require.Error(t, err)
				assert.Nil(t, session)
				detail, ok := errors.AsType[*azdext.LocalError](err)
				require.True(t, ok)
				assert.Equal(t, exterrors.CodeViewerAssetsInvalid, detail.Code)
				assert.Contains(t, detail.Message, name)
				assert.Contains(t, detail.Message, "no analysis was started")
			})
		}
	}
}

func TestPrepareValidatesSourceBeforeStaticResources(t *testing.T) {
	input := localInput(t, "invalid.pdf", []byte("not a document"))
	_, err := prepare(t.Context(), input, io.Discard, fstest.MapFS{})
	detail, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerSourceInvalid, detail.Code)
}
