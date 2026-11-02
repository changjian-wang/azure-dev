// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"encoding/json"
	"io"

	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/viewer"
)

type analyzeViewer interface {
	Show(context.Context, json.RawMessage) error
	Close() error
}

type viewerFactory func(context.Context, analysis.Input, io.Writer) (analyzeViewer, error)

func prepareViewer(ctx context.Context, input analysis.Input, output io.Writer) (analyzeViewer, error) {
	return viewer.Prepare(ctx, input, output)
}

func discoverViewerInput(ctx context.Context, options analysis.InputOptions) ([]analysis.Input, error) {
	if len(options.Sources) != 0 || len(options.URLs) != 0 ||
		options.Pattern != nil || options.Recursive || len(options.Positional)+len(options.Files) != 1 {
		return nil, commandError(exterrors.CodeConflictingArguments,
			"--view requires one local file and cannot be combined with --source, --url, "+
				"--pattern, --recursive, or multiple inputs.",
			"Select one local PDF or supported image, or omit --view.")
	}
	selected := options.Positional
	if len(options.Files) != 0 {
		selected = options.Files
	}
	input, err := analysis.DiscoverFile(ctx, selected[0])
	if err != nil {
		return nil, err
	}
	return []analysis.Input{input}, nil
}
