// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"azure.ai.content-understanding/internal/analysis"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func localInput(t *testing.T, name string, body []byte) analysis.Input {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, body, 0o600))
	input, err := analysis.DiscoverFile(t.Context(), path)
	require.NoError(t, err)
	return input
}

func heifHeader(major string, compatible ...string) []byte {
	body := make([]byte, 16+4*len(compatible))
	size := len(body)
	if size > math.MaxUint32 {
		panic("the test HEIF header is too large")
	}
	binary.BigEndian.PutUint32(body, uint32(size))
	copy(body[4:], "ftyp")
	copy(body[8:], major)
	for index, brand := range compatible {
		copy(body[16+index*4:], brand)
	}
	return body
}

func TestViewerSourceFormatsMatchTheFrontend(t *testing.T) {
	for _, test := range []struct {
		name, contentType string
		body              []byte
	}{
		{"pdf", "application/pdf", []byte("prefix\n%PDF-1.7\n")},
		{"png", "image/png", []byte{137, 80, 78, 71, 13, 10, 26, 10}},
		{"jpeg", "image/jpeg", []byte{255, 216, 255}},
		{"bmp", "image/bmp", []byte("BM")},
		{"heic", "image/heif", heifHeader("heic")},
		{"heif", "image/heif", heifHeader("xxxx", "mif1")},
		{"empty", "", nil},
		{"text", "", []byte("not an image")},
		{"gif", "", []byte("GIF89a")},
		{"avif", "", heifHeader("avif", "mif1")},
		{"avif compatible", "", heifHeader("heic", "avif")},
		{"sequence", "", heifHeader("heic", "msf1")},
		{"unknown brand", "", heifHeader("xxxx")},
		{"truncated ftyp", "", heifHeader("heic")[:12]},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := localInput(t, "source.bin", test.body)
			file, err := input.Open()
			require.NoError(t, err)
			defer func() { require.NoError(t, file.Close()) }()
			contentType, err := sourceContentType(t.Context(), file, int64(len(test.body)))
			if test.contentType == "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.contentType, contentType)
			}
		})
	}
}

func TestViewerSourceHEIFSizeAndCancellation(t *testing.T) {
	for _, size := range []uint32{0, 15, 17, 1000} {
		header := heifHeader("heic", "mif1")
		binary.BigEndian.PutUint32(header, size)
		input := localInput(t, "source.heic", header)
		file, err := input.Open()
		require.NoError(t, err)
		_, err = sourceContentType(t.Context(), file, int64(len(header)))
		require.Error(t, err)
		require.NoError(t, file.Close())
	}
	input := localInput(t, "source.heic", heifHeader("heic", "mif1"))
	file, err := input.Open()
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = sourceContentType(ctx, file, 20)
	require.ErrorIs(t, err, context.Canceled)
	_, err = Prepare(ctx, input, nil)
	require.ErrorIs(t, err, context.Canceled)
}
