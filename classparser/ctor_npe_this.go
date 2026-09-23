package javaclassparser

import (
	"github.com/yaklang/javajive/internal/jdecenv"
	"regexp"
	"sort"
	"strings"
)

// fixCtorNPECheckBeforeThis drops a requireNonNull idiom that javac emits
// immediately before `this(` / `super(` (`ident.getClass();` or
// `Objects.requireNonNull(ident);`). Those statements make `this()` not the
// first constructor call (illegal before Java 22). Lucene Analyzer$TokenStreamComponents
// is `var1.getClass(); this(var1::setReader,…)`.
// Kill-switch: JDEC_CTOR_NPE_THIS_OFF=1.
func fixCtorNPECheckBeforeThis(body string) string {
	if jdecenv.Get("JDEC_CTOR_NPE_THIS_OFF") == "1" {
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

// fixEnumClinitIllegalNew moves enum construction back to the constant list.
// javac injects name/ordinal in <clinit>, and array constructor arguments may
// be spilled into a one-use local first. Removing only `CONST = new Enum(...)`
// is insufficient: the source constant must receive the source arguments,
// with any proven one-use local initializer substituted at that argument's
// original position so allocation and side effects keep their order.
// Kill-switch: JDEC_ENUM_CLINIT_NEW_OFF=1.
var enumClinitNewRe = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z_$][A-Za-z0-9_$]*) = new [A-Za-z0-9_.$]+\("([A-Za-z_$][A-Za-z0-9_$]*)",\d+(?:,(.*))?\);\r?\n`)

type enumSourceRange struct{ start, end int }

func fixEnumClinitIllegalNew(body string) string {
	if jdecenv.Get("JDEC_ENUM_CLINIT_NEW_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "enum ") || !strings.Contains(body, " = new ") {
		return body
	}
	type enumConstantRewrite struct{ name, args string }
	var rewrites []enumConstantRewrite
	var removals []enumSourceRange
	matches := enumClinitNewRe.FindAllStringSubmatchIndex(body, -1)
	for _, match := range matches {
		lhs := body[match[2]:match[3]]
		name := body[match[4]:match[5]]
		if lhs != name {
			continue
		}
		args := ""
		if match[6] >= 0 {
			args = body[match[6]:match[7]]
		}
		blockStart, blockEnd, ok := enumClinitBlockBounds(body, match[0])
		if !ok {
			continue
		}
		indent := enumLineIndent(body, match[0])
		var renderedArgs []string
		var localRemovals []enumSourceRange
		supported := true
		for _, arg := range splitTopLevelArgs(args) {
			rendered, consumed, ok := expandEnumClinitLocals(body, blockStart, blockEnd, match[0], indent, arg, map[string]bool{})
			if !ok {
				supported = false
				break
			}
			renderedArgs = append(renderedArgs, rendered)
			localRemovals = append(localRemovals, consumed...)
		}
		if !supported {
			// Keep the original illegal expression visible for unsupported cases;
			// silently deleting it would drop constructor arguments and behavior.
			continue
		}
		if args != "" {
			if _, ok := patchEnumConstantArgs(body, name, strings.Join(renderedArgs, ", ")); !ok {
				continue
			}
			rewrites = append(rewrites, enumConstantRewrite{name, strings.Join(renderedArgs, ", ")})
		}
		removals = append(removals, localRemovals...)
		removals = append(removals, enumSourceRange{match[0], match[1]})
	}
	// Match ranges refer to the original body. Removing from right to left keeps
	// every earlier span stable, including a local immediately before its enum
	// constructor assignment.
	sort.Slice(removals, func(i, j int) bool { return removals[i].start > removals[j].start })
	lastStart := len(body) + 1
	for _, removal := range removals {
		if removal.start < 0 || removal.end > len(body) || removal.start >= removal.end || removal.end > lastStart {
			continue
		}
		body = body[:removal.start] + body[removal.end:]
		lastStart = removal.start
	}
	for _, rewrite := range rewrites {
		body, _ = patchEnumConstantArgs(body, rewrite.name, rewrite.args)
	}
	// javac synthesizes $VALUES; it is illegal in enum source. Newer javac
	// commonly materializes the array in a temporary and then assigns that
	// local, so match the assignment rather than only `new Enum[]{...}`.
	body = enumValuesAssignRe.ReplaceAllString(body, "")
	return body
}

var enumValuesAssignRe = regexp.MustCompile(`(?m)^[ \t]*\$VALUES[ \t]*=[ \t]*[^;\r\n]+;\r?\n?`)

func patchEnumConstantArgs(body, name, args string) (string, bool) {
	if args == "" {
		return body, false
	}
	for lineStart := 0; lineStart < len(body); {
		lineEnd := strings.IndexByte(body[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(body)
		} else {
			lineEnd += lineStart
		}
		line := body[lineStart:lineEnd]
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		if strings.HasPrefix(trimmed, name) {
			pos := len(name)
			afterName := pos
			for afterName < len(trimmed) && (trimmed[afterName] == ' ' || trimmed[afterName] == '\t') {
				afterName++
			}
			if afterName == len(trimmed) || trimmed[afterName] == ',' || trimmed[afterName] == ';' || trimmed[afterName] == '(' || trimmed[afterName] == '{' {
				constantEnd := pos
				if afterName < len(trimmed) && trimmed[afterName] == '(' {
					close := enumMatchingDelimiter(trimmed, afterName, '(', ')')
					if close < 0 {
						return body, false
					}
					constantEnd = close + 1
				}
				replacement := trimmed[:pos] + "(" + args + ")" + trimmed[constantEnd:]
				return body[:lineStart+indent] + replacement + body[lineEnd:], true
			}
		}
		if lineEnd == len(body) {
			break
		}
		lineStart = lineEnd + 1
	}
	return body, false
}

func enumClinitBlockBounds(body string, at int) (int, int, bool) {
	lineStart := strings.LastIndex(body[:at], "\n") + 1
	for lineStart >= 0 {
		lineEnd := strings.IndexByte(body[lineStart:], '\n')
		if lineEnd < 0 {
			return 0, 0, false
		}
		lineEnd += lineStart
		line := strings.TrimSpace(body[lineStart:lineEnd])
		if strings.HasPrefix(line, "static") && strings.HasSuffix(line, "{") && !strings.Contains(line, "(") {
			close := strings.Index(body[lineEnd:], "\n\t}")
			if close < 0 {
				return 0, 0, false
			}
			return lineStart, lineEnd + close + len("\n\t}"), true
		}
		if lineStart == 0 {
			break
		}
		lineStart = strings.LastIndex(body[:lineStart-1], "\n") + 1
	}
	return 0, 0, false
}

func enumLineIndent(body string, lineStart int) string {
	end := lineStart
	for end < len(body) && (body[end] == ' ' || body[end] == '\t') {
		end++
	}
	return body[lineStart:end]
}

func expandEnumClinitLocals(body string, blockStart, blockEnd, before int, indent, expression string, active map[string]bool) (string, []enumSourceRange, bool) {
	locals := enumLocalTokens(expression)
	var removals []enumSourceRange
	for _, name := range locals {
		if active[name] || enumIdentifierCount(expression, name) != 1 || enumIdentifierCount(body[blockStart:blockEnd], name) != 2 {
			return "", nil, false
		}
		decl, ok := findEnumLocalInitializer(body, blockStart, before, indent, name)
		if !ok {
			return "", nil, false
		}
		active[name] = true
		rhs, nested, ok := expandEnumClinitLocals(body, blockStart, decl.rangeSpan.start, decl.rangeSpan.start, indent, decl.rhs, active)
		delete(active, name)
		if !ok {
			return "", nil, false
		}
		expression = replaceEnumIdentifier(expression, name, rhs)
		removals = append(removals, nested...)
		removals = append(removals, decl.rangeSpan)
	}
	return strings.TrimSpace(expression), removals, true
}

type enumLocalDecl struct {
	rangeSpan enumSourceRange
	rhs       string
}

func findEnumLocalInitializer(body string, blockStart, before int, indent, name string) (enumLocalDecl, bool) {
	// A compiler spill immediately before its only constructor consumer is the
	// only shape we move. Even a harmless-looking statement between them may
	// call user code, so do not float the initializer across it.
	lineEnd := before
	for lineEnd > blockStart {
		lineStart := strings.LastIndex(body[blockStart:lineEnd], "\n")
		if lineStart < 0 {
			lineStart = blockStart
		} else {
			lineStart = blockStart + lineStart + 1
		}
		line := strings.TrimSpace(body[lineStart:lineEnd])
		if eq := strings.Index(line, " = "); eq >= 0 && strings.HasSuffix(line, ";") && enumLineIndent(body, lineStart) == indent {
			fields := strings.Fields(strings.TrimSpace(line[:eq]))
			if len(fields) > 0 && fields[len(fields)-1] == name {
				if len(fields) == 1 {
					// A later reassignment means the value at the constructor site is
					// not the declaration initializer, so the source move is unsafe.
					return enumLocalDecl{}, false
				}
				rhs := strings.TrimSpace(strings.TrimSuffix(line[eq+3:], ";"))
				if rhs == "" {
					return enumLocalDecl{}, false
				}
				decl := enumLocalDecl{enumSourceRange{lineStart, lineEnd + 1}, rhs}
				if strings.TrimSpace(body[decl.rangeSpan.end:before]) != "" {
					return enumLocalDecl{}, false
				}
				return decl, true
			}
		}
		if lineStart == blockStart {
			break
		}
		lineEnd = lineStart - 1
	}
	return enumLocalDecl{}, false
}

func enumLocalTokens(expression string) []string {
	var out []string
	var quote byte
	for i := 0; i < len(expression); {
		ch := expression[i]
		if quote != 0 {
			if ch == '\\' {
				i += 2
				continue
			}
			if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			i++
			continue
		}
		if !isEnumIdentStart(ch) {
			i++
			continue
		}
		start := i
		i++
		for i < len(expression) && isEnumIdentPart(expression[i]) {
			i++
		}
		token := expression[start:i]
		if isEnumTempName(token) {
			out = append(out, token)
		}
	}
	return out
}

func enumIdentifierCount(source, name string) int {
	count := 0
	for _, token := range enumLocalTokens(source) {
		if token == name {
			count++
		}
	}
	return count
}

func replaceEnumIdentifier(source, target, replacement string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(source); {
		ch := source[i]
		if quote != 0 {
			out.WriteByte(ch)
			if ch == '\\' && i+1 < len(source) {
				i++
				out.WriteByte(source[i])
			} else if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			out.WriteByte(ch)
			i++
			continue
		}
		if !isEnumIdentStart(ch) {
			out.WriteByte(ch)
			i++
			continue
		}
		start := i
		i++
		for i < len(source) && isEnumIdentPart(source[i]) {
			i++
		}
		token := source[start:i]
		if token == target {
			out.WriteString(replacement)
		} else {
			out.WriteString(token)
		}
	}
	return out.String()
}

func isEnumTempName(name string) bool {
	if len(name) < 4 || !strings.HasPrefix(name, "var") {
		return false
	}
	for i := 3; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return true
}

func isEnumIdentStart(ch byte) bool {
	return ch == '_' || ch == '$' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}
func isEnumIdentPart(ch byte) bool {
	return isEnumIdentStart(ch) || ch >= '0' && ch <= '9'
}

func enumMatchingDelimiter(source string, start int, open, close byte) int {
	depth := 0
	var quote byte
	for i := start; i < len(source); i++ {
		ch := source[i]
		if quote != 0 {
			if ch == '\\' {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == open {
			depth++
		} else if ch == close {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// fixBareNestedImports drops `import Outer.Inner;` lines whose first segment
// starts with an uppercase letter. Those are dotted nested types missing their
// package (`import Advice.OnMethodEnter` instead of `import net.bytebuddy.asm.Advice`).
// Real packages are lowercase. Mockito MockMethodAdvice$ForEquals.
// Kill-switch: JDEC_BARE_NESTED_IMPORT_OFF=1.
var bareNestedImportRe = regexp.MustCompile(`(?m)^import [A-Z][A-Za-z0-9_]*(?:\.[A-Z][A-Za-z0-9_]*)+;\n`)

func fixBareNestedImports(body string) string {
	if jdecenv.Get("JDEC_BARE_NESTED_IMPORT_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "import ") {
		return body
	}
	return bareNestedImportRe.ReplaceAllString(body, "")
}
