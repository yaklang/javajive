package javaclassparser

import (
	"os"
	"strings"
)

// fixAlreadyCaughtDuplicateCatch drops a later catch(T) when an earlier catch
// in the same try already covers T (typically via multicatch). javac rejects
// "exception T has already been caught". Real hit: junit
// ManagementFactory$FactoryHolder.getBeanObject unions NSME onto
// IllegalAccessException then emits a second catch(NSME).
// Kill-switch: JDEC_ALREADY_CAUGHT_OFF=1.
func fixAlreadyCaughtDuplicateCatch(body string) string {
	if os.Getenv("JDEC_ALREADY_CAUGHT_OFF") == "1" {
		return body
	}
	for i := 0; i < 16; i++ {
		next := dropOneAlreadyCaught(body)
		if next == body {
			return body
		}
		body = next
	}
	return body
}

func dropOneAlreadyCaught(body string) string {
	from := 0
	for {
		rel, tryTok := indexTryOpen(body[from:])
		if rel < 0 {
			return body
		}
		tryOpen := from + rel + len(tryTok) - 1
		tryClose := matchingCloseBrace(body, tryOpen)
		if tryClose < 0 {
			from += rel + 1
			continue
		}
		clauses := collectCatchClauses(body, tryClose+1)
		if len(clauses) < 2 {
			from = tryClose + 1
			continue
		}
		seen := map[string]bool{}
		for _, cl := range clauses {
			dup := len(cl.types) > 0
			for _, t := range cl.types {
				if !seen[t] {
					dup = false
					break
				}
			}
			if dup {
				return body[:cl.start] + body[cl.end:]
			}
			for _, t := range cl.types {
				seen[t] = true
			}
		}
		from = tryClose + 1
	}
}

type catchClauseSpan struct {
	start, end int
	types      []string
}

func collectCatchClauses(body string, pos int) []catchClauseSpan {
	var out []catchClauseSpan
	for pos < len(body) {
		ws := pos
		for ws < len(body) && (body[ws] == ' ' || body[ws] == '\t' || body[ws] == '\n' || body[ws] == '\r') {
			ws++
		}
		if ws >= len(body) || !strings.HasPrefix(body[ws:], "catch(") {
			return out
		}
		rest := body[ws+len("catch("):]
		closeParen := strings.Index(rest, ")")
		if closeParen < 0 {
			return out
		}
		decl := rest[:closeParen]
		types, _ := splitCatchDecl(decl)
		braceRel := strings.Index(rest[closeParen:], "{")
		if braceRel < 0 {
			return out
		}
		open := ws + len("catch(") + closeParen + braceRel
		close := matchingCloseBrace(body, open)
		if close < 0 {
			return out
		}
		out = append(out, catchClauseSpan{start: ws, end: close + 1, types: types})
		pos = close + 1
	}
	return out
}

func splitCatchDecl(decl string) ([]string, string) {
	decl = strings.TrimSpace(decl)
	if decl == "" {
		return nil, ""
	}
	fields := strings.Fields(strings.ReplaceAll(decl, "|", " | "))
	if len(fields) == 0 {
		return nil, ""
	}
	name := fields[len(fields)-1]
	var types []string
	for _, f := range fields[:len(fields)-1] {
		if f == "|" {
			continue
		}
		types = append(types, f)
	}
	return types, name
}
