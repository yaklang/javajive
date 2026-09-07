package javaclassparser

import (
	"os"
	"strings"
)

// Shape-based dump reconstructs. They key on Java syntax (int local as a bare
// if, boolean method returning an int OR-accumulator, try body that throws
// NSME, empty synchronized as the last statement of a trailing else), not on
// a dumped ident / tab run / class-name unique.
// Kill-switch: JDEC_ORIG14_REMAINING_OFF=1 (same family as the orig14 remainder
// they replace). Nested empty-sync also honors JDEC_EMPTY_SYNC_RETURN_OFF.

func shapeReconstructOff() bool {
	return os.Getenv("JDEC_ORIG14_REMAINING_OFF") == "1"
}

// fixIntBareIf rewrites `if (varN){` to `if ((varN) != (0)){` when the nearest
// declaration of varN in the enclosing member is `int`.
func fixIntBareIf(body string) string {
	if shapeReconstructOff() {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "if (")
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len("if ("):]
		ident, ok, after := readJavaIdent(rest)
		if !ok || (!isDecompilerLocal(ident) && !isFinalCaptureLocal(ident)) || !strings.HasPrefix(after, "){") {
			from = i + 1
			continue
		}
		chunkStart := prevMemberStart(body, i)
		chunk := body[chunkStart:i]
		if !identDeclIsInt(chunk, ident) {
			from = i + 1
			continue
		}
		repl := "if ((" + ident + ") != (0)){"
		body = body[:i] + repl + after[2:]
		from = i + len(repl)
	}
}

// fixBoolOrAccumulator retypes `int varN = 0` to `boolean varN = false` when
// the same member does `varN = (varN) | (` and `return varN` from a boolean
// method.
func fixBoolOrAccumulator(body string) string {
	if shapeReconstructOff() {
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
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		methodHead := body[prevMemberStart(body, i):i]
		if !strings.Contains(methodHead, "boolean ") {
			from = i + 1
			continue
		}
		if !strings.Contains(chunk, ident+" = ("+ident+") | (") {
			from = i + 1
			continue
		}
		if !strings.Contains(chunk, "return "+ident+";") {
			from = i + 1
			continue
		}
		body = body[:i] + "boolean " + ident + " = false;" + rest[len(" = 0;"):]
		from = i + len("boolean "+ident+" = false;")
	}
}

// fixMissingNSMECatch adds NoSuchMethodException to a catch whose try body
// actually invokes getConstructor/getMethod/newInstanceOf/findConstructor,
// when NSME is not already in this catch or a sibling catch.
func fixMissingNSMECatch(body string) string {
	if shapeReconstructOff() {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "}catch(")
		if rel < 0 {
			return body
		}
		i := from + rel
		endRel := strings.Index(body[i:], "){")
		if endRel < 0 {
			return body
		}
		clause := body[i : i+endRel+2]
		if strings.Contains(clause, "NoSuchMethodException") {
			from = i + 2
			continue
		}
		tryBody := enclosingTryBody(body, i)
		if tryBody == "" {
			tryBody = tryBodyBeforeCatch(body, i)
		}
		if tryBody == "" {
			from = i + 2
			continue
		}
		exposed := flattenNestedTryKeepCatchBodies(tryBody)
		if !tryThrowsNSME(exposed) && !thisCallThrowsNSME(exposed, body) {
			from = i + 2
			continue
		}
		if tryCatchAlreadyCoversNSME(body, i) {
			from = i + 2
			continue
		}
		catchBodyStart := i + endRel + 2
		catchBodyEnd := matchingCloseBrace(body, catchBodyStart-1)
		if catchBodyEnd > catchBodyStart && strings.Contains(body[catchBodyStart:catchBodyEnd], "getTargetException(") {
			from = i + 2
			continue
		}
		// Insert before the catch parameter ident: `){` preceded by ` varN`.
		insertAt := strings.LastIndex(clause, " var")
		if insertAt < 0 {
			from = i + 2
			continue
		}
		neu := clause[:insertAt] + " | NoSuchMethodException" + clause[insertAt:]
		body = body[:i] + neu + body[i+len(clause):]
		from = i + len(neu)
	}
}

