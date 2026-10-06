package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumSwitchComputedProducerRequiresOriginalCallsAndOrderedOperands(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ProducerProbe.java": `public class ProducerProbe {public static int pick(ProducerMode first,ProducerMode second,long padding){switch(Selector.pick(first,second,padding)){case FIRST:return 1;case SECOND:return 2;default:return 3;}}} enum ProducerMode{FIRST,SECOND,THIRD} class Selector{static ProducerMode pick(ProducerMode first,ProducerMode second,long padding){return padding==42?first:second;}}`}, "none", "8")
	object, e := Parse(files["ProducerProbe.class"])
	if e != nil {
		t.Fatal(e)
	}
	var original *nativeEnumSelectorProducer
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name != "pick" {
			continue
		}
		for _, a := range method.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			dec := core.NewDecompiler(code.Code, nil)
			if dec.ParseOpcode() != nil {
				t.Fatal("original decoding")
			}
			ops := constructorMotionOps(dec)
			for i, op := range ops {
				member := constructorMotionMember(object, op, core.OP_GETSTATIC)
				if member == nil || member.Description != "[I" {
					continue
				}
				var ok bool
				original, _, ok = nativeEnumSelectorPacket(object, ops, i+1, map[int]string{0: "LProducerMode;", 1: "LProducerMode;", 2: "J"}, "ProducerMode", nil)
				if !ok {
					t.Fatal("original ordered packet")
				}
			}
		}
	}
	if original == nil || len(original.operands) != 3 {
		t.Fatal("original call with three ordered operands")
	}
	for _, variant := range []string{"original", "renamed source variables", "wrapped slots", "identity binding cast", "missing call origin", "wrong pc", "wrong owner", "wrong method", "wrong descriptor", "wrong kind", "wrong static flag", "special invocation", "swapped equal-typed arguments", "missing argument", "extra argument", "unwitnessed parameter", "changed parameter seed", "wrong parameter slot", "static parameter relabeled this", "runtime cast", "conversion cast", "wrong result type", "evaluated static receiver", "wrong symbolic owner", "custom symbolic owner", "work", "memory", "canceled", "depth"} {
		t.Run(variant, func(t *testing.T) {
			enumType := types.NewJavaClass("ProducerMode")
			longType := types.NewJavaPrimer(types.JavaLong)
			refs := []*values.JavaRef{}
			for i, typ := range []types.JavaType{enumType, enumType, longType} {
				seed := values.NewJavaLiteral(i, typ)
				ref := values.NewJavaRef(coreutils.NewRootVariableId(), seed, typ)
				ref.IsParam = true
				if variant != "unwitnessed parameter" || i != 0 {
					ref.MarkOriginalParameter(i)
				}
				refs = append(refs, ref)
			}
			call := &values.FunctionCallExpression{IsStatic: true, Kind: values.InvokeStatic, Object: values.NewJavaClassValue(types.NewJavaClass("Selector")), ClassName: "Selector", FunctionName: "pick", Descriptor: "(LProducerMode;LProducerMode;J)LProducerMode;", HasOriginPC: true, OriginPC: original.pc, Arguments: []values.JavaValue{refs[0], refs[1], refs[2]}, FuncType: &types.JavaFuncType{ReturnType: enumType}}
			var source values.JavaValue = call
			var work *workbudget.Budget
			depth := 0
			switch variant {
			case "renamed source variables":
				refs[0].Id = coreutils.NewRootVariableId()
			case "wrapped slots":
				call.Arguments[0] = values.NewSlotValue(refs[0], enumType)
			case "identity binding cast":
				call.Arguments[0] = &values.CastExpression{Binding: true, TargetType: enumType, Value: refs[0]}
			case "missing call origin":
				call.HasOriginPC = false
			case "wrong pc":
				call.OriginPC++
			case "wrong owner":
				call.ClassName = "Other"
			case "wrong method":
				call.FunctionName = "other"
			case "wrong descriptor":
				call.Descriptor = "(LProducerMode;)LProducerMode;"
			case "wrong kind":
				call.Kind = values.InvokeVirtual
			case "wrong static flag":
				call.IsStatic = false
			case "special invocation":
				call.IsSpecialInvoke = true
			case "swapped equal-typed arguments":
				call.Arguments[0], call.Arguments[1] = call.Arguments[1], call.Arguments[0]
			case "missing argument":
				call.Arguments = call.Arguments[:2]
			case "extra argument":
				call.Arguments = append(call.Arguments, refs[0])
			case "changed parameter seed":
				refs[0].Val = values.NewJavaLiteral(0, enumType)
			case "wrong parameter slot":
				call.Arguments[0] = refs[1]
			case "static parameter relabeled this":
				refs[0].IsThis = true
			case "runtime cast":
				source = &values.CastExpression{TargetType: enumType, Value: call}
			case "conversion cast":
				source = &values.CastExpression{Binding: true, TargetType: types.NewJavaClass("Other"), Value: call}
			case "wrong result type":
				call.FuncType.ReturnType = types.NewJavaClass("Other")
			case "evaluated static receiver":
				call.Object.(*values.JavaClassValue).HasOriginPC = true
			case "wrong symbolic owner":
				call.Object = values.NewJavaClassValue(types.NewJavaClass("Other"))
			case "custom symbolic owner":
				call.Object = values.NewCustomValue(func(*class_context.ClassContext) string { return "Selector" }, func() types.JavaType { return types.NewJavaClass("Selector") })
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
				depth = 32
			}
			ctx := &class_context.ClassContext{ClassName: "ProducerProbe", IsStatic: true}
			got := nativeEnumSelectorSource(original, source, ctx, work, depth)
			want := variant == "original" || variant == "renamed source variables" || variant == "wrapped slots" || variant == "identity binding cast"
			if got != want {
				t.Fatalf("producer admission=%v want %v", got, want)
			}
		})
	}
}

