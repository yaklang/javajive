package javaclassparser

import (
	"os"
	"strings"
)

// fixEnumNoArgCtorThisAfterLocals rewrites a synthetic enum no-arg constructor
// that materializes name/ordinal as Object locals and then calls
// `this(var1,var2, args)` — illegal (this() must be first) and the
// (Object,Object,…) overload does not exist. The real this() is the
// payload ctor: `this(args)`.
// Kill-switch: JDEC_ENUM_CTOR_THIS_FIRST_OFF=1.
func fixEnumNoArgCtorThisAfterLocals(body string) string {
	if os.Getenv("JDEC_ENUM_CTOR_THIS_FIRST_OFF") == "1" {
		return body
	}
	const mid = "{\n\tObject var1 = null;\n\tObject var2 = null;\n\t\tthis(var1,var2,"
	from := 0
	for {
		rel := strings.Index(body[from:], mid)
		if rel < 0 {
			return body
		}
		i := from + rel
		argsStart := i + len(mid)
		thisOpen := argsStart - len("var1,var2,") - 1
		if thisOpen < 0 || thisOpen >= len(body) || body[thisOpen] != '(' {
			from = i + 1
			continue
		}
		depth := 1
		j := thisOpen + 1
		for j < len(body) && depth > 0 {
			switch body[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		if depth != 0 || !strings.HasPrefix(body[j:], ";\n\t}") {
			from = i + 1
			continue
		}
		rest := body[argsStart : j-1]
		repl := "{\n\t\tthis(" + rest + ");\n\t}"
		end := j + len(";\n\t}")
		body = body[:i] + repl + body[end:]
		from = i + len(repl)
	}
}
