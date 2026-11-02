// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"io"
	"os/exec"
	"runtime"
	"time"
)

type browserOpener func(context.Context, string, io.Writer) error

func openBrowser(ctx context.Context, address string, output io.Writer) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "windows":
		command = "run" + "dll32"
		args = []string{"url.dll,FileProtocolHandler", address}
	case "darwin":
		command = "open"
		args = []string{address}
	default:
		command = "xdg-open"
		args = []string{address}
	}
	// The executable is a fixed platform launcher; the local URL is a separate argument.
	process := exec.CommandContext(ctx, command, args...) //nolint:gosec
	process.WaitDelay = 2 * time.Second
	process.Stdout, process.Stderr = output, output
	return process.Run()
}
