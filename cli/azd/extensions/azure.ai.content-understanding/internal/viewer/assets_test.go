// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedAssetsContainProductionPageAndCodecs(t *testing.T) {
	assets, err := embeddedAssets()
	require.NoError(t, err)
	page := string(assets["index.html"])
	assert.Contains(t, page, `<div id="root"></div>`)
	references := regexp.MustCompile(`(?:src|href)="\./([^"]+)"`).FindAllStringSubmatch(page, -1)
	require.NotEmpty(t, references)
	for _, reference := range references {
		assert.NotEmpty(t, assets[reference[1]], reference[1])
	}
	var pdfWorker, heifCodec bool
	for name, content := range assets {
		pdfWorker = pdfWorker || strings.Contains(name, "pdf.worker")
		heifCodec = heifCodec || strings.HasSuffix(name, ".wasm") && bytes.HasPrefix(content, []byte{0, 'a', 's', 'm'})
	}
	assert.True(t, pdfWorker)
	assert.True(t, heifCodec)
}

func assetFixture(t *testing.T, files map[string][]byte) ([]byte, []byte) {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	records := make([]resourceRecord, 0, len(files))
	for name, content := range files {
		stream, err := writer.Create(name)
		require.NoError(t, err)
		_, err = stream.Write(content)
		require.NoError(t, err)
		records = append(records, assetRecord(name, content))
	}
	require.NoError(t, writer.Close())
	document := struct {
		SchemaVersion int              `json:"schemaVersion"`
		Archive       resourceRecord   `json:"archive"`
		Files         []resourceRecord `json:"files"`
	}{2, assetRecord("viewer.zip", archive.Bytes()), records}
	manifest, err := json.Marshal(document)
	require.NoError(t, err)
	return archive.Bytes(), manifest
}

func assetRecord(name string, body []byte) resourceRecord {
	hash := sha256.Sum256(body)
	return resourceRecord{Path: name, Bytes: int64(len(body)), SHA256: hex.EncodeToString(hash[:])}
}

func TestAssetsRejectCorruptionAndUnsafeEntries(t *testing.T) {
	files := map[string][]byte{
		"index.html": []byte("<html></html>"), "LICENSE.txt": []byte("license"),
		"THIRD-PARTY-NOTICES.txt": []byte("notices"),
	}
	archive, manifest := assetFixture(t, files)
	assets, err := readAssets(archive, manifest)
	require.NoError(t, err)
	assert.Equal(t, files, assets)
	invalidArchive := bytes.Clone(archive)
	invalidArchive[0] ^= 0xff
	indexHash := assetRecord("index.html", files["index.html"]).SHA256
	for _, test := range []struct {
		name     string
		archive  []byte
		manifest []byte
	}{
		{"manifest JSON", archive, []byte("{")},
		{"schema", archive, bytes.Replace(manifest, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":1`), 1)},
		{"archive", invalidArchive, manifest},
		{"file hash", archive, bytes.Replace(manifest, []byte(indexHash), []byte(strings.Repeat("0", 64)), 1)},
		{"file count", archive, bytes.Replace(manifest, []byte(`"files":[`), []byte(`"files":[{},`), 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := readAssets(test.archive, test.manifest)
			require.Error(t, err)
		})
	}
	for _, name := range []string{"../outside", "assets/../outside", "session.json", "source"} {
		t.Run(name, func(t *testing.T) {
			files[name] = []byte("unexpected")
			archive, manifest := assetFixture(t, files)
			_, err := readAssets(archive, manifest)
			require.Error(t, err)
			delete(files, name)
		})
	}
	delete(files, "LICENSE.txt")
	archive, manifest = assetFixture(t, files)
	_, err = readAssets(archive, manifest)
	require.Error(t, err)
}
