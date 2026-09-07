package javaclassparser

import (
	"os"
	"regexp"
	"strings"
)

// fixCtorNPECheckBeforeThis drops a requireNonNull idiom that javac emits
// immediately before `this(` / `super(` (`ident.getClass();` or
// `Objects.requireNonNull(ident);`). Those statements make `this()` not the
// first constructor call (illegal before Java 22). Lucene Analyzer$TokenStreamComponents
// is `var1.getClass(); this(var1::setReader,…)`.
// Kill-switch: JDEC_CTOR_NPE_THIS_OFF=1.
func fixCtorNPECheckBeforeThis(body string) string {
	if os.Getenv("JDEC_CTOR_NPE_THIS_OFF") == "1" {
		return body
	}
	body = stripPreludeBeforeCtorCall(body, "this(")
	body = stripPreludeBeforeCtorCall(body, "super(")
	return body
}

func stripPreludeBeforeCtorCall(body, call string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], call)
		if rel < 0 {
			return body
		}
		at := from + rel
		j := at
		for j > 0 && (body[j-1] == ' ' || body[j-1] == '\t' || body[j-1] == '\n' || body[j-1] == '\r') {
			j--
		}
		removed := false
		if strings.HasSuffix(body[:j], ".getClass();") {
			start := j - len(".getClass();")
			k := start
			for k > 0 && isJavaIdentChar(body[k-1]) {
				k--
			}
			if k < start {
				body = body[:k] + body[at:]
				from = k + len(call)
				removed = true
			}
		} else if strings.HasSuffix(body[:j], ");") {
			open := strings.LastIndex(body[:j], "Objects.requireNonNull(")
			if open >= 0 && !strings.ContainsAny(body[open:j], "\n") {
				body = body[:open] + body[at:]
				from = open + len(call)
				removed = true
			}
		}
		if !removed {
			from = at + 1
		}
	}
}

// fixEnumClinitIllegalNew removes `CONST = new EnumType("CONST", ordinal);`
// from an enum's <clinit>. javac emits those in bytecode; they are illegal in
// source (`enum classes may not be instantiated`). Nested enums dump as
// `TypeDefinition$Sort` with the same clinit shape.
// Kill-switch: JDEC_ENUM_CLINIT_NEW_OFF=1.
var enumClinitNewRe = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z_][A-Za-z0-9_]*) = new [A-Za-z0-9_$]+\("([A-Za-z_][A-Za-z0-9_]*)",\d+((?:,(?:true|false))*)\);\n`)

func fixEnumClinitIllegalNew(body string) string {
	if os.Getenv("JDEC_ENUM_CLINIT_NEW_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "enum ") || !strings.Contains(body, " = new ") {
		return body
	}
	type extra struct{ name, args string }
	var extras []extra
	body = enumClinitNewRe.ReplaceAllStringFunc(body, func(m string) string {
		sm := enumClinitNewRe.FindStringSubmatch(m)
		if len(sm) < 4 {
			return ""
		}
		args := strings.TrimPrefix(sm[3], ",")
		if args != "" {
			extras = append(extras, extra{sm[1], args})
		}
		return ""
	})
	for _, e := range extras {
		body = patchEnumConstantArgs(body, e.name, e.args)
	}
	// javac synthesizes $VALUES; it is illegal in enum source.
	if strings.Contains(body, "$VALUES = new ") {
		body = enumValuesAssignRe.ReplaceAllString(body, "")
	}
	return body
}

var enumValuesAssignRe = regexp.MustCompile(`(?m)^[ \t]+\$VALUES = new [A-Za-z0-9_$]+\[\]\{[^;]*\};\n`)

func patchEnumConstantArgs(body, name, args string) string {
	re := regexp.MustCompile(`(?m)^([ \t]+)` + regexp.QuoteMeta(name) + `([,;])`)
	return re.ReplaceAllString(body, "${1}"+name+"("+args+")${2}")
}

// fixBareNestedImports drops `import Outer.Inner;` lines whose first segment
// starts with an uppercase letter. Those are dotted nested types missing their
// package (`import Advice.OnMethodEnter` instead of `import net.bytebuddy.asm.Advice`).
// Real packages are lowercase. Mockito MockMethodAdvice$ForEquals.
// Kill-switch: JDEC_BARE_NESTED_IMPORT_OFF=1.
var bareNestedImportRe = regexp.MustCompile(`(?m)^import [A-Z][A-Za-z0-9_]*(?:\.[A-Z][A-Za-z0-9_]*)+;\n`)

func fixBareNestedImports(body string) string {
	if os.Getenv("JDEC_BARE_NESTED_IMPORT_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "import ") {
		return body
	}
	return bareNestedImportRe.ReplaceAllString(body, "")
}
