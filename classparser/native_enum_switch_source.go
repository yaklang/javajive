package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

func (c *ClassObjectDumper) wireNativeEnumSwitchSource(p *nativeMemberFamily, ctx *class_context.ClassContext) {
	if p == nil || len(p.enumSwitchTables) == 0 {
		return
	}
	ctx.SourceEnumSwitch = func(value any, labels []int) (string, map[int]string, bool) {
		raw, ok := value.(values.JavaValue)
		if !ok {
			return "", nil, false
		}
		array, ok := values.UnpackSoltValue(raw).(*values.JavaArrayMember)
		if !ok || array == nil || !array.HasOriginPC {
			return "", nil, false
		}
		field, ok := values.UnpackSoltValue(array.Object).(*values.JavaClassMember)
		if !ok || field == nil {
			return "", nil, false
		}
		table := p.enumSwitchTables[strings.ReplaceAll(field.Name, ".", "/")]
		if table == nil {
			return "", nil, false
		}
		fail := func() (string, map[int]string, bool) { p.failed = true; return "", nil, false }
		use := table.uses[c.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][array.OriginPC]
		arr := table.tables[field.Member]
		if use == nil || arr == nil || field.Description != "[I" || !field.HasOriginPC || field.OriginPC != use.getPC || field.Member != use.field || !field.OriginalStaticFieldRead(use.getPC, field.Name, use.field, "[I") || len(labels) != len(use.keys) {
			return fail()
		}
		call, ok := values.UnpackSoltValue(array.Index).(*values.FunctionCallExpression)
		if !ok || call == nil || !call.HasOriginPC || call.OriginPC != use.ordinalPC || call.IsStatic || call.IsSpecialInvoke || call.Kind != values.InvokeVirtual || call.FunctionName != "ordinal" || call.Descriptor != "()I" || len(call.Arguments) != 0 || strings.ReplaceAll(call.ClassName, ".", "/") != arr.enum || call.Object == nil {
			return fail()
		}
		ref, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
		if !ok {
			return fail()
		}
		slot, original := ref.OriginalParameterSlot()
		if !original || slot != use.parameterSlot || !nativeProofWork(c.Work, int64(4+len(labels))) || c.Work != nil && c.Work.CheckAlloc(int64(len(labels))*96+int64(len(use.marker))) != nil {
			return fail()
		}
		typ, known := values.SourceTypeErasure(call.Object.Type(), ctx)
		if !known || typ != "L"+arr.enum+";" {
			return fail()
		}
		mapping := map[int]string{}
		for _, k := range labels {
			if !use.keys[k] || mapping[k] != "" || arr.entries[k] == "" {
				return fail()
			}
			mapping[k] = arr.entries[k]
		}
		use.rendered = true
		return call.Object.String(ctx) + use.marker, mapping, true
	}
}

func nativeEnumSwitchSourceComplete(p *nativeMemberFamily, source string, work *workbudget.Budget) bool {
	if p == nil || p.failed {
		return false
	}
	if len(p.enumSwitchTables) == 0 {
		return true
	}
	markers, closed := nativeEnumSwitchSourceMarkers(source, work)
	if !closed {
		return false
	}
	expected := map[string]bool{}
	for _, table := range p.enumSwitchTables {
		count := 0
		for _, methods := range table.uses {
			for _, uses := range methods {
				for _, use := range uses {
					count++
					if use == nil || !use.rendered || use.marker == "" || expected[use.marker] || markers[use.marker] != 1 {
						return false
					}
					expected[use.marker] = true
				}
			}
		}
		if count == 0 || !nativeEnumSwitchCaseRegistrationClosed(table, source, work) {
			return false
		}
	}
	return len(markers) == len(expected)
}

