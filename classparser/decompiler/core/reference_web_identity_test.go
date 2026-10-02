package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/omap"
	"testing"
)

func TestReferenceWebPartitionRebindsOnlyCompleteOwnedLoads(t *testing.T) {
	for _, kind := range []string{"conflict", "no conflict", "entry", "missing store", "missing value", "parameter", "consistent identity"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.String")
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			old.Id.SetName("var3")
			null := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("java.lang.Object"))
			null.Id.SetName("var8")
			a, b, c := op(OP_ASTORE_1, 1), op(OP_ASTORE_1, 2), op(OP_ASTORE_1, 3)
			load1, load2 := op(OP_ALOAD_1, 4), op(OP_ALOAD_1, 5)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral("a", typ)}
			b.stackConsumed = []values.JavaValue{values.NewJavaLiteral("b", typ)}
			c.stackConsumed = []values.JavaValue{values.JavaNull}
			first, second := values.NewSlotValue(old, typ), values.NewSlotValue(null, typ)
			load1.stackProduced = []values.JavaValue{first}
			load2.stackProduced = []values.JavaValue{second}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 2, c: 2, load1: 1, load2: 2}, entryWeb: map[int]int{}}
			d := &Decompiler{cachedSlotWebs: webs, opCodes: []*OpCode{a, b, c, load1, load2}, opcodeIdToRef: map[*OpCode][][2]any{a: {{old, true}}, b: {{old, false}}, c: {{null, true}}}}
			switch kind {
			case "no conflict":
				webs.webOf[a] = 2
				webs.webOf[load1] = 2
			case "entry":
				webs.entryWeb[1] = 2
			case "missing store":
				webs.webOf[op(OP_ASTORE_1, 6)] = 2
			case "missing value":
				c.stackConsumed = nil
			case "consistent identity":
				d.opcodeIdToRef[c][0][0] = old
			case "parameter":
				old.IsParam = true
			}
			d.varUserMap = omap.NewEmptyOrderedMap[*values.JavaRef, []*VarFoldRule]()
			own, foreign := values.JavaValue(null), values.JavaValue(old)
			ownPair := &VarFoldRule{CurrentOpcode: load2, Replace: func(v values.JavaValue) { own = v }}
			foreignPair := &VarFoldRule{CurrentOpcode: load1, Replace: func(v values.JavaValue) { foreign = v }}
			opaquePair := &VarFoldRule{}
			d.varUserMap.Set(old, []*VarFoldRule{foreignPair, opaquePair})
			d.varUserMap.Set(null, []*VarFoldRule{ownPair})
			d.partitionSharedReferenceWebs()
			joined := d.opcodeIdToRef[b][0][0].(*values.JavaRef)
			want := kind == "conflict"
			if (joined != old) != want {
				t.Fatalf("repartitioned=%v want=%v", joined != old, want)
			}
			if want {
				pairs, _ := d.varUserMap.Get(joined)
				preserved, _ := d.varUserMap.Get(old)
				if own != joined || foreign != old || len(pairs) != 1 || pairs[0] != ownPair || len(preserved) != 2 || preserved[0] != foreignPair || preserved[1] != opaquePair || len(d.disFoldRef) != 1 || d.disFoldRef[0] != joined {
					t.Fatal("lost owned callbacks, foreign folding opportunity or opaque rule")
				}
			}
			if want && (joined != d.opcodeIdToRef[c][0][0] || second.GetValue() != joined || first.GetValue() == joined || old.Id == joined.Id || null.Id == joined.Id || joined.SolvedWebIdentity == nil) {
				t.Fatal("partition changed a foreign use/alias or lost a joined load")
			}
		})
	}
}

func TestSingletonReferenceWebUsesItsOwnDefinitionType(t *testing.T) {
	for _, scenario := range []string{"narrow stale owner", "same type", "null", "missing value", "entry", "unshared", "class literal"} {
		t.Run(scenario, func(t *testing.T) {
			cls := types.NewJavaClass("java.lang.Class")
			obj := types.NewJavaClass("java.lang.Object")
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, cls)
			a, b, load := op(OP_ASTORE_1, 1), op(OP_ASTORE_1, 2), op(OP_ALOAD_1, 3)
			a.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, cls)}
			b.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, obj)}
			slot := values.NewSlotValue(old, cls)
			load.stackProduced = []values.JavaValue{slot}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 2, load: 2}, entryWeb: map[int]int{}}
			d := &Decompiler{cachedSlotWebs: webs, opCodes: []*OpCode{a, b, load}, opcodeIdToRef: map[*OpCode][][2]any{a: {{old, true}}, b: {{old, false}}}}
			switch scenario {
			case "same type":
				b.stackConsumed = a.stackConsumed
			case "class literal":
				b.stackConsumed = []values.JavaValue{values.NewJavaClassValue(types.NewJavaClass("java.util.ArrayList"))}
			case "null":
				b.stackConsumed = []values.JavaValue{values.JavaNull}
			case "missing value":
				b.stackConsumed = nil
			case "entry":
				webs.entryWeb[1] = 2
			case "unshared":
				d.opcodeIdToRef[a][0][0] = values.NewJavaRef(utils.NewRootVariableId(), nil, cls)
			}
			d.partitionSharedReferenceWebs()
			got := d.opcodeIdToRef[b][0][0].(*values.JavaRef)
			if (got != old) != (scenario == "narrow stale owner") {
				t.Fatalf("partitioned=%v", got != old)
			}
			if scenario == "narrow stale owner" && (got.Type().String(&class_context.ClassContext{}) != "Object" || slot.GetValue() != got || d.opcodeIdToRef[a][0][0] != old || got.Val != nil) {
				t.Fatal("lost definition/web ownership")
			}
		})
	}
}
