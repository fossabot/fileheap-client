package client

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"

	"github.com/allenai/fileheap-client/api"
)

// Writer writes a file's data.
//
// Implementation is inspired by cloud.google.com/go/storage.Writer
type Writer struct {
	// Create state
	ctx    context.Context
	file   *FileRef
	length int64
	digest []byte

	// Write state
	pw   *io.PipeWriter
	done chan struct{}

	// Terminal state
	lock sync.Mutex
	err  error
}

// Close completes the write and flushes any buffered data.
func (w *Writer) Close() error {
	if w.pw == nil {
		w.open()
	}
	if err := w.pw.Close(); err != nil {
		return err
	}
	<-w.done
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.err
}

// Write implements the io.Writer interface.
//
// Because writes happen asynchronously, Write may return a nil even on failure.
// Always inspect the error returned from Writer.Close to determine if upload
// was successful.
func (w *Writer) Write(p []byte) (n int, err error) {
	w.lock.Lock()
	err = w.err
	w.lock.Unlock()
	if err != nil {
		return 0, err
	}

	// If the connection isn't established, do so now.
	if w.pw == nil {
		w.open()
	}
	return w.pw.Write(p)
}

func (w *Writer) open() {
	pr, pw := io.Pipe()
	w.pw = pw

	setErr := func(err error) {
		w.lock.Lock()
		w.err = err
		w.lock.Unlock()
		pr.CloseWithError(err)
	}

	go func() {
		defer close(w.done)

		path := path.Join("/packages", w.file.pkg.id, "files", w.file.path)
		u := w.file.pkg.client.baseURL.ResolveReference(&url.URL{Path: path})
		req, err := http.NewRequest(http.MethodPut, u.String(), pr)
		if err != nil {
			setErr(err)
			return
		}
		req.Header.Set("User-Agent", userAgent)

		req.ContentLength = w.length
		if w.length == 0 {
			// A zero content length with a non-nil body is treated as unknown.
			// Explicity set the body to send zero content length.
			req.Body = http.NoBody
		}
		req.Header.Set(api.HeaderDigest, "SHA256 "+base64.StdEncoding.EncodeToString(w.digest))

		client := &http.Client{}
		resp, err := client.Do(req.WithContext(w.ctx))
		if err != nil {
			setErr(err)
			return
		}
		if err := errorFromResponse(resp); err != nil {
			setErr(err)
			return
		}
	}()
}
