package javaclassparser

import (
	"os"
	"strings"
)

// fixThrowObjectAsThrowable retypes `Object varN = null` to Throwable when
// the same method does `throw varN`. TreeUnmarshaller catch(RuntimeException)
// stores a ConversionException into an Object local and rethrows it.
// Kill-switch: JDEC_THROW_OBJECT_OFF=1.
func fixThrowObjectAsThrowable(body string) string {
	if os.Getenv("JDEC_THROW_OBJECT_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "throw var") || !strings.Contains(body, "Object var") {
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
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		if !strings.HasPrefix(rest, " = null;") && !strings.HasPrefix(rest, ";") {
			from = i + 1
			continue
		}
		methodEnd := nextMemberStart(body, i)
		chunk := body[i:methodEnd]
		if strings.Contains(chunk, "throw "+ident+";") || strings.Contains(chunk, "throw (Throwable) "+ident) {
			typ := assignedExceptionType(chunk, ident)
			body = body[:i] + typ + " " + ident + rest
			from = i + len(typ+" "+ident)
			continue
		}
		from = i + 1
	}
}

func assignedExceptionType(chunk, ident string) string {
	needle := ident + " = new "
	rel := strings.Index(chunk, needle)
	if rel < 0 {
		return "Throwable"
	}
	name, ok, _ := readJavaIdent(chunk[rel+len(needle):])
	if ok && (strings.HasSuffix(name, "Exception") || strings.HasSuffix(name, "Error")) {
		return name
	}
	return "Throwable"
}
