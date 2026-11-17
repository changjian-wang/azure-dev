// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"unicode/utf8"

	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/localio"
)

// MaxLabelJSONBytes bounds optional local label input before a viewer session starts.
const MaxLabelJSONBytes = 500_000

const (
	unreadableLabelsWarning = "Label comparison unavailable: the adjacent labels file could not be read. " +
		"Showing analysis results without labels."
	invalidLabelsWarning = "Label comparison unavailable: the adjacent labels file is invalid. " +
		"Showing analysis results without labels."
)

func loadAdjacentLabels(
	ctx context.Context, input analysis.Input, contentType string,
) (json.RawMessage, []string, error) {
	if contentType != "application/pdf" || input.Path == "" {
		return nil, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	labels, err := localio.ReadBytes(input.Path+".labels.json", MaxLabelJSONBytes)
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, []string{unreadableLabelsWarning}, nil
	}
	raw := json.RawMessage(labels)
	if err := ValidateLabels(raw); err != nil {
		return nil, []string{invalidLabelsWarning}, nil
	}
	return raw, nil, nil
}

// ValidateLabels checks the local label envelope without converting its typed values.
func ValidateLabels(labels json.RawMessage) error {
	var document struct {
		FieldLabels map[string]json.RawMessage `json:"fieldLabels"`
	}
	if len(labels) == 0 || len(labels) > MaxLabelJSONBytes || !utf8.Valid(labels) ||
		json.Unmarshal(labels, &document) != nil || document.FieldLabels == nil {
		return localError(exterrors.CodeViewerLabelsInvalid,
			"Label JSON must be a bounded UTF-8 object containing a fieldLabels object.",
			"Use a saved <PDF>.labels.json file with a root fieldLabels object.")
	}
	return nil
}
