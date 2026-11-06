// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	viewerstatic "azure.ai.content-understanding/cu-viewer-static"
	"azure.ai.content-understanding/internal/analysis"
	"azure.ai.content-understanding/internal/exterrors"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; " +
	"worker-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; " +
	"font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; " +
	"frame-ancestors 'none'; form-action 'none'"

type Session struct {
	assets      fs.FS
	source      *os.File
	sourceInfo  fs.FileInfo
	sourceType  string
	sourcePath  string
	sessionJSON []byte
	fieldOrders FieldOrderStore
	listener    net.Listener
	server      *http.Server
	prefix      string
	address     string
	output      io.Writer
	open        browserOpener
	closeOnce   sync.Once
	closeErr    error
}

// Prepare checks the local source and resources and reserves a loopback port before analysis.
func Prepare(ctx context.Context, input analysis.Input, output io.Writer) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return prepare(ctx, input, output, viewerstatic.Files)
}

func prepare(
	ctx context.Context, input analysis.Input, output io.Writer, assets fs.FS,
) (_ *Session, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := input.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, file.Close())
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, localError(exterrors.CodeViewerSourceInvalid,
			"The CU Viewer source could not be inspected.", "Use a readable local PDF or supported image.")
	}
	contentType, err := sourceContentType(ctx, file, info.Size())
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &azdext.LocalError{
			Code: exterrors.CodeViewerSourceInvalid, Category: azdext.LocalErrorCategoryValidation,
			Message:    "--view requires a local PDF, PNG, JPEG, BMP, HEIF, or HEIC file.",
			Suggestion: "Use a supported source, or omit --view. AVIF and HEIF image sequences are not supported.",
		}
	}
	if err := validateAssets(assets); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, localError(exterrors.CodeViewerListenFailed,
			"The CU Viewer could not reserve a local port.", "Allow loopback networking and retry.")
	}
	session := &Session{
		assets: assets, source: file, sourceInfo: info, sourceType: contentType,
		sourcePath: "source/" + filepath.Base(input.Path),
		listener:   listener, prefix: "/" + rand.Text() + "/",
		output: output, open: openBrowser,
	}
	session.address = "http://" + listener.Addr().String() + session.prefix
	session.server = &http.Server{
		Handler: session, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second,
		ErrorLog: log.New(output, "CU Viewer: ", 0),
	}
	return session, nil
}

// Show publishes the complete response, opens the browser, and waits for cancellation.
func (s *Session) Show(ctx context.Context, response json.RawMessage, fieldOrders FieldOrderStore) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !json.Valid(response) || len(bytes.TrimSpace(response)) == 0 || bytes.TrimSpace(response)[0] != '{' {
		return localError(exterrors.CodeViewerResultInvalid,
			"The CU Viewer did not receive a complete analysis response.", "Retry the analysis.")
	}
	if !s.sourceUnchanged() {
		return localError(exterrors.CodeViewerSourceInvalid,
			"The source changed during analysis.", "Keep the source unchanged and retry the analysis.")
	}
	data := struct {
		Source        string          `json:"source"`
		Name          string          `json:"name"`
		Result        json.RawMessage `json:"result"`
		FieldOrderURL string          `json:"fieldOrderUrl,omitempty"`
	}{
		Source: "./source/" + url.PathEscape(s.sourceInfo.Name()),
		Name:   s.sourceInfo.Name(), Result: response,
	}
	s.fieldOrders = fieldOrders
	if fieldOrders != nil {
		data.FieldOrderURL = "./field-order"
	}
	var err error
	s.sessionJSON, err = json.Marshal(data)
	if err != nil {
		return localError(exterrors.CodeViewerResultInvalid,
			"The CU Viewer session could not be encoded.", "Retry the analysis.")
	}
	done := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = s.server.Serve(s.listener)
		close(done)
	}()
	defer func() {
		resultErr = errors.Join(resultErr, s.Close())
		<-done
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
			resultErr = errors.Join(resultErr, localError(exterrors.CodeViewerServeFailed,
				"The CU Viewer local service stopped unexpectedly.", "Retry the analysis with --view."))
		}
	}()
	if err := s.checkReady(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return localError(exterrors.CodeViewerServeFailed,
			"The CU Viewer local service is not responding.", "Allow loopback networking and retry.")
	}
	if err := s.open(ctx, s.address, s.output); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return localError(exterrors.CodeViewerBrowserFailed,
			"The default browser could not be opened. Completed analysis results were retained.",
			"Configure a default browser and retry with --view.")
	}
	if _, err := fmt.Fprintf(s.output, "CU Viewer: %s\nPress Ctrl+C to stop the local viewer.\n", s.address); err != nil {
		return localError(exterrors.CodeViewerOutputFailed,
			"The CU Viewer status could not be written.", "Check the command's diagnostic output destination.")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return localError(exterrors.CodeViewerServeFailed,
			"The CU Viewer local service stopped unexpectedly.", "Retry the analysis with --view.")
	}
}

