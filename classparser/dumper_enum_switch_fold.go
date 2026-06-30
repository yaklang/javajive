package javaclassparser

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/internal/log"
)

// Enum-switch ($SwitchMap) CROSS-CLASS folding (Bug V).
//
// For source `switch(enumValue){ case CONST: ... }` javac emits, per outer class, a synthetic holder
// `Outer$N` carrying `static int[] $SwitchMap$<EnumFqcn>` whose <clinit> fills
// `$SwitchMap$...[Enum.CONST.ordinal()] = k;`, and lowers the switch to
// `switch(Outer$N.$SwitchMap$...[enumValue.ordinal()]){ case k: ... }`. Decompiled per class that is
// UNCOMPILABLE in isolation: `Outer$N` is an anonymous class whose flat `Outer$N` source name does
// not resolve against the original jar (javac knows it only as the enclosing class's anonymous #N).
//
// We fold the switch back to the idiomatic `switch(enumValue){ case CONST: ... }`, which javac
// re-lowers on recompile. The synthetic holder `Outer$N` is intentionally LEFT emitted (it compiles
// standalone, and javac also reuses it as a synthetic private-constructor access marker -- `(Outer$N)
// null` -- so suppressing it is unsafe in general). Folding only the switch is therefore monotonic:
// a well-formed `switch(enum)` never makes a previously-passing unit fail.
//
// The synthetic holder bytes are supplied via the dumper's foldSiblingResolver; a nil resolver (the
// single-class path) makes this a no-op. Load-bearing kill-switch: JDEC_NO_ENUM_SWITCH_FOLD.

// switchMapSelectorRe matches a switch selector of the exact $SwitchMap idiom shape:
// `Outer$N.$SwitchMap$<EnumFqcn>[<sel>.ordinal()]`, capturing holder / array field / selector expr.
var switchMapSelectorRe = regexp.MustCompile(`^([\w$]+\$\d+)\.(\$SwitchMap\$[\w$]+)\[(.+)\.ordinal\(\)\]$`)

// enumSwitchMapEntryRe matches one holder <clinit> entry `$SwitchMap$X[Enum.CONST.ordinal()] = k;`
// (possibly wrapped in try/catch(NoSuchFieldError)), capturing array field / constant name / int key.
var enumSwitchMapEntryRe = regexp.MustCompile(`(\$SwitchMap\$[\w$]+)\[[^\]]*?(\w+)\.ordinal\(\)\]\s*=\s*(\d+)\s*;`)

// switchMapSwitch describes one located `switch(Outer$N.$SwitchMap$E[sel.ordinal()]){...}` in source.
type switchMapSwitch struct {
	selOpen     int // index of '(' opening the switch selector
	selClose    int // index of the matching ')'
	bodyOpen    int // index of '{' opening the switch body
	bodyClose   int // index of the matching '}'
	headerClose int // selClose+1, used as the skip point when a switch is not foldable
	holder      string
	array       string
	sel         string
}

// foldEnumSwitchMaps rewrites every recognizable `switch(Outer$N.$SwitchMap$E[sel.ordinal()])` in src
// back to `switch(sel)` with integer case labels remapped to enum constant names. It is tightly
// guarded: a switch is folded only when its holder resolves, its <clinit> map is non-empty, and EVERY
// integer case label has a constant mapping; otherwise that switch is left byte-for-byte untouched.
// Disabled by a nil resolver or JDEC_NO_ENUM_SWITCH_FOLD.
func (c *ClassObjectDumper) foldEnumSwitchMaps(src string) string {
	if c.foldSiblingResolver == nil || os.Getenv("JDEC_NO_ENUM_SWITCH_FOLD") != "" {
		return src
	}
	if !strings.Contains(src, "$SwitchMap$") {
		return src
	}
	debug := os.Getenv("JDEC_FOLD_DEBUG") != ""
	pkgPath := strings.ReplaceAll(c.PackageName, ".", "/")
	searchFrom := 0
	for {
		sw := findSwitchMapSwitch(src, searchFrom)
		if sw == nil {
			break
		}
		newSrc, ok := c.tryFoldOneSwitchMap(src, sw, pkgPath, debug)
		if ok {
			src = newSrc
			// Re-scan from the (now rewritten) selector: it no longer matches the $SwitchMap shape,
			// so the loop advances naturally past it without recomputing shifted indices.
			searchFrom = sw.selOpen
		} else {
			searchFrom = sw.headerClose
		}
	}
	return src
}

