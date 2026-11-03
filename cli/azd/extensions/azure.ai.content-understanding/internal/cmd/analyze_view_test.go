// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/cuclient"
	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/testutil"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAnalyzeViewer struct {
	prepares, shows, closes       int
	input                         analysis.Input
	response                      json.RawMessage
	writer                        io.Writer
	prepareErr, showErr, closeErr error
	onShow                        func()
}

func (v *fakeAnalyzeViewer) prepare(
	_ context.Context, input analysis.Input, writer io.Writer,
) (analyzeViewer, error) {
	v.prepares++
	v.input, v.writer = input, writer
	if v.prepareErr != nil {
		return nil, v.prepareErr
	}
	return v, nil
}

func (v *fakeAnalyzeViewer) Show(_ context.Context, response json.RawMessage) error {
	v.shows++
	v.response = bytes.Clone(response)
	if v.onShow != nil {
		v.onShow()
	}
	if v.showErr != nil {
		return v.showErr
	}
	_, err := fmt.Fprintln(v.writer, "Viewer status.")
	return err
}

func (v *fakeAnalyzeViewer) Close() error {
	v.closes++
	return v.closeErr
}

func (f *analyzeFixture) executeWithViewer(
	ctx context.Context, openViewer viewerFactory, args ...string,
) (string, string, error) {
	root := f.rootWithViewer(openViewer)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"analyze"}, args...))
	err := root.ExecuteContext(ctx)
	return stdout.String(), stderr.String(), err
}

func TestAnalyzeViewUsesOneCompleteResponseWithoutChangingOutput(t *testing.T) {
	for _, test := range []struct {
		name       string
		flags      []string
		save, full bool
		suppressed bool
		usage      bool
	}{
		{name: "default"},
		{name: "json", flags: []string{"--output", "json"}, full: true},
		{name: "explicit full", flags: []string{"--llm-input=false"}, full: true},
		{name: "none", flags: []string{"--output", "none"}, suppressed: true},
		{name: "usage", flags: []string{"--usage"}, usage: true},
		{name: "saved Markdown", save: true},
		{name: "saved JSON", flags: []string{"--output", "json"}, save: true, full: true},
		{name: "saved without console", flags: []string{"--output", "none", "--llm-input=false"},
			save: true, full: true, suppressed: true},
		{name: "no prompt", flags: []string{"--no-prompt"}},
		{name: "skip policy without existing result", flags: []string{"--on-existing", "skip"}, save: true},
		{name: "preview inline", flags: []string{"--inline", "--api-version", cuclient.PreviewAPIVersion}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newAnalyzeFixture(t)
			view := &fakeAnalyzeViewer{}
			path := filepath.Join(testutil.TempDir(t), "invoice.pdf")
			require.NoError(t, os.WriteFile(path, []byte("%PDF-1.7\n"), 0o600))
			args := append([]string{path, "--analyzer", "prebuilt-invoice", "--view"}, test.flags...)
			output := filepath.Join(testutil.TempDir(t), "result")
			if test.save {
				args = append(args, "--output-file", output)
			}
			stdout, stderr, err := f.executeWithViewer(t.Context(), view.prepare, args...)
			require.NoError(t, err)
			assert.EqualValues(t, 1, f.client.calls.Load())
			assert.Equal(t, 1, f.opens)
			assert.Equal(t, 1, f.closes)
			assert.Equal(t, 1, view.prepares)
			assert.Equal(t, 1, view.shows)
			assert.Equal(t, 1, view.closes)
			assert.Equal(t, path, view.input.Path)
			assert.JSONEq(t, analyzeEnvelope, string(view.response))
			assert.Contains(t, string(view.response), "9007199254740993")
			assert.Equal(t, "Viewer status.\n", stderr)
			switch {
			case test.suppressed:
				assert.Empty(t, stdout)
			case test.save:
				assert.Contains(t, stdout, `"succeeded"`)
			case test.full:
				assert.JSONEq(t, analyzeEnvelope, stdout)
			case test.usage:
				var wrapper struct {
					Result string `json:"result"`
				}
				require.NoError(t, json.Unmarshal([]byte(stdout), &wrapper))
				assert.Contains(t, wrapper.Result, "Picture")
			default:
				assert.Contains(t, stdout, "Picture")
				assert.False(t, json.Valid([]byte(stdout)))
			}
			if test.save {
				// #nosec G304 -- Reads only the output created in this test's private directory.
				body, err := os.ReadFile(output)
				require.NoError(t, err)
				if test.full {
					assert.JSONEq(t, analyzeEnvelope, string(body))
				} else {
					assert.Contains(t, string(body), "Picture")
					assert.False(t, json.Valid(body))
				}
			}
		})
	}
}

