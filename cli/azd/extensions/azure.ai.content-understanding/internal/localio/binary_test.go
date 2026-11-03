// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package localio

import (
	"path/filepath"
	"testing"

	"azure.ai.content-understanding/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoundedBinaryReadsKeepExistingUTF8StateRules(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "resources", "viewer.zip")
	binary := []byte{0, 0xff, 0xfe, 0x80}
	require.NoError(t, Write(path, binary, false))
	body, err := ReadBytes(path, int64(len(binary)))
	require.NoError(t, err)
	assert.Equal(t, binary, body)
	_, err = Read(path)
	require.Error(t, err)
	_, err = ReadBytes(path, 3)
	require.Error(t, err)
	_, err = ReadBytes(path, -1)
	require.Error(t, err)
}
