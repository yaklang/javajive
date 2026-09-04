package javaclassparser

import (
	"os"
	"strings"
)

// fixLeakedExceptionSentinel reconstructs two CFG-split shapes that leak a caught-throwable
// placeholder as the bare type name `Exception` (invalid Java: "cannot find symbol"):
//
//  1. If-diamond merge then try/catch around a loop nest: one arm keeps the try body, the other
//     keeps only `T varN = Exception; return X`. Copy the sibling try/catch over the leaked arm
//     (guava InetAddresses.textToNumericFormatV6).
//  2. If-diamond merge then try/finally: 0-arg tryLock() success inlines the finally's any-handler
//     (`var = Exception; unlock; throw`) instead of joining the post-merge try. Replace that arm
//     with the sibling timed-tryLock else body (guava Monitor.enterWhen(Guard,long,TimeUnit)).
//
// Catch parameters renamed `varN_1` while the rethrow still uses uninitialized `varN` are rewritten
// to `throw varN_1`. Kill-switch: JDEC_LEAKED_EXCEPTION_SENTINEL_OFF.
func fixLeakedExceptionSentinel(body string) string {
	if os.Getenv("JDEC_LEAKED_EXCEPTION_SENTINEL_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "= Exception;") && !strings.Contains(body, "= Exception\n") {
		return body
	}
	body = rewriteLeakedTryLockFinally(body)
	body = rewriteLeakedCatchFromSibling(body)
	body = rewriteCatchThrowSplitId(body)
	return body
}

func applyLeakedExceptionSentinel(res *dumpedMethods) {
	if res == nil {
		return
	}
	res.code = fixLeakedExceptionSentinel(res.code)
	res.bodyCode = fixLeakedExceptionSentinel(res.bodyCode)
}

// rewriteLeakedCatchFromSibling replaces `T varN = Exception; <terminator>` with a sibling
// `try { ... } catch(T ...) { <terminator> }` found in the same method.
func rewriteLeakedCatchFromSibling(body string) string {
	const needle = " = Exception;"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		lineStart := strings.LastIndex(body[:i], "\n") + 1
		stmt := strings.TrimSpace(body[lineStart:i])
		parts := strings.Fields(stmt)
		var typ string
		if len(parts) == 2 {
			typ = parts[0]
		} else {
			from = i + 1
			continue
		}
		after := body[i+len(needle):]
		trimAfter := strings.TrimLeft(after, " \t\n")
		termEnd := strings.Index(trimAfter, ";")
		if termEnd < 0 {
			from = i + 1
			continue
		}
		term := strings.TrimSpace(trimAfter[:termEnd+1])
		if !strings.HasPrefix(term, "return") && !strings.HasPrefix(term, "throw") {
			from = i + 1
			continue
		}
		tryBlock := findSiblingTryCatch(body, typ, term)
		if tryBlock == "" {
			from = i + 1
			continue
		}
		ws := len(after) - len(strings.TrimLeft(after, " \t\n"))
		termInBody := i + len(needle) + ws + termEnd + 1
		body = body[:lineStart] + tryBlock + body[termInBody:]
		from = lineStart + len(tryBlock)
	}
}

func findSiblingTryCatch(body, typ, term string) string {
	from := 0
	for {
		rel, tryTok := indexTryOpen(body[from:])
		if rel < 0 {
			return ""
		}
		open := from + rel + len(tryTok) - 1
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from += rel + 1
			continue
		}
		tryInner := body[open+1 : close]
		if strings.Contains(tryInner, "= Exception") {
			from = close
			continue
		}
		after := strings.TrimLeft(body[close+1:], " \t\n")
		if !strings.HasPrefix(after, "catch(") {
			from = close
			continue
		}
		declEnd := strings.Index(after, "){")
		if declEnd < 0 {
			declEnd = strings.Index(after, ") {")
		}
		if declEnd < 0 {
			from = close
			continue
		}
		decl := strings.TrimSpace(after[len("catch("):declEnd])
		fields := strings.Fields(decl)
		if len(fields) == 0 || (typ != "" && fields[0] != typ) {
			from = close
			continue
		}
		catchOpenRel := strings.Index(body[close+1:], "{")
		if catchOpenRel < 0 {
			from = close
			continue
		}
		cOpen := close + 1 + catchOpenRel
		cClose := matchingCloseBrace(body, cOpen)
		if cClose < 0 {
			from = close
			continue
		}
		catchInner := body[cOpen+1 : cClose]
		if strings.Contains(catchInner, term) {
			return body[from+rel : cClose+1]
		}
		from = cClose
	}
}