// tryFoldOneSwitchMap attempts to fold the single switch described by sw. On success it returns the
// rewritten source and true; on any guard failure it returns src unchanged and false.
func (c *ClassObjectDumper) tryFoldOneSwitchMap(src string, sw *switchMapSwitch, pkgPath string, debug bool) (string, bool) {
	internal := sw.holder
	if pkgPath != "" {
		internal = pkgPath + "/" + sw.holder
	}
	data, ok := c.foldSiblingResolver(internal)
	if !ok || len(data) == 0 {
		if debug {
			log.Infof("enum-switch fold: holder %s not resolved", internal)
		}
		return src, false
	}
	m := parseSwitchMap(data, sw.array)
	if len(m) == 0 {
		if debug {
			log.Infof("enum-switch fold: holder %s array %s yielded empty map", internal, sw.array)
		}
		return src, false
	}
	body := src[sw.bodyOpen+1 : sw.bodyClose]
	newBody, ok := remapSwitchCases(body, m)
	if !ok {
		if debug {
			log.Infof("enum-switch fold: %s case labels not fully mapped, skipping", internal)
		}
		return src, false
	}
	var b strings.Builder
	b.WriteString(src[:sw.selOpen+1])
	b.WriteString(sw.sel)
	b.WriteString(src[sw.selClose : sw.bodyOpen+1])
	b.WriteString(newBody)
	b.WriteString(src[sw.bodyClose:])
	if debug {
		log.Infof("enum-switch fold: folded switch on %s.%s -> switch(%s)", sw.holder, sw.array, sw.sel)
	}
	return b.String(), true
}

// findSwitchMapSwitch returns the next foldable-shaped switch at/after from, or nil. It scans
// comment/quote aware (so `switch` in a string/comment is ignored) and only returns switches whose
// selector matches switchMapSelectorRe and whose body braces balance.
func findSwitchMapSwitch(src string, from int) *switchMapSwitch {
	for {
		kw := nextNormalKeyword(src, "switch", from)
		if kw < 0 {
			return nil
		}
		selOpen := nextNormalChar(src, '(', kw+len("switch"))
		if selOpen < 0 {
			return nil
		}
		selClose := javaMatchParen(src, selOpen)
		if selClose < 0 {
			return nil
		}
		selector := strings.TrimSpace(src[selOpen+1 : selClose])
		m := switchMapSelectorRe.FindStringSubmatch(selector)
		if m == nil {
			from = selClose + 1
			continue
		}
		bodyOpen := javaIndexBraceFrom(src, selClose)
		if bodyOpen < 0 {
			from = selClose + 1
			continue
		}
		if strings.TrimSpace(src[selClose+1:bodyOpen]) != "" {
			from = selClose + 1
			continue
		}
		bodyClose := javaMatchBrace(src, bodyOpen)
		if bodyClose < 0 {
			from = selClose + 1
			continue
		}
		return &switchMapSwitch{
			selOpen:     selOpen,
			selClose:    selClose,
			bodyOpen:    bodyOpen,
			bodyClose:   bodyClose,
			headerClose: selClose + 1,
			holder:      m[1],
			array:       m[2],
			sel:         strings.TrimSpace(m[3]),
		}
	}
}

