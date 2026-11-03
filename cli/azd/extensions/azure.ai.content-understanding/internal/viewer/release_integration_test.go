// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/localio"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentReleaseDownloadAndOfflineServing(t *testing.T) {
	export := os.Getenv("CU_VIEWER_TEST_EXPORT")
	if export == "" {
		t.Skip("Set CU_VIEWER_TEST_EXPORT to a real local versioned export for delivery integration.")
	}
	manifest, err := localio.ReadBytes(filepath.Join(export, "manifest.json"), maxManifestBytes)
	require.NoError(t, err)
	archive, err := localio.ReadBytes(filepath.Join(export, "viewer.zip"), maxArchiveBytes)
	require.NoError(t, err)
	var document resourceDocument
	require.NoError(t, json.Unmarshal(manifest, &document))
	require.Equal(t, viewerProtocol, document.ProtocolVersion)
	resolver, remote, requests := resourceServer(t, releaseFixture{
		document.ViewerVersion, document.ProtocolVersion, archive, manifest,
	})
	source := os.Getenv("CU_VIEWER_TEST_SOURCE")
	resultPath := os.Getenv("CU_VIEWER_TEST_RESULT")
	input, err := analysis.DiscoverFile(t.Context(), source)
	require.NoError(t, err)
	response, err := os.ReadFile(resultPath) // #nosec G304 G703 -- Explicit local integration fixture.
	require.NoError(t, err)
	load := func() (map[string][]byte, error) {
		snapshot, err := resolver.resolve(t.Context(), io.Discard, false)
		if err != nil {
			return nil, err
		}
		return snapshot.assets, nil
	}
	session, err := prepareWithResources(t.Context(), input, io.Discard, load)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	assert.Len(t, session.assets, len(document.Files))
	assert.EqualValues(t, 3, requests.Load())
	remote.Close()
	offline, err := prepareWithResources(t.Context(), input, io.Discard, load)
	require.NoError(t, err)
	require.NoError(t, offline.Close())
	assert.EqualValues(t, 3, requests.Load())
	opened := make(chan string, 1)
	session.open = func(_ context.Context, address string, _ io.Writer) error { opened <- address; return nil }
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- session.Show(ctx, json.RawMessage(response)) }()
	var address string
	select {
	case address = <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("the runtime-downloaded Viewer did not become ready")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	require.NoError(t, err)
	page, err := client.Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(page.Body)
	require.NoError(t, errors.Join(err, page.Body.Close()))
	assert.Equal(t, http.StatusOK, page.StatusCode)
	assert.Contains(t, string(body), `<div id="root"></div>`)
	assert.Contains(t, page.Header.Get("Content-Security-Policy"), "wasm-unsafe-eval")
	var singleToolbar bool
	for name, body := range session.assets {
		if strings.HasSuffix(name, ".js") && strings.Contains(string(body), "cu-source-toolbar-bottom") {
			singleToolbar = true
		}
	}
	assert.True(t, singleToolbar, "the current integration export must use the approved React preview, not the old HTML study")
	if stop := os.Getenv("CU_VIEWER_TEST_STOP"); stop != "" {
		t.Logf("Runtime Viewer URL: %s", address)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.NewTimer(2 * time.Minute)
		defer timeout.Stop()
	wait:
		for {
			select {
			case <-ticker.C:
				exists, err := localio.Check(stop)
				require.NoError(t, err)
				if exists {
					break wait
				}
			case <-timeout.C:
				t.Fatal("the bounded browser integration check did not stop")
			case <-t.Context().Done():
				t.Fatal("the browser integration context was cancelled")
			}
		}
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}
