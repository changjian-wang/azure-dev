// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/testutil"
	"azure.ai.content-understanding/internal/viewer"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const localLabels = `{"fieldLabels":{"Name":{"type":"string","valueString":"Local","source":"D(1,1,1,1,1)"}}}`

type fakeLabelViewer struct {
	prepares, shows, closes int
	input                   analysis.Input
	labels                  json.RawMessage
	result                  json.RawMessage
	warnings                []string
	writer                  io.Writer
}

func (v *fakeLabelViewer) prepare(_ context.Context, input analysis.Input, writer io.Writer) (labelViewer, error) {
	v.prepares++
	v.input, v.writer = input, writer
	return v, nil
}

func (v *fakeLabelViewer) ShowLabels(_ context.Context, labels, result json.RawMessage, warnings []string) error {
	v.shows++
	v.labels = bytes.Clone(labels)
	v.result = bytes.Clone(result)
	v.warnings = append([]string(nil), warnings...)
	_, err := fmt.Fprintln(v.writer, "Local label viewer.")
	return err
}

func (v *fakeLabelViewer) Close() error {
	v.closes++
	return nil
}

func executeLabels(t *testing.T, ctx context.Context, view *fakeLabelViewer, args ...string) (string, string, error) {
	t.Helper()
	return executeLabelSubcommand(t, ctx, view, "view", args...)
}

func executeLabelSubcommand(
	t *testing.T, ctx context.Context, view *fakeLabelViewer, subcommand string, args ...string,
) (string, string, error) {
	t.Helper()
	root, extCtx := azdext.NewExtensionRootCommand(azdext.ExtensionCommandOptions{Name: "cu", Use: "cu"})
	root.SilenceErrors, root.SilenceUsage = true, true
	root.AddCommand(newLabelCommand(extCtx, view.prepare))
	protectCommandErrors(root)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	invocation := append([]string{"label", subcommand}, args...)
	root.SetArgs(invocation)
	if err := ValidateInvocation(root, invocation); err != nil {
		return stdout.String(), stderr.String(), err
	}
	err := root.ExecuteContext(ctx)
	return stdout.String(), stderr.String(), err
}

func TestLabelShowIsCanonicalWithoutAuthentication(t *testing.T) {
	directory := testutil.TempDir(t)
	pdf := filepath.Join(directory, "invoice.pdf")
	require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600))
	require.NoError(t, os.WriteFile(pdf+".labels.json", []byte(localLabels), 0o600))
	view := &fakeLabelViewer{}
	stdout, _, err := executeLabelSubcommand(t, t.Context(), view, "show", pdf)
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, 1, view.shows)
	assert.Equal(t, localLabels, string(view.labels))
}

func TestLabelShowAutomaticallyLoadsOnlyTheFullSourceFilenameResult(t *testing.T) {
	directory := testutil.TempDir(t)
	t.Chdir(directory)
	const filename = "invoice with spaces.PDF"
	pdf := filepath.Join(directory, filename)
	require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600))
	require.NoError(t, os.WriteFile(pdf+".labels.json", []byte(localLabels), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "invoice with spaces.result.json"), []byte("wrong"), 0o600))
	require.NoError(t, os.WriteFile(pdf+".resultjson", []byte("wrong"), 0o600))
	for _, result := range []string{
		`{"status":"Succeeded","result":{"apiVersion":"2025-11-01","contents":[{"kind":"document","fields":{` +
			`"Count":{"type":"number","valueNumber":9007199254740993}}}]},"future":{"untouched":true}}`,
		`{"apiVersion":"2026-06-01-preview","contents":[{"kind":"document","fields":{}}]}`,
		`{"apiVersion":"2025-11-01","contents":[{"kind":"document","markdown":"No extracted fields"}]}`,
	} {
		require.NoError(t, os.WriteFile(pdf+".result.json", []byte(result), 0o600))
		view := &fakeLabelViewer{}
		stdout, stderr, err := executeLabelSubcommand(t, t.Context(), view, "show", filename, "--no-prompt", "--output", "none")
		require.NoError(t, err)
		assert.Empty(t, stdout)
		assert.Equal(t, "Local label viewer.\n", stderr)
		assert.Equal(t, result, string(view.result))
		assert.Empty(t, view.warnings)
		body, err := os.ReadFile(pdf + ".result.json")
		require.NoError(t, err)
		assert.Equal(t, result, string(body))
	}
}