// parseSwitchMap decompiles the synthetic holder bytes and builds intKey -> constantName for the given
// array field, by parsing its <clinit> entries. Returns nil on any failure. The sub-dumper has no
// resolver, so this never recurses into folding.
func parseSwitchMap(data []byte, arrayName string) map[int]string {
	obj, err := Parse(data)
	if err != nil {
		return nil
	}
	src, err := obj.Dump()
	if err != nil || src == "" {
		return nil
	}
	out := map[int]string{}
	for _, mm := range enumSwitchMapEntryRe.FindAllStringSubmatch(src, -1) {
		if mm[1] != arrayName {
			continue
		}
		n, err := strconv.Atoi(mm[3])
		if err != nil {
			continue
		}
		out[n] = mm[2]
	}
	return out
}

// remapSwitchCases rewrites each depth-0 `case <int>:` label inside the switch body to `case <CONST>:`
// per m. It returns false (abort, leave switch untouched) if there are no integer cases or if any
// integer case has no mapping -- folding a partial switch would drop branches. Nested switches (at
// depth>0) are left to their own fold iteration. Scanning is comment/quote aware.
func remapSwitchCases(body string, m map[int]string) (string, bool) {
	type repl struct {
		start, end int
		text       string
	}
	var repls []repl
	depth := 0
	st := scanNormal
	for i := 0; i < len(body); i++ {
		st = scanAdvance(body, &i, st, &depth)
		if st != scanNormal || i >= len(body) {
			continue
		}
		switch body[i] {
		case '{':
			depth++
			continue
		case '}':
			depth--
			continue
		}
		if depth != 0 {
			continue
		}
		if !strings.HasPrefix(body[i:], "case") {
			continue
		}
		if i > 0 && isJavaIdentChar(body[i-1]) {
			continue
		}
		j := i + len("case")
		if j >= len(body) || isJavaIdentChar(body[j]) {
			continue
		}
		for j < len(body) && (body[j] == ' ' || body[j] == '\t') {
			j++
		}
		ns := j
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
		}
		if j == ns {
			continue // non-integer case label (not a $SwitchMap case)
		}
		k := j
		for k < len(body) && (body[k] == ' ' || body[k] == '\t') {
			k++
		}
		if k >= len(body) || body[k] != ':' {
			continue
		}
		n, err := strconv.Atoi(body[ns:j])
		if err != nil {
			continue
		}
		name, ok := m[n]
		if !ok {
			return "", false
		}
		repls = append(repls, repl{ns, j, name})
		i = k
	}
	if len(repls) == 0 {
		return "", false
	}
	out := body
	for x := len(repls) - 1; x >= 0; x-- {
		r := repls[x]
		out = out[:r.start] + r.text + out[r.end:]
	}
	return out, true
}

// nextNormalKeyword returns the index of the next whole-word occurrence of kw in normal (non-comment,
// non-string) lexical state at/after from, or -1.
func nextNormalKeyword(src, kw string, from int) int {
	if from < 0 {
		from = 0
	}
	st := scanNormal
	depth := 0
	for i := from; i < len(src); i++ {
		st = scanAdvance(src, &i, st, &depth)
		if st != scanNormal || i >= len(src) {
			continue
		}
		if !strings.HasPrefix(src[i:], kw) {
			continue
		}
		if i > 0 && isJavaIdentChar(src[i-1]) {
			continue
		}
		after := i + len(kw)
		if after < len(src) && isJavaIdentChar(src[after]) {
			continue
		}
		return i
	}
	return -1
}

// nextNormalChar returns the index of the next occurrence of ch in normal lexical state at/after from,
// or -1.
func nextNormalChar(src string, ch byte, from int) int {
	if from < 0 {
		from = 0
	}
	st := scanNormal
	depth := 0
	for i := from; i < len(src); i++ {
		st = scanAdvance(src, &i, st, &depth)
		if st != scanNormal || i >= len(src) {
			continue
		}
		if src[i] == ch {
			return i
		}
	}
	return -1
}

// javaMatchParen returns the index of the ')' matching the '(' at openIdx, comment/quote aware.
func javaMatchParen(src string, openIdx int) int {
	depth := 0
	st := scanNormal
	for i := openIdx; i < len(src); i++ {
		st = scanAdvance(src, &i, st, &depth)
		if st != scanNormal || i >= len(src) {
			continue
		}
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