func TestAnalyzeViewUnsupportedInputsFailBeforeAuthenticationOrWrites(t *testing.T) {
	directory := testutil.TempDir(t)
	pdf := filepath.Join(directory, "invoice.pdf")
	require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600))
	unsupported := schemaFile(t, "not a supported source")
	for _, test := range []struct {
		name string
		args []string
	}{
		{"no input", nil},
		{"multiple", []string{pdf, unsupported}},
		{"source", []string{"--source", directory}},
		{"directory", []string{directory}},
		{"url flag", []string{"--url", "https://example.com/input.pdf?sig=private"}},
		{"url positional", []string{"https://user:private@example.com/input.pdf?sig=private#private"}},
		{"dry run", []string{pdf, "--dry-run"}},
		{"pattern", []string{pdf, "--pattern", "*.pdf"}},
		{"empty pattern", []string{pdf, "--pattern="}},
		{"recursive", []string{pdf, "--recursive"}},
		{"unsupported source", []string{unsupported}},
		{"missing source", []string{pdf + ".missing"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newAnalyzeFixture(t)
			outputDirectory := filepath.Join(testutil.TempDir(t), "not-created")
			args := append([]string{"--view", "--output-file", filepath.Join(outputDirectory, "result")}, test.args...)
			stdout, stderr, err := f.executeWithViewer(t.Context(), prepareViewer, args...)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private")
			assert.Empty(t, stdout)
			assert.Empty(t, stderr)
			assert.Zero(t, f.opens)
			assert.Zero(t, f.client.calls.Load())
			_, err = os.Stat(outputDirectory)
			require.ErrorIs(t, err, fs.ErrNotExist)
		})
	}
}

func TestAnalyzeViewRejectsSkippedAnalysisBeforePreparingViewer(t *testing.T) {
	f := newAnalyzeFixture(t)
	view := &fakeAnalyzeViewer{}
	path := schemaFile(t, "%PDF-1.7\n")
	output := schemaFile(t, "existing result")
	stdout, stderr, err := f.executeWithViewer(t.Context(), view.prepare,
		path, "--view", "--output-file", output, "--on-existing", "skip")
	require.ErrorContains(t, err, "--view")
	assert.Contains(t, err.Error(), "--on-existing skip")
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.Zero(t, f.opens)
	assert.Zero(t, f.client.calls.Load())
	assert.Zero(t, view.prepares)
}

func TestAnalyzeViewIsOptIn(t *testing.T) {
	for _, flags := range [][]string{nil, {"--view=false"}, {"--view=false", "--dry-run"}} {
		f := newAnalyzeFixture(t)
		view := &fakeAnalyzeViewer{}
		path := schemaFile(t, "ordinary analysis is not limited to viewer formats")
		_, stderr, err := f.executeWithViewer(t.Context(), view.prepare, append([]string{path}, flags...)...)
		require.NoError(t, err)
		assert.Empty(t, stderr)
		assert.Zero(t, view.prepares)
		assert.Zero(t, view.shows)
		assert.Zero(t, view.closes)
	}
}

func TestAnalyzeViewClosesOnFailuresAndRetainsCompletedResults(t *testing.T) {
	for _, stage := range []string{"prepare", "host", "analysis", "show", "close", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			f := newAnalyzeFixture(t)
			view := &fakeAnalyzeViewer{}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := &azdext.LocalError{
				Code: "viewer_test_failure", Category: azdext.LocalErrorCategoryDependency,
				Message: "A local viewer operation failed.",
			}
			switch stage {
			case "prepare":
				view.prepareErr = failure
			case "host":
				f.openErr = failure
			case "analysis":
				f.client.call = func(context.Context, cuclient.AnalyzeRequest) (cuclient.AnalyzeResponse, error) {
					return cuclient.AnalyzeResponse{}, failure
				}
			case "show":
				view.showErr = failure
			case "close":
				view.closeErr = failure
			case "cancel":
				view.onShow = cancel
				view.showErr = context.Canceled
			}
			path := schemaFile(t, "%PDF-1.7\n")
			output := filepath.Join(testutil.TempDir(t), "invoice.result.json")
			_, stderr, err := f.executeWithViewer(ctx, view.prepare,
				path, "--view", "--output", "json", "--output-file", output)
			require.Error(t, err)
			if stage == "prepare" {
				assert.Zero(t, f.opens)
				assert.Zero(t, view.closes)
			} else {
				assert.Equal(t, 1, view.closes)
			}
			if stage == "show" || stage == "close" || stage == "cancel" {
				assert.Equal(t, 1, view.shows)
				assert.EqualValues(t, 1, f.client.calls.Load())
				// #nosec G304 -- Reads only this test's requested result output.
				body, readErr := os.ReadFile(output)
				require.NoError(t, readErr)
				assert.JSONEq(t, analyzeEnvelope, string(body))
			} else {
				assert.Zero(t, view.shows)
				_, readErr := os.Stat(output)
				require.ErrorIs(t, readErr, fs.ErrNotExist)
			}
			if stage == "cancel" {
				local, ok := errors.AsType[*azdext.LocalError](err)
				require.True(t, ok)
				assert.Equal(t, exterrors.CodeCancelled, local.Code)
			}
			if stage == "show" {
				assert.Contains(t, stderr, "Analysis completed, but CU Viewer could not be opened.")
				assert.Contains(t, stderr, "saved files were retained")
			} else if stage != "close" {
				assert.Empty(t, stderr)
			}
		})
	}
}
