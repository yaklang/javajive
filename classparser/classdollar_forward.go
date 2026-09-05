package javaclassparser

import (
	"os"
	"strings"
)

// fixClassDollarForwardRef moves `static Class class$X;` before the first
// static field initializer that references class$X. javac rejects the
// use-before-declare as "illegal forward reference" (xstream
// CustomObjectInputStream.DATA_HOLDER_KEY). Kill-switch:
// JDEC_CLASSDOLLAR_FORWARD_OFF=1.
func fixClassDollarForwardRef(body string) string {
	if os.Getenv("JDEC_CLASSDOLLAR_FORWARD_OFF") == "1" {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "static Class class$")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("static Class "):])
		if !ok || !strings.HasPrefix(ident, "class$") {
			from = i + 1
			continue
		}
		if !strings.HasPrefix(rest, ";") {
			from = i + 1
			continue
		}
		declEnd := i + len("static Class ") + len(ident)
		if declEnd < len(body) && body[declEnd] == ';' {
			declEnd++
		}
		decl := strings.TrimLeft(body[i:declEnd], "\n")
		// Include a preceding tab/newline so we can reinsert cleanly.
		lineStart := strings.LastIndex(body[:i], "\n") + 1
		fullDecl := body[lineStart:declEnd]
		useAt := strings.Index(body[:i], ident)
		if useAt < 0 {
			from = declEnd
			continue
		}
		useLine := strings.LastIndex(body[:useAt], "\n") + 1
		if useLine >= lineStart {
			from = declEnd
			continue
		}
		body = body[:useLine] + fullDecl + "\n" + body[useLine:lineStart] + body[declEnd:]
		from = useLine + len(fullDecl) + 1
		_ = decl
	}
}
