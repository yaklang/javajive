package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestEffectfulStackPhiRequiresClosedForwardRegion(t *testing.T) {
	for _, kind := range []string{"valid", "field store", "static store", "pure pop", "outside entry", "back edge", "handler", "protected effect", "extra stack word", "conditional predecessor", "missing simulation"} {
		t.Run(kind, func(t *testing.T) {
			merge := &OpCode{CurrentOffset: 40, Instr: &Instruction{OpCode: OP_ARETURN}}
			root := &OpCode{CurrentOffset: 1, Instr: &Instruction{OpCode: OP_IFEQ}}
			pop := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_POP}, Source: []*OpCode{root}}
			left := &OpCode{CurrentOffset: 20, Instr: &Instruction{OpCode: OP_GOTO}, Source: []*OpCode{pop}, Target: []*OpCode{merge}}
			right := &OpCode{CurrentOffset: 30, Instr: &Instruction{OpCode: OP_INVOKESTATIC}, Source: []*OpCode{root}, Target: []*OpCode{merge}}
			root.Target = []*OpCode{pop, right}
			pop.Target = []*OpCode{left}
			merge.Source = []*OpCode{left, right}
			typ := types.NewJavaClass("java.lang.String")
			a := values.NewJavaLiteral("a", types.NewJavaPrimer(types.JavaString))
			b := values.NewFunctionCallExpression(nil, &values.JavaClassMember{Name: "p.Factory", Member: "text", Description: "()Ljava/lang/String;"}, types.NewJavaFuncType("", nil, typ))
			pop.stackConsumed = []values.JavaValue{b}
			left.StackEntry = newStackItem(NewEmptyStackEntry(), a)
			right.StackEntry = newStackItem(NewEmptyStackEntry(), b)
			slot := values.NewSlotValue(a, a.Type())
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{merge: NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())}}
			accepted := kind == "valid" || kind == "field store" || kind == "static store"
			switch kind {
			case "field store":
				pop.Instr.OpCode = OP_PUTFIELD
			case "static store":
				pop.Instr.OpCode = OP_PUTSTATIC
			case "pure pop":
				pop.stackConsumed = []values.JavaValue{a}
			case "outside entry":
				pop.Source = append(pop.Source, &OpCode{CurrentOffset: 2})
			case "back edge":
				pop.Target = []*OpCode{root}
			case "handler":
				pop.IsCatch = true
			case "protected effect":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 20}}
			case "extra stack word":
				right.StackEntry = newStackItem(right.StackEntry, b)
			case "conditional predecessor":
				left.Target = append(left.Target, right)
			case "missing simulation":
				d.opcodeToSimulateStack = nil
			}
			if got := d.lowerEffectfulStackPhi(merge, []*OpCode{root}, slot); got != accepted {
				t.Fatalf("lowered=%v want %v", got, accepted)
			}
			if !accepted {
				if slot.GetValue() != a || len(d.effectfulStackPhiEdges) != 0 || len(d.disFoldRef) != 0 {
					t.Fatal("rejected proof changed IR")
				}
				return
			}
			ref, ok := slot.GetValue().(*values.JavaRef)
			if !ok || ref.WebDeclType == nil || len(d.disFoldRef) != 1 || d.disFoldRef[0] != ref {
				t.Fatal("unsolved or foldable phi")
			}
			if d.effectfulStackPhiEdges[left].LeftValue != ref || d.effectfulStackPhiEdges[right].LeftValue != ref || d.effectfulStackPhiEdges[left].JavaValue != a || d.effectfulStackPhiEdges[right].JavaValue != b {
				t.Fatal("incoming edge values changed")
			}
			if len(pop.stackConsumed) != 1 || pop.stackConsumed[0] != b {
				t.Fatal("discarded invocation lost")
			}
		})
	}
}

