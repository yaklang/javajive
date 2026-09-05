package javaclassparser

import (
	"os"
	"strings"
)

// fixObjectUsedAsInt retypes `Object varN = null` to `int varN = 0` when the
// local is used as an int (compared with -1, added, or passed as a write
// length). commons-io IOUtils.copyLarge: a read() int reuses an Object slot.
// Kill-switch: JDEC_OBJECT_AS_INT_OFF=1.
func fixObjectUsedAsInt(body string) string {
	if os.Getenv("JDEC_OBJECT_AS_INT_OFF") == "1" {
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
	if os.Getenv("JDEC_INT_INSTANCEOF_OBJECT_OFF") == "1" {
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
	return strings.Contains(chunk, "(-1) != ("+ident) ||
		strings.Contains(chunk, ident+") != (-1)") ||
		strings.Contains(chunk, ") + ("+ident+")") ||
		strings.Contains(chunk, ident+") + (") ||
		strings.Contains(chunk, ".write(") && strings.Contains(chunk, ","+ident+")") ||
		strings.Contains(chunk, ident+" = "+ident) && strings.Contains(chunk, ".read(") ||
		strings.Contains(chunk, ident+") > (") ||
		strings.Contains(chunk, ") > ("+ident) ||
		(strings.Contains(chunk, "("+ident+" = ") && strings.Contains(chunk, ") > (")) ||
		(strings.Contains(chunk, "("+ident+" = ") && strings.Contains(chunk, ") == (") && !strings.Contains(chunk, "== (null)")) ||
		strings.Contains(chunk, ident+") & (") ||
		strings.Contains(chunk, ident+") | (") ||
		strings.Contains(chunk, "("+ident+") & (") ||
		strings.Contains(chunk, "("+ident+") | (") ||
		strings.Contains(chunk, ident+") / (") ||
		strings.Contains(chunk, "("+ident+") / (") ||
		(strings.Contains(chunk, ident+") == (") && !strings.Contains(chunk, ident+") == (null)")) ||
		(strings.Contains(chunk, "("+ident+") == (") && !strings.Contains(chunk, "("+ident+") == (null)"))
}