// catchHasNSMESuper reports whether the catch type union already covers
// NoSuchMethodException via a superclass. `Exception | NoSuchMethodException`
// is illegal ("alternatives cannot be related by subclassing").
// flattenNestedTryKeepCatchBodies replaces nested `try{…}catch(…){BODY}` with
// BODY so NSME checks see only calls that can still propagate to the outer catch.
func flattenNestedTryKeepCatchBodies(s string) string {
	for n := 0; n < 8; n++ {
		idx := strings.Index(s, "try{")
		if idx < 0 {
			return s
		}
		open := idx + len("try{") - 1
		tryClose := matchingCloseBrace(s, open)
		if tryClose < 0 {
			return s
		}
		pos := tryClose
		var bodies []string
		foundCatch := false
		for {
			k := pos + 1
			for k < len(s) && (s[k] == ' ' || s[k] == '\t' || s[k] == '\n' || s[k] == '\r' || s[k] == '}') {
				k++
			}
			if k >= len(s) || !strings.HasPrefix(s[k:], "catch(") {
				break
			}
			foundCatch = true
			rel := strings.Index(s[k:], "){")
			if rel < 0 {
				return s
			}
			copen := k + rel + 1
			cclose := matchingCloseBrace(s, copen)
			if cclose < 0 {
				return s
			}
			bodies = append(bodies, s[copen+1:cclose])
			pos = cclose
		}
		if !foundCatch {
			s = s[:idx] + s[idx+3:]
			continue
		}
		s = s[:idx] + strings.Join(bodies, "\n") + s[pos+1:]
	}
	return s
}

func catchHasNSMESuper(typeUnion string) bool {
	typeUnion = strings.TrimSpace(typeUnion)
	typeUnion = strings.TrimPrefix(typeUnion, "}catch(")
	typeUnion = strings.TrimPrefix(typeUnion, "catch(")
	typeUnion = strings.TrimSuffix(typeUnion, "){")
	typeUnion = strings.TrimSuffix(typeUnion, ")")
	if i := strings.LastIndex(typeUnion, " "); i >= 0 {
		last := strings.TrimSpace(typeUnion[i+1:])
		if last != "" && !strings.ContainsAny(last, "|&<>.") {
			typeUnion = typeUnion[:i]
		}
	}
	for _, t := range strings.Split(typeUnion, "|") {
		t = strings.TrimSpace(t)
		switch t {
		case "Exception", "Throwable", "ReflectiveOperationException",
			"java.lang.Exception", "java.lang.Throwable",
			"java.lang.ReflectiveOperationException":
			return true
		}
	}
	return false
}

func enclosingTryBody(body string, catchAt int) string {
	i := catchAt
	for i >= 0 && i < len(body) && body[i] == '}' {
		open := matchingOpenBrace(body, i)
		if open < 0 {
			return ""
		}
		pre := strings.TrimRight(body[:open], " \t\n")
		if strings.HasSuffix(pre, "try") && (len(pre) == 3 || !isJavaIdentChar(pre[len(pre)-4])) {
			return body[open+1 : i]
		}
		prev := strings.LastIndex(body[:open], "}catch(")
		if prev < 0 {
			return ""
		}
		i = prev
	}
	return ""
}

func thisCallThrowsNSME(tryBody, full string) bool {
	from := 0
	for {
		rel := strings.Index(tryBody[from:], "this.")
		if rel < 0 {
			return false
		}
		i := from + rel + len("this.")
		ident, ok, after := readJavaIdent(tryBody[i:])
		if !ok || !strings.HasPrefix(after, "(") {
			from = i
			continue
		}
		if methodDeclThrowsNSME(full, ident) && methodBodyThrowsNSME(full, ident) {
			return true
		}
		from = i
	}
}

func methodDeclThrowsNSME(full, ident string) bool {
	needle := ident + "("
	from := 0
	for {
		rel := strings.Index(full[from:], needle)
		if rel < 0 {
			return false
		}
		at := from + rel
		if at > 0 && (full[at-1] == '.' || isJavaIdentChar(full[at-1])) {
			from = at + 1
			continue
		}
		rest := full[at+len(needle):]
		close := strings.Index(rest, ")")
		if close >= 0 && close < 280 {
			sig := strings.TrimLeft(rest[close+1:], " \t")
			if strings.HasPrefix(sig, "throws") {
				end := strings.IndexAny(sig, "{;")
				if end < 0 {
					end = len(sig)
					if end > 160 {
						end = 160
					}
				}
				if strings.Contains(sig[:end], "NoSuchMethodException") {
					return true
				}
			}
		}
		from = at + 1
	}
}

