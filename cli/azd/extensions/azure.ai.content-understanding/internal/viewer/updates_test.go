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

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func expireUpdateCheck(t *testing.T, resolver *resourceResolver) {
	t.Helper()
	check, err := resolver.readUpdateCheck()
	require.NoError(t, err)
	check.CheckedAt = time.Now().UTC().Add(-updateCheckInterval)
	require.NoError(t, resolver.saveUpdateCheck(check))
}

type updateRoundTripper func(*http.Request) (*http.Response, error)

func (transport updateRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func monitorUpdateRequests(resolver *resourceResolver) (requests, archives *atomic.Int64) {
	requests, archives = &atomic.Int64{}, &atomic.Int64{}
	base := resolver.client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	resolver.client.Transport = updateRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if strings.HasSuffix(request.URL.Path, "/viewer.zip") {
			archives.Add(1)
		}
		return base.RoundTrip(request)
	})
	return requests, archives
}

func TestUpdateCheckFreshnessUsesAnExact24HourBoundary(t *testing.T) {
	now := time.Date(2026, time.November, 4, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		checked time.Time
		fresh   bool
	}{
		{"missing", time.Time{}, false},
		{"just checked", now, true},
		{"just before expiry", now.Add(-24*time.Hour + time.Nanosecond), true},
		{"exactly 24 hours", now.Add(-24 * time.Hour), false},
		{"expired", now.Add(-24*time.Hour - time.Nanosecond), false},
		{"future timestamp", now.Add(time.Nanosecond), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.fresh, (updateCheck{CheckedAt: test.checked}).fresh(now))
		})
	}
}

func TestResourcesDailyNoticeDoesNotDownloadOrActivateUntilManualUpdate(t *testing.T) {
	resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
	original, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	pointer, err := localio.Read(filepath.Join(resolver.cacheDir, "current.json"))
	require.NoError(t, err)

	newer, _, count := resourceServer(t, resourceRelease(t, "1.2.0-preview.1", viewerProtocol),
		resourceRelease(t, "3.0.0", viewerProtocol+1))
	newer.cacheDir = resolver.cacheDir
	_, archives := monitorUpdateRequests(newer)
	var progress bytes.Buffer
	_, err = newer.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Zero(t, count.Load(), "a fresh check must not query releases")
	assert.Empty(t, progress.String())

	expireUpdateCheck(t, newer)
	current, err := newer.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Equal(t, original.info, current.info)
	assert.Contains(t, progress.String(), "1.0.0 -> 1.2.0-preview.1")
	assert.Contains(t, progress.String(), "azd ai cu viewer update")
	assert.NotContains(t, progress.String(), "3.0.0")
	assert.NotContains(t, progress.String(), "Downloading")
	assert.EqualValues(t, 3, count.Load(), "only the release list and candidate manifests are requested")
	assert.Zero(t, archives.Load())
	after, err := localio.Read(filepath.Join(newer.cacheDir, "current.json"))
	require.NoError(t, err)
	assert.Equal(t, pointer, after)
	_, err = os.Stat(filepath.Join(newer.cacheDir, "versions", "1.2.0-preview.1"))
	require.ErrorIs(t, err, os.ErrNotExist)

	progress.Reset()
	_, err = newer.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Contains(t, progress.String(), "1.0.0 -> 1.2.0-preview.1")
	assert.EqualValues(t, 3, count.Load(), "the known notice must not trigger another check")
	assert.Zero(t, archives.Load())

	progress.Reset()
	updated, err := newer.resolve(t.Context(), &progress, true)
	require.NoError(t, err)
	assert.Equal(t, "1.2.0-preview.1", updated.info.Version)
	assert.EqualValues(t, 1, archives.Load(), "only an explicit update downloads the new archive")
	assert.EqualValues(t, 7, count.Load(), "an explicit update ignores the daily check cooldown")
	check, err := newer.readUpdateCheck()
	require.NoError(t, err)
	assert.Equal(t, updated.info.Version, check.LatestVersion)
	assert.False(t, check.Failed)
	progress.Reset()
	_, err = newer.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Empty(t, progress.String(), "an applied update must clear the old notice")
	assert.EqualValues(t, 7, count.Load())
}

