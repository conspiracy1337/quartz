// Package shlex splits a user-typed argument string (JVM arguments, wrapper
// commands) into words without ever involving a shell: no variable expansion,
// globbing, or command substitution.
//
// Rules: whitespace separates words; '…' is literal; "…" groups and allows \" ;
// outside quotes a backslash escapes only a quote or whitespace. Every other
// backslash is kept, so Windows paths like C:\Games\java and \\server\share
// survive unchanged.
package shlex

import (
	"errors"
	"strings"
	"unicode"
)

// ErrUnterminated is returned when a quote is never closed.
var ErrUnterminated = errors.New("shlex: unterminated quote")

// Split returns the words of s.
func Split(s string) ([]string, error) {
	var (
		words   []string
		cur     strings.Builder
		inWord  bool
		quote   rune // 0, '\'' or '"'
		escaped bool
	)
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		if escaped {
			escaped = false
			cur.WriteRune(r)
			continue
		}
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		switch quote {
		case '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case '"':
			switch {
			case r == '\\' && next == '"':
				escaped = true
			case r == '"':
				quote = 0
			default:
				cur.WriteRune(r)
			}
		default:
			switch {
			case unicode.IsSpace(r):
				flush()
			case r == '\\' && (next == '"' || next == '\'' || (next != 0 && unicode.IsSpace(next))):
				inWord = true
				escaped = true
			case r == '\'' || r == '"':
				inWord = true
				quote = r
			default:
				inWord = true
				cur.WriteRune(r)
			}
		}
	}
	if quote != 0 {
		return nil, ErrUnterminated
	}
	flush()
	return words, nil
}
