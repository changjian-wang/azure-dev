// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/localio"
	"azure.ai.content-understanding/internal/testutil"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type releaseFixture struct {
	version  string
	protocol int
	archive  []byte
	manifest []byte
}

func resourceRelease(t *testing.T, version string, protocol int) releaseFixture {
	t.Helper()
	archive, manifest := assetFixture(t, map[string][]byte{
		"index.html":     []byte(`<div id="root"></div><script src="./assets/main.js"></script>`),
		"assets/main.js": []byte("viewer"), "LICENSE.txt": []byte("MIT"),
		"THIRD-PARTY-NOTICES.txt": []byte("notices"),
		"assets/pdf.worker.mjs":   []byte("worker"), "assets/heif.wasm": {0, 'a', 's', 'm'},
	})
	var document resourceDocument
	require.NoError(t, json.Unmarshal(manifest, &document))
	document.ViewerVersion, document.ProtocolVersion = version, protocol
	manifest, err := json.Marshal(document)
	require.NoError(t, err)
	return releaseFixture{version, protocol, archive, manifest}
}

func resourceServer(
	t *testing.T, releases ...releaseFixture,
) (*resourceResolver, *httptest.Server, *atomic.Int64) {
	t.Helper()
	count := &atomic.Int64{}
	index := []viewerRelease{}
	files := map[string][]byte{}
	for _, release := range releases {
		tag := "v" + release.version
		item := viewerRelease{Tag: tag}
		item.Assets = append(item.Assets, struct {
			Name string `json:"name"`
		}{"manifest.json"},
			struct {
				Name string `json:"name"`
			}{"viewer.zip"})
		index = append(index, item)
		files["/download/"+tag+"/manifest.json"] = release.manifest
		files["/download/"+tag+"/viewer.zip"] = release.archive
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		count.Add(1)
		if request.URL.Path == "/releases" {
			if err := json.NewEncoder(w).Encode(index); err != nil {
				t.Error(err)
			}
			return
		}
		data, exists := files[request.URL.Path]
		if !exists {
			http.NotFound(w, request)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	return &resourceResolver{
		cacheDir:    filepath.Join(testutil.TempDir(t), "viewer"),
		releasesURL: server.URL + "/releases", downloadsURL: server.URL + "/download/", client: server.Client(),
	}, server, count
}

func TestResourcesFirstUseCachesAndThenWorksWithoutNetwork(t *testing.T) {
	resolver, server, count := resourceServer(t, resourceRelease(t, "0.1.0-preview", 1))
	var progress bytes.Buffer
	first, err := resolver.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Equal(t, "0.1.0-preview", first.info.Version)
	assert.Contains(t, first.assets, "assets/pdf.worker.mjs")
	assert.Contains(t, progress.String(), "not cached")
	assert.Contains(t, progress.String(), "Downloading CU Viewer 0.1.0-preview")
	assert.EqualValues(t, 3, count.Load())
	server.Close()
	progress.Reset()
	cached, err := resolver.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Equal(t, first.assets, cached.assets)
	assert.Empty(t, progress.String())
	assert.EqualValues(t, 3, count.Load())
}

func TestResourcesSelectLatestCompatibleVersionWithoutDownloadingIncompatibleArchive(t *testing.T) {
	future := resourceRelease(t, "3.0.0", 2)
	future.manifest = []byte(`{"protocolVersion":2,"schemaVersion":99}`)
	resolver, _, count := resourceServer(t, resourceRelease(t, "0.9.0", 1), future,
		resourceRelease(t, "1.1.0-preview", 1))
	snapshot, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0-preview", snapshot.info.Version)
	assert.EqualValues(t, 4, count.Load())
}

func TestResourcesNoPublishedReleaseIsAnActionableFailure(t *testing.T) {
	resolver, _, _ := resourceServer(t)
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.Error(t, err)
	detail, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerUnavailable, detail.Code)
	assert.Contains(t, detail.Message, "No compatible CU Viewer release")
	assert.Contains(t, detail.Suggestion, "Omit --view")
	_, statErr := os.Stat(filepath.Join(resolver.cacheDir, "current.json"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestResourcesIntegrityFailureNeverReplacesWorkingCache(t *testing.T) {
	resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", 1))
	first, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	pointer, err := localio.Read(filepath.Join(resolver.cacheDir, "current.json"))
	require.NoError(t, err)
	bad := resourceRelease(t, "1.1.0", 1)
	bad.archive = bytes.Clone(bad.archive)
	bad.archive[0] ^= 0xff
	newer, _, _ := resourceServer(t, bad)
	newer.cacheDir = resolver.cacheDir
	_, err = newer.resolve(t.Context(), io.Discard, true)
	require.Error(t, err)
	detail, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerAssetsInvalid, detail.Code)
	after, err := localio.Read(filepath.Join(resolver.cacheDir, "current.json"))
	require.NoError(t, err)
	assert.Equal(t, pointer, after)
	cached, err := resolver.cached()
	require.NoError(t, err)
	assert.Equal(t, first.info, cached.info)
}

func TestResourcesExplicitUpdateActivatesNewVersionButOrdinaryViewKeepsCache(t *testing.T) {
	resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", 1))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	newer, _, count := resourceServer(t, resourceRelease(t, "1.2.0", 1))
	newer.cacheDir = resolver.cacheDir
	current, err := newer.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", current.info.Version)
	assert.Zero(t, count.Load())
	updated, err := newer.resolve(t.Context(), io.Discard, true)
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", updated.info.Version)
	cached, err := resolver.cached()
	require.NoError(t, err)
	assert.Equal(t, updated.info, cached.info)
}

func TestResourcesFailedOfflineUpdateRetainsValidCache(t *testing.T) {
	resolver, server, _ := resourceServer(t, resourceRelease(t, "1.0.0", 1))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	server.Close()
	_, err = resolver.resolve(t.Context(), io.Discard, true)
	require.Error(t, err)
	cached, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", cached.info.Version)
}

func TestResourcesCorruptCacheIsReportedAndRepaired(t *testing.T) {
	for _, corrupt := range []string{"current", "manifest", "archive", "manifest-missing", "archive-missing"} {
		t.Run(corrupt, func(t *testing.T) {
			resolver, _, count := resourceServer(t, resourceRelease(t, "1.0.0", 1))
			_, err := resolver.resolve(t.Context(), io.Discard, false)
			require.NoError(t, err)
			pointerBytes, err := localio.Read(filepath.Join(resolver.cacheDir, "current.json"))
			require.NoError(t, err)
			var pointer cachePointer
			require.NoError(t, json.Unmarshal(pointerBytes, &pointer))
			path := filepath.Join(resolver.cacheDir, "current.json")
			if strings.HasPrefix(corrupt, "manifest") {
				path = resolver.snapshotBase(pointer) + ".json"
			}
			if strings.HasPrefix(corrupt, "archive") {
				path = resolver.snapshotBase(pointer) + ".zip"
			}
			if strings.HasSuffix(corrupt, "-missing") {
				require.NoError(t, os.Remove(path))
			} else {
				require.NoError(t, localio.Write(path, []byte("corrupt"), true))
			}
			var progress bytes.Buffer
			snapshot, err := resolver.resolve(t.Context(), &progress, false)
			require.NoError(t, err)
			assert.Equal(t, "1.0.0", snapshot.info.Version)
			assert.Contains(t, progress.String(), "could not be verified")
			assert.EqualValues(t, 6, count.Load())
		})
	}
}

func TestResourcesConcurrentPreparationsDownloadOnce(t *testing.T) {
	resolver, _, count := resourceServer(t, resourceRelease(t, "1.0.0", 1))
	var workers sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		workers.Go(func() {
			_, err := resolver.resolve(t.Context(), io.Discard, false)
			results <- err
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	assert.EqualValues(t, 3, count.Load())
}

func TestResourcesCancellationWhileWaitingForCacheLock(t *testing.T) {
	resolver, _, count := resourceServer(t, resourceRelease(t, "1.0.0", 1))
	path := filepath.Join(resolver.cacheDir, "download.lock")
	require.NoError(t, localio.Write(path, nil, false))
	lock := flock.New(path)
	require.NoError(t, lock.Lock())
	defer lock.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := resolver.resolve(ctx, io.Discard, false)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Zero(t, count.Load())
}

func TestResourcesRejectUnsafeCacheAndOversizedResponses(t *testing.T) {
	resolver, _, count := resourceServer(t, resourceRelease(t, "1.0.0", 1))
	require.NoError(t, os.WriteFile(resolver.cacheDir, []byte("not a directory"), 0o600))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.Error(t, err)
	detail, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerCacheFailed, detail.Code)
	assert.Zero(t, count.Load())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 101))
	}))
	defer server.Close()
	resolver.client = server.Client()
	_, err = resolver.get(t.Context(), server.URL, 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size limit")
}

