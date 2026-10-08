package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumSwitchMultipleTablesRequireCompleteIndependentOrders(t *testing.T) {
	const a = "/*jdec-owned-enum-switch:a*/"
	const b = "/*jdec-owned-enum-switch:b*/"
	first := `switch(x` + a + `){case SECOND:break;case FIRST:break;}`
	second := `switch(y` + b + `){case FIRST:break;case SECOND:break;}`
	for _, variant := range []string{"original", "nested postorder", "reversed tables", "wrong original init order", "missing original init order", "duplicate original field", "unknown original field", "duplicate enum", "unknown use field", "cross-table key order", "missing table", "repeated table use", "quoted table cannot initialize", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			useA := &nativeEnumSwitchUse{field: "a", rendered: true, marker: a, keys: map[int]bool{1: true, 2: true}}
			useB := &nativeEnumSwitchUse{field: "b", rendered: true, marker: b, keys: map[int]bool{1: true, 2: true}}
			table := &nativeEnumSwitchTable{
				tables: map[string]*nativeEnumSwitchArray{
					"a": {enum: "EnumA", entries: map[int]string{1: "SECOND", 2: "FIRST"}},
					"b": {enum: "EnumB", entries: map[int]string{1: "FIRST", 2: "SECOND"}},
				},
				initializationOrder: []string{"b", "a"},
				uses:                map[string]map[string]map[int]*nativeEnumSwitchUse{"Owner": {"run()V": {1: useA, 2: useB}}},
			}
			source := first + second
			var work *workbudget.Budget
			switch variant {
			case "nested postorder":
				source = strings.Replace(first, "case SECOND:break;", "case SECOND:"+second+"break;", 1)
				table.initializationOrder = []string{"a", "b"}
			case "reversed tables":
				source = second + first
			case "wrong original init order":
				table.initializationOrder = []string{"a", "b"}
			case "missing original init order":
				table.initializationOrder = nil
			case "duplicate original field":
				table.initializationOrder[0] = "a"
			case "unknown original field":
				table.initializationOrder[0] = "unknown"
			case "duplicate enum":
				table.tables["b"].enum = "EnumA"
			case "unknown use field":
				useB.field = "unknown"
			case "cross-table key order":
				source = first + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(second, "FIRST", "TMP"), "SECOND", "FIRST"), "TMP", "SECOND")
			case "missing table":
				source = first
			case "repeated table use":
				source += second
			case "quoted table cannot initialize":
				source = first + `String s="` + b + `";`
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			family := &nativeMemberFamily{enumSwitchTables: map[string]*nativeEnumSwitchTable{"Helper": table}}
			if got := nativeEnumSwitchSourceComplete(family, source, work); got != (variant == "original" || variant == "nested postorder") {
				t.Fatalf("source admission=%v", got)
			}
		})
	}
}

func TestNativeEnumSwitchNestedSameTableRegistersChildrenFirst(t *testing.T) {
	outer, inner := "/*jdec-owned-enum-switch:outer*/", "/*jdec-owned-enum-switch:inner*/"
	for _, order := range []string{"original", "lexical preorder"} {
		t.Run(order, func(t *testing.T) {
			arr := &nativeEnumSwitchArray{enum: "EnumA", entries: map[int]string{1: "SECOND", 2: "FIRST"}}
			if order != "original" {
				arr.entries = map[int]string{1: "FIRST", 2: "SECOND"}
			}
			table := &nativeEnumSwitchTable{tables: map[string]*nativeEnumSwitchArray{"a": arr}, initializationOrder: []string{"a"}, uses: map[string]map[string]map[int]*nativeEnumSwitchUse{"Owner": {"run()V": {
				1: {field: "a", rendered: true, marker: outer, keys: map[int]bool{2: true}},
				2: {field: "a", rendered: true, marker: inner, keys: map[int]bool{1: true}},
			}}}}
			source := `switch(x` + outer + `){case FIRST:switch(y` + inner + `){case SECOND:break;}break;}`
			if order != "original" {
				// Keep each switch's permitted original labels unchanged so only
				// the compiler registration order is the distinguishing fact.
				table.uses["Owner"]["run()V"][1].keys = map[int]bool{1: true}
				table.uses["Owner"]["run()V"][2].keys = map[int]bool{2: true}
			}
			if got := nativeEnumSwitchCaseRegistrationClosed(table, source, nil); got != (order == "original") {
				t.Fatalf("postorder admission=%v", got)
			}
		})
	}
}
