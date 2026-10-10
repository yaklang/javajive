package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeEnumSwitchSourceRequiresOriginalOperandAndCompleteCases(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, nativeEnumSwitchFamilySources("SwitchFamilyOwner"), "none", "8")
	for _, variant := range []string{"original", "field origin", "field witness", "field descriptor", "array origin", "ordinal origin", "ordinal kind", "ordinal descriptor", "ordinal owner", "static call", "special call", "extra argument", "missing parameter witness", "different same-typed parameter", "parameter seed changed", "this", "custom receiver", "stack receiver", "source erasure", "missing key", "unknown key", "duplicate key", "method identity", "unregistered use", "memory", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["SwitchFamilyOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || !z.nativeEnumSwitchUsersClosed(p, root, z.originalMemberIndex(), nil) {
				t.Fatal("original admission")
			}
			table := p.enumSwitchTables["SwitchFamilyOwner$2"]
			use := table.uses["SwitchFamilyOwner$1"]["run(LSwitchFamilyOwnerMode;)I"][7]
			if use == nil {
				t.Fatal("original use")
			}
			originalSeed := values.NewJavaLiteral("seed", types.NewJavaClass("SwitchFamilyOwnerMode"))
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), originalSeed, types.NewJavaClass("SwitchFamilyOwnerMode"))
			ref.IsParam = true
			if variant != "missing parameter witness" {
				slot := use.parameterSlot
				if variant == "different same-typed parameter" {
					slot++
				}
				ref.MarkOriginalParameter(slot)
			}
			field := values.NewJavaClassMember("SwitchFamilyOwner$2", use.field, "[I", types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)))
			field.OriginPC, field.HasOriginPC = use.getPC, true
			if variant != "field witness" {
				field.MarkOriginalFieldRead(field, use.getPC)
			}
			call := &values.FunctionCallExpression{Object: ref, FunctionName: "ordinal", ClassName: "SwitchFamilyOwnerMode", Descriptor: "()I", Kind: values.InvokeVirtual, OriginPC: use.ordinalPC, HasOriginPC: true}
			array := &values.JavaArrayMember{Object: field, Index: call, OriginPC: use.arrayPC, HasOriginPC: true}
			ctx := &class_context.ClassContext{FunctionName: "run", CurrentMethodDesc: "(LSwitchFamilyOwnerMode;)I"}
			obj, _ := Parse(files["SwitchFamilyOwner$1.class"])
			reader := z.nativeMemberReader(obj)
			labels := []int{1, 2}
			switch variant {
			case "field origin":
				field.OriginPC++
			case "field descriptor":
				field.Description = "[J"
			case "array origin":
				array.HasOriginPC = false
			case "ordinal origin":
				call.OriginPC++
			case "ordinal kind":
				call.Kind = values.InvokeInterface
			case "ordinal descriptor":
				call.Descriptor = "(I)I"
			case "ordinal owner":
				call.ClassName = "java.lang.Enum"
			case "static call":
				call.IsStatic = true
			case "special call":
				call.IsSpecialInvoke = true
			case "extra argument":
				call.Arguments = []values.JavaValue{values.JavaNull}
			case "parameter seed changed":
				ref.Val = values.NewJavaLiteral("seed", types.NewJavaClass("SwitchFamilyOwnerMode"))
			case "this":
				ref.IsThis = true
			case "custom receiver":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { return "other" }, func() types.JavaType { return ref.Type() })
			case "stack receiver":
				ref.StackVar = values.JavaNull
			case "source erasure":
				ref.ResetVarType(types.NewJavaClass("java.lang.Object"))
			case "missing key":
				labels = []int{1}
			case "unknown key":
				labels = []int{1, 3}
			case "duplicate key":
				labels = []int{1, 1}
			case "method identity":
				ctx.FunctionName = "other"
			case "unregistered use":
				delete(table.uses[obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc], use.arrayPC)
			case "memory":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				_ = reader.Work.CheckAlloc(2)
			case "work":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(c, workbudget.Limits{})
			}
			reader.wireNativeEnumSwitchSource(p, ctx)
			selector, mapping, ok := ctx.SourceEnumSwitch(array, labels)
			if ok != (variant == "original") {
				t.Fatalf("source admission=%v selector=%q mapping=%v", ok, selector, mapping)
			}
			if ok && (!strings.Contains(selector, use.marker) || mapping[1] != "A" || mapping[2] != "B") {
				t.Fatal("original selector/labels were not preserved")
			}
		})
	}
}

func TestNativeEnumSwitchSourceRegistrationCannotBorrowQuotedOrDuplicateProofs(t *testing.T) {
	arr := &nativeEnumSwitchArray{enum: "EnumType", entries: map[int]string{1: "SECOND", 2: "FIRST"}}
	use := &nativeEnumSwitchUse{field: "field", rendered: true, keys: map[int]bool{1: true, 2: true}, marker: "/*jdec-owned-enum-switch:proof*/"}
	table := &nativeEnumSwitchTable{tables: map[string]*nativeEnumSwitchArray{"field": arr}, initializationOrder: []string{"field"}, uses: map[string]map[string]map[int]*nativeEnumSwitchUse{"Owner": {"run()V": {7: use}}}}
	p := &nativeMemberFamily{enumSwitchTables: map[string]*nativeEnumSwitchTable{"helper": table}}
	good := `switch(value/*jdec-owned-enum-switch:proof*/){case SECOND:break;case FIRST:break;default:break;}`
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"original", good, true},
		{"quoted data harmless", `String s="case FIRST: /*jdec-owned-enum-switch:proof*/";` + good, true},
		{"quoted replacement", `String s="/*jdec-owned-enum-switch:proof*/";switch(value){case SECOND:break;case FIRST:break;}`, false},
		{"duplicate actual use", good + good, false},
		{"missing use", strings.ReplaceAll(good, use.marker, ""), false},
		{"changed compiler key registration", strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(good, "SECOND", "TEMP"), "FIRST", "SECOND"), "TEMP", "FIRST"), false},
		{"missing constant", strings.ReplaceAll(good, "case FIRST:break;", ""), false},
		{"unknown proof", strings.ReplaceAll(good, "proof", "other"), false},
		{"comment case data harmless", `/* case FIRST: */` + good, true},
		{"unrelated switch cannot supply registration", `switch(other){case SECOND:break;case FIRST:break;}` + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(good, "SECOND", "TEMP"), "FIRST", "SECOND"), "TEMP", "FIRST"), false},
		{"detached ownership comment", use.marker + strings.ReplaceAll(good, use.marker, ""), false},
		{"nested switch cannot supply missing constant", strings.ReplaceAll(good, "case FIRST:break;", "switch(other){case FIRST:break;}"), false},
		{"unrelated same-name cases ignored", `switch(other){case FIRST:break;case SECOND:break;}` + good, true},
		{"nested same-name cases ignored", strings.ReplaceAll(good, "case SECOND:break;", "case SECOND:switch(other){case FIRST:break;case SECOND:break;}break;"), true},
		{"marker in body cannot bind selector", strings.ReplaceAll(good, use.marker, "")[:strings.Index(strings.ReplaceAll(good, use.marker, ""), "{")+1] + use.marker + strings.ReplaceAll(good, use.marker, "")[strings.Index(strings.ReplaceAll(good, use.marker, ""), "{")+1:], false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeEnumSwitchSourceComplete(p, tc.source, nil); got != tc.want {
				t.Fatalf("admission=%v", got)
			}
		})
	}
}
