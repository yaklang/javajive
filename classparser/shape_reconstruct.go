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
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(after, "){") {
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
		if !tryThrowsNSME(tryBody) && !strings.Contains(tryBody, ".newInstanceOf(") &&
			!strings.Contains(tryBody, ".findConstructor(") && !thisCallThrowsNSME(tryBody, body) {
			from = i + 2
			continue
		}
		if siblingCatchHasNSME(body, i+endRel+2) {
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
		if methodDeclThrowsNSME(full, ident) {
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
		if at > 0 && full[at-1] == '.' {
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

func siblingCatchHasNSME(body string, afterCatchBrace int) bool {
	j := afterCatchBrace
	depth := 1
	for j < len(body) && depth > 0 {
		switch body[j] {
		case '{':
			depth++
		case '}':
			depth--
		}
		j++
	}
	rest := strings.TrimLeft(body[j:], " \t\n")
	for strings.HasPrefix(rest, "catch(") {
		end := strings.Index(rest, "){")
		if end < 0 {
			return false
		}
		if strings.Contains(rest[:end], "NoSuchMethodException") {
			return true
		}
		// skip this catch body
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
		rest = strings.TrimLeft(rest[k:], " \t\n")
	}
	return false
}

// fixObjectGetKeyAssignedToInt retypes `int varN = 0` to `Object varN = null`
// when the member later does `varN = ident.getKey()` or `varN = ident.getValue()`.
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
		if strings.Contains(chunk, ident+" = ") &&
			(strings.Contains(chunk, ".getKey()") || strings.Contains(chunk, ".getValue()")) &&
			strings.Contains(chunk, ident+") == (") {
			body = body[:i] + "Object " + ident + " = null;" + rest[len(" = 0;"):]
			from = i + len("Object "+ident+" = null;")
			continue
		}
		from = i + 1
	}
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
		peekEnd := after + 96
		if peekEnd > len(body) {
			peekEnd = len(body)
		}
		peek := body[after:peekEnd]
		if strings.Contains(peek, "return ") {
			from = i + 1
			continue
		}
		// skip whitespace and closing braces of else/method
		j := after
		for j < len(body) && (body[j] == ' ' || body[j] == '\t' || body[j] == '\n' || body[j] == '\r' || body[j] == '}') {
			j++
		}
		// must have consumed at least one } after the sync close
		if !strings.Contains(body[after:j], "}") {
			from = i + 1
			continue
		}
		head := body[prevMemberStart(body, i):i]
		ret := returnTypeOfMember(head)
		if ret == "" {
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
		if !strings.Contains(ln, "(") || strings.HasPrefix(ln, "if ") || strings.HasPrefix(ln, "for ") ||
			strings.HasPrefix(ln, "while ") || strings.HasPrefix(ln, "switch ") || strings.HasPrefix(ln, "catch") {
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
		if idx := strings.IndexAny(typ, "<["); idx > 0 {
			typ = typ[:idx]
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