func TestResourcesHTTPFailuresDoNotLeakResponseBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private bearer token secret", http.StatusForbidden)
	}))
	defer server.Close()
	resolver := &resourceResolver{client: server.Client()}
	_, err := resolver.get(t.Context(), server.URL, 1000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.NotContains(t, err.Error(), "private bearer")
}

func TestPrepareRuntimeFailureStopsBeforeListeningAndReleasesSource(t *testing.T) {
	input := localInput(t, "invoice.pdf", []byte("%PDF-1.7\n"))
	var calls int
	_, err := prepareWithResources(t.Context(), input, io.Discard, func() (map[string][]byte, error) {
		calls++
		return nil, unavailable("The test release is unavailable.")
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
	assert.Contains(t, err.Error(), "no analysis was started")
	renamed := input.Path + ".moved"
	require.NoError(t, os.Rename(input.Path, renamed))
}

func TestPrepareUnsupportedSourceNeverStartsResourceDownload(t *testing.T) {
	input := localInput(t, "invalid.txt", []byte("not a supported document"))
	called := false
	_, err := prepareWithResources(t.Context(), input, io.Discard, func() (map[string][]byte, error) {
		called = true
		return nil, errors.New("unexpected download")
	})
	require.Error(t, err)
	assert.False(t, called)
}

func TestResourcesRejectInvalidManifestsAndUnsafeArchives(t *testing.T) {
	for _, problem := range []string{"version", "schema", "archive-size", "archive-path", "unsafe-entry"} {
		t.Run(problem, func(t *testing.T) {
			release := resourceRelease(t, "1.0.0", 1)
			var document resourceDocument
			require.NoError(t, json.Unmarshal(release.manifest, &document))
			switch problem {
			case "version":
				document.ViewerVersion = "1.1.0"
			case "schema":
				document.SchemaVersion = 3
			case "archive-size":
				document.Archive.Bytes = maxArchiveBytes + 1
			case "archive-path":
				document.Archive.Path = "../outside"
			case "unsafe-entry":
				var manifest []byte
				release.archive, manifest = assetFixture(t, map[string][]byte{
					"index.html": []byte("page"), "LICENSE.txt": []byte("MIT"),
					"THIRD-PARTY-NOTICES.txt": []byte("notices"), "../outside": []byte("unsafe"),
				})
				require.NoError(t, json.Unmarshal(manifest, &document))
				document.ViewerVersion, document.ProtocolVersion = "1.0.0", 1
			}
			var err error
			release.manifest, err = json.Marshal(document)
			require.NoError(t, err)
			resolver, _, _ := resourceServer(t, release)
			_, err = resolver.resolve(t.Context(), io.Discard, false)
			require.Error(t, err)
			_, statErr := os.Stat(filepath.Join(resolver.cacheDir, "current.json"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestResourcesCancellationInterruptsNetworkWithoutPublishingCache(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	resolver := &resourceResolver{
		cacheDir:    filepath.Join(testutil.TempDir(t), "viewer"),
		releasesURL: server.URL, client: server.Client(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := resolver.resolve(ctx, io.Discard, false); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the resource download did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the resource download did not cancel")
	}
	_, err := os.Stat(filepath.Join(resolver.cacheDir, "current.json"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

type failedResourceOutput struct{}

func (failedResourceOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestResourcesStatusWriteFailureIsExplicit(t *testing.T) {
	resolver, _, count := resourceServer(t, resourceRelease(t, "1.0.0", 1))
	_, err := resolver.resolve(t.Context(), failedResourceOutput{}, false)
	require.Error(t, err)
	detail, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerOutputFailed, detail.Code)
	assert.Zero(t, count.Load())
}
