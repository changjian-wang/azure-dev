// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/localio"

	"github.com/Masterminds/semver/v3"
	"github.com/gofrs/flock"
)

const (
	viewerProtocol   = 1
	maxArchiveBytes  = 32 * 1024 * 1024
	maxManifestBytes = 1024 * 1024
	releaseAPI       = "https://api.github.com/repos/changjian-wang/cu-viewer/releases?per_page=100"
	releaseDownloads = "https://github.com/changjian-wang/cu-viewer/releases/download/"
)

// ResourceInfo identifies the validated, locally cached Viewer, not a document or analysis result.
type ResourceInfo struct {
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocolVersion"`
	CachePath       string `json:"cachePath"`
}

type resourceSnapshot struct {
	info   ResourceInfo
	assets map[string][]byte
}

type cachePointer struct {
	Version        string `json:"version"`
	ManifestSHA256 string `json:"manifestSHA256"`
}

type resourceResolver struct {
	cacheDir     string
	releasesURL  string
	downloadsURL string
	client       *http.Client
}

func newResourceResolver() (*resourceResolver, error) {
	root, err := os.UserCacheDir()
	if err != nil || !filepath.IsAbs(root) {
		return nil, cacheFailure("The user cache directory could not be located.")
	}
	return &resourceResolver{
		cacheDir:    filepath.Join(root, "azure-ai-content-understanding", "viewer", "protocol-1"),
		releasesURL: releaseAPI, downloadsURL: releaseDownloads,
		client: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != "https" || len(via) >= 5 {
				return errors.New("Viewer downloads require bounded HTTPS redirects")
			}
			return nil
		}},
	}, nil
}

// UpdateResources explicitly checks for a compatible release; normal --view uses a valid cache without networking.
func UpdateResources(ctx context.Context, output io.Writer) (ResourceInfo, error) {
	resolver, err := newResourceResolver()
	if err != nil {
		return ResourceInfo{}, err
	}
	snapshot, err := resolver.resolve(ctx, output, true)
	if err != nil {
		return ResourceInfo{}, err
	}
	return snapshot.info, nil
}

func cacheFailure(message string) error {
	return localError(exterrors.CodeViewerCacheFailed, message,
		"Check the local user cache permissions and disk space, then retry; or omit --view to analyze without a viewer.")
}

func unavailable(message string) error {
	return localError(exterrors.CodeViewerUnavailable, message,
		"Check connectivity/proxy settings and the published Viewer releases, then retry --view or "+
			"`azd ai cu viewer update`. A compatible cached version works offline. Omit --view to analyze without viewing.")
}

func reportResource(output io.Writer, format string, values ...any) error {
	if _, err := fmt.Fprintf(output, format+"\n", values...); err != nil {
		return localError(exterrors.CodeViewerOutputFailed,
			"The CU Viewer preparation status could not be written.", "Check the command's diagnostic output destination.")
	}
	return nil
}

func (r *resourceResolver) resolve(
	ctx context.Context, output io.Writer, update bool,
) (_ *resourceSnapshot, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !update {
		snapshot, err := r.cached()
		if err == nil {
			return snapshot, nil
		}
		message := "CU Viewer is not cached; preparing a compatible version..."
		if !errors.Is(err, fs.ErrNotExist) {
			message = "Cached CU Viewer could not be verified; preparing a fresh compatible copy..."
		}
		if err := reportResource(output, "%s", message); err != nil {
			return nil, err
		}
	} else if err := reportResource(output, "Checking for a compatible CU Viewer release..."); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(r.cacheDir, "download.lock")
	if err := localio.Write(lockPath, nil, false); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, cacheFailure("The CU Viewer cache could not be prepared safely.")
	}
	if _, err := localio.Check(lockPath); err != nil {
		return nil, cacheFailure("The CU Viewer cache lock could not be verified.")
	}
	lock := flock.New(lockPath, flock.SetFlag(os.O_RDWR))
	defer func() {
		if err := lock.Close(); err != nil {
			resultErr = errors.Join(resultErr, cacheFailure("The CU Viewer cache lock could not be released."))
		}
	}()
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, cacheFailure("The CU Viewer cache could not be locked.")
	}
	if !update {
		if snapshot, err := r.cached(); err == nil {
			return snapshot, nil
		}
	}
	document, manifest, archiveURL, err := r.compatibleRelease(ctx)
	if err != nil {
		return nil, err
	}
	if err := reportResource(output, "Downloading CU Viewer %s...", document.ViewerVersion); err != nil {
		return nil, err
	}
	archive, err := r.get(ctx, archiveURL, maxArchiveBytes)
	if err != nil {
		return nil, err
	}
	assets, err := readAssets(archive, manifest)
	if err != nil {
		return nil, localError(exterrors.CodeViewerAssetsInvalid,
			"The downloaded CU Viewer failed its archive/resource integrity checks.",
			"No new version was activated. Retry after checking the Viewer release; any previous cache is unchanged.")
	}
	digest := sha256.Sum256(manifest)
	pointer := cachePointer{Version: document.ViewerVersion, ManifestSHA256: hex.EncodeToString(digest[:])}
	base := r.snapshotBase(pointer)
	for _, file := range []struct {
		path string
		body []byte
	}{
		{base + ".zip", archive}, {base + ".json", manifest},
	} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := localio.Write(file.path, file.body, true); err != nil {
			return nil, cacheFailure("The downloaded CU Viewer could not be cached safely; no new version was activated.")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(pointer)
	if err != nil {
		return nil, cacheFailure("The CU Viewer cache record could not be encoded.")
	}
	if err := localio.Write(filepath.Join(r.cacheDir, "current.json"), data, true); err != nil {
		return nil, cacheFailure("The CU Viewer cache could not activate the downloaded version.")
	}
	if err := reportResource(output, "CU Viewer %s is ready.", document.ViewerVersion); err != nil {
		return nil, err
	}
	return &resourceSnapshot{info: ResourceInfo{document.ViewerVersion, viewerProtocol, r.cacheDir}, assets: assets}, nil
}

