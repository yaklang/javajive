package javaclassparser

import (
	"github.com/yaklang/javajive/internal/jdecenv"
	"regexp"
	"sort"
	"strconv"
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

// fixCtorDelegationArgumentSpills folds one-use argument locals back into a
// delegating this(...) call. A decompiler may spill an invokespecial argument
// to a source local even though Java requires this(...) to be the first
// constructor statement. We only move an uninterrupted prelude at the start
// of the constructor and require declaration order to match argument order.
// Substitution keeps every expression at its original argument index, so Java
// still evaluates direct calls and restored initializer expressions left to
// right in the same order.
// Kill-switch: JDEC_CTOR_DELEGATION_SPILLS_OFF=1.
func fixCtorDelegationArgumentSpills(body string) string {
	if jdecenv.Get("JDEC_CTOR_DELEGATION_SPILLS_OFF") == "1" {
		return body
	}
	searchEnd := len(body)
	for searchEnd > 0 {
		at := strings.LastIndex(body[:searchEnd], "this(")
		if at < 0 {
			break
		}
		if !javaCodePosition(body, at) {
			searchEnd = at
			continue
		}
		lineStart := strings.LastIndex(body[:at], "\n") + 1
		if strings.TrimSpace(body[lineStart:at]) != "" {
			searchEnd = at
			continue
		}
		open := at + len("this")
		close := enumMatchingDelimiter(body, open, '(', ')')
		if close < 0 {
			searchEnd = at
			continue
		}
		semicolon := close + 1
		for semicolon < len(body) && (body[semicolon] == ' ' || body[semicolon] == '\t' || body[semicolon] == '\r') {
			semicolon++
		}
		if semicolon >= len(body) || body[semicolon] != ';' {
			searchEnd = at
			continue
		}
		blockStart, blockEnd, ok := javaBlockBoundsAt(body, at)
		if !ok {
			searchEnd = at
			continue
		}
		indent := enumLineIndent(body, lineStart)
		var reverse []enumLocalDecl
		var reverseNames []string
		cursor := lineStart
		for cursor > blockStart+1 {
			prevEnd := cursor
			if prevEnd > 0 && body[prevEnd-1] == '\n' {
				prevEnd--
			}
			prevStart := strings.LastIndex(body[:prevEnd], "\n") + 1
			if prevStart <= blockStart {
				break
			}
			if strings.TrimSpace(body[prevStart:prevEnd]) == "" {
				cursor = prevStart
				continue
			}
			decl, name, ok := parseEnumLocalInitializerLine(body, prevStart, prevEnd, indent)
			if !ok {
				break
			}
			reverse = append(reverse, decl)
			reverseNames = append(reverseNames, name)
			cursor = prevStart
		}
		if len(reverse) == 0 {
			searchEnd = at
			continue
		}
		decls := make([]enumLocalDecl, len(reverse))
		names := make([]string, len(reverseNames))
		for i := range reverse {
			decls[i] = reverse[len(reverse)-1-i]
			names[i] = reverseNames[len(reverseNames)-1-i]
		}
		first := decls[0].rangeSpan.start
		if strings.TrimSpace(body[blockStart+1:first]) != "" {
			searchEnd = at
			continue
		}
		methodBody := body[blockStart:blockEnd]
		declared := make(map[string]bool, len(names))
		unique := true
		for i, name := range names {
			if declared[name] || enumIdentifierCount(methodBody, name) != 2 {
				unique = false
				break
			}
			declared[name] = true
			if i+1 < len(decls) && strings.TrimSpace(body[decls[i].rangeSpan.end:decls[i+1].rangeSpan.start]) != "" {
				unique = false
				break
			}
		}
		if !unique {
			searchEnd = at
			continue
		}
		args := splitTopLevelArgs(body[open+1 : close])
		if len(args) == 0 {
			searchEnd = at
			continue
		}
		argTemps := make([]string, len(args))
		ordered := 0
		valid := true
		lastTempArg := -1
		for i, arg := range args {
			for _, token := range enumLocalTokens(arg) {
				if !declared[token] {
					continue
				}
				if strings.TrimSpace(arg) != token || enumIdentifierCount(arg, token) != 1 || ordered >= len(names) || names[ordered] != token || argTemps[i] != "" {
					valid = false
					break
				}
				argTemps[i] = token
				lastTempArg = i
				ordered++
			}
			if !valid {
				break
			}
		}
		if !valid || ordered != len(names) {
			searchEnd = at
			continue
		}
		parameters := ctorParameterNames(body, blockStart)
		for i := 0; i < lastTempArg; i++ {
			if argTemps[i] == "" && !ctorSafeReadOnlyArgument(args[i]) && !ctorParameterArgument(args[i], parameters) {
				valid = false
				break
			}
		}
		if !valid {
			searchEnd = at
			continue
		}
		expanded := make(map[string]string, len(names))
		for i, name := range names {
			for _, token := range enumLocalTokens(decls[i].rhs) {
				if declared[token] {
					valid = false
					break
				}
			}
			if !valid {
				break
			}
			expanded[name] = decls[i].rhs
		}
		if !valid {
			searchEnd = at
			continue
		}
		for i := range args {
			if name := argTemps[i]; name != "" {
				args[i] = expanded[name]
			}
		}
		body = body[:open+1] + strings.Join(args, ",") + body[close:]
		sort.Slice(decls, func(i, j int) bool { return decls[i].rangeSpan.start > decls[j].rangeSpan.start })
		for _, decl := range decls {
			span := decl.rangeSpan
			body = body[:span.start] + body[span.end:]
		}
		searchEnd = first
	}
	return fixCtorDelegationTrailingArgumentSpills(body)
}

// fixCtorDelegationTrailingArgumentSpills handles a complementary decompiler
// shape: javac's super(...) is rendered first, while one-use argument arrays
// are rendered as local initializers immediately after it. Moving only a
// contiguous, argument-ordered run back into its exact argument slots restores
// the required first-statement form and Java's left-to-right evaluation order.
func fixCtorDelegationTrailingArgumentSpills(body string) string {
	searchEnd := len(body)
	for searchEnd > 0 {
		at, keyword := lastCtorDelegationCall(body, searchEnd)
		if at < 0 {
			break
		}
		if !javaCodePosition(body, at) {
			searchEnd = at
			continue
		}
		lineStart := strings.LastIndex(body[:at], "\n") + 1
		if strings.TrimSpace(body[lineStart:at]) != "" {
			searchEnd = at
			continue
		}
		open := at + len(keyword)
		close := enumMatchingDelimiter(body, open, '(', ')')
		if close < 0 {
			searchEnd = at
			continue
		}
		semicolon := close + 1
		for semicolon < len(body) && (body[semicolon] == ' ' || body[semicolon] == '\t' || body[semicolon] == '\r') {
			semicolon++
		}
		if semicolon >= len(body) || body[semicolon] != ';' {
			searchEnd = at
			continue
		}
		blockStart, blockEnd, ok := javaBlockBoundsAt(body, at)
		if !ok || strings.TrimSpace(body[blockStart+1:lineStart]) != "" {
			searchEnd = at
			continue
		}
		lineEnd := strings.IndexByte(body[semicolon:], '\n')
		if lineEnd < 0 {
			lineEnd = len(body)
		} else {
			lineEnd += semicolon
		}
		if strings.TrimSpace(body[semicolon+1:lineEnd]) != "" {
			searchEnd = at
			continue
		}
		indent := enumLineIndent(body, lineStart)
		args := splitTopLevelArgs(body[open+1 : close])
		if len(args) == 0 {
			searchEnd = at
			continue
		}

		var following []enumLocalDecl
		var followingNames []string
		cursor := lineEnd
		if cursor < len(body) && body[cursor] == '\n' {
			cursor++
		}
		for cursor < blockEnd {
			end := strings.IndexByte(body[cursor:], '\n')
			if end < 0 {
				end = blockEnd
			} else {
				end += cursor
			}
			if strings.TrimSpace(body[cursor:end]) == "" {
				if end >= blockEnd {
					break
				}
				cursor = end + 1
				continue
			}
			decl, name, ok := parseEnumLocalInitializerLine(body, cursor, end, indent)
			if !ok {
				break
			}
			following = append(following, decl)
			followingNames = append(followingNames, name)
			cursor = decl.rangeSpan.end
		}
		if len(following) == 0 {
			searchEnd = at
			continue
		}
		available := make(map[string]int, len(followingNames))
		for i, name := range followingNames {
			available[name] = i
		}
		needed := make([]string, 0, len(followingNames))
		argNames := make(map[int][]string)
		seen := make(map[string]bool)
		lastNeededArg := -1
		for i, arg := range args {
			tokens, safeReferences := ctorSpillTempTokens(arg)
			if !safeReferences {
				needed = nil
				break
			}
			for _, token := range tokens {
				if _, isFollowingDecl := available[token]; !isFollowingDecl {
					continue
				}
				if seen[token] || ctorSpillIdentifierCount(arg, token) != 1 {
					needed = nil
					break
				}
				seen[token] = true
				needed = append(needed, token)
				argNames[i] = append(argNames[i], token)
				lastNeededArg = i
			}
			if needed == nil {
				break
			}
		}
		if len(needed) == 0 || len(needed) > len(followingNames) {
			searchEnd = at
			continue
		}
		ordered := true
		for i, name := range needed {
			if followingNames[i] != name || enumIdentifierCount(body[blockStart:blockEnd], name) != 2 {
				ordered = false
				break
			}
			rhsTokens, safeReferences := ctorSpillTempTokens(following[i].rhs)
			if !safeReferences {
				ordered = false
				break
			}
			for _, token := range rhsTokens {
				if seen[token] {
					ordered = false
					break
				}
			}
			if !ordered {
				break
			}
		}
		parameters := ctorParameterNames(body, blockStart)
		methodBody := body[blockStart:blockEnd]
		for _, arg := range args {
			tokens, safeReferences := ctorSpillTempTokens(arg)
			if !safeReferences {
				ordered = false
				break
			}
			for _, token := range tokens {
				_, isContiguous := available[token]
				// Do not partially rewrite a call if it also references a
				// generated local outside the contiguous declaration run.
				if !parameters[token] && !isContiguous && enumIdentifierCount(methodBody, token) > 1 {
					ordered = false
					break
				}
			}
			if !ordered {
				break
			}
		}
		for i := 0; ordered && i < lastNeededArg; i++ {
			if len(argNames[i]) == 0 && !ctorSafeReadOnlyArgument(args[i]) && !ctorParameterArgument(args[i], parameters) {
				ordered = false
			}
		}
		if !ordered {
			searchEnd = at
			continue
		}
		expanded := append([]string(nil), args...)
		for i := range expanded {
			for _, name := range argNames[i] {
				decl := following[available[name]]
				expanded[i] = replaceCtorSpillIdentifier(expanded[i], name, decl.rhs)
			}
		}
		for i := len(needed) - 1; i >= 0; i-- {
			span := following[i].rangeSpan
			body = body[:span.start] + body[span.end:]
		}
		body = body[:open+1] + strings.Join(expanded, ",") + body[close:]
		searchEnd = at
	}
	return body
}

// ctorSpillTempTokens reads only Java code, not strings, comments, or text
// blocks. A generated varN name can also be a qualified member or method name;
// those shapes are ambiguous without a Java parser, so the spill rewrite
// rejects them instead of replacing the wrong identifier.
func ctorSpillTempTokens(source string) ([]string, bool) {
	var tokens []string
	var prevPrev, prev byte
	for i := 0; i < len(source); {
		if next, skipped := skipJavaNonCode(source, i); skipped {
			i = next
			continue
		}
		ch := source[i]
		if isJavaWhitespace(ch) {
			i++
			continue
		}
		if !isEnumIdentStart(ch) {
			prevPrev, prev = prev, ch
			i++
			continue
		}
		start := i
		i++
		for i < len(source) && isEnumIdentPart(source[i]) {
			i++
		}
		name := source[start:i]
		if isEnumTempName(name) {
			nextAt := nextJavaCodeOffset(source, i)
			next := byte(0)
			if nextAt < len(source) {
				next = source[nextAt]
			}
			next2At := nextJavaCodeOffset(source, nextAt+1)
			next2 := byte(0)
			if next2At < len(source) {
				next2 = source[next2At]
			}
			next3At := nextJavaCodeOffset(source, next2At+1)
			next3 := byte(0)
			if next3At < len(source) {
				next3 = source[next3At]
			}
			prefixIncrement := (prev == '+' || prev == '-') && prevPrev == prev
			postfixIncrement := (next == '+' || next == '-') && next2 == next
			compoundAssignment := strings.ContainsRune("+-*/%&|^", rune(next)) && next2 == '=' ||
				(next == '<' || next == '>') && next2 == next && next3 == '='
			if prev == '.' || prev == ':' && prevPrev == ':' || prefixIncrement ||
				next == '.' || next == ':' || next == '(' || next == '[' ||
				next == '=' && next2 != '=' || postfixIncrement || compoundAssignment {
				return nil, false
			}
			tokens = append(tokens, name)
		}
		prevPrev, prev = prev, source[i-1]
	}
	return tokens, true
}

func ctorSpillIdentifierCount(source, name string) int {
	tokens, safe := ctorSpillTempTokens(source)
	if !safe {
		return -1
	}
	count := 0
	for _, token := range tokens {
		if token == name {
			count++
		}
	}
	return count
}

func nextJavaCodeOffset(source string, at int) int {
	for i := at; i < len(source); {
		if next, skipped := skipJavaNonCode(source, i); skipped {
			i = next
			continue
		}
		if isJavaWhitespace(source[i]) {
			i++
			continue
		}
		return i
	}
	return len(source)
}

func isJavaWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

func replaceCtorSpillIdentifier(source, target, replacement string) string {
	var out strings.Builder
	for i := 0; i < len(source); {
		if next, skipped := skipJavaNonCode(source, i); skipped {
			out.WriteString(source[i:next])
			i = next
			continue
		}
		if !isEnumIdentStart(source[i]) {
			out.WriteByte(source[i])
			i++
			continue
		}
		start := i
		i++
		for i < len(source) && isEnumIdentPart(source[i]) {
			i++
		}
		name := source[start:i]
		if name == target {
			out.WriteString(replacement)
		} else {
			out.WriteString(name)
		}
	}
	return out.String()
}

func lastCtorDelegationCall(source string, before int) (int, string) {
	if before > len(source) {
		before = len(source)
	}
	thisAt := strings.LastIndex(source[:before], "this(")
	superAt := strings.LastIndex(source[:before], "super(")
	if thisAt > superAt {
		return thisAt, "this"
	}
	if superAt >= 0 {
		return superAt, "super"
	}
	return -1, ""
}

var ctorCastTypeExpression = regexp.MustCompile(`^(?:boolean|byte|short|char|int|long|float|double|[A-Za-z_$][A-Za-z0-9_.$]*)(?:\s*<[^()]+>)?(?:\s*\[\s*\])*$`)

func ctorSafeReadOnlyArgument(expression string) bool {
	expression = strings.TrimSpace(expression)
	for len(expression) > 1 && expression[0] == '(' && enumMatchingDelimiter(expression, 0, '(', ')') == len(expression)-1 {
		expression = strings.TrimSpace(expression[1 : len(expression)-1])
	}
	if expression == "null" || expression == "true" || expression == "false" || enumMovedNumber.MatchString(expression) {
		return true
	}
	if _, err := strconv.Unquote(expression); err == nil {
		return true
	}
	if len(expression) > 2 && expression[0] == '(' {
		close := enumMatchingDelimiter(expression, 0, '(', ')')
		if close > 0 && close+1 < len(expression) && ctorCastTypeExpression.MatchString(strings.TrimSpace(expression[1:close])) {
			return ctorSafeReadOnlyArgument(expression[close+1:])
		}
	}
	return false
}

func ctorParameterArgument(expression string, parameters map[string]bool) bool {
	expression = strings.TrimSpace(expression)
	for len(expression) > 1 && expression[0] == '(' && enumMatchingDelimiter(expression, 0, '(', ')') == len(expression)-1 {
		expression = strings.TrimSpace(expression[1 : len(expression)-1])
	}
	return parameters[expression]
}

func ctorParameterNames(source string, blockStart int) map[string]bool {
	parameters := make(map[string]bool)
	if blockStart <= 0 || blockStart > len(source) {
		return parameters
	}
	header := source[:blockStart]
	close := strings.LastIndexByte(header, ')')
	if close < 0 {
		return parameters
	}
	for open := strings.LastIndexByte(header[:close], '('); open >= 0; open = strings.LastIndexByte(header[:open], '(') {
		if enumMatchingDelimiter(header, open, '(', ')') != close || !javaCodePosition(header, open) {
			continue
		}
		for _, name := range enumLocalTokens(header[open+1 : close]) {
			parameters[name] = true
		}
		return parameters
	}
	return parameters
}

func javaCodePosition(source string, target int) bool {
	if target < 0 || target >= len(source) {
		return false
	}
	for i := 0; i <= target; {
		if next, skipped := skipJavaNonCode(source, i); skipped {
			if next > target {
				return false
			}
			i = next
			continue
		}
		if i == target {
			return true
		}
		i++
	}
	return false
}

func javaBlockBoundsAt(source string, at int) (int, int, bool) {
	var stack []int
	for i := 0; i < at; {
		if next, skipped := skipJavaNonCode(source, i); skipped {
			i = next
			continue
		}
		switch source[i] {
		case '{':
			stack = append(stack, i)
		case '}':
			if len(stack) == 0 {
				return 0, 0, false
			}
			stack = stack[:len(stack)-1]
		}
		i++
	}
	if len(stack) == 0 {
		return 0, 0, false
	}
	start := stack[len(stack)-1]
	depth := 0
	for i := start; i < len(source); {
		if next, skipped := skipJavaNonCode(source, i); skipped {
			i = next
			continue
		}
		if source[i] == '{' {
			depth++
		} else if source[i] == '}' {
			depth--
			if depth == 0 {
				return start, i + 1, true
			}
		}
		i++
	}
	return 0, 0, false
}

func skipJavaNonCode(source string, at int) (int, bool) {
	if at < 0 || at >= len(source) {
		return at, false
	}
	if source[at] == '/' && at+1 < len(source) {
		if source[at+1] == '/' {
			end := strings.IndexByte(source[at+2:], '\n')
			if end < 0 {
				return len(source), true
			}
			return at + 2 + end, true
		}
		if source[at+1] == '*' {
			end := strings.Index(source[at+2:], "*/")
			if end < 0 {
				return len(source), true
			}
			return at + 2 + end + 2, true
		}
	}
	if source[at] == '"' && strings.HasPrefix(source[at:], `"""`) {
		for i := at + 3; i < len(source); {
			if source[i] == '\\' {
				i += 2
				continue
			}
			if strings.HasPrefix(source[i:], `"""`) {
				return i + 3, true
			}
			i++
		}
		return len(source), true
	}
	if source[at] == '"' || source[at] == '\'' {
		quote := source[at]
		for i := at + 1; i < len(source); {
			if source[i] == '\\' {
				i += 2
				continue
			}
			if source[i] == quote {
				return i + 1, true
			}
			i++
		}
		return len(source), true
	}
	return at, false
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
		payloadArgs := splitTopLevelArgs(args)
		directArgs, directRemovals, hadDirectLocals, directOK := expandEnumClinitDirectArgumentLocals(body, blockStart, blockEnd, match[0], indent, payloadArgs)
		supported := true
		if directOK && hadDirectLocals {
			renderedArgs = directArgs
			localRemovals = append(localRemovals, directRemovals...)
		} else {
			for _, arg := range payloadArgs {
				if !supported {
					break
				}
				rendered, consumed, ok := expandEnumClinitLocals(body, blockStart, blockEnd, match[0], indent, arg, map[string]bool{})
				if !ok {
					supported = false
					break
				}
				renderedArgs = append(renderedArgs, rendered)
				localRemovals = append(localRemovals, consumed...)
			}
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

// expandEnumClinitDirectArgumentLocals expands direct local arguments from a
// compiler-generated enum constructor call. It only moves a run of unique,
// single-use local initializers when their declaration order is identical to
// the constructor argument order and no other statement lies between them.
// This is the shape javac emits when an enum argument stack is spilled into
// locals; preserving that order also preserves side effects inside initializers.
func expandEnumClinitDirectArgumentLocals(body string, blockStart, blockEnd, before int, indent string, args []string) ([]string, []enumSourceRange, bool, bool) {
	names := make([]string, 0, len(args))
	argNames := make(map[int]string)
	seen := make(map[string]bool)
	for i, arg := range args {
		tokens := enumLocalTokens(arg)
		if len(tokens) == 0 {
			continue
		}
		name := strings.TrimSpace(arg)
		if len(tokens) != 1 || name != tokens[0] || seen[name] {
			return nil, nil, true, false
		}
		seen[name] = true
		names = append(names, name)
		argNames[i] = name
	}
	if len(names) == 0 {
		return args, nil, false, true
	}
	if blockStart < 0 || blockEnd > len(body) || before <= blockStart || before > blockEnd {
		return nil, nil, true, false
	}
	block := body[blockStart:blockEnd]
	decls := make(map[string]enumLocalDecl, len(names))
	lastStart := -1
	for _, name := range names {
		if enumIdentifierCount(block, name) != 2 {
			return nil, nil, true, false
		}
		decl, ok := findEnumLocalDeclaration(body, blockStart, before, indent, name)
		if !ok || decl.rangeSpan.start <= lastStart {
			return nil, nil, true, false
		}
		if len(enumLocalTokens(decl.rhs)) != 0 {
			return nil, nil, true, false
		}
		decls[name] = decl
		lastStart = decl.rangeSpan.start
	}

	// The declarations must form one uninterrupted run, in the same order as
	// their arguments. Crossing an arbitrary statement could move a call,
	// assignment, or exception point across enum construction.
	first := decls[names[0]].rangeSpan.start
	expected := 0
	for lineStart := first; lineStart < before; {
		lineEnd := strings.IndexByte(body[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(body)
		} else {
			lineEnd += lineStart
		}
		if strings.TrimSpace(body[lineStart:lineEnd]) != "" {
			if expected >= len(names) {
				return nil, nil, true, false
			}
			decl, name, ok := parseEnumLocalInitializerLine(body, lineStart, lineEnd, indent)
			if !ok || name != names[expected] || decl.rangeSpan.start != decls[name].rangeSpan.start {
				return nil, nil, true, false
			}
			expected++
		}
		if lineEnd == len(body) {
			break
		}
		lineStart = lineEnd + 1
	}
	if expected != len(names) {
		return nil, nil, true, false
	}

	expanded := append([]string(nil), args...)
	removals := make([]enumSourceRange, 0, len(names))
	for i, name := range argNames {
		decl := decls[name]
		expanded[i] = replaceEnumIdentifier(expanded[i], name, decl.rhs)
		removals = append(removals, decl.rangeSpan)
	}
	return expanded, removals, true, true
}

func findEnumLocalDeclaration(body string, blockStart, before int, indent, name string) (enumLocalDecl, bool) {
	lineStart := strings.IndexByte(body[blockStart:before], '\n')
	if lineStart < 0 {
		return enumLocalDecl{}, false
	}
	lineStart += blockStart + 1
	var found enumLocalDecl
	count := 0
	for lineStart < before {
		lineEnd := strings.IndexByte(body[lineStart:before], '\n')
		if lineEnd < 0 {
			lineEnd = before
		} else {
			lineEnd += lineStart
		}
		decl, candidate, ok := parseEnumLocalInitializerLine(body, lineStart, lineEnd, indent)
		if ok && candidate == name {
			found = decl
			count++
		}
		if lineEnd == before {
			break
		}
		lineStart = lineEnd + 1
	}
	return found, count == 1
}

func parseEnumLocalInitializerLine(body string, lineStart, lineEnd int, indent string) (enumLocalDecl, string, bool) {
	if lineStart < 0 || lineEnd > len(body) || lineStart >= lineEnd || enumLineIndent(body, lineStart) != indent {
		return enumLocalDecl{}, "", false
	}
	line := strings.TrimSpace(body[lineStart:lineEnd])
	if !strings.HasSuffix(line, ";") {
		return enumLocalDecl{}, "", false
	}
	eq := strings.Index(line, " = ")
	if eq < 0 {
		return enumLocalDecl{}, "", false
	}
	lhs := strings.Fields(strings.TrimSpace(line[:eq]))
	if len(lhs) < 2 {
		return enumLocalDecl{}, "", false
	}
	name := lhs[len(lhs)-1]
	if !isEnumTempName(name) {
		return enumLocalDecl{}, "", false
	}
	rhs := strings.TrimSpace(strings.TrimSuffix(line[eq+3:], ";"))
	if rhs == "" {
		return enumLocalDecl{}, "", false
	}
	end := lineEnd
	if end < len(body) && body[end] == '\n' {
		end++
	}
	return enumLocalDecl{rangeSpan: enumSourceRange{lineStart, end}, rhs: rhs}, name, true
}

var enumMovedNumber = regexp.MustCompile(`^[+-]?(?:0[xX][0-9A-Fa-f_]+|0[bB][01_]+|(?:[0-9][0-9_]*)(?:\.[0-9_]*)?(?:[eE][+-]?[0-9_]+)?)(?:[fFdDlL])?$`)

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