func TestUpdateNoticesUseSemanticVersionOrdering(t *testing.T) {
	for _, test := range []struct {
		current, latest string
		notice          bool
	}{
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "0.9.0", false},
		{"1.0.0", "1.0.0-preview.1", false},
		{"1.0.0", "1.0.0+build.2", false},
		{"1.0.0", "1.1.0", true},
		{"1.9.0", "1.10.0", true},
		{"0.1.0-preview.1", "0.1.0-preview.2", true},
		{"0.1.0-preview.1", "0.1.0", true},
	} {
		t.Run(test.current+"/"+test.latest, func(t *testing.T) {
			resolver, _, count := resourceServer(t, resourceRelease(t, test.current, viewerProtocol))
			_, err := resolver.resolve(t.Context(), io.Discard, false)
			require.NoError(t, err)
			require.NoError(t, resolver.saveUpdateCheck(updateCheck{
				CheckedAt: time.Now().UTC(), LatestVersion: test.latest,
			}))
			var progress bytes.Buffer
			snapshot, err := resolver.resolve(t.Context(), &progress, false)
			require.NoError(t, err)
			assert.Equal(t, test.current, snapshot.info.Version)
			assert.Equal(t, test.notice, strings.Contains(progress.String(), "new CU Viewer version"))
			assert.EqualValues(t, 3, count.Load())
		})
	}
}

func TestOfflineUpdateCheckWarnsAndKeepsKnownNoticeWithoutRetryingDaily(t *testing.T) {
	resolver, server, _ := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	require.NoError(t, resolver.saveUpdateCheck(updateCheck{
		CheckedAt: time.Now().UTC().Add(-updateCheckInterval), LatestVersion: "1.1.0",
	}))
	server.Close()
	requests, archives := monitorUpdateRequests(resolver)
	var progress bytes.Buffer
	snapshot, err := resolver.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", snapshot.info.Version)
	assert.Contains(t, progress.String(), "Warning: CU Viewer updates could not be checked")
	assert.Contains(t, progress.String(), "1.0.0 -> 1.1.0")
	check, err := resolver.readUpdateCheck()
	require.NoError(t, err)
	assert.True(t, check.Failed)
	assert.True(t, check.fresh(time.Now()))
	assert.EqualValues(t, 1, requests.Load())
	assert.Zero(t, archives.Load())
	progress.Reset()
	_, err = resolver.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Contains(t, progress.String(), "Warning: CU Viewer updates could not be checked")
	assert.Contains(t, progress.String(), "1.0.0 -> 1.1.0")
	assert.EqualValues(t, 1, requests.Load(), "a failed check is also rate-limited")
}

func TestConcurrentCachedViewsCheckForUpdatesOnce(t *testing.T) {
	resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	expireUpdateCheck(t, resolver)
	newer, _, count := resourceServer(t, resourceRelease(t, "1.1.0", viewerProtocol))
	newer.cacheDir = resolver.cacheDir
	_, archives := monitorUpdateRequests(newer)
	var workers sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		workers.Go(func() {
			snapshot, err := newer.resolve(t.Context(), io.Discard, false)
			if err == nil && snapshot.info.Version != "1.0.0" {
				err = errors.New("a cached view activated another version")
			}
			results <- err
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	assert.EqualValues(t, 2, count.Load())
	assert.Zero(t, archives.Load())
}

func TestUpdateCheckFailuresDoNotLeakOrReplaceCachedResources(t *testing.T) {
	for _, failure := range []string{"HTTP failure", "invalid release list", "invalid manifest", "incompatible release"} {
		t.Run(failure, func(t *testing.T) {
			resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
			original, err := resolver.resolve(t.Context(), io.Discard, false)
			require.NoError(t, err)
			expireUpdateCheck(t, resolver)
			release := resourceRelease(t, "1.1.0", viewerProtocol)
			if failure == "invalid manifest" {
				release.manifest = []byte(`{"private":"https://user:password@host/?sig=private#private"`)
			} else if failure == "incompatible release" {
				release.protocol++
				var document resourceDocument
				require.NoError(t, json.Unmarshal(release.manifest, &document))
				document.ProtocolVersion++
				release.manifest, err = json.Marshal(document)
				require.NoError(t, err)
			}
			newer, server, _ := resourceServer(t, release)
			newer.cacheDir = resolver.cacheDir
			if failure == "HTTP failure" || failure == "invalid release list" {
				response := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					if failure == "HTTP failure" {
						http.Error(writer, "private user:password?sig=private", http.StatusTooManyRequests)
					} else {
						_, _ = io.WriteString(writer, `{"private":"user:password?sig=private"}`)
					}
				}))
				defer response.Close()
				newer.releasesURL, newer.client = response.URL, response.Client()
			} else {
				newer.client = server.Client()
			}
			_, archives := monitorUpdateRequests(newer)
			var progress bytes.Buffer
			snapshot, err := newer.resolve(t.Context(), &progress, false)
			require.NoError(t, err)
			assert.Equal(t, original.info, snapshot.info)
			assert.Contains(t, progress.String(), "Warning: CU Viewer updates could not be checked")
			assert.NotContains(t, progress.String(), "new CU Viewer version")
			assert.NotContains(t, progress.String(), "private")
			assert.NotContains(t, progress.String(), "password")
			assert.Zero(t, archives.Load())
		})
	}
}

