// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldOrderPersistsAcrossSessionsAndIsolatesAnalyzersAndResources(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("AZD_CONFIG_DIR", directory)
	path := filepath.Join(directory, "config.json")
	original := `{"marker":9007199254740993,
		"extensions":{"ai-content-understanding":{"CU_API_VERSION":"2026-06-01-preview"}},
		"extension":{"installed":{"azure.ai.content-understanding":{"version":"0.1.1-preview","Viewer":{"other":true}},
		"another.extension":{"version":"1.0.0"}}}}`
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	first, err := NewFieldOrderStore("https://RESOURCE.services.ai.azure.com/", "prebuilt-invoice")
	require.NoError(t, err)
	second, err := NewFieldOrderStore("https://resource.services.ai.azure.com", "custom-invoice")
	require.NoError(t, err)
	otherResource, err := NewFieldOrderStore("https://other.services.ai.azure.com", "prebuilt-invoice")
	require.NoError(t, err)
	order, err := first.Load(t.Context())
	require.NoError(t, err)
	assert.Empty(t, order)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(body))

	a := FieldOrder{"$root": {"/B", "/A"}, "/Nested": {"/Nested/A~1B~0"}}
	b := FieldOrder{"$root": {"/Other"}}
	require.NoError(t, first.Save(t.Context(), a))
	require.NoError(t, second.Save(t.Context(), b))
	require.NoError(t, otherResource.Save(t.Context(), b))
	reopened, err := NewFieldOrderStore("https://resource.services.ai.azure.com/contentunderstanding", "prebuilt-invoice")
	require.NoError(t, err)
	order, err = reopened.Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, a, order)
	require.NoError(t, first.Save(t.Context(), FieldOrder{}))
	order, err = first.Load(t.Context())
	require.NoError(t, err)
	assert.Empty(t, order)
	for _, store := range []FieldOrderStore{second, otherResource} {
		order, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Equal(t, b, order)
	}
	body, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "9007199254740993")
	var saved struct {
		Extensions map[string]json.RawMessage `json:"extensions"`
		Extension  struct {
			Installed map[string]json.RawMessage `json:"installed"`
		} `json:"extension"`
	}
	require.NoError(t, json.Unmarshal(body, &saved))
	assert.JSONEq(t, `{"CU_API_VERSION":"2026-06-01-preview"}`, string(saved.Extensions["ai-content-understanding"]))
	assert.JSONEq(t, `{"version":"1.0.0"}`, string(saved.Extension.Installed["another.extension"]))
	var entry struct {
		Version string `json:"version"`
		Viewer  struct {
			Other  bool                  `json:"other"`
			Orders map[string]FieldOrder `json:"fieldOrderByAnalyzer"`
		} `json:"Viewer"`
	}
	require.NoError(t, json.Unmarshal(saved.Extension.Installed["azure.ai.content-understanding"], &entry))
	assert.Equal(t, "0.1.1-preview", entry.Version)
	assert.True(t, entry.Viewer.Other)
	assert.Len(t, entry.Viewer.Orders, 2)
	assert.NotContains(t, entry.Viewer.Orders,
		"https://resource.services.ai.azure.com/contentunderstanding#prebuilt-invoice")
}

