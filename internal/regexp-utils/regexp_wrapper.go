// Package regexp_utils provides a minimal, self-contained subset of the original
// yaklang regexp helper. javajive only needs RE2-compatible patterns, so this is
// backed directly by the Go standard library regexp engine (no PCRE2 dependency).
package regexp_utils

import "regexp"

// RegexpWrapper wraps a standard-library *regexp.Regexp and mirrors the small API
// surface that the decompiler relies on.
type RegexpWrapper struct {
	re *regexp.Regexp
}

// NewRegexpWrapper compiles the given RE2 pattern. It panics on an invalid
// pattern, matching the original "must compile at init" usage.
func NewRegexpWrapper(pattern string) *RegexpWrapper {
	return &RegexpWrapper{re: regexp.MustCompile(pattern)}
}

// ReplaceAllStringFunc replaces every match of the pattern with the result of
// repl. The error return mirrors the original signature; the stdlib engine never
// fails at replace time, so it is always nil.
func (r *RegexpWrapper) ReplaceAllStringFunc(src string, repl func(string) string) (string, error) {
	if r == nil || r.re == nil {
		return src, nil
	}
	return r.re.ReplaceAllStringFunc(src, repl), nil
}

// MatchString reports whether the wrapped pattern matches src.
func (r *RegexpWrapper) MatchString(src string) bool {
	if r == nil || r.re == nil {
		return false
	}
	return r.re.MatchString(src)
}
