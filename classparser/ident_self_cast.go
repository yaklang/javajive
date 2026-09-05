package javaclassparser

import (
	"os"
	"strings"
)

// fixIdentSelfCast drops a cast whose type token is the same decompiler local as
// the operand: `(varN) varN` and `(varN)(varN)`. Those are leaked catch-placeholder
// casts (the type name was the local, not a class). Real casts use a type name
// distinct from the value (`(Throwable) var1`). Kill-switch: JDEC_IDENT_SELF_CAST_OFF=1.
func fixIdentSelfCast(body string) string {
	if os.Getenv("JDEC_IDENT_SELF_CAST_OFF") == "1" {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "(")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+1:])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, ")") {
			from = i + 1
			continue
		}
		rest = rest[1:]
		if strings.HasPrefix(rest, "(") {
			ident2, ok2, rest2 := readJavaIdent(rest[1:])
			if ok2 && ident2 == ident && strings.HasPrefix(rest2, ")") {
				body = body[:i] + ident + rest2[1:]
				from = i + len(ident)
				continue
			}
		}
		trimmed := strings.TrimLeft(rest, " \t")
		ident2, ok2, rest2 := readJavaIdent(trimmed)
		if ok2 && ident2 == ident {
			body = body[:i] + ident + rest2
			from = i + len(ident)
			continue
		}
		from = i + 1
	}
}

// nextMemberStart returns the index of the next class-member boundary after
// from. Dump members sit at one-tab indent; method bodies use two or more.
// The old marker list (public/protected/private/static/int/void/boolean)
// missed `byte[]` / `String` / `long` methods, so int-instanceof chunks
// swallowed the next method and retyped a real int local to Object.
// Kill-switch: JDEC_MEMBER_BOUND_OFF=1 restores the marker list.
func nextMemberStart(body string, from int) int {
	if os.Getenv("JDEC_MEMBER_BOUND_OFF") == "1" {
		return nextMemberStartLegacy(body, from)
	}
	i := from
	for {
		rel := strings.Index(body[i:], "\n\t")
		if rel < 0 {
			return len(body)
		}
		j := i + rel
		rest := body[j+2:]
		if rest == "" {
			return len(body)
		}
		c := rest[0]
		// Body indent, blank line, or a comment is not a member boundary.
		if c == '\t' || c == '\n' || c == '\r' || c == ' ' || c == '/' || c == '*' {
			i = j + 2
			continue
		}
		return j
	}
}

func nextMemberStartLegacy(body string, from int) int {
	markers := []string{"\n\tpublic ", "\n\tprotected ", "\n\tprivate ", "\n\tstatic ", "\n\tint ", "\n\tvoid ", "\n\tboolean "}
	best := len(body)
	for _, m := range markers {
		if rel := strings.Index(body[from:], m); rel >= 0 && from+rel < best {
			best = from + rel
		}
	}
	return best
}

func isDecompilerLocal(s string) bool {
	if !strings.HasPrefix(s, "var") || len(s) < 4 {
		return false
	}
	if s[3] < '0' || s[3] > '9' {
		return false
	}
	for i := 4; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return false
	}
	return true
}
