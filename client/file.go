package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/allenai/fileheap-client/api"
	"github.com/pkg/errors"
)

// FileIterator is an iterator over files within a package.
type FileIterator struct {
	pkg    *PackageRef
	path   string
	ctx    context.Context
	files  []api.FileInfo
	cursor string

	// Whether the final request has been made.
	lastRequest bool
}

// Next gets the next file in the iterator. If iterator is expended it will
// return the sentinel error Done.
func (i *FileIterator) Next() (*FileRef, *api.FileInfo, error) {
	if len(i.files) != 0 {
		result := i.files[0]
		i.files = i.files[1:]
		return &FileRef{pkg: i.pkg, path: result.Path}, &result, nil
	}

	if i.lastRequest {
		return nil, nil, ErrDone
	}

	path := path.Join("/packages", i.pkg.id, "manifest")
	query := url.Values{"cursor": {i.cursor}, "path": {i.path}}
	resp, err := i.pkg.client.sendRequest(i.ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var body api.ManifestPage
	if err := parseResponse(resp, &body); err != nil {
		return nil, nil, err
	}

	i.files = body.Files
	i.cursor = body.Cursor
	if body.Cursor == "" {
		i.lastRequest = true
	}

	return i.Next()
}

// FileRef is a reference to a single file within a package.
//
// Callers should not assume the ref is valid.
type FileRef struct {
	pkg  *PackageRef
	path string
}

// Path gets the path of a file.
func (f *FileRef) Path() string { return f.path }

// URL gets the URL of a file.
func (f *FileRef) URL() string {
	path := path.Join("/packages", f.pkg.id, "files", f.path)
	u := f.pkg.client.baseURL.ResolveReference(&url.URL{Path: path})
	return u.String()
}

// Info returns metadata about the file.
//
// In order to speed the common case, the FileRef caches the result of Info.
// Clients requiring updates should create a new FileRef for each call.
func (f *FileRef) Info(ctx context.Context) (*api.FileInfo, error) {
	req, err := http.NewRequest(http.MethodHead, f.URL(), nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	httpClient := http.Client{}
	resp, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrFileNotFound
	}

	info := &api.FileInfo{Path: f.path, Size: resp.ContentLength}
	if d := resp.Header.Get(api.HeaderDigest); d != "" {
		info.Digest, err = base64.StdEncoding.DecodeString(d)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if t := resp.Header.Get("Last-Modified"); t != "" {
		info.Updated, err = time.Parse(api.HTTPTimeFormat, t)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return info, nil
}

// Delete deletes the file. This invalidates the FileRef.
func (f *FileRef) Delete(ctx context.Context) error {
	path := path.Join("/packages", f.pkg.id, "files", f.path)
	resp, err := f.pkg.client.sendRequest(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrFileNotFound
	}
	return nil
}

// NewReader reads the contents of a stored file.
//
// If the file doesn't exist, this returns ErrFileNotFound.
//
// The caller must call Close on the returned Reader when finished reading.
func (f *FileRef) NewReader(ctx context.Context) (*Reader, error) {
	req, err := http.NewRequest(http.MethodGet, f.URL(), nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	httpClient := http.Client{}
	resp, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrFileNotFound
	}

	return &Reader{body: resp.Body, size: resp.ContentLength}, nil
}

// NewRangeReader reads at most length bytes from a file starting at the given offset.
// If length is negative, the file is read until the end.
//
// If the file doesn't exist, this returns ErrFileNotFound.
//
// The caller must call Close on the returned Reader when finished reading.
func (f *FileRef) NewRangeReader(ctx context.Context, offset, length int64) (*Reader, error) {
	req, err := http.NewRequest(http.MethodGet, f.URL(), nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if length < 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	}

	httpClient := http.Client{}
	resp, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrFileNotFound
	}

	return &Reader{body: resp.Body, size: resp.ContentLength}, nil
}

// NewWriter returns a storage Writer that writes to the file associated with
// this reference.
//
// The file will be replaced if it exists or created if not. The file
// becomes available when Close returns successfully. The previous file is
// readable until the new file replaces it.
//
// It is the caller's responsibility to call Close when writing is complete.
func (f *FileRef) NewWriter(ctx context.Context, opts *WriteOpts) (*Writer, error) {
	if opts == nil {
		opts = &WriteOpts{}
	}

	return &Writer{
		ctx:    ctx,
		file:   f,
		length: opts.Length,
		digest: opts.Digest,
		done:   make(chan struct{}),
	}, nil
}

// WriteOpts allows clients to set attributes on a file during upload.
type WriteOpts struct {
	// (required) Total length of the upload to be written.
	Length int64

	// (required) Digest is the SHA256 hash of the object's content. If set and
	// the service has matching data, the service will copy the necessary data
	// internally. The digest may also be used to validate later writes.
	Digest []byte
}