func (r *resourceResolver) snapshotBase(pointer cachePointer) string {
	return filepath.Join(r.cacheDir, "versions", pointer.Version, pointer.ManifestSHA256)
}

func validResourceVersion(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	_, err := semver.StrictNewVersion(value)
	return err == nil
}

func (r *resourceResolver) cached() (*resourceSnapshot, error) {
	body, err := localio.ReadBytes(filepath.Join(r.cacheDir, "current.json"), maxManifestBytes)
	if err != nil {
		return nil, err
	}
	var pointer cachePointer
	if err := json.Unmarshal(body, &pointer); err != nil {
		return nil, errors.New("invalid Viewer cache record")
	}
	hash, decodeErr := hex.DecodeString(pointer.ManifestSHA256)
	if !validResourceVersion(pointer.Version) || decodeErr != nil || len(hash) != sha256.Size ||
		pointer.ManifestSHA256 != strings.ToLower(pointer.ManifestSHA256) {
		return nil, errors.New("invalid Viewer cache identity")
	}
	base := r.snapshotBase(pointer)
	manifest, err := localio.ReadBytes(base+".json", maxManifestBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("the cached Viewer manifest is missing")
		}
		return nil, err
	}
	digest := sha256.Sum256(manifest)
	if hex.EncodeToString(digest[:]) != pointer.ManifestSHA256 {
		return nil, errors.New("Viewer cache manifest digest changed")
	}
	var document resourceDocument
	if err := json.Unmarshal(manifest, &document); err != nil ||
		document.ViewerVersion != pointer.Version || document.ProtocolVersion != viewerProtocol {
		return nil, errors.New("incompatible Viewer cache")
	}
	archive, err := localio.ReadBytes(base+".zip", maxArchiveBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("the cached Viewer archive is missing")
		}
		return nil, err
	}
	assets, err := readAssets(archive, manifest)
	if err != nil {
		return nil, err
	}
	return &resourceSnapshot{info: ResourceInfo{pointer.Version, viewerProtocol, r.cacheDir}, assets: assets}, nil
}

type viewerRelease struct {
	Tag    string `json:"tag_name"`
	Draft  bool   `json:"draft"`
	Assets []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func (r *resourceResolver) compatibleRelease(
	ctx context.Context,
) (resourceDocument, []byte, string, error) {
	body, err := r.get(ctx, r.releasesURL, 4*maxManifestBytes)
	if err != nil {
		return resourceDocument{}, nil, "", err
	}
	var releases []viewerRelease
	if err := json.Unmarshal(body, &releases); err != nil || len(releases) > 100 {
		return resourceDocument{}, nil, "", unavailable("The CU Viewer release list is invalid.")
	}
	versions := make(map[string]*semver.Version)
	selected := []viewerRelease{}
	for _, release := range releases {
		version := strings.TrimPrefix(release.Tag, "v")
		if release.Draft || !validResourceVersion(version) {
			continue
		}
		names := map[string]bool{}
		for _, asset := range release.Assets {
			names[asset.Name] = true
		}
		if !names["viewer.zip"] || !names["manifest.json"] {
			continue
		}
		versions[release.Tag], _ = semver.StrictNewVersion(version)
		selected = append(selected, release)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		return versions[selected[i].Tag].GreaterThan(versions[selected[j].Tag])
	})
	for _, release := range selected {
		base := r.downloadsURL + url.PathEscape(release.Tag) + "/"
		manifest, err := r.get(ctx, base+"manifest.json", maxManifestBytes)
		if err != nil {
			return resourceDocument{}, nil, "", err
		}
		var document resourceDocument
		if err := json.Unmarshal(manifest, &document); err != nil {
			return resourceDocument{}, nil, "", unavailable("A published CU Viewer manifest is invalid.")
		}
		if document.ProtocolVersion != viewerProtocol {
			continue
		}
		if document.SchemaVersion != 2 || document.ViewerVersion != strings.TrimPrefix(release.Tag, "v") ||
			document.Archive.Path != "viewer.zip" || document.Archive.Bytes <= 0 ||
			document.Archive.Bytes > maxArchiveBytes {
			return resourceDocument{}, nil, "", unavailable("A published CU Viewer manifest is invalid.")
		}
		return document, manifest, base + "viewer.zip", nil
	}
	return resourceDocument{}, nil, "", unavailable(
		"No compatible CU Viewer release is published. A Viewer resource release is required before first use.")
}

func (r *resourceResolver) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, unavailable("The CU Viewer release address is invalid.")
	}
	request.Header.Set("Accept", "application/json, application/octet-stream")
	request.Header.Set("User-Agent", "azd-ai-cu-viewer")
	response, err := r.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, unavailable("The CU Viewer release service could not be reached.")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, unavailable(fmt.Sprintf("The CU Viewer release request failed (HTTP %d).", response.StatusCode))
	}
	if response.ContentLength > limit {
		return nil, unavailable("The CU Viewer release response exceeds its size limit.")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, unavailable("The CU Viewer download was interrupted.")
	}
	if int64(len(body)) > limit {
		return nil, unavailable("The CU Viewer release response exceeds its size limit.")
	}
	return body, nil
}
