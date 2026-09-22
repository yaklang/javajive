package javaclassparser

import "strings"

// sourceCodeMask preserves offsets/newlines while removing literals and comments.
// Diagnostic sentinels are code tokens, never substrings of user string data.
func sourceCodeMask(src string) string {
	out := []byte(src)
	st := scanNormal
	depth := 0
	for i := 0; i < len(src); i++ {
		start := i
		before := st
		st = scanAdvance(src, &i, st, &depth)
		if before != scanNormal || st != scanNormal || i != start {
			for j := start; j <= i && j < len(out); j++ {
				if out[j] != '\n' && out[j] != '\r' {
					out[j] = ' '
				}
			}
		}
	}
	return string(out)
}
func hasExceptionSentinel(src string) bool {
	code := sourceCodeMask(src)
	return strings.Contains(code, "= Exception;") || strings.Contains(code, "= Exception\n")
}
