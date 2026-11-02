// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"azure.ai.content-understanding/internal/exterrors"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const viewerResponse = `{"status":"Succeeded","result":{"contents":[]},"future":9007199254740993}`

func testSession(t *testing.T, output io.Writer) *Session {
	t.Helper()
	input := localInput(t, "invoice with spaces.pdf", []byte("%PDF-1.7\nsource bytes"))
	session, err := Prepare(t.Context(), input, output)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	return session
}

func viewerRequest(
	t *testing.T, client *http.Client, method, address string, headers map[string]string,
) (*http.Response, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, address, nil)
	require.NoError(t, err)
	for name, value := range headers {
		if name == "Host" {
			request.Host = value
		} else {
			request.Header.Set(name, value)
		}
	}
	response, err := client.Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, errors.Join(err, response.Body.Close()))
	return response, body
}

func TestViewerSessionServesOnlyItsResourcesAndStops(t *testing.T) {
	var output bytes.Buffer
	session := testSession(t, &output)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan string, 1)
	session.open = func(_ context.Context, address string, _ io.Writer) error {
		opened <- address
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- session.Show(ctx, json.RawMessage(viewerResponse)) }()
	var address string
	select {
	case address = <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("the viewer did not become ready")
	}
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, body := viewerRequest(t, client, http.MethodGet, address, nil)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, string(body), `<div id="root"></div>`)
	assert.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	assert.Contains(t, response.Header.Get("Content-Security-Policy"), "'wasm-unsafe-eval'")

	response, body = viewerRequest(t, client, http.MethodGet, address+"session.json", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var document struct {
		Source string          `json:"source"`
		Name   string          `json:"name"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	assert.Equal(t, "invoice with spaces.pdf", document.Name)
	assert.JSONEq(t, viewerResponse, string(document.Result))
	assert.Contains(t, string(document.Result), "9007199254740993")
	assert.NotContains(t, string(body), session.source.Name())
	base, err := url.Parse(address)
	require.NoError(t, err)
	source, err := base.Parse(document.Source)
	require.NoError(t, err)
	assert.Equal(t, base.Host, source.Host)
	assert.True(t, strings.HasPrefix(source.Path, base.Path))
	response, body = viewerRequest(t, client, http.MethodGet, source.String(), nil)
	assert.Equal(t, "application/pdf", response.Header.Get("Content-Type"))
	assert.Equal(t, "%PDF-1.7\nsource bytes", string(body))
	response, body = viewerRequest(t, client, http.MethodGet, source.String(), map[string]string{"Range": "bytes=0-4"})
	assert.Equal(t, http.StatusPartialContent, response.StatusCode)
	assert.Equal(t, "%PDF-", string(body))
	response, body = viewerRequest(t, client, http.MethodHead, source.String(), nil)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "21", response.Header.Get("Content-Length"))
	assert.Empty(t, body)

	for name := range session.assets {
		if strings.HasSuffix(name, ".wasm") {
			response, _ := viewerRequest(t, client, http.MethodGet, address+name, nil)
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, "application/wasm", response.Header.Get("Content-Type"))
		}
	}
	for _, path := range []string{"assets/", "../outside", "%2e%2e/outside", "source/another.pdf", "missing"} {
		response, body := viewerRequest(t, client, http.MethodGet, address+path, nil)
		assert.Equal(t, http.StatusNotFound, response.StatusCode, path)
		assert.NotContains(t, string(body), "source bytes")
	}
	for _, headers := range []map[string]string{
		{"Host": "untrusted.example"},
		{"Origin": "https://untrusted.example"},
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "same-site"},
	} {
		response, _ := viewerRequest(t, client, http.MethodGet, address+"session.json", headers)
		assert.Equal(t, http.StatusForbidden, response.StatusCode)
	}
	response, _ = viewerRequest(t, client, http.MethodGet, address+"session.json",
		map[string]string{"Origin": "http://" + base.Host, "Sec-Fetch-Site": "same-origin"})
	assert.Equal(t, http.StatusOK, response.StatusCode)
	response, _ = viewerRequest(t, client, http.MethodPost, address+"session.json", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
	assert.Equal(t, "GET, HEAD", response.Header.Get("Allow"))

	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("the viewer did not stop after cancellation")
	}
	assert.Contains(t, output.String(), address)
	assert.Contains(t, output.String(), "Ctrl+C")
	_, err = client.Get(address)
	require.Error(t, err)
	_, err = session.source.Stat()
	require.Error(t, err)
	require.NoError(t, session.Close())
}

type brokenViewerWriter struct{}

func (brokenViewerWriter) Write([]byte) (int, error) {
	return 0, errors.New("private writer failure")
}

func TestViewerFailuresAreExplicitAndReleaseResources(t *testing.T) {
	for _, mode := range []string{"browser", "writer", "listener"} {
		t.Run(mode, func(t *testing.T) {
			var output bytes.Buffer
			session := testSession(t, &output)
			session.open = func(context.Context, string, io.Writer) error { return nil }
			want := exterrors.CodeViewerServeFailed
			switch mode {
			case "browser":
				want = exterrors.CodeViewerBrowserFailed
				session.open = func(context.Context, string, io.Writer) error {
					return errors.New("https://private-user:private-pass@host/path?sig=private-sig#private-fragment")
				}
			case "writer":
				want = exterrors.CodeViewerOutputFailed
				session.output = brokenViewerWriter{}
			case "listener":
				require.NoError(t, session.listener.Close())
			}
			err := session.Show(t.Context(), json.RawMessage(viewerResponse))
			local, ok := errors.AsType[*azdext.LocalError](err)
			require.True(t, ok)
			assert.Equal(t, want, local.Code)
			assert.NotContains(t, err.Error(), "private")
			assert.NotContains(t, output.String(), "private")
			assert.Empty(t, output.String())
			_, err = session.source.Stat()
			require.Error(t, err)
		})
	}
}

func TestViewerRejectsInvalidResultsAndChangedSources(t *testing.T) {
	for _, result := range []string{"", "{", "null", "[]"} {
		session := testSession(t, io.Discard)
		called := false
		session.open = func(context.Context, string, io.Writer) error { called = true; return nil }
		err := session.Show(t.Context(), json.RawMessage(result))
		require.Error(t, err)
		assert.False(t, called)
	}
	session := testSession(t, io.Discard)
	require.NoError(t, os.WriteFile(session.source.Name(), []byte("changed"), 0o600))
	err := session.Show(t.Context(), json.RawMessage(viewerResponse))
	require.Error(t, err)
	local, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerSourceInvalid, local.Code)
}
