// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"io"

	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/viewer"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

type viewerUpdater func(context.Context, io.Writer) (viewer.ResourceInfo, error)

func newViewerCommand(outputFormat *string, update viewerUpdater) *cobra.Command {
	group := &cobra.Command{Use: "viewer", Short: "Manage locally cached CU Viewer resources"}
	group.AddCommand(azdext.RegisterFlagOptions(&cobra.Command{
		Use: "update", Args: cobra.NoArgs,
		Short: "Download and activate a compatible CU Viewer without running analysis",
		Long: "Explicitly check the public cu-viewer releases for a compatible resource version.\n\n" +
			"--view otherwise reuses a verified local cache without checking for updates.\n" +
			"A failed update does not replace a previously working cached Viewer.",
		Example: "  azd ai cu viewer update\n" +
			"    Prepare the Viewer for offline use, or explicitly update its UI.\n\n" +
			"  azd ai cu viewer update --output json\n" +
			"    Return the activated version and cache location; progress is written to stderr.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := update(cmd.Context(), cmd.ErrOrStderr())
			if err != nil {
				return exterrors.FromError(err)
			}
			return exterrors.FromError(writeServiceResult(cmd, *outputFormat, result))
		},
	}, azdext.FlagOptions{Name: "output", AllowedValues: []string{"json", "none"}}))
	return group
}
