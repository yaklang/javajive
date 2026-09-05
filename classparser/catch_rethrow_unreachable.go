package javaclassparser

import (
	"os"
	"strings"
)

// fixCatchRethrowUnreachableReturn empties a catch whose only statement is
// `throw new RuntimeException(var);` when a `return` follows the catch chain.
// That throw is inserted by wrapUncaught* around an already-caught getMethod;
// the original catch was empty and fell through to the return (junit
// JUnit38ClassRunner.getAnnotations). javac rejects the leftover return as
// "unreachable statement". Kill-switch: JDEC_CATCH_RETHROW_UNREACHABLE_RETURN_OFF=1.
func fixCatchRethrowUnreachableReturn(body string) string {
	if os.Getenv("JDEC_CATCH_RETHROW_UNREACHABLE_RETURN_OFF") == "1" {
		return body
	}
	from := 0
	const head = "catch("
	for {
		rel := strings.Index(body[from:], head)
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len(head):]
		openRel := strings.Index(rest, "{")
		if openRel < 0 {
			from = i + 1
			continue
		}
		open := i + len(head) + openRel
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if !isRuntimeExceptionRethrowOnly(inner) {
			from = close
			continue
		}
		after := strings.TrimLeft(body[close+1:], " \t\n\r")
		if !strings.HasPrefix(after, "return") {
			from = close
			continue
		}
		body = body[:open+1] + body[close:]
		from = open + 1
	}
}

func isRuntimeExceptionRethrowOnly(inner string) bool {
	inner = strings.TrimSpace(inner)
	if !strings.HasPrefix(inner, "throw new RuntimeException(") {
		return false
	}
	end := strings.Index(inner, ";")
	if end < 0 {
		return false
	}
	rest := strings.TrimSpace(inner[end+1:])
	return rest == ""
}
