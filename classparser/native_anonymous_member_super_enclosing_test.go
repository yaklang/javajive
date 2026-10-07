package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousMemberSuperEnclosingRequiresOriginalParameterPath(t *testing.T) {
	files := nativeCompileClasses(t, anonymousNestedPrivateMemberSuperFixture())
	for _, variant := range []string{"original", "missing forest", "foreign members", "missing unit", "copied unit", "foreign object", "wrong descriptor", "wrong base pc", "wrong read pc", "wrong read field", "wrong parameter role", "missing capture", "wrong capture parameter", "wrong capture pc", "changed capture store", "changed parameter load", "missing named parent", "wrong delegation pc", "wrong delegation descriptor", "control entry", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["AllocationOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original lexical forest")
			}
			f := p.anonymousForest
			unit := f.units["AllocationOwner$1$1"]
			if unit == nil || unit.memberSuper == nil || unit.memberEnclosingPath == nil {
				t.Fatal("original anonymous member SUPER")
			}
			var ops []*core.OpCode
			var entries []int
			for _, m := range unit.object.Methods {
				mn, _ := sourceBridgeUTF8(unit.object, m.NameIndex)
				if mn != "<init>" {
					continue
				}
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						dec := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(unit.object.ConstantPool, i) })
						if e := dec.ParseOpcode(); e != nil {
							t.Fatal(e)
						}
						ops = constructorMotionOps(dec)
						var known bool
						entries, known = nativeMemberLexicalControlEntries(dec, code, nil)
						if !known {
							t.Fatal("control entries")
						}
					}
				}
			}
			if len(ops) == 0 {
				t.Fatal("original constructor instructions")
			}
			desc := unit.descriptor
			var work *workbudget.Budget
			switch variant {
			case "missing forest":
				f = nil
			case "foreign members":
				f.members = &nativeMemberFamily{}
			case "missing unit":
				delete(f.units, unit.object.GetClassName())
			case "copied unit":
				copy := *unit
				unit = &copy
			case "foreign object":
				copy := *unit.object
				unit.object = &copy
			case "wrong descriptor":
				desc = "()V"
			case "wrong base pc":
				unit.memberEnclosingPath.basePC++
			case "wrong read pc":
				unit.memberEnclosingPath.pc++
			case "wrong read field":
				unit.memberEnclosingPath.field = "foreign"
			case "wrong parameter role":
				unit.memberEnclosingPath.parameterOwner = "Foreign"
			case "missing capture":
				delete(unit.fields, unit.enclosingField)
			case "wrong capture parameter":
				unit.fields[unit.enclosingField] = 1
			case "wrong capture pc":
				unit.capturePCs[unit.enclosingField]++
			case "changed capture store":
				copy := *ops[1].Instr
				copy.OpCode = core.OP_ACONST_NULL
				ops[1].Instr = &copy
			case "changed parameter load":
				for _, op := range ops {
					if int(op.CurrentOffset) == unit.memberEnclosingPath.basePC {
						copy := *op.Instr
						copy.OpCode = core.OP_ALOAD_0
						op.Instr = &copy
					}
				}
			case "missing named parent":
				delete(p.children, unit.memberSuper.object.GetClassName())
			case "wrong delegation pc":
				unit.superPC++
			case "wrong delegation descriptor":
				unit.superDescriptor = "()V"
			case "control entry":
				entries = []int{unit.memberEnclosingPath.pc}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, got := nativeAnonymousMemberSuperRead(unit, f, desc, ops, entries, work)
			if got != (variant == "original") {
				t.Fatalf("original parameter/capture path accepted=%v", got)
			}
		})
	}
}
