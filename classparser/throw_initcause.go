package javaclassparser

import (
	"os"
	"strings"
)

// fixThrowInitCauseCast wraps `throw new E().initCause(t)` as
// `throw (E) new E().initCause(t)`. initCause returns Throwable, which is
// checked; the original throw is of the Error/RuntimeException constructed.
// Real hit: xstream XStream.class$ / javac class$ helper.
// Kill-switch: JDEC_THROW_INITCAUSE_OFF=1.
func fixThrowInitCauseCast(body string) string {
	if os.Getenv("JDEC_THROW_INITCAUSE_OFF") == "1" {
		return body
	}
	const needle = "throw new "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || !strings.HasPrefix(rest, "().initCause(") {
			from = i + 1
			continue
		}
		body = body[:i] + "throw (" + ident + ") new " + ident + rest
		from = i + len("throw ("+ident+") new "+ident)
	}
}
