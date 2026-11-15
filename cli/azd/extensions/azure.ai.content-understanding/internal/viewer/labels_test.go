// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLabelSessionPublishesLabelsWithoutAnalysisOrPreferences(t *testing.T) {
	var output bytes.Buffer
	session := testSession(t, &output)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan string, 1)
	session.open = func(_ context.Context, address string, _ io.Writer) error {
		opened <- address
		return nil
	}
	const labels = `{"fieldLabels":{"Count":{"type":"number","valueNumber":9007199254740993}}}`
	done := make(chan error, 1)
	go func() { done <- session.ShowLabels(ctx, json.RawMessage(labels), nil, nil) }()
	var address string
	select {
	case address = <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("the local label viewer did not become ready")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, body := viewerRequest(t, client, http.MethodGet, address+"session.json", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &document))
	assert.JSONEq(t, labels, string(document["labels"]))
	assert.NotContains(t, document, "result")
	assert.NotContains(t, document, "fieldOrderUrl")
	assert.NotContains(t, string(body), session.source.Name())
	assert.Nil(t, session.fieldOrders)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("the local label viewer did not stop")
	}
}

func TestLabelsValidationRejectsInvalidRootsAndAcceptsEmptyLabels(t *testing.T) {
	require.NoError(t, ValidateLabels(json.RawMessage(`{"fieldLabels":{}}`)))
	for _, value := range []string{"null", "[]", "{}", `{"fieldLabels":null}`, `{"fieldLabels":[]}`} {
		require.Error(t, ValidateLabels(json.RawMessage(value)))
	}
}

func TestSavedLabelResultValidationPreservesSupportedEnvelopeAndFieldSemantics(t *testing.T) {
	for _, body := range []string{
		`{"apiVersion":"2025-11-01","contents":[]}`,
		`{"apiVersion":"2025-11-01","contents":[{"kind":"document"}]}`,
		`{"apiVersion":"2026-06-01-preview","contents":[{"kind":"document","fields":{}}]}`,
		`{"status":"Succeeded","result":{"apiVersion":"2025-11-01","contents":[{"kind":"document",` +
			`"fields":{"Count":{"type":"number","valueNumber":9007199254740993}}}]}}`,
	} {
		require.NoError(t, ValidateLabelResult(json.RawMessage(body)), body)
	}
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"apiVersion":"2025-11-01","contents":null}`,
		`{"apiVersion":"2025-11-01","contents":[null]}`,
		`{"apiVersion":"2025-11-01","contents":[{"kind":""}]}`,
		`{"apiVersion":"2025-11-01","contents":[{"kind":"document","fields":null}]}`,
		`{"apiVersion":"2025-11-01","contents":[{"kind":"document","fields":[]}]} `,
		`{"status":"Running","result":{"apiVersion":"2025-11-01","contents":[]}}`,
		`{"status":"Succeeded","apiVersion":"2025-11-01","contents":[],"result":{}}`,
		`{"apiVersion":"2099-01-01-preview","contents":[]}`,
		string([]byte{0xff, 0xfe}),
	} {
		require.Error(t, ValidateLabelResult(json.RawMessage(body)))
	}
}

func TestLabelSessionIncludesSavedResultAndWarningsWithoutPreferenceRPCs(t *testing.T) {
	const labels = `{"fieldLabels":{"Name":{"type":"string","valueString":"Local"}}}`
	for _, result := range []json.RawMessage{nil, json.RawMessage(
		`{"apiVersion":"2025-11-01","contents":[{"kind":"document","fields":{` +
			`"Count":{"type":"number","valueNumber":9007199254740993}}}]}`),
	} {
		session := testSession(t, io.Discard)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		opened := make(chan string, 1)
		session.open = func(_ context.Context, address string, _ io.Writer) error {
			opened <- address
			return nil
		}
		var warnings []string
		if result == nil {
			warnings = []string{"Result comparison unavailable: showing reference labels."}
		}
		done := make(chan error, 1)
		go func() { done <- session.ShowLabels(ctx, json.RawMessage(labels), result, warnings) }()
		var address string
		select {
		case address = <-opened:
		case <-time.After(10 * time.Second):
			t.Fatal("the local comparison viewer did not become ready")
		}
		response, body := viewerRequest(t, &http.Client{Timeout: 5 * time.Second}, http.MethodGet,
			address+"session.json", nil)
		require.Equal(t, http.StatusOK, response.StatusCode)
		var document map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &document))
		assert.JSONEq(t, labels, string(document["labels"]))
		assert.NotContains(t, document, "fieldOrderUrl")
		if result == nil {
			assert.NotContains(t, document, "result")
			assert.JSONEq(t, `["Result comparison unavailable: showing reference labels."]`, string(document["warnings"]))
		} else {
			assert.JSONEq(t, string(result), string(document["result"]))
			assert.Contains(t, string(body), "9007199254740993")
			assert.NotContains(t, document, "warnings")
		}
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	}
}

func TestPrepareLabelSourceRejectsImagesBeforeOpeningAListener(t *testing.T) {
	input := localInput(t, "image.png", []byte("\x89PNG\r\n\x1a\n"))
	session, err := prepareSource(t.Context(), input, io.Discard, testAssets(), true)
	require.ErrorContains(t, err, "requires a local PDF")
	assert.Nil(t, session)
}
