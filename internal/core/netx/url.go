package netx

import "net/url"

// SafeHTTPS parses raw and accepts only an https URL with a host and no
// credentials: the only kind of link the launcher hands to a browser.
func SafeHTTPS(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, false
	}
	return u, true
}
