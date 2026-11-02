// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

func sourceContentType(ctx context.Context, file *os.File, size int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	header := make([]byte, 1024)
	count, err := file.ReadAt(header, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	header = header[:count]
	switch {
	case bytes.Contains(header, []byte("%PDF-")):
		return "application/pdf", nil
	case bytes.HasPrefix(header, []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return "image/png", nil
	case bytes.HasPrefix(header, []byte{255, 216, 255}):
		return "image/jpeg", nil
	case bytes.HasPrefix(header, []byte("BM")):
		return "image/bmp", nil
	}
	if len(header) < 16 || string(header[4:8]) != "ftyp" {
		return "", errors.New("unsupported viewer source")
	}
	boxSize := int64(binary.BigEndian.Uint32(header[:4]))
	if boxSize < 16 || boxSize > size || boxSize%4 != 0 {
		return "", errors.New("invalid HEIF file-type box")
	}
	supported, excluded := heifBrand(string(header[8:12]))
	reader := bufio.NewReader(io.NewSectionReader(file, 16, boxSize-16))
	var brand [4]byte
	for offset := int64(16); offset < boxSize; offset += 4 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := io.ReadFull(reader, brand[:]); err != nil {
			return "", err
		}
		match, reject := heifBrand(string(brand[:]))
		supported, excluded = supported || match, excluded || reject
	}
	if !supported || excluded {
		return "", errors.New("unsupported HEIF source")
	}
	return "image/heif", nil
}

func heifBrand(brand string) (supported, excluded bool) {
	switch brand {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1":
		return true, false
	case "avif", "avis", "msf1":
		return false, true
	default:
		return false, false
	}
}
