// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"azure.ai.content-understanding/internal/analysis"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticViewerBrowserFixture(t *testing.T) {
	source := os.Getenv("CU_VIEWER_TEST_SOURCE")
	resultPath := os.Getenv("CU_VIEWER_TEST_RESULT")
	if source == "" && resultPath == "" {
		t.Skip("Set CU_VIEWER_TEST_SOURCE and CU_VIEWER_TEST_RESULT for a real local browser fixture.")
	}
	require.NotEmpty(t, source, "The browser fixture needs a local source.")
	require.NotEmpty(t, resultPath, "The browser fixture needs the paired saved JSON.")
	input, err := analysis.DiscoverFile(t.Context(), source)
	require.NoError(t, err)
	// #nosec G304 G703 -- Explicit local integration fixture, never embedded or uploaded.
	result, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	session, err := Prepare(t.Context(), input, io.Discard)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	var metadata struct {
		AnalyzerID string `json:"analyzerId"`
		Result     struct {
			AnalyzerID string `json:"analyzerId"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(result, &metadata))
	analyzer := metadata.Result.AnalyzerID
	if analyzer == "" {
		analyzer = metadata.AnalyzerID
	}
	if analyzer == "" {
		analyzer = "browser-fixture"
	}
	fieldOrders, err := NewFieldOrderStore(analyzer)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	opened := make(chan string, 1)
	session.open = func(_ context.Context, address string, _ io.Writer) error {
		opened <- address
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- session.Show(ctx, json.RawMessage(result), fieldOrders) }()
	var address string
	select {
	case address = <-opened:
	case err := <-done:
		t.Fatalf("Viewer failed before browser readiness: %v", err)
	case <-ctx.Done():
		t.Fatal("Viewer readiness timed out.")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, body := viewerRequest(t, client, http.MethodGet, address, nil)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, string(body), `<div id="root"></div>`)
	assert.Contains(t, response.Header.Get("Content-Security-Policy"), "wasm-unsafe-eval")
	response, body = viewerRequest(t, client, http.MethodGet, address+"session.json", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var document struct {
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	assert.JSONEq(t, string(result), string(document.Result))
	t.Logf("Static Viewer URL: %s", address)

	if stop := os.Getenv("CU_VIEWER_TEST_STOP"); stop != "" {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
	wait:
		for {
			select {
			case <-ticker.C:
				_, err := os.Stat(stop)
				if err == nil {
					break wait
				}
				require.ErrorIs(t, err, os.ErrNotExist)
			case err := <-done:
				t.Fatalf("Viewer stopped before the browser check completed: %v", err)
			case <-ctx.Done():
				t.Fatal("The bounded browser check did not stop.")
			}
		}
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	_, err = client.Get(address)
	require.Error(t, err, "The local listener must be released after the fixture stops.")
}
