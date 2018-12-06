package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/allenai/fileheap-client/api"
	"github.com/goware/urlx"
	"github.com/pkg/errors"
)

const userAgent = "fileheap/0.1.0"

// Client provides an API interface to FileHeap.
type Client struct {
	baseURL *url.URL
}

// New creates a new client connected the given address.
//
// Address should be in the form [scheme://]host[:port], where scheme defaults
// to "https" and port defaults to the standard port for the given scheme, i.e.
// 80 for http and 443 for https.
func New(address string) (*Client, error) {
	u, err := urlx.ParseWithDefaultScheme(address, "https")
	if err != nil {
		return nil, err
	}

	if u.Path != "" || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("address must be base server address in the form [scheme://]host[:port]")
	}
	return &Client{u}, nil
}

// sendRequest sends a request with an optional JSON-encoded body and returns the response.
func (c *Client) sendRequest(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body interface{},
) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b := &bytes.Buffer{}
		if err := json.NewEncoder(b).Encode(body); err != nil {
			return nil, err
		}
		reader = b
	}

	u := c.baseURL.ResolveReference(&url.URL{Path: path, RawQuery: query.Encode()})
	req, err := http.NewRequest(method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req.WithContext(ctx))
}

// errorFromResponse creates an error from an HTTP response, or nil on success.
func errorFromResponse(resp *http.Response) error {
	// Anything less than 400 isn't an error, so don't produce one.
	if resp.StatusCode < 400 {
		return nil
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read response")
	}

	var apiErr api.Error
	if err := json.Unmarshal(bytes, &apiErr); err != nil {
		return errors.Wrap(err, "failed to parse response")
	}

	return apiErr
}

// responseValue parses the response body and stores the result in the given value.
// The value parameter should be a pointer to the desired structure.
func parseResponse(resp *http.Response, value interface{}) error {
	if err := errorFromResponse(resp); err != nil {
		return err
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(bytes, value)
}
