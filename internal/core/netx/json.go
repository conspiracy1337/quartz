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

// maxJSONBody caps every JSON response the launcher decodes from an API.
const maxJSONBody = 4 << 20

// DoJSON sends req and decodes a JSON response into out (skipped when out is
// nil). Errors are the same as Do's, so callers inspect *StatusError and its
// Body for API error codes.
func (c *Client) DoJSON(req *http.Request, out any) error {
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxJSONBody))
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxJSONBody {
		return fmt.Errorf("netx: %s: response larger than %d bytes", redactURL(req.URL), maxJSONBody)
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