func TestDeclinedExpressionPhiRetainsClosedRegionProof(t *testing.T) {
	for _, kind := range []string{"closed", "outside entry", "protected", "unequal depth", "nonprivate edge"} {
		t.Run(kind, func(t *testing.T) {
			root := &OpCode{CurrentOffset: 1, Instr: &Instruction{OpCode: OP_IFEQ}}
			left := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_GOTO}, Source: []*OpCode{root}}
			right := &OpCode{CurrentOffset: 20, Instr: &Instruction{OpCode: OP_CHECKCAST}, Source: []*OpCode{root}}
			merge := &OpCode{CurrentOffset: 30, Instr: &Instruction{OpCode: OP_ASTORE}, Source: []*OpCode{left, right}}
			root.Target = []*OpCode{left, right}
			left.Target = []*OpCode{merge}
			right.Target = []*OpCode{merge}
			typ := types.NewJavaClass("java.lang.String")
			a := values.NewJavaLiteral(nil, typ)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			right.stackProduced = []values.JavaValue{ref}
			left.StackEntry = newStackItem(NewEmptyStackEntry(), a)
			right.StackEntry = newStackItem(NewEmptyStackEntry(), ref)
			slot := values.NewSlotValue(a, typ)
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{merge: NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())}}
			switch kind {
			case "outside entry":
				right.Source = append(right.Source, &OpCode{CurrentOffset: 2})
			case "protected":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 25}}
			case "unequal depth":
				right.StackEntry = newStackItem(right.StackEntry, a)
			case "nonprivate edge":
				right.Target = append(right.Target, left)
			}
			if got := d.lowerClosedStackPhi(merge, []*OpCode{root}, slot, false); got != (kind == "closed") {
				t.Fatalf("lowered=%v", got)
			}
			if kind != "closed" {
				if slot.GetValue() != a || len(d.effectfulStackPhiEdges) != 0 || len(d.disFoldRef) != 0 {
					t.Fatal("rejected expression proof changed IR")
				}
				return
			}
			found := false
			for _, pinned := range d.disFoldRef {
				found = found || pinned == ref
			}
			if !found || d.effectfulStackPhiEdges[right].JavaValue != ref {
				t.Fatal("cast producer lost its edge use")
			}
		})
	}
}