func TestDefaultFieldOrderDoesNotCreateConfiguration(t *testing.T) {
	t.Setenv("AZD_CONFIG_DIR", t.TempDir())
	store, err := NewFieldOrderStore("https://resource.services.ai.azure.com", "prebuilt-invoice")
	require.NoError(t, err)
	order, err := store.Load(t.Context())
	require.NoError(t, err)
	assert.Empty(t, order)
	require.NoError(t, store.Save(t.Context(), FieldOrder{}))
	_, err = os.Stat(store.(*fieldOrderStore).path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestConcurrentAnalyzerCustomizationsMergeFreshConfiguration(t *testing.T) {
	t.Setenv("AZD_CONFIG_DIR", t.TempDir())
	stores := make([]FieldOrderStore, 8)
	for index := range stores {
		store, err := NewFieldOrderStore("https://resource.services.ai.azure.com", "analyzer-"+string(rune('a'+index)))
		require.NoError(t, err)
		stores[index] = store
	}
	var workers sync.WaitGroup
	failures := make(chan error, len(stores))
	for _, store := range stores {
		workers.Go(func() { failures <- store.Save(t.Context(), FieldOrder{"$root": {"/B", "/A"}}) })
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	for _, store := range stores {
		order, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Equal(t, FieldOrder{"$root": {"/B", "/A"}}, order)
	}
}

func TestInvalidFieldOrdersAndCorruptSettingsAreNotWritten(t *testing.T) {
	directory := t.TempDir()
	store := &fieldOrderStore{path: filepath.Join(directory, "config.json"), analyzer: "analyzer"}
	original := `{"unrelated":{"keep":true}}`
	require.NoError(t, os.WriteFile(store.path, []byte(original), 0o600))
	for _, order := range []FieldOrder{
		nil, {"$root": nil}, {"wrong": {"/A"}}, {"$root": {"/A", "/A"}}, {"$root": {"/A/B"}},
		{"/A~2": {"/A~2/B"}}, {"$root": {"/" + strings.Repeat("a", maxFieldOrderSize)}},
	} {
		require.Error(t, store.Save(t.Context(), order))
		body, err := os.ReadFile(store.path)
		require.NoError(t, err)
		assert.Equal(t, original, string(body))
	}
	for _, malformed := range []string{"{", "null", `{"extension":{"installed":false}}`} {
		require.NoError(t, os.WriteFile(store.path, []byte(malformed), 0o600))
		_, err := store.Load(t.Context())
		require.Error(t, err)
		require.Error(t, store.Save(t.Context(), FieldOrder{"$root": {"/A"}}))
		body, err := os.ReadFile(store.path)
		require.NoError(t, err)
		assert.Equal(t, malformed, string(body))
	}
}

func TestFieldOrderRejectsCredentialBearingEndpointsWithoutDisclosure(t *testing.T) {
	_, err := NewFieldOrderStore(
		"https://private-user:private-pass@resource.services.ai.azure.com?sig=private-sig#private-fragment",
		"prebuilt-invoice")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private")
}

type memoryFieldOrders struct {
	order FieldOrder
	err   error
	saves int
}

func (s *memoryFieldOrders) Load(context.Context) (FieldOrder, error) {
	return s.order, s.err
}

func (s *memoryFieldOrders) Save(_ context.Context, order FieldOrder) error {
	if s.err != nil {
		return s.err
	}
	s.saves++
	s.order = order
	return nil
}

func TestFieldOrderHTTPIsSessionScopedValidatedAndExplicitOnFailure(t *testing.T) {
	var output bytes.Buffer
	session := testSession(t, &output)
	store := &memoryFieldOrders{order: FieldOrder{"$root": {"/B", "/A"}}}
	session.fieldOrders = store
	request := func(method, body string, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), method, session.address+"field-order", strings.NewReader(body))
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		session.ServeHTTP(response, req)
		return response
	}
	response := request(http.MethodGet, "", nil)
	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"$root":["/B","/A"]}`, response.Body.String())
	response = request(http.MethodPut, `{}`, map[string]string{"Content-Type": "application/json"})
	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Empty(t, store.order)
	for _, test := range []struct {
		body, contentType string
		code              int
	}{
		{`{"$root":["/A","/A"]}`, "application/json", http.StatusBadRequest},
		{`{"$root":["/A/B"]}`, "application/json", http.StatusBadRequest},
		{`null`, "application/json", http.StatusBadRequest},
		{`{} {}`, "application/json", http.StatusBadRequest},
		{`{}`, "text/plain", http.StatusUnsupportedMediaType},
		{strings.Repeat(" ", maxFieldOrderSize) + "{}", "application/json", http.StatusRequestEntityTooLarge},
	} {
		response := request(http.MethodPut, test.body, map[string]string{"Content-Type": test.contentType})
		assert.Equal(t, test.code, response.Code)
	}
	for _, headers := range []map[string]string{
		{"Content-Type": "application/json", "Origin": "https://other.example"},
		{"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site"},
	} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodPut, `{}`, headers).Code)
	}
	assert.Equal(t, 1, store.saves)
	assert.Equal(t, http.StatusMethodNotAllowed, request(http.MethodPost, `{}`, nil).Code)
	store.err = errors.New("https://private-user:private-pass@host/path?sig=private-sig#private-fragment")
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := request(method, `{}`, map[string]string{"Content-Type": "application/json"})
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "private")
	}
	assert.NotContains(t, output.String(), "private")
}
