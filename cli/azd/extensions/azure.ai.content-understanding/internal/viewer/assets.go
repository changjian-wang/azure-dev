// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sync"
)

//go:embed resources/viewer/viewer.zip
var resourceArchive []byte

//go:embed resources/viewer/manifest.json
var resourceManifest []byte

var embeddedAssets = sync.OnceValues(func() (map[string][]byte, error) {
	return readAssets(resourceArchive, resourceManifest)
})

type resourceRecord struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func readAssets(archive, manifest []byte) (map[string][]byte, error) {
	var document struct {
		SchemaVersion int              `json:"schemaVersion"`
		Archive       resourceRecord   `json:"archive"`
		Files         []resourceRecord `json:"files"`
	}
	if err := json.Unmarshal(manifest, &document); err != nil {
		return nil, fmt.Errorf("decode viewer resource manifest: %w", err)
	}
	if document.SchemaVersion != 2 || document.Archive.Path != "viewer.zip" ||
		!matchesResource(archive, document.Archive) {
		return nil, errors.New("invalid viewer resource archive")
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open viewer resource archive: %w", err)
	}
	if len(reader.File) != len(document.Files) {
		return nil, errors.New("viewer resource count does not match the manifest")
	}
	records := make(map[string]resourceRecord, len(document.Files))
	for _, record := range document.Files {
		if !fs.ValidPath(record.Path) || record.Path == "." || record.Bytes < 0 ||
			record.Path == "session.json" || record.Path == "source" {
			return nil, errors.New("invalid viewer resource path")
		}
		if _, exists := records[record.Path]; exists {
			return nil, errors.New("duplicate viewer resource path")
		}
		records[record.Path] = record
	}
	assets := make(map[string][]byte, len(records))
	for _, file := range reader.File {
		record, exists := records[file.Name]
		if !exists || !file.Mode().IsRegular() {
			return nil, errors.New("unexpected viewer archive entry")
		}
		if file.UncompressedSize64 > math.MaxInt64 {
			return nil, errors.New("viewer archive entry is too large")
		}
		if int64(file.UncompressedSize64) != record.Bytes {
			return nil, errors.New("unexpected viewer archive entry")
		}
		if _, exists := assets[file.Name]; exists {
			return nil, errors.New("duplicate viewer archive entry")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open viewer archive entry: %w", err)
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, record.Bytes))
		if err := errors.Join(readErr, stream.Close()); err != nil {
			return nil, fmt.Errorf("read viewer archive entry: %w", err)
		}
		if !matchesResource(content, record) {
			return nil, errors.New("viewer resource does not match the manifest")
		}
		assets[file.Name] = content
	}
	for _, required := range []string{"index.html", "LICENSE.txt", "THIRD-PARTY-NOTICES.txt"} {
		if len(assets[required]) == 0 {
			return nil, errors.New("a required viewer resource is missing")
		}
	}
	return assets, nil
}

func matchesResource(content []byte, record resourceRecord) bool {
	digest := sha256.Sum256(content)
	return int64(len(content)) == record.Bytes && hex.EncodeToString(digest[:]) == record.SHA256
}
