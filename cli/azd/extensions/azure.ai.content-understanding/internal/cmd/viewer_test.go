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
	"testing"

	"azure.ai.content-understanding/internal/exterrors"
	"azure.ai.content-understanding/internal/viewer"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewerUpdateIsExplicitAndKeepsProgressOffJSONStdout(t *testing.T) {
	format := "default"
	called := 0
	command := newViewerCommand(&format, func(ctx context.Context, progress io.Writer) (viewer.ResourceInfo, error) {
		called++
		require.NoError(t, ctx.Err())
		_, err := fmt.Fprintln(progress, "Downloading CU Viewer...")
		return viewer.ResourceInfo{Version: "1.0.0", ProtocolVersion: 1, CachePath: "test-cache"}, err
	})
	command.PersistentFlags().StringVar(&format, "output", "default", "")
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"update", "--output", "json"})
	require.NoError(t, command.Execute())
	assert.Equal(t, 1, called)
	var result viewer.ResourceInfo
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "1.0.0", result.Version)
	assert.Contains(t, stderr.String(), "Downloading CU Viewer")
	assert.NotContains(t, stdout.String(), "Downloading")
}

func TestViewerUpdateFailureIsNonSuccessAndDoesNotProduceJSONResult(t *testing.T) {
	format := "json"
	command := newViewerCommand(&format, func(context.Context, io.Writer) (viewer.ResourceInfo, error) {
		return viewer.ResourceInfo{}, &azdext.LocalError{
			Code: exterrors.CodeViewerUnavailable, Category: azdext.LocalErrorCategoryDependency,
			Message: "No compatible Viewer is published.",
		}
	})
	command.SilenceUsage = true
	command.SilenceErrors = true
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"update"})
	err := command.Execute()
	require.Error(t, err)
	detail, ok := errors.AsType[*azdext.LocalError](err)
	require.True(t, ok)
	assert.Equal(t, exterrors.CodeViewerUnavailable, detail.Code)
	assert.Empty(t, stdout.String())
}
