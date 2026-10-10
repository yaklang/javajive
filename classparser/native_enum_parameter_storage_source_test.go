package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumParameterStorageRequiresCompleteBoundSourceWriters(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "loop", "branch", "missing writer", "cached validation", "duplicate writer", "new RHS", "other equal-typed ref", "new UID", "new entry seed", "declaration", "unsealed writer", "expression writer", "opaque source", "cycle", "wrong storage slot", "wrong storage descriptor", "wrong writer PC", "work", "memory", "canceled", "depth"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("StoreMode")
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			ref.IsParam = true
			ref.MarkOriginalParameter(1)
			rhs := values.NewJavaLiteral("B", typ)
			a := statements.NewAssignStatement(ref, rhs, false)
			a.OriginPC, a.HasOriginPC = 11, true
			a.MarkOriginalParameterStore(11, 1)
			s := &nativeEnumParameterStorage{slot: 1, descriptor: "LStoreMode;", stores: map[int]bool{11: true}, markers: map[int]string{11: "/*jdec-owned-parameter-store:test*/"}}
			body := []statements.Statement{a, statements.NewReturnStatement(ref)}
			var work *workbudget.Budget
			switch variant {
			case "renamed":
				ref.Id = utils.NewRootVariableId()
			case "loop":
				body = []statements.Statement{&statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), Body: body}}
			case "branch":
				body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: body}}
			case "missing writer":
				body = body[1:]
			case "cached validation":
				s.validated = true
				body = body[1:]
			case "duplicate writer":
				body = append(body, a)
			case "new RHS":
				a.JavaValue = values.NewJavaLiteral("B", typ)
			case "other equal-typed ref":
				other := values.NewJavaRef(ref.Id, nil, typ)
				other.IsParam, other.VarUid = true, ref.VarUid
				other.MarkOriginalParameter(1)
				body[1] = statements.NewReturnStatement(other)
			case "new UID":
				ref.VarUid += "other"
			case "new entry seed":
				ref.Val = rhs
			case "declaration":
				a.IsDeclare = true
			case "unsealed writer":
				body = append(body, statements.NewAssignStatement(ref, rhs, false))
			case "expression writer":
				body = append(body, &statements.ExpressionStatement{Expression: &values.AssignmentExpression{Target: ref, Value: rhs}})
			case "opaque source":
				body = append(body, &statements.CustomStatement{Name: "break", StringFunc: func(*class_context.ClassContext) string { return "first=other" }})
			case "cycle":
				loop := &statements.WhileStatement{ConditionValue: ref}
				loop.Body = []statements.Statement{loop}
				body = append(body, loop)
			case "wrong storage slot":
				s.slot = 2
			case "wrong storage descriptor":
				s.descriptor = "Ljava/lang/Object;"
			case "wrong writer PC":
				s.stores = map[int]bool{12: true}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				_ = work.CheckAlloc(2)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "depth":
				for i := 0; i < 33; i++ {
					body = []statements.Statement{&statements.WhileStatement{ConditionValue: ref, Body: body}}
				}
			}
			d := &ClassObjectDumper{nativeMemberBody: body, Work: work}
			ctx := &class_context.ClassContext{}
			got := d.nativeEnumParameterStorageBody(s, ctx)
			want := variant == "original" || variant == "renamed" || variant == "loop" || variant == "branch"
			if got != want {
				t.Fatalf("bound source writers=%v want %v", got, want)
			}
			if got && (!s.validated || s.bound != ref) {
				t.Fatal("no exact source binder")
			}
			if !got && s.validated {
				t.Fatal("failed body borrowed a prior successful validation")
			}
		})
	}
}

func TestNativeEnumParameterStorageFinalSourceRequiresRetainedUniqueStores(t *testing.T) {
	for _, variant := range []string{"original", "quoted data harmless", "quoted substitution", "missing writer", "duplicate writer", "unknown writer", "other binder", "unvalidated storage", "unterminated string", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("StoreMode")
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			ref.IsParam = true
			ref.MarkOriginalParameter(1)
			marker := "/*jdec-owned-parameter-store:test*/"
			switchMarker := "/*jdec-owned-enum-switch:use*/"
			s := &nativeEnumParameterStorage{slot: 1, descriptor: "LStoreMode;", stores: map[int]bool{11: true}, markers: map[int]string{11: marker}, bound: ref, validated: true}
			selector := &nativeEnumSelectorProducer{slot: 1, result: "LStoreMode;", storage: s}
			use := &nativeEnumSwitchUse{field: "field", rendered: true, keys: map[int]bool{1: true, 2: true}, marker: switchMarker, selector: selector}
			table := &nativeEnumSwitchTable{tables: map[string]*nativeEnumSwitchArray{"field": {enum: "StoreMode", entries: map[int]string{1: "A", 2: "B"}}}, initializationOrder: []string{"field"}, uses: map[string]map[string]map[int]*nativeEnumSwitchUse{"Owner": {"run(LStoreMode;)V": {7: use}}}}
			p := &nativeMemberFamily{enumSwitchTables: map[string]*nativeEnumSwitchTable{"helper": table}}
			source := "x=StoreMode.B" + marker + ";switch(x" + switchMarker + "){case A:break;case B:break;}"
			var work *workbudget.Budget
			switch variant {
			case "quoted data harmless":
				source = `String data="` + marker + `";` + source
			case "quoted substitution":
				source = `String data="` + marker + `";` + strings.Replace(source, marker, "", 1)
			case "missing writer":
				source = strings.Replace(source, marker, "", 1)
			case "duplicate writer":
				source += marker
			case "unknown writer":
				source += strings.Replace(marker, "test", "other", 1)
			case "other binder":
				other := *s
				other.bound = values.NewJavaRef(ref.Id, nil, typ)
				selector.operands = []*nativeEnumSelectorProducer{{storage: &other}}
			case "unvalidated storage":
				s.validated = false
			case "unterminated string":
				source += "\""
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				_ = work.CheckAlloc(2)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "quoted data harmless"
			if got := nativeEnumSwitchSourceComplete(p, source, work); got != want {
				t.Fatalf("final retained stores=%v want %v", got, want)
			}
		})
	}
}
