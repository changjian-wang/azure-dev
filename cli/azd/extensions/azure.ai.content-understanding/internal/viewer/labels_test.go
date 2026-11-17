// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"azure.ai.content-understanding/internal/analysis"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const localLabels = `{"fieldLabels":{"Count":{"type":"number","valueNumber":9007199254740993}}}`

func TestPrepareAutomaticallyLoadsAdjacentLabelsForPdf(t *testing.T) {
	maxLabels := append([]byte(localLabels), bytes.Repeat([]byte(" "), MaxLabelJSONBytes-len(localLabels))...)
	for _, test := range []struct {
		name        string
		source      string
		labels      []byte
		asDirectory bool
		asSymlink   bool
		wantLabels  string
		wantWarning string
	}{
		{name: "missing labels", source: "invoice.pdf"},
		{name: "valid labels", source: "invoice.pdf", labels: []byte(localLabels), wantLabels: localLabels},
		{name: "empty labels", source: "invoice.pdf",
			labels: []byte(`{"fieldLabels":{}}`), wantLabels: `{"fieldLabels":{}}`},
		{name: "labels at size limit", source: "invoice.pdf", labels: maxLabels, wantLabels: string(maxLabels)},
		{name: "invalid labels", source: "invoice.pdf", labels: []byte(`{"fieldLabels":[]}`),
			wantWarning: invalidLabelsWarning},
		{name: "invalid UTF-8 labels", source: "invoice.pdf",
			labels: []byte("{\"fieldLabels\":{\"Name\":{\"valueString\":\"\xff\"}}}"), wantWarning: invalidLabelsWarning},
		{name: "oversized labels", source: "invoice.pdf", labels: append(bytes.Clone(maxLabels), ' '),
			wantWarning: unreadableLabelsWarning},
		{name: "non-regular labels", source: "invoice.pdf", asDirectory: true,
			wantWarning: unreadableLabelsWarning},
		{name: "symlinked labels", source: "invoice.pdf", asSymlink: true,
			wantWarning: unreadableLabelsWarning},
		{name: "labels are ignored for images", source: "receipt.png", labels: []byte(localLabels)},
	} {
		t.Run(test.name, func(t *testing.T) {
			sourceBody := []byte("%PDF-1.7\n")
			if test.source == "receipt.png" {
				sourceBody = []byte{137, 80, 78, 71, 13, 10, 26, 10}
			}
			input := localInput(t, test.source, sourceBody)
			if test.asDirectory {
				require.NoError(t, os.Mkdir(input.Path+".labels.json", 0o700))
			} else if test.asSymlink {
				if runtime.GOOS == "windows" {
					t.Skip("Creating symbolic links on Windows requires elevated privileges.")
				}
				target := input.Path + ".reference.json"
				require.NoError(t, os.WriteFile(target, []byte(localLabels), 0o600))
				require.NoError(t, os.Symlink(target, input.Path+".labels.json"))
			} else if test.labels != nil {
				require.NoError(t, os.WriteFile(input.Path+".labels.json", test.labels, 0o600))
			}
			session, err := prepare(t.Context(), input, io.Discard, testAssets())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, session.Close()) })
			assert.Equal(t, test.wantLabels, string(session.labels))
			if test.wantWarning == "" {
				assert.Empty(t, session.warnings)
			} else {
				require.Equal(t, []string{test.wantWarning}, session.warnings)
			}
		})
	}
}

func TestAdjacentLabelsUseTheResolvedPdfPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Creating symbolic links on Windows requires elevated privileges.")
	}
	input := localInput(t, "invoice.pdf", []byte("%PDF-1.7\n"))
	require.NoError(t, os.WriteFile(input.Path+".labels.json", []byte(localLabels), 0o600))
	alias := filepath.Join(filepath.Dir(input.Path), "alias.pdf")
	require.NoError(t, os.Symlink(input.Path, alias))
	require.NoError(t, os.WriteFile(alias+".labels.json", []byte(`{"fieldLabels":[]}`), 0o600))
	resolved, err := analysis.DiscoverFile(t.Context(), alias)
	require.NoError(t, err)
	session, err := prepare(t.Context(), resolved, io.Discard, testAssets())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	assert.Equal(t, localLabels, string(session.labels))
	assert.Empty(t, session.warnings)
}

func TestViewerSessionPublishesAutoDetectedLabelsWithAnalysisResult(t *testing.T) {
	var output bytes.Buffer
	input := localInput(t, "invoice.pdf", []byte("%PDF-1.7\nsource bytes"))
	require.NoError(t, os.WriteFile(input.Path+".labels.json", []byte(localLabels), 0o600))
	session, err := prepare(t.Context(), input, &output, testAssets())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan string, 1)
	session.open = func(_ context.Context, address string, _ io.Writer) error {
		opened <- address
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- session.Show(ctx, json.RawMessage(viewerResponse), nil) }()

	var address string
	select {
	case address = <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("the viewer did not become ready")
	}
	response, body := viewerRequest(t, &http.Client{Timeout: 5 * time.Second},
		http.MethodGet, address+"session.json", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &document))
	assert.JSONEq(t, localLabels, string(document["labels"]))
	assert.Contains(t, string(document["labels"]), "9007199254740993")
	assert.JSONEq(t, viewerResponse, string(document["result"]))
	assert.NotContains(t, document, "warnings")
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestInvalidAdjacentLabelsAreReportedAndAnalysisResultRemainsViewable(t *testing.T) {
	var output bytes.Buffer
	input := localInput(t, "invoice.pdf", []byte("%PDF-1.7\nsource bytes"))
	require.NoError(t, os.WriteFile(input.Path+".labels.json", []byte("{"), 0o600))
	session, err := prepare(t.Context(), input, &output, testAssets())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan string, 1)
	session.open = func(_ context.Context, address string, _ io.Writer) error {
		opened <- address
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- session.Show(ctx, json.RawMessage(viewerResponse), nil) }()

	var address string
	select {
	case address = <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("the viewer did not become ready")
	}
	response, body := viewerRequest(t, &http.Client{Timeout: 5 * time.Second},
		http.MethodGet, address+"session.json", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &document))
	assert.NotContains(t, document, "labels")
	assert.JSONEq(t, `["`+invalidLabelsWarning+`"]`, string(document["warnings"]))
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	assert.Contains(t, output.String(), invalidLabelsWarning)
}

func TestLabelsValidationRejectsInvalidRootsAndAcceptsEmptyLabels(t *testing.T) {
	require.NoError(t, ValidateLabels(json.RawMessage(`{"fieldLabels":{}}`)))
	for _, value := range []string{"null", "[]", "{}", `{"fieldLabels":null}`, `{"fieldLabels":[]}`} {
		require.Error(t, ValidateLabels(json.RawMessage(value)))
	}
}

func TestPrepareHonorsCancellationBeforeAdjacentLabelReads(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	input := analysis.Input{Path: "missing.pdf"}
	_, err := prepare(ctx, input, io.Discard, testAssets())
	require.ErrorIs(t, err, context.Canceled)
}
