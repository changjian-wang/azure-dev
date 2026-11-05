// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"io/fs"

	"azure.ai.content-understanding/internal/exterrors"
)

func validateAssets(assets fs.FS) error {
	for _, name := range []string{"index.html", "LICENSE.txt", "THIRD-PARTY-NOTICES.txt"} {
		body, err := fs.ReadFile(assets, name)
		if err != nil || len(body) == 0 {
			return localError(exterrors.CodeViewerAssetsInvalid,
				"The bundled CU Viewer resource "+name+" is unavailable; no analysis was started.",
				"Rebuild the extension with the complete cu-viewer-static directory and its licenses.")
		}
	}
	return nil
}
