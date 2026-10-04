package javaclassparser

import "strings"

// Diagnostic sentinels are code tokens, never substrings of user literals,
// text blocks or comments. Reuse the lexical shield used by source rewrites.
func hasExceptionSentinel(src string) bool {
	found := false
	rewriteJavaCode(src, func(code string) string {
		found = strings.Contains(code, "= Exception;") || strings.Contains(code, "= Exception\n")
		return code
	})
	return found
}
