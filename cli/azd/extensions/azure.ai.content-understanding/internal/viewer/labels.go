// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"encoding/json"
	"io"
	"unicode/utf8"

	viewerstatic "azure.ai.content-understanding/cu-viewer-static"
	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/exterrors"
)

// MaxLabelJSONBytes bounds local label input before a viewer session starts.
const MaxLabelJSONBytes = 500_000

// ValidateLabels checks the local label envelope without converting its typed values.
func ValidateLabels(labels json.RawMessage) error {
	var document struct {
		FieldLabels map[string]json.RawMessage `json:"fieldLabels"`
	}
	if len(labels) == 0 || len(labels) > MaxLabelJSONBytes || !utf8.Valid(labels) ||
		json.Unmarshal(labels, &document) != nil || document.FieldLabels == nil {
		return localError(exterrors.CodeViewerLabelsInvalid,
			"Label JSON must be a bounded UTF-8 object containing a fieldLabels object.",
			"Use the saved <PDF>.labels.json file; an Analyze result is not a label file.")
	}
	return nil
}

// PrepareLabels checks a local PDF and reserves the bundled viewer without authentication.
func PrepareLabels(ctx context.Context, input analysis.Input, output io.Writer) (*Session, error) {
	return prepareSource(ctx, input, output, viewerstatic.Files, true)
}

// ShowLabels publishes local labels without an Analyze result or analyzer preference store.
func (s *Session) ShowLabels(ctx context.Context, labels json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateLabels(labels); err != nil {
		return err
	}
	if s.sourceType != "application/pdf" {
		return localError(exterrors.CodeViewerSourceInvalid,
			"Label view currently requires a local PDF.", "Use `azd ai cu label view <PDF>`.")
	}
	return s.show(ctx, nil, labels, nil)
}
