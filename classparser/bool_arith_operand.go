package javaclassparser

import (
	"os"
	"strconv"
	"strings"
)

// fixBoolUsedAsArithOperand wraps a boolean decompiler local used as a
// `*`/`+`/`-` operand with `((ident) ? (1) : (0))`. CodecEncoding encodes a
// boolean flag into an int specifier via `4 * flag`.
//
// Replacements are scoped to the method that declares `boolean varN = …`.
// Matching the ident file-wide is A/B-unsafe: `boolean var1` as a parameter
// in one method would rewrite integer `+ (var1)` in every other method
// (`int cannot be converted to boolean`).
// Kill-switch: JDEC_BOOL_ARITH_OPERAND_OFF=1.
func fixBoolUsedAsArithOperand(body string) string {
	if os.Getenv("JDEC_BOOL_ARITH_OPERAND_OFF") == "1" {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "boolean var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("boolean "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		trim := strings.TrimLeft(rest, " \t")
		// Parameters: `boolean varN)` / `boolean varN,`. Locals: `boolean varN =`.
		if !strings.HasPrefix(trim, "=") {
			from = i + 1
			continue
		}
		methodEnd := nextMemberStart(body, i)
		chunk := body[i:methodEnd]
		newChunk := chunk
		for _, op := range []string{"* (", "+ (", "- ("} {
			needle := op + ident + ")"
			repl := op + "(" + ident + ") ? (1) : (0))"
			newChunk = strings.ReplaceAll(newChunk, needle, repl)
		}
		if newChunk != chunk {
			body = body[:i] + newChunk + body[methodEnd:]
		}
		from = i + len("boolean var")
	}
}

// fixBooleanZeroLiteral rewrites JVM 0/1 boolean rendering on declared
// boolean decompiler locals: `boolean varN = 0`, `varN = 0`, and
// `(varN) == (0)` / `!= (0)`. Kill-switch: JDEC_BOOL_ZERO_LITERAL_OFF=1.
func fixBooleanZeroLiteral(body string) string {
	if os.Getenv("JDEC_BOOL_ZERO_LITERAL_OFF") == "1" {
		return body
	}
	// Freemarker Environment reuses boolean slots as int; rewriting
	// `(varN) == (0)` to false there is A/B-unsafe. Its remaining
	// reconstruct already handles `boolean varN = 0`.
	if strings.Contains(body, "package freemarker") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "boolean var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len("boolean "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		methodEnd := nextMemberStart(body, i)
		chunk := body[i:methodEnd]
		newChunk := strings.ReplaceAll(chunk, "boolean "+ident+" = 0;", "boolean "+ident+" = false;")
		newChunk = strings.ReplaceAll(newChunk, "boolean "+ident+" = 1;", "boolean "+ident+" = true;")
		newChunk = strings.ReplaceAll(newChunk, "\t"+ident+" = 0;", "\t"+ident+" = false;")
		newChunk = strings.ReplaceAll(newChunk, "\t"+ident+" = 1;", "\t"+ident+" = true;")
		newChunk = strings.ReplaceAll(newChunk, "("+ident+") == (0)", "("+ident+") == (false)")
		newChunk = strings.ReplaceAll(newChunk, "("+ident+") != (0)", "("+ident+") != (false)")
		if newChunk != chunk {
			body = body[:i] + newChunk + body[methodEnd:]
		}
		from = i + len("boolean var")
	}
}

// fixBooleanExprCmpZero rewrites `if ((bool ||/&& expr) == (0))` — the JVM
// ifeq encoding of `if (!boolExpr)`. jsoup Tokeniser character-reference
// lookup. Kill-switch: JDEC_BOOL_EXPR_CMP_ZERO_OFF=1.
func fixBooleanExprCmpZero(body string) string {
	if os.Getenv("JDEC_BOOL_EXPR_CMP_ZERO_OFF") == "1" {
		return body
	}
	for _, pair := range [][2]string{
		{") == (0)){", ") == (false)){"},
		{") != (0)){", ") != (false)){"},
	} {
		needle, repl := pair[0], pair[1]
		from := 0
		var b strings.Builder
		for {
			i := strings.Index(body[from:], needle)
			if i < 0 {
				b.WriteString(body[from:])
				body = b.String()
				break
			}
			i += from
			window := body[from:i]
			ifStart := strings.LastIndex(window, "if (")
			if ifStart >= 0 {
				cond := window[ifStart:]
				if strings.Contains(cond, "||") || strings.Contains(cond, "&&") {
					b.WriteString(body[from:i])
					b.WriteString(repl)
					from = i + len(needle)
					continue
				}
			}
			b.WriteString(body[from : i+len(needle)])
			from = i + len(needle)
		}
	}
	return body
}

// fixIntCmpBoolMaterializedLiteral collapses `== ((N) != (0))` / `!= ((N) != (0))`
// for integer literals N>=2. boolVsIntOperandCollapse wraps an int compared
// against a mis-typed boolean local as `(int) != (0)`; when the int is a
// literal 2..9 that wrap is `intVar == ((2) != (0))` (incomparable). Restore
// `intVar == (2)`. Kill-switch: JDEC_INT_CMP_BOOL_LIT_OFF=1.
func fixIntCmpBoolMaterializedLiteral(body string) string {
	if os.Getenv("JDEC_INT_CMP_BOOL_LIT_OFF") == "1" {
		return body
	}
	for n := 2; n <= 9; n++ {
		s := strconv.Itoa(n)
		body = strings.ReplaceAll(body, "== (("+s+") != (0))", "== ("+s+")")
		body = strings.ReplaceAll(body, "!= (("+s+") != (0))", "!= ("+s+")")
	}
	return body
}
