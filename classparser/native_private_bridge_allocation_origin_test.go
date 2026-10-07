package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativePrivateBridgeNestedAllocationsKeepOriginalUninitializedReceivers(t *testing.T) {
	files := nativeCompileClasses(t, nativeNestedPrivateBridgeFixture)
	for _, scenario := range []string{"original", "missing origin", "wrong owner", "wrong descriptor", "outside code", "interior invoke PC", "multiple origin tables", "nil table", "changed symbolic target", "non-null marker", "graph budget", "memory budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["NestedBridgeRoot.class"]...))
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			obj := p.children["NestedBridgeRoot$Caller"].object
			d := z.nativeMemberReader(obj)
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "make" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original allocating method")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("original instructions")
			}
			ops := constructorMotionOps(decoder)
			origins, known := d.nativeMemberAllocationInvocations(method, code)
			if !known {
				t.Fatalf("two original NEW/constructor receivers: known=%v origins=%v code=%x", known, origins, code.Code)
			}
			var sites []int
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_NEW {
					name, _ := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
					if name != root.GetClassName() {
						continue
					}
					sites = append(sites, i)
				}
			}
			if len(sites) != 2 {
				t.Fatal("two original allocations")
			}
			outer, inner := int(ops[sites[0]].CurrentOffset), int(ops[sites[1]].CurrentOffset)
			if origins[outer].pc <= origins[inner].pc {
				t.Fatal("inside initializes first")
			}
			if _, known := nativeRootBridgeAllocation(obj, ops, sites[0], root, p.rootAccessBridges, nil); known {
				t.Fatal("linear scan cannot certify nested receivers")
			}
			tables := []map[int]nativeMemberAllocationInvocation{origins}
			record := origins[outer]
			switch scenario {
			case "original":
			case "missing origin":
				delete(origins, outer)
			case "wrong owner":
				record.owner = "Foreign"
				origins[outer] = record
			case "wrong descriptor":
				record.descriptor = "()V"
				origins[outer] = record
			case "outside code":
				record.pc = len(code.Code) + 1
				origins[outer] = record
			case "interior invoke PC":
				record.pc++
				origins[outer] = record
			case "multiple origin tables":
				tables = append(tables, origins)
			case "nil table":
				tables[0] = nil
			case "changed symbolic target":
				for _, op := range ops {
					if int(op.CurrentOffset) == record.pc {
						index := core.Convert2bytesToInt(op.Data)
						ref := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
						ref.ClassIndex = obj.ThisClass
					}
				}
			case "non-null marker":
				for i, op := range ops {
					if int(op.CurrentOffset) == record.pc {
						copy := *ops[i-1]
						instruction := *copy.Instr
						instruction.OpCode = core.OP_ALOAD_0
						copy.Instr = &instruction
						ops[i-1] = &copy
					}
				}
			case "graph budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				d.Work.Charge(workbudget.CounterGraphScans, 1)
			case "memory budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if scenario == "memory budget" {
				if _, known := d.nativeMemberAllocationInvocations(method, code); known {
					t.Fatal("exhausted frame allocation obtained origin proof")
				}
				return
			}
			plan, known := nativeRootBridgeAllocation(obj, ops, sites[0], root, p.rootAccessBridges, d.Work, tables...)
			if known != (scenario == "original") {
				t.Fatalf("original nested allocation closure=%v", known)
			}
			if known {
				if plan == nil || plan.rootObject != root || plan.newPC != outer || plan.invokePC != origins[outer].pc {
					t.Fatal("outer receiver became inner")
				}
				second, known := nativeRootBridgeAllocation(obj, ops, sites[1], root, p.rootAccessBridges, nil, origins)
				if !known || second == nil || second.newPC != inner || second.invokePC != origins[inner].pc || second.invokePC == plan.invokePC {
					t.Fatal("inner receiver became outer")
				}
			}
		})
	}
}
