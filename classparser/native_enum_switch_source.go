package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

func (c *ClassObjectDumper) wireNativeEnumSwitchSource(p *nativeMemberFamily, ctx *class_context.ClassContext) {
	if ctx == nil {
		return
	}
	ctx.SourceParameterStore = nil
	if p == nil || len(p.enumSwitchTables) == 0 {
		return
	}
	// Registration itself is bounded. If it exhausts a resource or observes
	// cancellation, retain a callable refusal projection rather than leaving
	// a half-wired context with a nil callback.
	ctx.SourceEnumSwitch = func(any, []int) (string, map[int]string, bool) { return "", nil, false }
	stores := map[string]map[[2]int][]*nativeEnumParameterStorage{}
	bindings := 0
	for _, table := range p.enumSwitchTables {
		for method, uses := range table.uses[c.obj.GetClassName()] {
			for _, use := range uses {
				if !nativeEnumSelectorStorages(use.selector, c.Work, 0, func(storage *nativeEnumParameterStorage) bool {
					for pc := range storage.stores {
						bindings++
						if bindings > 4096 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(128) != nil {
							return false
						}
						if stores[method] == nil {
							stores[method] = map[[2]int][]*nativeEnumParameterStorage{}
						}
						key := [2]int{storage.slot, pc}
						stores[method][key] = append(stores[method][key], storage)
					}
					return true
				}) {
					p.failed = true
					return
				}
			}
		}
	}
	if len(stores) > 0 {
		ctx.SourceParameterStore = func(value any) (string, bool) {
			a, known := value.(*statements.AssignStatement)
			if !known {
				return "", false
			}
			pc, slot, sealed := a.OriginalParameterStore()
			if !sealed {
				return "", false
			}
			marker := ""
			for _, storage := range stores[ctx.FunctionName+ctx.CurrentMethodDesc][[2]int{slot, pc}] {
				if _, valid := storage.assignment(a, ctx); !valid || !nativeProofWork(c.Work, 1) || storage.markers[pc] == "" || marker != "" && marker != storage.markers[pc] {
					p.failed = true
					return "", false
				}
				marker = storage.markers[pc]
			}
			return marker, marker != ""
		}
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
		if !nativeEnumSelectorStorages(use.selector, c.Work, 0, func(storage *nativeEnumParameterStorage) bool { return c.nativeEnumParameterStorageBody(storage, ctx) }) || !nativeEnumSelectorSource(use.selector, call.Object, ctx, c.Work, 0) || !nativeProofWork(c.Work, int64(4+len(labels))) || c.Work != nil && c.Work.CheckAlloc(int64(len(labels))*96+int64(len(use.marker))) != nil {
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
	storeMarkers, closed := nativeEnumSourceMarkers(source, "/*jdec-owned-parameter-store:", work)
	if !closed {
		return false
	}
	expectedStores := map[string]*values.JavaRef{}
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
					if !nativeEnumSelectorStorages(use.selector, work, 0, func(storage *nativeEnumParameterStorage) bool {
						if !storage.validated || storage.bound == nil || len(storage.markers) != len(storage.stores) {
							return false
						}
						for pc := range storage.stores {
							marker := storage.markers[pc]
							if marker == "" || storeMarkers[marker] != 1 || expectedStores[marker] != nil && expectedStores[marker] != storage.bound {
								return false
							}
							expectedStores[marker] = storage.bound
						}
						return true
					}) {
						return false
					}
				}
			}
		}
		if count == 0 || !nativeEnumSwitchCaseRegistrationClosed(table, source, work) {
			return false
		}
	}
	return len(markers) == len(expected) && len(storeMarkers) == len(expectedStores)
}

// javac lowers a switch's children before registering its own enum cases.
// It then prepends each table initializer, reversing the tables' registration
// order. Prove both orders from owned switches; an equal label in another enum,
// a comment, or an ordinary block contributes neither certificate.
func nativeEnumSwitchCaseRegistrationClosed(table *nativeEnumSwitchTable, source string, work *workbudget.Budget) bool {
	if !nativeEnumSwitchInitializationClosed(table, work) || !nativeProofWork(work, int64(len(source))) || work != nil && work.CheckAlloc(int64(len(source))*32+65536) != nil {
		return false
	}
	uses := map[string]*nativeEnumSwitchUse{}
	for _, methods := range table.uses {
		for _, method := range methods {
			for _, use := range method {
				if use == nil || use.marker == "" || table.tables[use.field] == nil || uses[use.marker] != nil {
					return false
				}
				uses[use.marker] = use
			}
		}
	}
	if len(uses) == 0 {
		return false
	}
	// A switch registers on leaving its body: nested switches have already
	// registered by then. Empty markers represent unrelated lexical scopes.
	type scope struct {
		marker string
		cases  []string
	}
	stack := []scope{}
	openings := map[int]string{}
	bound := map[string]bool{}
	labels := map[string]map[string]bool{}
	allowedLabels := map[string]map[string]bool{}
	registered := map[string]map[string]bool{}
	fieldOrder := []string{}

	for marker, use := range uses {
		arr := table.tables[use.field]
		allowedLabels[marker] = map[string]bool{}
		for key := range use.keys {
			name := arr.entries[key]
			if name == "" || allowedLabels[marker][name] || !nativeProofWork(work, 1) {
				return false
			}
			allowedLabels[marker][name] = true
		}
	}
	register := func(s scope) bool {
		if s.marker == "" {
			return true
		}
		use := uses[s.marker]
		arr := table.tables[use.field]
		seen := registered[use.field]
		if seen == nil {
			seen = map[string]bool{}
			registered[use.field] = seen
			fieldOrder = append(fieldOrder, use.field)
		}
		for _, name := range s.cases {
			if !nativeProofWork(work, 1) {
				return false
			}
			if !seen[name] {
				if name != arr.entries[len(seen)+1] {
					return false
				}
				seen[name] = true
			}
		}
		return true
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
			stack = append(stack, scope{marker: openings[i]})
		case '}':
			if len(stack) == 0 || !register(stack[len(stack)-1]) {
				return false
			}
			stack = stack[:len(stack)-1]
		}
		if !keyword(i, "case") || len(stack) == 0 || stack[len(stack)-1].marker == "" {
			continue
		}
		current := &stack[len(stack)-1]
		marker := current.marker
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
		current.cases = append(current.cases, name)
		i = end
	}
	if len(stack) != 0 || state != scanNormal && state != scanLineComment || len(bound) != len(uses) || len(fieldOrder) != len(table.initializationOrder) {
		return false
	}
	for i, field := range table.initializationOrder {
		if field != fieldOrder[len(fieldOrder)-1-i] || len(registered[field]) != len(table.tables[field].entries) {
			return false
		}
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
	return nativeEnumSourceMarkers(source, "/*jdec-owned-enum-switch:", work)
}

func nativeEnumSourceMarkers(source, prefix string, work *workbudget.Budget) (map[string]int, bool) {
	if !nativeProofWork(work, int64(len(source))) || work != nil && work.CheckAlloc(int64(len(source))+65536) != nil {
		return nil, false
	}
	markers := map[string]int{}
	state := scanNormal
	depth := 0
	for i := 0; i < len(source); i++ {
		if state == scanNormal && strings.HasPrefix(source[i:], prefix) {
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