func (s *Session) checkReady(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, s.address, nil)
	if err != nil {
		return err
	}
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if err := response.Body.Close(); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return errors.New("viewer readiness check failed")
	}
	return nil
}

func (s *Session) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	origin := request.Header.Get("Origin")
	site := request.Header.Get("Sec-Fetch-Site")
	if request.Host != s.listener.Addr().String() ||
		(origin != "" && origin != "http://"+request.Host) ||
		(site != "" && site != "same-origin" && site != "none") {
		http.Error(writer, "Forbidden", http.StatusForbidden)
		return
	}
	name, found := strings.CutPrefix(request.URL.Path, s.prefix)
	if !found || (name != "" && !fs.ValidPath(name)) {
		http.NotFound(writer, request)
		return
	}
	if name == "field-order" && s.fieldOrders != nil {
		s.serveFieldOrder(writer, request)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if name == s.sourcePath {
		if !s.sourceUnchanged() {
			http.Error(writer, "The viewer source is no longer available or has changed.", http.StatusGone)
			return
		}
		writer.Header().Set("Content-Type", s.sourceType)
		http.ServeContent(writer, request, s.sourceInfo.Name(), s.sourceInfo.ModTime(),
			io.NewSectionReader(s.source, 0, s.sourceInfo.Size()))
		return
	}
	body := s.sessionJSON
	if name != "session.json" {
		if name == "" {
			name = "index.html"
		}
		info, err := fs.Stat(s.assets, name)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
			http.NotFound(writer, request)
			return
		}
		if err == nil {
			body, err = fs.ReadFile(s.assets, name)
		}
		if err != nil {
			s.server.ErrorLog.Printf("A bundled Viewer resource could not be read: %s", name)
			http.Error(writer, "The viewer resource could not be read.", http.StatusInternalServerError)
			return
		}
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	switch filepath.Ext(name) {
	case ".js", ".mjs":
		contentType = "text/javascript; charset=utf-8"
	case ".wasm":
		contentType = "application/wasm"
	}
	if contentType != "" {
		writer.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(writer, request, name, time.Time{}, bytes.NewReader(body))
}

func (s *Session) serveFieldOrder(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet, http.MethodHead:
		order, err := s.fieldOrders.Load(request.Context())
		if err != nil {
			s.server.ErrorLog.Print("Analyzer field order could not be read from azd settings.")
			http.Error(writer, "Analyzer field order could not be read from azd settings.", http.StatusInternalServerError)
			return
		}
		body, err := json.Marshal(order)
		if err != nil {
			s.server.ErrorLog.Print("Analyzer field order could not be encoded.")
			http.Error(writer, "Analyzer field order could not be encoded.", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.ServeContent(writer, request, "field-order.json", time.Time{}, bytes.NewReader(body))
	case http.MethodPut:
		contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if err != nil || contentType != "application/json" {
			http.Error(writer, "Field order requires application/json.", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxFieldOrderSize))
		if err != nil {
			code := http.StatusBadRequest
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				code = http.StatusRequestEntityTooLarge
			}
			http.Error(writer, "Field order could not be read within the 1 MiB limit.", code)
			return
		}
		var order FieldOrder
		if err := json.Unmarshal(body, &order); err != nil || validateFieldOrder(order) != nil {
			http.Error(writer, "Field order requires unique direct child JSON-pointer paths.", http.StatusBadRequest)
			return
		}
		if err := s.fieldOrders.Save(request.Context(), order); err != nil {
			s.server.ErrorLog.Print("Analyzer field order was not saved to azd settings.")
			http.Error(writer, "Analyzer field order was not saved to azd settings.", http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.Header().Set("Allow", "GET, HEAD, PUT")
		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Session) sourceUnchanged() bool {
	info, err := s.source.Stat()
	return err == nil && info.Mode().IsRegular() && info.Size() == s.sourceInfo.Size() &&
		info.ModTime().Equal(s.sourceInfo.ModTime())
}

func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		serverErr := s.server.Close()
		listenerErr := s.listener.Close()
		if errors.Is(listenerErr, net.ErrClosed) {
			listenerErr = nil
		}
		if errors.Join(serverErr, listenerErr, s.source.Close()) != nil {
			s.closeErr = localError(exterrors.CodeViewerCloseFailed,
				"The CU Viewer could not release all local resources.", "Close the viewer command and retry.")
		}
	})
	return s.closeErr
}

func localError(code, message, suggestion string) error {
	return &azdext.LocalError{
		Code: code, Category: azdext.LocalErrorCategoryDependency,
		Message: message, Suggestion: suggestion,
	}
}
