package netx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxJSONBody caps every JSON response the launcher decodes from an API,
// unless the request's context raises it (WithBodyCap).
const maxJSONBody = 4 << 20

type bodyCapKey struct{}

// WithBodyCap raises (or lowers) the JSON size cap for requests made with
// the returned context, for the few known-large answers such as a project's
// full version list.
func WithBodyCap(ctx context.Context, n int64) context.Context {
	return context.WithValue(ctx, bodyCapKey{}, n)
}

func bodyCap(ctx context.Context) int64 {
	if n, ok := ctx.Value(bodyCapKey{}).(int64); ok && n > 0 {
		return n
	}
	return maxJSONBody
}

// DoJSON sends req and decodes a JSON response into out (skipped when out is
// nil, or on 204 No Content). Errors are the same as Do's, so callers inspect
// *StatusError and its Body for API error codes.
func (c *Client) DoJSON(req *http.Request, out any) error {
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	limit := bodyCap(req.Context())
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, limit))
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("netx: %s: response larger than %d bytes", redactURL(req.URL), limit)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("netx: %s: decode: %w", redactURL(req.URL), err)
	}
	return nil
}

// JSON sends in (when non-nil) as a JSON body with method to rawURL and
// decodes the response into out.
func (c *Client) JSON(ctx context.Context, method, rawURL string, h http.Header, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	for k, vs := range h {
		req.Header[k] = vs
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.DoJSON(req, out)
}

// PostForm posts an urlencoded form and decodes the JSON response into out.
func (c *Client) PostForm(ctx context.Context, rawURL string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.DoJSON(req, out)
}
