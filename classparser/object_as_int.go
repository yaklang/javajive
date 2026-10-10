package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/internal/jdecenv"
)

// fixObjectUsedAsInt retypes `Object varN = null` to `int varN = 0` when the
// local is used as an int (compared with -1, added, or passed as a write
// length). commons-io IOUtils.copyLarge: a read() int reuses an Object slot.
// Kill-switch: JDEC_OBJECT_AS_INT_OFF=1.
func fixObjectUsedAsInt(body string) string {
	if jdecenv.Get("JDEC_OBJECT_AS_INT_OFF") == "1" {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "Object var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("Object "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = null;") {
			from = i + 1
			continue
		}
		methodEnd := nextMemberStart(body, i)
		chunk := body[i:methodEnd]
		if objectUsedAsInt(chunk, ident) {
			body = body[:i] + "int " + ident + " = 0;" + rest[len(" = null;"):]
			from = i + len("int "+ident+" = 0;")
			continue
		}
		from = i + 1
	}
}

func fixIntUsedAsInstanceof(body string) string {
	if jdecenv.Get("JDEC_INT_INSTANCEOF_OBJECT_OFF") == "1" {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "int var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("int "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = 0;") {
			from = i + 1
			continue
		}
		methodEnd := nextMemberStart(body, i)
		chunk := body[i:methodEnd]
		if strings.Contains(chunk, ident+" instanceof") || strings.Contains(chunk, ident+".getClass()") {
			body = body[:i] + "Object " + ident + " = null;" + rest[len(" = 0;"):]
			from = i + len("Object "+ident+" = null;")
			continue
		}
		from = i + 1
	}
}

func objectUsedAsInt(chunk, ident string) bool {
	embeddedRef, embeddedInt := embeddedAssignComparisonEvidence(chunk, ident)
	localEvidence := jdecenv.Get("JDEC_OBJECT_AS_INT_LOCAL_EVIDENCE_OFF") != "1"
	// A local used as a monitor, assigned `new`, or compared to null is a
	// reference slot. Those coinciding with `) > (` elsewhere in the member
	// must not flip it back to int (ConstructorResolver lock object).
	if strings.Contains(chunk, "synchronized("+ident) ||
		strings.Contains(chunk, ident+" = new ") ||
		strings.Contains(chunk, ident+") == (null)") ||
		strings.Contains(chunk, ident+") != (null)") ||
		(localEvidence && embeddedRef) {
		return false
	}
	direct := strings.Contains(chunk, "(-1) != ("+ident) ||
		strings.Contains(chunk, ident+") != (-1)") ||
		strings.Contains(chunk, ") + ("+ident+")") ||
		strings.Contains(chunk, ident+") + (") ||
		strings.Contains(chunk, "Character.charCount("+ident+")") ||
		strings.Contains(chunk, "Character.charCount(("+ident) ||
		strings.Contains(chunk, "] = "+ident+" = ") ||
		strings.Contains(chunk, ".write(") && strings.Contains(chunk, ","+ident+")") ||
		strings.Contains(chunk, ident+" = "+ident) && strings.Contains(chunk, ".read(") ||
		strings.Contains(chunk, ident+") > (") ||
		strings.Contains(chunk, ") > ("+ident) ||
		strings.Contains(chunk, ident+") & (") ||
		strings.Contains(chunk, ident+") | (") ||
		strings.Contains(chunk, "("+ident+") & (") ||
		strings.Contains(chunk, "("+ident+") | (") ||
		strings.Contains(chunk, ident+") / (") ||
		strings.Contains(chunk, "("+ident+") / (") ||
		(strings.Contains(chunk, ident+") == (") && !strings.Contains(chunk, ident+") == (null)")) ||
		(strings.Contains(chunk, "("+ident+") == (") && !strings.Contains(chunk, "("+ident+") == (null)"))
	if localEvidence {
		return direct || embeddedInt
	}
	// Legacy compatibility path retained only for the load-bearing kill switch.
	// It joined two unrelated facts from anywhere in the method: an embedded
	// assignment to ident and an arbitrary comparison involving another value.
	return direct ||
		(strings.Contains(chunk, "("+ident+" = ") && strings.Contains(chunk, ") > (")) ||
		(strings.Contains(chunk, "("+ident+" = ") && strings.Contains(chunk, ") == (") && !strings.Contains(chunk, "== (null)"))
}

// embeddedAssignComparisonEvidence classifies only the comparison immediately
// following an embedded assignment to ident.  A late source repair used to
// combine `(ident = ...)` with any `>`/`==` elsewhere in the method, so an
// Object proven by `(ident = get()) != null` was retyped to int when an unrelated
// branch compared two numbers (Caffeine BoundedLocalCache.refreshIfNeeded).
//
// The closing parenthesis is found with a small Java-expression scanner rather
// than a regexp: method calls can nest parentheses and string/character literals
// can contain them.  Relational operators prove a numeric category; equality
// proves it only against a numeric literal. Equality against null is stronger
// reference evidence and wins in objectUsedAsInt.
func embeddedAssignComparisonEvidence(chunk, ident string) (reference, numeric bool) {
	marker := "(" + ident + " = "
	for from := 0; from < len(chunk); {
		rel := strings.Index(chunk[from:], marker)
		if rel < 0 {
			break
		}
		open := from + rel
		close := matchingJavaParen(chunk, open)
		if close < 0 {
			break
		}
		rest := strings.TrimSpace(chunk[close+1:])
		op := ""
		for _, candidate := range []string{"<=", ">=", "==", "!=", "<", ">"} {
			if strings.HasPrefix(rest, candidate) {
				op = candidate
				break
			}
		}
		if op != "" {
			rhs := strings.TrimSpace(rest[len(op):])
			switch op {
			case "<", ">", "<=", ">=":
				numeric = true
			case "==", "!=":
				if comparisonOperandStartsNull(rhs) {
					reference = true
				} else if comparisonOperandStartsNumber(rhs) {
					numeric = true
				}
			}
		}
		from = close + 1
	}
	return reference, numeric
}

func matchingJavaParen(s string, open int) int {
	if open < 0 || open >= len(s) || s[open] != '(' {
		return -1
	}
	depth := 0
	var quote byte
	escaped := false
	for i := open; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func comparisonOperandToken(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "(") {
		s = strings.TrimSpace(s[1:])
	}
	return s
}

func comparisonOperandStartsNull(s string) bool {
	s = comparisonOperandToken(s)
	if !strings.HasPrefix(s, "null") {
		return false
	}
	return len(s) == len("null") || !isJavaIdentByte(s[len("null")])
}

func comparisonOperandStartsNumber(s string) bool {
	s = comparisonOperandToken(s)
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		s = strings.TrimSpace(s[1:])
	}
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}
