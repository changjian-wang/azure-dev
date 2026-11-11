// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"

	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/localio"
	"azure.ai.content-understanding/internal/viewer"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

type labelViewer interface {
	ShowLabels(context.Context, json.RawMessage) error
	Close() error
}

type labelViewerFactory func(context.Context, analysis.Input, io.Writer) (labelViewer, error)

func prepareLabelViewer(ctx context.Context, input analysis.Input, output io.Writer) (labelViewer, error) {
	return viewer.PrepareLabels(ctx, input, output)
}

func newLabelCommand(extCtx *azdext.ExtensionContext, open labelViewerFactory) *cobra.Command {
	group := &cobra.Command{
		Use: "label", Short: "Inspect local Content Understanding labels",
	}
	view := &cobra.Command{
		Use: "view <PDF>", Short: "View a local PDF and its .labels.json file",
		Long: "Open a local PDF and the adjacent <PDF>.labels.json in the read-only Label tab.\n\n" +
			"Labels are read from fieldLabels, including nested objects and arrays.\n" +
			"No Analyze result, Azure authentication, upload, or service call is required.\n" +
			"The viewer runs locally until Ctrl+C. Source and label files are never modified.",
		Example: `  azd ai cu label view invoice.pdf
    Read invoice.pdf.labels.json and show its labels on invoice.pdf.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return exterrors.FromError(runLabelView(cmd, extCtx, open, args[0]))
		},
	}
	group.AddCommand(azdext.RegisterFlagOptions(view, azdext.FlagOptions{
		Name: "output", AllowedValues: []string{"default", "none"},
		Usage: "Output format: default or none; local viewer status is written to stderr",
	}))
	return group
}

func runLabelView(
	cmd *cobra.Command, extCtx *azdext.ExtensionContext, open labelViewerFactory, source string,
) (resultErr error) {
	ctx := cmd.Context()
	if err := ctx.Err(); err != nil {
		return err
	}
	if extCtx.OutputFormat != "" && extCtx.OutputFormat != "default" && extCtx.OutputFormat != "none" {
		return commandError(exterrors.CodeConflictingArguments,
			"label view supports only --output default or --output none.",
			"Use --output default or --output none for the local viewer.")
	}
	input, err := analysis.DiscoverFile(ctx, source)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return commandError(exterrors.CodeViewerSourceInvalid,
			"Label view requires a readable regular local PDF file.",
			"Use `azd ai cu label view <PDF>` with an existing local PDF.")
	}
	labelPath, err := filepath.Abs(source + ".labels.json")
	if err != nil {
		return commandError(exterrors.CodeViewerLabelsInvalid,
			"The local label path could not be resolved.", "Keep <PDF>.labels.json next to the PDF.")
	}
	labels, err := localio.ReadBytes(labelPath, viewer.MaxLabelJSONBytes)
	if err != nil {
		return commandError(exterrors.CodeViewerLabelsInvalid,
			"The adjacent label file could not be read.",
			"Place a readable regular UTF-8 <PDF>.labels.json file next to the PDF; the limit is 500,000 bytes.")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := viewer.ValidateLabels(labels); err != nil {
		return err
	}
	session, err := open(ctx, input, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, session.Close()) }()
	return session.ShowLabels(ctx, labels)
}
