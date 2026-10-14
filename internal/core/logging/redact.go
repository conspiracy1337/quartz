package logging

import (
	"regexp"
	"strings"
	"sync"
)

// Redacted replaces every secret the Redactor removes.
const Redacted = "[redacted]"

// Redactor strips secrets and personal data from text before it is logged,
// shown in a console or uploaded. One instance is shared by the launcher log,
// game logs and mclo.gs uploads so the rules never drift apart.
type Redactor struct {
	rules    []rule
	userName string

	mu      sync.RWMutex
	secrets map[string]struct{} // literal runtime secrets (proxy password, session tokens)
}

// minSecretLen keeps placeholder values such as the offline token "0" from
// turning every matching character in a log into "[redacted]".
const minSecretLen = 6

// AddSecret makes r hide s verbatim from now on. Values too short to be real
// secrets are ignored. It is safe to call concurrently with Redact.
func (r *Redactor) AddSecret(s string) {
	if len(s) < minSecretLen {
		return
	}
	r.mu.Lock()
	if r.secrets == nil {
		r.secrets = map[string]struct{}{}
	}
	r.secrets[s] = struct{}{}
	r.mu.Unlock()
}

// RemoveSecret stops hiding s, for example after a session ends.
func (r *Redactor) RemoveSecret(s string) {
	r.mu.Lock()
	delete(r.secrets, s)
	r.mu.Unlock()
}

// rule is a pattern and a word every match contains (ASCII, compared
// without case): most lines lack the word, so the pattern never runs on them.
type rule struct {
	re   *regexp.Regexp
	word string
}

var defaultRules = []rule{
	// JSON Web Tokens (Minecraft and Xbox access tokens).
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "eyj"},
	// "--accessToken <value>" style arguments and key=value / "key": "value" pairs.
	{regexp.MustCompile(`(?i)(--?(?:access|refresh|session|xbl|xsts|client)[_-]?token[ =])\S+`), "token"},
	{regexp.MustCompile(`(?i)("?(?:access|refresh|session|id)_?token"?\s*[:=]\s*"?)[^"\s,}]+`), "token"},
	{regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|xbl3\.0)\s+)\S+`), "authorization"},
	// Microsoft device and authorization codes in URLs.
	{regexp.MustCompile(`(?i)([?&](?:code|token|access_token)=)[^&\s]+`), "="},
	// E-mail addresses.
	{regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), "@"},
}

// NewRedactor builds a Redactor that also hides the OS user name inside
// home-folder paths (/home/<name>, C:\Users\<name>, /Users/<name>).
func NewRedactor(userName string) *Redactor {
	return &Redactor{rules: defaultRules, userName: userName}
}

// Redact returns s with every secret replaced.
func (r *Redactor) Redact(s string) string {
	r.mu.RLock()
	for secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, Redacted)
	}
	r.mu.RUnlock()
	for _, ru := range r.rules {
		if !containsFold(s, ru.word) {
			continue
		}
		if ru.re.NumSubexp() > 0 {
			s = ru.re.ReplaceAllString(s, "${1}"+Redacted)
		} else {
			s = ru.re.ReplaceAllString(s, Redacted)
		}
	}
	if len(r.userName) > 1 {
		for _, prefix := range []string{"/home/", "/Users/", `\Users\`, `\\Users\\`, "/users/"} {
			s = strings.ReplaceAll(s, prefix+r.userName, prefix+"<user>")
		}
	}
	return s
}

// containsFold reports whether s contains word (lower-case ASCII), ignoring
// ASCII case, without allocating.
func containsFold(s, word string) bool {
	n := len(word)
	for i := 0; i+n <= len(s); i++ {
		j := 0
		for j < n {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != word[j] {
				break
			}
			j++
		}
		if j == n {
			return true
		}
	}
	return false
}
