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

// MaxLabelResultJSONBytes bounds optional saved results read by label show.
const MaxLabelResultJSONBytes = 64 * 1024 * 1024

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

// ValidateLabelResult checks supported saved-result shapes without converting field values.
func ValidateLabelResult(response json.RawMessage) error {
	invalid := func(message string) error {
		return localError(exterrors.CodeViewerResultInvalid, message,
			"Use a complete saved successful CU response or inner result with a supported API version.")
	}
	var document map[string]json.RawMessage
	if len(response) == 0 || len(response) > MaxLabelResultJSONBytes || !utf8.Valid(response) ||
		json.Unmarshal(response, &document) != nil || document == nil {
		return invalid("The saved result must be bounded UTF-8 JSON with an object root.")
	}
	if inner, exists := document["result"]; exists {
		var status string
		if json.Unmarshal(document["status"], &status) != nil || status != "Succeeded" {
			return invalid("The saved CU operation must have status Succeeded.")
		}
		var result map[string]json.RawMessage
		if json.Unmarshal(inner, &result) != nil || result == nil {
			return invalid("The saved CU operation result must be an object.")
		}
		document = result
	}
	var apiVersion string
	if json.Unmarshal(document["apiVersion"], &apiVersion) != nil ||
		(apiVersion != "2025-11-01" && apiVersion != "2026-06-01-preview") {
		return invalid("The saved result API version is not supported by this Viewer.")
	}
	var contents []map[string]json.RawMessage
	if json.Unmarshal(document["contents"], &contents) != nil || contents == nil {
		return invalid("The saved result must contain a contents array.")
	}
	for _, content := range contents {
		var kind string
		if content == nil || json.Unmarshal(content["kind"], &kind) != nil || kind == "" {
			return invalid("Each saved result content must be an object with a nonempty kind.")
		}
		if rawFields, exists := content["fields"]; exists {
			var fields map[string]json.RawMessage
			if json.Unmarshal(rawFields, &fields) != nil || fields == nil {
				return invalid("Saved result fields must be an object when provided.")
			}
		}
	}
	return nil
}

// PrepareLabels checks a local PDF and reserves the bundled viewer without authentication.
func PrepareLabels(ctx context.Context, input analysis.Input, output io.Writer) (*Session, error) {
	return prepareSource(ctx, input, output, viewerstatic.Files, true)
}

// ShowLabels publishes labels and an optional saved result without analyzer preference RPCs.
func (s *Session) ShowLabels(
	ctx context.Context, labels, response json.RawMessage, warnings []string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateLabels(labels); err != nil {
		return err
	}
	if response != nil {
		if err := ValidateLabelResult(response); err != nil {
			return err
		}
	}
	if s.sourceType != "application/pdf" {
		return localError(exterrors.CodeViewerSourceInvalid,
			"Label viewing currently requires a local PDF.", "Use `azd ai cu label show <PDF>`.")
	}
	return s.show(ctx, response, labels, nil, warnings)
}
