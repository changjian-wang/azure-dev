// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/localio"
	"azure.ai.content-understanding/internal/viewer"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

type labelViewer interface {
	ShowLabels(context.Context, json.RawMessage, json.RawMessage, []string) error
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
		Use: "show <PDF>", Aliases: []string{"view"}, Short: "Show local labels and compare an adjacent saved result",
		Long: "Open a local PDF and the adjacent <PDF>.labels.json in the read-only Fields tab.\n\n" +
			"An adjacent <PDF>.result.json is loaded automatically; no additional option is needed.\n" +
			"When its first content provides fields, compare label and extracted values side by side.\n" +
			"Without a result or extracted fields, show reference labels in two columns.\n" +
			"Unreadable, invalid or unsupported results produce a warning and do not prevent label viewing.\n" +
			"No Azure authentication, upload, or service call is required. The viewer runs until Ctrl+C.\n" +
			"PDF, labels and result files are never modified. The legacy `label view` alias is retained.",
		Example: `  azd ai cu label show invoice.pdf
    Read invoice.pdf.labels.json and automatically compare invoice.pdf.result.json when available.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return exterrors.FromError(runLabelShow(cmd, extCtx, open, args[0]))
		},
	}
	group.AddCommand(azdext.RegisterFlagOptions(view, azdext.FlagOptions{
		Name: "output", AllowedValues: []string{"default", "none"},
		Usage: "Output format: default or none; local viewer status is written to stderr",
	}))
	return group
}

func runLabelShow(
	cmd *cobra.Command, extCtx *azdext.ExtensionContext, open labelViewerFactory, source string,
) (resultErr error) {
	ctx := cmd.Context()
	if err := ctx.Err(); err != nil {
		return err
	}
	if extCtx.OutputFormat != "" && extCtx.OutputFormat != "default" && extCtx.OutputFormat != "none" {
		return commandError(exterrors.CodeConflictingArguments,
			"label show supports only --output default or --output none.",
			"Use --output default or --output none for the local viewer.")
	}
	input, err := analysis.DiscoverFile(ctx, source)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return commandError(exterrors.CodeViewerSourceInvalid,
			"Label viewing requires a readable regular local PDF file.",
			"Use `azd ai cu label show <PDF>` with an existing local PDF.")
	}
	sourcePath, err := filepath.Abs(source)
	if err != nil {
		return commandError(exterrors.CodeViewerLabelsInvalid,
			"The local label path could not be resolved.", "Keep <PDF>.labels.json next to the PDF.")
	}
	labels, err := localio.ReadBytes(sourcePath+".labels.json", viewer.MaxLabelJSONBytes)
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
	result, readErr := localio.ReadBytes(sourcePath+".result.json", viewer.MaxLabelResultJSONBytes)
	var warnings []string
	if readErr != nil {
		result = nil
		if !errors.Is(readErr, os.ErrNotExist) {
			warnings = append(warnings,
				"Result comparison unavailable: the adjacent result could not be read. "+
					"Use a regular UTF-8 <PDF>.result.json of at most 64 MiB in a readable private directory. "+
					"Showing labels without comparison.")
		}
	} else if err := viewer.ValidateLabelResult(result); err != nil {
		warnings = append(warnings, "Result comparison unavailable: "+err.Error()+" Showing labels without comparison.")
		result = nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := open(ctx, input, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, session.Close()) }()
	for _, warning := range warnings {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning:", warning); err != nil {
			return commandError(exterrors.CodeViewerOutputFailed,
				"The label viewer warning could not be written.", "Check the command's diagnostic output destination.")
		}
	}
	return session.ShowLabels(ctx, labels, result, warnings)
}