func TestLabelShowKeepsLabelsAndSurfacesInvalidOrUnreadableResultWarnings(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"unsupported version", `{"apiVersion":"2099-01-01-preview","contents":[]}`},
		{"null fields", `{"apiVersion":"2025-11-01","contents":[{"kind":"document","fields":null}]}`},
		{"array fields", `{"apiVersion":"2025-11-01","contents":[{"kind":"document","fields":[]}]}`},
		{"not succeeded", `{"status":"Failed","result":{"apiVersion":"2025-11-01","contents":[]}}`},
		{"unsafe details", `{"private":"https://user:password@example.com/?sig=private-secret"`},
		{"not a regular file", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := testutil.TempDir(t)
			pdf := filepath.Join(directory, "invoice.pdf")
			require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600))
			require.NoError(t, os.WriteFile(pdf+".labels.json", []byte(localLabels), 0o600))
			if test.body == "" {
				require.NoError(t, os.Mkdir(pdf+".result.json", 0o700))
			} else {
				require.NoError(t, os.WriteFile(pdf+".result.json", []byte(test.body), 0o600))
			}
			view := &fakeLabelViewer{}
			stdout, stderr, err := executeLabelSubcommand(t, t.Context(), view, "show", pdf)
			require.NoError(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, strings.ToLower(stderr), "comparison unavailable")
			assert.NotContains(t, stderr, "password")
			assert.NotContains(t, stderr, "private-secret")
			assert.Nil(t, view.result)
			require.Len(t, view.warnings, 1)
			assert.Contains(t, stderr, view.warnings[0])
			assert.Equal(t, localLabels, string(view.labels))
			assert.Equal(t, 1, view.shows)
		})
	}
}

func TestLabelViewWarnsWhenAdjacentResultIsInvalidWithoutChangingInputs(t *testing.T) {
	directory := testutil.TempDir(t)
	pdf := filepath.Join(directory, "invoice.pdf")
	require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600))
	require.NoError(t, os.WriteFile(pdf+".labels.json", []byte(localLabels), 0o600))
	const invalidResult = `{"result": broken}`
	require.NoError(t, os.WriteFile(pdf+".result.json", []byte(invalidResult), 0o600))
	view := &fakeLabelViewer{}
	stdout, stderr, err := executeLabels(t, t.Context(), view, pdf)
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, strings.ToLower(stderr), "comparison unavailable")
	assert.Equal(t, 1, view.shows)
	body, err := os.ReadFile(pdf + ".result.json")
	require.NoError(t, err)
	assert.Equal(t, invalidResult, string(body))
}

func TestLabelViewReadsAdjacentLabelsWithoutAuthenticationOrResultFiles(t *testing.T) {
	t.Setenv("AZURE_TENANT_ID", "")
	for _, flags := range [][]string{nil, {"--no-prompt"}, {"--output", "none"}} {
		directory := testutil.TempDir(t)
		pdf := filepath.Join(directory, "invoice with spaces.pdf")
		require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600))
		require.NoError(t, os.WriteFile(pdf+".labels.json", []byte(localLabels), 0o600))
		view := &fakeLabelViewer{}
		stdout, stderr, err := executeLabels(t, t.Context(), view, append([]string{pdf}, flags...)...)
		require.NoError(t, err)
		assert.Empty(t, stdout)
		assert.Equal(t, "Local label viewer.\n", stderr)
		assert.Equal(t, 1, view.prepares)
		assert.Equal(t, 1, view.shows)
		assert.Equal(t, 1, view.closes)
		assert.Equal(t, pdf, view.input.Path)
		assert.Equal(t, localLabels, string(view.labels))
		assert.NoFileExists(t, pdf+".result.json")
		body, err := os.ReadFile(pdf + ".labels.json")
		require.NoError(t, err)
		assert.Equal(t, localLabels, string(body))
	}
}

func TestLabelViewRejectsInvalidInputsBeforePreparingViewer(t *testing.T) {
	directory := testutil.TempDir(t)
	pdf := filepath.Join(directory, "invoice.pdf")
	require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600))
	for _, test := range []struct {
		name string
		args []string
		body []byte
	}{
		{"missing argument", nil, nil},
		{"multiple arguments", []string{pdf, pdf}, nil},
		{"URL", []string{"https://user:private@example.com/invoice.pdf?sig=private"}, nil},
		{"missing PDF", []string{pdf + ".missing"}, nil},
		{"missing labels", []string{pdf}, nil},
		{"malformed JSON", []string{pdf}, []byte("{")},
		{"Analyze result instead of labels", []string{pdf}, []byte(`{"result":{"contents":[]}}`)},
		{"array labels", []string{pdf}, []byte(`{"fieldLabels":[]}`)},
		{"null labels", []string{pdf}, []byte(`{"fieldLabels":null}`)},
		{"oversized labels", []string{pdf}, []byte(strings.Repeat(" ", viewer.MaxLabelJSONBytes+1))},
		{"invalid UTF-8", []string{pdf}, []byte{0xff, 0xfe}},
		{"unsupported output", []string{pdf, "--output", "json"}, []byte(localLabels)},
	} {
		t.Run(test.name, func(t *testing.T) {
			labelPath := pdf + ".labels.json"
			if test.body != nil {
				require.NoError(t, os.WriteFile(labelPath, test.body, 0o600))
				t.Cleanup(func() { require.NoError(t, os.Remove(labelPath)) })
			}
			view := &fakeLabelViewer{}
			stdout, stderr, err := executeLabels(t, t.Context(), view, test.args...)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private")
			assert.Empty(t, stdout)
			assert.Empty(t, stderr)
			assert.Zero(t, view.prepares)
			assert.Zero(t, view.shows)
		})
	}
}

func TestLabelViewHonorsCancellationBeforeLocalReads(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	view := &fakeLabelViewer{}
	_, _, err := executeLabels(t, ctx, view, "missing.pdf")
	require.Error(t, err)
	assert.Zero(t, view.prepares)
}