func indexTryOpen(s string) (int, string) {
	a := strings.Index(s, "try{")
	b := strings.Index(s, "try {")
	switch {
	case a < 0 && b < 0:
		return -1, ""
	case a < 0:
		return b, "try {"
	case b < 0:
		return a, "try{"
	case a < b:
		return a, "try{"
	default:
		return b, "try {"
	}
}

// rewriteLeakedTryLockFinally replaces a 0-arg tryLock() success arm that inlined a leaked
// finally-handler (`= Exception`) with the sibling timed-tryLock else body (the real try/finally).
func rewriteLeakedTryLockFinally(body string) string {
	const mark = ".tryLock()){"
	from := 0
	for {
		rel := strings.Index(body[from:], mark)
		if rel < 0 {
			return body
		}
		open := from + rel + len(mark) - 1
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from += rel + 1
			continue
		}
		inner := body[open+1 : close]
		if !strings.Contains(inner, "= Exception") {
			from = close
			continue
		}
		ifHead := strings.LastIndex(body[:from+rel], "if (")
		if ifHead < 0 {
			from = close
			continue
		}
		recv := strings.TrimSpace(body[ifHead+4 : from+rel])
		if recv == "" || strings.ContainsAny(recv, "\n{}") {
			from = close
			continue
		}
		rest := body[close+1:]
		timed := "if (!(" + recv + ".tryLock("
		tRel := strings.Index(rest, timed)
		if tRel < 0 {
			from = close
			continue
		}
		tOpenRel := strings.Index(rest[tRel:], "{")
		if tOpenRel < 0 {
			from = close
			continue
		}
		tOpen := close + 1 + tRel + tOpenRel
		tClose := matchingCloseBrace(body, tOpen)
		if tClose < 0 {
			from = close
			continue
		}
		afterT := strings.TrimLeft(body[tClose+1:], " \t\n")
		if !strings.HasPrefix(afterT, "else{") && !strings.HasPrefix(afterT, "else {") {
			from = close
			continue
		}
		elseOpenRel := strings.Index(body[tClose+1:], "{")
		if elseOpenRel < 0 {
			from = close
			continue
		}
		eOpen := tClose + 1 + elseOpenRel
		eClose := matchingCloseBrace(body, eOpen)
		if eClose < 0 {
			from = close
			continue
		}
		good := body[eOpen+1 : eClose]
		if !strings.Contains(good, "try{") && !strings.Contains(good, "try {") {
			from = close
			continue
		}
		body = body[:open+1] + good + body[close:]
		from = open + 1 + len(good)
	}
}

// rewriteCatchThrowSplitId rewrites `catch(T varN_1){ ... throw varN; }` to `throw varN_1`
// when the catch parameter was split from the leaked outer throwable slot.
func rewriteCatchThrowSplitId(body string) string {
	from := 0
	const head = "catch("
	for {
		rel := strings.Index(body[from:], head)
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len(head):]
		paren := strings.Index(rest, "){")
		if paren < 0 {
			paren = strings.Index(rest, ") {")
		}
		if paren < 0 {
			from = i + 1
			continue
		}
		decl := strings.TrimSpace(rest[:paren])
		fields := strings.Fields(decl)
		if len(fields) != 2 || !strings.HasSuffix(fields[1], "_1") {
			from = i + 1
			continue
		}
		name := fields[1]
		base := strings.TrimSuffix(name, "_1")
		openRel := strings.Index(rest, "{")
		if openRel < 0 {
			from = i + 1
			continue
		}
		open := i + len(head) + openRel
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := rewriteCatchThrowSplitId(body[open+1 : close])
		oldThrow := "throw " + base + ";"
		newThrow := "throw " + name + ";"
		if strings.Contains(inner, oldThrow) {
			inner = strings.ReplaceAll(inner, oldThrow, newThrow)
		}
		if inner != body[open+1:close] {
			body = body[:open+1] + inner + body[close:]
			from = open + 1 + len(inner)
			continue
		}
		from = close
	}
}