// methodBodyThrowsNSME is true when a same-class method named ident actually
// contains a getConstructor/getMethod/newInstanceOf call (not merely declares
// throws NSME, which over-matches unused overloads).
func methodBodyThrowsNSME(full, ident string) bool {
	needle := ident + "("
	from := 0
	for {
		rel := strings.Index(full[from:], needle)
		if rel < 0 {
			return false
		}
		at := from + rel
		if at > 0 && (full[at-1] == '.' || isJavaIdentChar(full[at-1])) {
			from = at + 1
			continue
		}
		rest := full[at+len(needle):]
		close := strings.Index(rest, ")")
		if close < 0 || close > 280 {
			from = at + 1
			continue
		}
		sig := strings.TrimLeft(rest[close+1:], " \t")
		if !strings.HasPrefix(sig, "throws") && !strings.HasPrefix(sig, "{") {
			from = at + 1
			continue
		}
		brace := strings.Index(rest, "{")
		if brace < 0 || brace > 400 {
			from = at + 1
			continue
		}
		open := at + len(needle) + brace
		end := matchingCloseBrace(full, open)
		if end < 0 {
			from = at + 1
			continue
		}
		if tryThrowsNSME(full[open+1 : end]) {
			return true
		}
		from = at + 1
	}
}

func siblingCatchHasNSME(body string, afterCatchBrace int) bool {
	return tryCatchAlreadyCoversNSME(body, strings.LastIndex(body[:afterCatchBrace], "}catch("))
}

// tryCatchAlreadyCoversNSME is true when this try already has NSME in any
// catch, or a superclass (Exception / Throwable / ReflectiveOperationException)
// that would make `… | NoSuchMethodException` illegal or redundant.
func tryCatchAlreadyCoversNSME(body string, catchAt int) bool {
	if catchAt < 0 {
		return false
	}
	tryClose := catchAt
	for tryClose >= 0 && tryClose < len(body) && body[tryClose] == '}' {
		open := matchingOpenBrace(body, tryClose)
		if open < 0 {
			break
		}
		pre := strings.TrimRight(body[:open], " \t\n")
		if strings.HasSuffix(pre, "try") && (len(pre) == 3 || !isJavaIdentChar(pre[len(pre)-4])) {
			break
		}
		prev := strings.LastIndex(body[:open], "}catch(")
		if prev < 0 {
			break
		}
		tryClose = prev
	}
	pos := tryClose
	for pos < len(body) && pos >= 0 && body[pos] == '}' {
		open := matchingOpenBrace(body, pos)
		if open < 0 {
			break
		}
		pos = open
	}
	rest := body[tryClose+1:]
	for {
		rest = strings.TrimLeft(rest, " \t\n")
		if !strings.HasPrefix(rest, "}catch(") && !strings.HasPrefix(rest, "catch(") {
			return false
		}
		end := strings.Index(rest, "){")
		if end < 0 {
			return false
		}
		clause := rest[:end]
		if strings.Contains(clause, "NoSuchMethodException") || catchHasNSMESuper(clause) {
			return true
		}
		k := end + 2
		d := 1
		for k < len(rest) && d > 0 {
			switch rest[k] {
			case '{':
				d++
			case '}':
				d--
			}
			k++
		}
		rest = rest[k:]
	}
}

// fixObjectGetKeyAssignedToInt retypes `int varN = 0` to `Object varN = null`
// when the member later does `varN = ident.getKey()` / `getValue()` AND uses
// that local as a reference compare (`(varN)==(varM)` / `== (null)`), AND does
// not also use it as a number (`++`, `+`, `<`, `== (0)`, …).
// Spring MapToMapConverter is `var11 = var10.getKey(); if ((var11)==(var13))`.
// protobuf/zxing hashCode and TreeBidiMap compare assign getKey into an int
// accumulator — those must stay int.
func fixObjectGetKeyAssignedToInt(body string) string {
	if shapeReconstructOff() {
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
		chunk := body[i:nextMemberStart(body, i)]
		if identAssignedFromMapEntry(chunk, ident) &&
			identUsedAsObjectCompare(chunk, ident) &&
			!identUsedAsNumber(chunk, ident) {
			body = body[:i] + "Object " + ident + " = null;" + rest[len(" = 0;"):]
			from = i + len("Object "+ident+" = null;")
			continue
		}
		from = i + 1
	}
}