// javac numbers a table by the first lexical occurrence of its own cases.
// A same-named case in another enum switch cannot prove this table's key order.
// Bind each ownership comment to a selector and collect only that switch's
// direct labels; nested switch bodies and ordinary blocks have distinct scopes.
func nativeEnumSwitchCaseRegistrationClosed(table *nativeEnumSwitchTable, source string, work *workbudget.Budget) bool {
	if table == nil || len(table.tables) != 1 || !nativeProofWork(work, int64(len(source))) || work != nil && work.CheckAlloc(int64(len(source))*32+65536) != nil {
		return false
	}
	var arr *nativeEnumSwitchArray
	for _, a := range table.tables {
		arr = a
	}
	if arr == nil {
		return false
	}
	uses := map[string]*nativeEnumSwitchUse{}
	for _, methods := range table.uses {
		for _, method := range methods {
			for _, use := range method {
				if use == nil || use.marker == "" || uses[use.marker] != nil {
					return false
				}
				uses[use.marker] = use
			}
		}
	}
	if len(uses) == 0 {
		return false
	}
	// Empty stack entries represent scopes that do not own this table.
	stack := []string{}
	openings := map[int]string{}
	bound := map[string]bool{}
	labels := map[string]map[string]bool{}
	allowedLabels := map[string]map[string]bool{}
	seen := map[string]bool{}

	for marker, use := range uses {
		allowedLabels[marker] = map[string]bool{}
		for key := range use.keys {
			name := arr.entries[key]
			if name == "" || allowedLabels[marker][name] || !nativeProofWork(work, 1) {
				return false
			}
			allowedLabels[marker][name] = true
		}
	}
	state, depth := scanNormal, 0
	keyword := func(i int, word string) bool {
		return strings.HasPrefix(source[i:], word) && (i == 0 || !isJavaIdentChar(source[i-1])) && (i+len(word) == len(source) || !isJavaIdentChar(source[i+len(word)]))
	}
	for i := 0; i < len(source); i++ {
		state = scanAdvance(source, &i, state, &depth)
		if state != scanNormal || i >= len(source) {
			continue
		}
		if keyword(i, "switch") {
			open := i + len("switch")
			for open < len(source) && strings.ContainsRune(" \t\r\n", rune(source[open])) {
				open++
			}
			if open >= len(source) || source[open] != '(' {
				return false
			}
			close := nativeEnumSwitchSelectorClose(source, open, work)
			if close < 0 {
				return false
			}
			markers, ok := nativeEnumSwitchSourceMarkers(source[open+1:close], work)
			if !ok || len(markers) > 1 {
				return false
			}
			marker := ""
			for m, count := range markers {
				if count != 1 || uses[m] == nil || bound[m] || !strings.HasSuffix(strings.TrimSpace(source[open+1:close]), m) {
					return false
				}
				marker = m
				bound[m] = true
				labels[m] = map[string]bool{}
			}
			body := close + 1
			for body < len(source) && strings.ContainsRune(" \t\r\n", rune(source[body])) {
				body++
			}
			if body >= len(source) || source[body] != '{' {
				return false
			}
			openings[body] = marker
			i = body - 1
			continue
		}
		switch source[i] {
		case '{':
			stack = append(stack, openings[i])
		case '}':
			if len(stack) == 0 {
				return false
			}
			stack = stack[:len(stack)-1]
		}
		if !keyword(i, "case") || len(stack) == 0 || stack[len(stack)-1] == "" {
			continue
		}
		marker := stack[len(stack)-1]
		end := i + 4
		for end < len(source) && source[end] != ':' && source[end] != '{' && source[end] != '}' {
			end++
		}
		if end == len(source) || source[end] != ':' {
			return false
		}
		name := strings.TrimSpace(source[i+4 : end])
		if !allowedLabels[marker][name] || labels[marker][name] {
			return false
		}
		labels[marker][name] = true
		if !seen[name] {
			if name != arr.entries[len(seen)+1] {
				return false
			}
			seen[name] = true
		}
		i = end
	}
	if len(stack) != 0 || state != scanNormal && state != scanLineComment || len(bound) != len(uses) || len(seen) != len(arr.entries) {
		return false
	}
	for marker, use := range uses {
		if len(labels[marker]) != len(use.keys) {
			return false
		}
	}
	return true
}

// Charge the matching scan itself, including nested parentheses. This keeps a
// deliberately nested selector from turning repeated lookahead into free work.
func nativeEnumSwitchSelectorClose(source string, open int, work *workbudget.Budget) int {
	state, depth := scanNormal, 0
	for i := open; i < len(source); i++ {
		if !nativeProofWork(work, 2) {
			return -1
		}
		state = scanAdvance(source, &i, state, &depth)
		if state != scanNormal || i >= len(source) {
			continue
		}
		switch source[i] {
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

// Count proof registrations in actual block comments only. Quoted marker-like
// data cannot stand in for a missing switch, and a repeated source use fails.
func nativeEnumSwitchSourceMarkers(source string, work *workbudget.Budget) (map[string]int, bool) {
	if !nativeProofWork(work, int64(len(source))) || work != nil && work.CheckAlloc(int64(len(source))+65536) != nil {
		return nil, false
	}
	markers := map[string]int{}
	state := scanNormal
	depth := 0
	for i := 0; i < len(source); i++ {
		if state == scanNormal && strings.HasPrefix(source[i:], "/*jdec-owned-enum-switch:") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return nil, false
			}
			marker := source[i : i+end+4]
			markers[marker]++
			i += end + 3
			continue
		}
		state = scanAdvance(source, &i, state, &depth)
	}
	return markers, state == scanNormal || state == scanLineComment
}
