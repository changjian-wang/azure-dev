// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"time"

	"azure.ai.content-understanding/internal/localio"

	"github.com/Masterminds/semver/v3"
)

const (
	updateCheckInterval = 24 * time.Hour
	updateCheckTimeout  = 3 * time.Second
)

type updateCheck struct {
	CheckedAt     time.Time `json:"checkedAt"`
	LatestVersion string    `json:"latestVersion,omitempty"`
	Failed        bool      `json:"failed,omitempty"`
}

func (check updateCheck) fresh(now time.Time) bool {
	return !check.CheckedAt.IsZero() && !check.CheckedAt.After(now) &&
		now.Sub(check.CheckedAt) < updateCheckInterval
}

func (r *resourceResolver) resolve(
	ctx context.Context, output io.Writer, update bool,
) (*resourceSnapshot, error) {
	snapshot, err := r.resolveResources(ctx, output, update)
	if err != nil {
		return nil, err
	}
	if !update {
		if err := r.checkForUpdates(ctx, output, snapshot.info.Version); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

func (r *resourceResolver) readUpdateCheck() (updateCheck, error) {
	body, err := localio.ReadBytes(filepath.Join(r.cacheDir, "update-check.json"), maxManifestBytes)
	if err != nil {
		return updateCheck{}, err
	}
	var check updateCheck
	if err := json.Unmarshal(body, &check); err != nil || check.CheckedAt.IsZero() ||
		(check.LatestVersion == "" && !check.Failed) ||
		(check.LatestVersion != "" && !validResourceVersion(check.LatestVersion)) {
		return updateCheck{}, errors.New("invalid Viewer update-check metadata")
	}
	return check, nil
}

func (r *resourceResolver) saveUpdateCheck(check updateCheck) error {
	body, err := json.Marshal(check)
	if err != nil {
		return err
	}
	return localio.Write(filepath.Join(r.cacheDir, "update-check.json"), body, true)
}

func (r *resourceResolver) checkForUpdates(
	ctx context.Context, output io.Writer, currentVersion string,
) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	check, readErr := r.readUpdateCheck()
	if readErr == nil && check.fresh(time.Now()) {
		return showUpdateNotice(output, currentVersion, check)
	}
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		if err := reportResource(output,
			"Warning: CU Viewer update-check metadata could not be verified; checking for updates again."); err != nil {
			return err
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	lock, err := r.lockCache(checkCtx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		check.Failed = true
		return showUpdateNotice(output, currentVersion, check)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			resultErr = errors.Join(resultErr, reportResource(output,
				"Warning: The CU Viewer update-check lock could not be released; cached viewing is unchanged."))
		}
	}()
	// Another process may have checked or manually updated the Viewer while this one waited.
	latestCheck, err := r.readUpdateCheck()
	if err == nil {
		check = latestCheck
		if check.fresh(time.Now()) {
			return showUpdateNotice(output, currentVersion, check)
		}
	} else if readErr == nil && !errors.Is(err, fs.ErrNotExist) {
		if err := reportResource(output,
			"Warning: CU Viewer update-check metadata could not be verified; checking for updates again."); err != nil {
			return err
		}
	}
	document, _, _, err := r.compatibleRelease(checkCtx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	check.CheckedAt, check.Failed = time.Now().UTC(), err != nil
	if err == nil {
		check.LatestVersion = document.ViewerVersion
	}
	if err := r.saveUpdateCheck(check); err != nil {
		if err := reportResource(output,
			"Warning: CU Viewer update-check metadata could not be saved; future checks may repeat."); err != nil {
			return err
		}
	}
	return showUpdateNotice(output, currentVersion, check)
}

func showUpdateNotice(output io.Writer, currentVersion string, check updateCheck) error {
	if check.Failed {
		if err := reportResource(output,
			"Warning: CU Viewer updates could not be checked; continuing with cached version %s. "+
				"Run `azd ai cu viewer update` to check manually.", currentVersion); err != nil {
			return err
		}
	}
	if check.LatestVersion == "" {
		return nil
	}
	current, err := semver.StrictNewVersion(currentVersion)
	if err != nil {
		return cacheFailure("The cached CU Viewer version is invalid.")
	}
	latest, err := semver.StrictNewVersion(check.LatestVersion)
	if err != nil {
		return cacheFailure("The CU Viewer update-check version is invalid.")
	}
	if latest.GreaterThan(current) {
		return reportResource(output,
			"A new CU Viewer version is available: %s -> %s.\n"+
				"Run `azd ai cu viewer update` to upgrade; this session will use the cached version.",
			currentVersion, check.LatestVersion)
	}
	return nil
}