// identAssignedFromMapEntry is true when some assignment `ident = …getKey()` /
// `ident = …getValue()` exists. A sibling `.getKey()` call in the same member
// (e.g. `format(map, entry.getKey())` next to an int counter) must not fire.
func identAssignedFromMapEntry(chunk, ident string) bool {
	needle := ident + " = "
	from := 0
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return false
		}
		at := from + rel
		if at > 0 && isJavaIdentChar(chunk[at-1]) {
			from = at + 1
			continue
		}
		rhsAt := at + len(needle)
		semi := strings.Index(chunk[rhsAt:], ";")
		if semi < 0 {
			return false
		}
		if rhsIsMapEntryAccess(chunk[rhsAt : rhsAt+semi]) {
			return true
		}
		from = rhsAt
	}
}

// rhsIsMapEntryAccess is true when the assignment RHS is `recv.getKey()` /
// `recv.getValue()` (optional wrapping parens). Checksum.getValue() buried in
// `(int)((hash.getValue()) >> (8))` and `parseInt(reader.getValue())` must not
// fire — those are int/String values, not Map.Entry objects.
func rhsIsMapEntryAccess(rhs string) bool {
	s := strings.TrimSpace(rhs)
	for {
		if len(s) < 2 || s[0] != '(' {
			break
		}
		depth := 0
		wrap := false
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					if i == len(s)-1 {
						s = strings.TrimSpace(s[1:i])
						wrap = true
					}
					i = len(s)
				}
			}
		}
		if !wrap {
			break
		}
	}
	return strings.HasSuffix(s, ".getKey()") || strings.HasSuffix(s, ".getValue()")
}

// identUsedAsObjectCompare is true when ident is compared with == / != against
// null or another identifier (reference equality), not a numeric literal.
func identUsedAsObjectCompare(chunk, ident string) bool {
	for _, op := range []string{"==", "!="} {
		for _, sp := range []string{"", " "} {
			needle := "(" + ident + ")" + sp + op + sp + "("
			from := 0
			for {
				rel := strings.Index(chunk[from:], needle)
				if rel < 0 {
					break
				}
				if objectCompareOperand(chunk[from+rel+len(needle):]) {
					return true
				}
				from += rel + 1
			}
			needleR := ")" + sp + op + sp + "(" + ident + ")"
			from = 0
			for {
				rel := strings.Index(chunk[from:], needleR)
				if rel < 0 {
					break
				}
				if objectCompareLeftOperand(chunk[:from+rel]) {
					return true
				}
				from += rel + 1
			}
		}
	}
	return false
}