func TestClosedBooleanStackPhiCopiesOnlyProvenIntConstants(t *testing.T) {
	for _, kind := range []string{"zero", "one", "Boolean-typed JVM one", "nonboolean constant", "arbitrary int local", "long", "float", "null", "class", "all float", "all long", "all class", "missing target type"} {
		t.Run(kind, func(t *testing.T) {
			root := &OpCode{CurrentOffset: 1, Instr: &Instruction{OpCode: OP_IFEQ}}
			left := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_GOTO}, Source: []*OpCode{root}}
			right := &OpCode{CurrentOffset: 20, Instr: &Instruction{OpCode: OP_ICONST_0}, Source: []*OpCode{root}}
			merge := &OpCode{CurrentOffset: 30, Instr: &Instruction{OpCode: OP_IRETURN}, Source: []*OpCode{left, right}}
			root.Target = []*OpCode{left, right}
			left.Target = []*OpCode{merge}
			right.Target = []*OpCode{merge}
			boolean := types.NewJavaPrimer(types.JavaBoolean)
			integer := types.NewJavaPrimer(types.JavaInteger)
			yes := values.NewJavaLiteral(true, boolean)
			literal := values.NewJavaLiteral(0, integer)
			var other values.JavaValue = literal
			var leftValue values.JavaValue = yes
			switch kind {
			case "one":
				literal.Data = 1
			case "Boolean-typed JVM one":
				literal.Data = 1
				literal.JavaType = boolean
			case "nonboolean constant":
				literal.Data = 2
			case "arbitrary int local":
				other = values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			case "long":
				other = values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaLong))
			case "float":
				other = values.NewJavaLiteral(float32(0), types.NewJavaPrimer(types.JavaFloat))
			case "null":
				other = values.JavaNull
			case "class":
				other = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
			case "all float":
				other = values.NewJavaLiteral(float32(0), types.NewJavaPrimer(types.JavaFloat))
				leftValue = other
			case "all long":
				other = values.NewJavaLiteral(int64(0), types.NewJavaPrimer(types.JavaLong))
				leftValue = other
			case "all class":
				other = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
				leftValue = other
			}
			left.StackEntry = newStackItem(NewEmptyStackEntry(), leftValue)
			right.StackEntry = newStackItem(NewEmptyStackEntry(), other)
			slot := values.NewSlotValue(yes, boolean)
			if kind == "missing target type" {
				slot = values.NewSlotValue(nil, nil)
			}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{merge: NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())}}
			originalSlot := slot.GetValue()
			accepted := kind == "zero" || kind == "one" || kind == "Boolean-typed JVM one"
			if got := d.lowerClosedStackPhi(merge, []*OpCode{root}, slot, false); got != accepted {
				t.Fatalf("lowered=%v want=%v", got, accepted)
			}
			if !accepted {
				if slot.GetValue() != originalSlot || left.StackEntry.value != leftValue || right.StackEntry.value != other || len(d.effectfulStackPhiEdges) != 0 || len(d.disFoldRef) != 0 {
					t.Fatal("rejected primitive proof published IR")
				}
				return
			}
			originalType := types.JavaInteger
			if kind == "Boolean-typed JVM one" {
				originalType = types.JavaBoolean
			}
			rhs, ok := d.effectfulStackPhiEdges[right].JavaValue.(*values.JavaLiteral)
			if !ok || rhs == literal || rhs.Data != (kind == "one" || kind == "Boolean-typed JVM one") || rhs.Type().RawType().(*types.JavaPrimer).Name != types.JavaBoolean || literal.Type().RawType().(*types.JavaPrimer).Name != originalType || right.StackEntry.value != literal {
				t.Fatal("normalization must copy inert constants without mutating original stack evidence")
			}
			for _, pred := range []*OpCode{left, right} {
				assign := d.effectfulStackPhiEdges[pred]
				if !assign.HasOriginPC || assign.OriginPC != int(pred.CurrentOffset) {
					t.Fatal("materialized assignment lost original routing edge placement")
				}
			}
		})
	}
}

func TestClosedStackPhiPrefixUsesValueIdentityAndCertifiedCopies(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaDouble)
	original := values.NewSlotValue(values.NewJavaRef(utils.NewRootVariableId(), nil, typ), typ)
	saved := values.NewJavaRef(utils.NewRootVariableId(), original, typ)
	other := values.NewSlotValue(values.NewJavaRef(utils.NewRootVariableId(), nil, typ), typ)
	choices := []values.JavaValue{original, saved, other}
	d := &Decompiler{stackLifetimeCopies: map[values.JavaValue]*values.JavaRef{original: saved}}
	cases := 0
	for depth := 0; depth <= 4; depth++ {
		limit := 1
		for i := 0; i < depth; i++ {
			limit *= 3
		}
		for left := 0; left < limit; left++ {
			for right := 0; right < limit; right++ {
				a, b := NewEmptyStackEntry(), NewEmptyStackEntry()
				x, y := left, right
				want := true
				for i := 0; i < depth; i++ {
					u, v := x%3, y%3
					x /= 3
					y /= 3
					want = want && (u == v || (u < 2 && v < 2))
					a = newStackItem(a, choices[u])
					b = newStackItem(b, choices[v])
				}
				if got := d.sameStackLifetimePrefix(a, b); got != want {
					t.Fatalf("depth%d symbols%d,%d got%v want%v", depth, left, right, got, want)
				}
				cases++
			}
		}
	}
	for depth := 511; depth <= 513; depth++ {
		a := NewEmptyStackEntry()
		for i := 0; i < depth; i++ {
			a = newStackItem(a, original)
		}
		if d.sameStackLifetimePrefix(a, a) != (depth <= 512) {
			t.Fatal("prefix work bound", depth)
		}
	}
	t.Logf("independent symbolic prefix states=%d", cases)
}
