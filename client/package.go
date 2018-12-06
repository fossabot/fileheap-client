package client

import (
	"context"
	"net/http"
	"net/url"
	"path"

	"github.com/allenai/fileheap-client/api"
)

// PackageOpts allows clients to set options during creation of a new package.
type PackageOpts struct{}

// NewPackage creates a new collection of files.
func (c *Client) NewPackage(ctx context.Context) (*PackageRef, *api.Package, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "/packages", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var body api.Package
	if err := parseResponse(resp, &body); err != nil {
		return nil, nil, err
	}

	return &PackageRef{client: c, id: body.ID}, &body, nil
}

// Package creates a reference to an existing package by ID.
func (c *Client) Package(id string) *PackageRef {
	return &PackageRef{client: c, id: id}
}

// PackageRef is a reference to a package.
//
// Callers should not assume the ref is valid.
type PackageRef struct {
	client *Client
	id     string
}

// Name returns the package's unique identifier.
func (p *PackageRef) Name() string { return p.id }

// URL gets the URL of a package.
func (p *PackageRef) URL() string {
	path := path.Join("/packages", p.id)
	u := p.client.baseURL.ResolveReference(&url.URL{Path: path})
	return u.String()
}

// Info returns metadata about the package.
func (p *PackageRef) Info(ctx context.Context) (*api.Package, error) {
	path := path.Join("/packages", p.id)
	resp, err := p.client.sendRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body api.Package
	if err := parseResponse(resp, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// Seal makes a package read-only. This operation is not reversible.
func (p *PackageRef) Seal(ctx context.Context) error {
	path := path.Join("/packages", p.id)
	body := &api.PackagePatch{ReadOnly: true}

	resp, err := p.client.sendRequest(ctx, http.MethodPatch, path, nil, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return errorFromResponse(resp)
}

// Delete deletes a package and all of its files.
//
// This invalidates the PackageRef and all associated file references.
func (p *PackageRef) Delete(ctx context.Context) error {
	path := path.Join("/packages", p.id)
	resp, err := p.client.sendRequest(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return errorFromResponse(resp)
}

// File returns a reference to a single file by path.
func (p *PackageRef) File(path string) *FileRef {
	return &FileRef{pkg: p, path: path}
}

// Files returns an iterator over all files in the package.
func (p *PackageRef) Files(ctx context.Context, path string) *FileIterator {
	return &FileIterator{pkg: p, ctx: ctx, path: path}
}