func objectCompareOperand(s string) bool {
	s = strings.TrimLeft(s, " \t")
	if strings.HasPrefix(s, "null)") || strings.HasPrefix(s, "null )") {
		return true
	}
	id, ok, rest := readJavaIdent(s)
	if !ok || id == "" {
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	return strings.HasPrefix(rest, ")")
}

func objectCompareLeftOperand(prefix string) bool {
	prefix = strings.TrimRight(prefix, " \t")
	if strings.HasSuffix(prefix, "(null") {
		return true
	}
	i := len(prefix) - 1
	for i >= 0 && isJavaIdentChar(prefix[i]) {
		i--
	}
	if i < 0 || prefix[i] != '(' {
		return false
	}
	id := prefix[i+1:]
	return id != "" && (id[0] < '0' || id[0] > '9')
}

// identUsedAsNumber is true when ident is incremented, used with an arithmetic
// / bitwise / relational operator, or compared with a numeric literal.
func identUsedAsNumber(chunk, ident string) bool {
	from := 0
	for {
		rel := strings.Index(chunk[from:], ident)
		if rel < 0 {
			return false
		}
		at := from + rel
		if at > 0 && isJavaIdentChar(chunk[at-1]) {
			from = at + 1
			continue
		}
		end := at + len(ident)
		if end < len(chunk) && isJavaIdentChar(chunk[end]) {
			from = at + 1
			continue
		}
		if afterIdentLooksNumeric(chunk[end:]) || beforeIdentLooksNumeric(chunk[:at]) {
			return true
		}
		from = end
	}
}

func afterIdentLooksNumeric(after string) bool {
	a := strings.TrimLeft(after, " \t)")
	if strings.HasPrefix(a, "++") || strings.HasPrefix(a, "--") {
		return true
	}
	for _, op := range []string{"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", ">>>="} {
		if strings.HasPrefix(a, op) {
			return true
		}
	}
	if strings.HasPrefix(a, "==") || strings.HasPrefix(a, "!=") {
		rhs := strings.TrimLeft(a[2:], " \t(")
		if rhs == "" {
			return false
		}
		return rhs[0] >= '0' && rhs[0] <= '9' || rhs[0] == '-'
	}
	if strings.HasPrefix(a, "&&") || strings.HasPrefix(a, "||") {
		return false
	}
	if a == "" {
		return false
	}
	switch a[0] {
	case '+', '-', '*', '/', '%', '<', '>', '&', '|', '^':
		return true
	}
	return false
}

func beforeIdentLooksNumeric(before string) bool {
	b := strings.TrimRight(before, " \t(")
	if strings.HasSuffix(b, "++") || strings.HasSuffix(b, "--") {
		return true
	}
	if b == "" {
		return false
	}
	c := b[len(b)-1]
	switch c {
	case '+', '-', '*', '/', '%', '<', '>', '&', '|', '^':
		if strings.HasSuffix(b, "&&") || strings.HasSuffix(b, "||") ||
			strings.HasSuffix(b, "==") || strings.HasSuffix(b, "!=") {
			return false
		}
		return true
	}
	return false
}

// fixUncheckedAwaitNanos wraps `ident = this.awaitNanos(...)` in try/catch
// InterruptedException when the enclosing method does not declare that throws.
func fixUncheckedAwaitNanos(body string) string {
	if shapeReconstructOff() {
		return body
	}
	const call = " = this.awaitNanos("
	from := 0
	for {
		rel := strings.Index(body[from:], call)
		if rel < 0 {
			return body
		}
		eq := from + rel
		// ident ends at eq; walk back
		identStart := eq
		for identStart > 0 {
			c := body[identStart-1]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
				identStart--
				continue
			}
			break
		}
		ident := body[identStart:eq]
		if !isDecompilerLocal(ident) {
			from = eq + 1
			continue
		}
		head := body[prevMemberStart(body, eq):eq]
		if strings.Contains(head, "throws InterruptedException") {
			from = eq + 1
			continue
		}
		// already in a try that catches IE?
		window := body[max(0, eq-200):eq]
		if strings.Contains(window, "try{") && !strings.Contains(window, "catch(") {
			from = eq + 1
			continue
		}
		closeCall := strings.Index(body[eq+len(call):], ");")
		if closeCall < 0 {
			from = eq + 1
			continue
		}
		stmtEnd := eq + len(call) + closeCall + 2
		stmt := body[identStart:stmtEnd]
		// indent of the statement
		lineStart := strings.LastIndex(body[:identStart], "\n")
		ind := ""
		if lineStart >= 0 {
			ind = body[lineStart+1 : identStart]
		}
		wrapped := "try{\n" + ind + "\t" + stmt + "\n" + ind + "}catch(InterruptedException var_ie){\n" + ind + "\t" + ident + " = false;\n" + ind + "}"
		body = body[:identStart] + wrapped + body[stmtEnd:]
		from = identStart + len(wrapped)
	}
}