func TestNativeEnumSwitchProducerRejectsReassignedParameterSeeds(t *testing.T) {
	for _, selector := range []string{"m", "SelectorEffects.choose(m)"} {
		sources := nativeEnumSwitchFamilySources("ReassignedSwitchOwner")
		source := sources["ReassignedSwitchOwner.java"]
		source = strings.Replace(source, "switch(m)", "m=ReassignedSwitchOwnerMode.C;switch("+selector+")", 1)
		source += `class SelectorEffects{static ReassignedSwitchOwnerMode choose(ReassignedSwitchOwnerMode value){return value;}}`
		sources["ReassignedSwitchOwner.java"] = source
		files := nativeCompileSourceReleaseClasses(t, sources, "none", "8")
		z := nativeArchive(t, files)
		root, e := Parse(files["ReassignedSwitchOwner.class"])
		if e != nil {
			t.Fatal(e)
		}
		d := z.nativeMemberReader(root)
		p := d.planNativeMemberFamily()
		if p == nil || !d.planNativeMemberAnonymousScopes(p) {
			t.Fatal("original ownership")
		}
		if z.nativeEnumSwitchUsersClosed(p, root, z.originalMemberIndex(), nil) {
			t.Fatal("reassigned parameter borrowed descriptor seed")
		}
		z.Close()
	}
}

func TestNativeEnumSwitchParameterStabilityUsesWrittenWordIntervals(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		want bool
	}{
		{"read only", []byte{core.OP_LLOAD_2, core.OP_POP2, core.OP_RETURN}, true},
		{"disjoint compact store", []byte{core.OP_ISTORE_0, core.OP_RETURN}, true},
		{"compact overlapping store", []byte{core.OP_ASTORE_3, core.OP_RETURN}, false},
		{"encoded overlapping store", []byte{core.OP_ISTORE, 3, core.OP_RETURN}, false},
		{"wide overlapping store", []byte{core.OP_WIDE, core.OP_ASTORE, 0, 3, core.OP_RETURN}, false},
		{"two-word store overlaps from below", []byte{core.OP_DSTORE_1, core.OP_RETURN}, false},
		{"two-word store overlaps from above", []byte{core.OP_LSTORE_3, core.OP_RETURN}, false},
		{"increment overlaps", []byte{core.OP_IINC, 3, 1, core.OP_RETURN}, false},
		{"wide increment overlaps", []byte{core.OP_WIDE, core.OP_IINC, 0, 3, 0, 1, core.OP_RETURN}, false},
		{"wide disjoint store", []byte{core.OP_WIDE, core.OP_ISTORE, 1, 0, core.OP_RETURN}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Decode instruction structure only. Deliberately abstract stack states let
			// this oracle isolate local-word overlap from unrelated verifier rules.
			dec := core.NewDecompiler(tc.code, nil)
			if e := dec.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			original := &nativeEnumSelectorProducer{slot: 2, result: "J"}
			slots := map[int]bool{}
			if !nativeEnumSelectorSlots(original, slots, 0) || !slots[2] || !slots[3] {
				t.Fatal("two-word parameter footprint")
			}
			if got := nativeEnumSelectorParametersUnchanged(constructorMotionOps(dec), slots); got != tc.want {
				t.Fatalf("stable=%v want %v", got, tc.want)
			}
		})
	}
}