func TestUpdateCheckTimeoutKeepsCachedViewingUsable(t *testing.T) {
	resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	expireUpdateCheck(t, resolver)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	resolver.releasesURL, resolver.client = server.URL, server.Client()
	var progress bytes.Buffer
	started := time.Now()
	snapshot, err := resolver.resolve(t.Context(), &progress, false)
	require.NoError(t, err)
	assert.Less(t, time.Since(started), updateCheckTimeout+time.Second)
	assert.Equal(t, "1.0.0", snapshot.info.Version)
	assert.Contains(t, progress.String(), "Warning: CU Viewer updates could not be checked")
}

func TestUpdateCheckRespectsParentCancellation(t *testing.T) {
	resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	expireUpdateCheck(t, resolver)
	before, err := localio.Read(filepath.Join(resolver.cacheDir, "update-check.json"))
	require.NoError(t, err)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	resolver.releasesURL, resolver.client = server.URL, server.Client()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := resolver.resolve(ctx, io.Discard, false); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the update check did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the update check did not acknowledge cancellation")
	}
	after, err := localio.Read(filepath.Join(resolver.cacheDir, "update-check.json"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestUpdateCheckMetadataFailuresWarnWithoutBlockingCachedViewing(t *testing.T) {
	for _, failure := range []string{"missing", "corrupt", "invalid version", "future timestamp", "directory"} {
		t.Run(failure, func(t *testing.T) {
			resolver, _, _ := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
			_, err := resolver.resolve(t.Context(), io.Discard, false)
			require.NoError(t, err)
			checkPath := filepath.Join(resolver.cacheDir, "update-check.json")
			switch failure {
			case "missing", "directory":
				require.NoError(t, os.Remove(checkPath))
				if failure == "directory" {
					require.NoError(t, os.Mkdir(checkPath, 0o700))
				}
			case "corrupt":
				require.NoError(t, localio.Write(checkPath, []byte("invalid"), true))
			case "invalid version", "future timestamp":
				check := updateCheck{CheckedAt: time.Now().UTC(), LatestVersion: "1.0.0"}
				if failure == "invalid version" {
					check.LatestVersion = "https://private:password@host/?sig=private"
				} else {
					check.CheckedAt = check.CheckedAt.Add(time.Hour)
				}
				require.NoError(t, resolver.saveUpdateCheck(check))
			}
			newer, _, count := resourceServer(t, resourceRelease(t, "1.1.0", viewerProtocol))
			newer.cacheDir = resolver.cacheDir
			var progress bytes.Buffer
			snapshot, err := newer.resolve(t.Context(), &progress, false)
			require.NoError(t, err)
			assert.Equal(t, "1.0.0", snapshot.info.Version)
			assert.Contains(t, progress.String(), "1.0.0 -> 1.1.0")
			assert.NotContains(t, progress.String(), "private")
			assert.NotContains(t, progress.String(), "password")
			assert.EqualValues(t, 2, count.Load())
			if failure == "corrupt" || failure == "invalid version" || failure == "directory" {
				assert.Contains(t, progress.String(), "metadata could not be verified")
			}
			if failure == "directory" {
				assert.Contains(t, progress.String(), "metadata could not be saved")
			} else {
				check, err := newer.readUpdateCheck()
				require.NoError(t, err)
				assert.True(t, check.fresh(time.Now()))
				assert.Equal(t, "1.1.0", check.LatestVersion)
			}
		})
	}
}

func TestUpdateNoticeWriterFailureIsExplicit(t *testing.T) {
	resolver, _, count := resourceServer(t, resourceRelease(t, "1.0.0", viewerProtocol))
	_, err := resolver.resolve(t.Context(), io.Discard, false)
	require.NoError(t, err)
	require.NoError(t, resolver.saveUpdateCheck(updateCheck{
		CheckedAt: time.Now().UTC(), LatestVersion: "1.1.0",
	}))
	_, err = resolver.resolve(t.Context(), failedResourceOutput{}, false)
	require.Error(t, err)
	detail, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerOutputFailed, detail.Code)
	assert.EqualValues(t, 3, count.Load())
	snapshot, err := resolver.cached()
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", snapshot.info.Version)
}