// fixEmptySyncInTrailingElse inserts a default return after an empty
// `synchronized(...) { }` that is the last statement of a method's trailing
// else (Http2Stream.closeInternal, OpenSslClientSessionCache.setSession).
func fixEmptySyncInTrailingElse(body string) string {
	if shapeReconstructOff() || os.Getenv("JDEC_EMPTY_SYNC_RETURN_OFF") == "1" {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "synchronized(")
		if rel < 0 {
			return body
		}
		i := from + rel
		brace := strings.Index(body[i:], "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		open := i + brace
		// empty body: {\n\n\t+}
		closeRel := strings.Index(body[open:], "}")
		if closeRel < 0 {
			from = i + 1
			continue
		}
		inner := body[open+1 : open+closeRel]
		if strings.TrimSpace(inner) != "" {
			from = i + 1
			continue
		}
		after := open + closeRel + 1
		j := after
		for j < len(body) && (body[j] == ' ' || body[j] == '\t' || body[j] == '\n' || body[j] == '\r' || body[j] == '}') {
			j++
		}
		closers := body[after:j]
		if !strings.Contains(closers, "}") || strings.Contains(closers, "return ") {
			from = i + 1
			continue
		}
		head := body[prevMemberStart(body, i):i]
		ret := returnTypeOfMember(head)
		// Only insert a primitive default. A mis-parsed previous method's class
		// type would emit `return null` into a void/boolean method (A/B-negative).
		if ret != "false" && ret != "0" && ret != "0L" && ret != "0.0F" && ret != "0.0" {
			from = i + 1
			continue
		}
		// indent of synchronized
		lineStart := strings.LastIndex(body[:i], "\n")
		ind := "\t\t\t"
		if lineStart >= 0 {
			ind = body[lineStart+1 : i]
		}
		ins := "\n" + ind + "return " + ret + ";"
		body = body[:after] + ins + body[after:]
		from = after + len(ins)
	}
}

func returnTypeOfMember(head string) string {
	// last method-ish signature in head
	lines := strings.Split(head, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		ln := strings.TrimSpace(lines[i])
		if !strings.Contains(ln, "(") || !strings.Contains(ln, ")") ||
			(!strings.Contains(ln, ") {") && !strings.HasSuffix(ln, "){") && !strings.Contains(ln, ") throws")) ||
			strings.HasPrefix(ln, "if ") || strings.HasPrefix(ln, "for ") ||
			strings.HasPrefix(ln, "while ") || strings.HasPrefix(ln, "switch ") || strings.HasPrefix(ln, "catch") ||
			strings.HasPrefix(ln, "synchronized(") || strings.HasPrefix(ln, "synchronized (") ||
			strings.HasPrefix(ln, "else") || strings.HasPrefix(ln, "try") || strings.HasPrefix(ln, "do") ||
			strings.HasPrefix(ln, "throw ") || strings.HasPrefix(ln, "return ") || strings.HasPrefix(ln, "new ") {
			continue
		}
		// strip modifiers
		fields := strings.Fields(ln)
		typ := ""
		for _, f := range fields {
			switch f {
			case "public", "protected", "private", "static", "final", "synchronized", "native", "default", "abstract":
				continue
			}
			typ = f
			break
		}
		typ = strings.TrimSpace(typ)
		if strings.Contains(typ, "(") {
			continue
		}
		if idx := strings.IndexAny(typ, "<["); idx > 0 {
			typ = typ[:idx]
		}
		if typ == "" || typ == "void" {
			return ""
		}
		return defaultReturnForType(typ)
	}
	return ""
}

func identDeclIsInt(chunk, ident string) bool {
	lastInt := strings.LastIndex(chunk, "int "+ident+" =")
	if lastInt < 0 {
		lastInt = strings.LastIndex(chunk, "int "+ident+";")
	}
	if lastInt < 0 {
		return false
	}
	for _, typ := range []string{"boolean " + ident, "Object " + ident, "long " + ident} {
		if j := strings.LastIndex(chunk, typ); j > lastInt {
			return false
		}
	}
	return true
}

func prevMemberStart(body string, at int) int {
	// Scan backward for a one-tab member line.
	i := at
	for i > 0 {
		nl := strings.LastIndex(body[:i], "\n\t")
		if nl < 0 {
			return 0
		}
		rest := body[nl+2:]
		if rest == "" {
			return 0
		}
		c := rest[0]
		if c != '\t' && c != '\n' && c != '\r' && c != ' ' && c != '/' && c != '*' && c != '}' {
			return nl
		}
		i = nl
	}
	return 0
}
