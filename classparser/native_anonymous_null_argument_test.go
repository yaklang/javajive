package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousNullOperandRequiresOriginalProducer(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"NullOperand.java": `class NullOperand {static Object get(){return null;}static Object cast(){return (String)null;}}`}, "none", "8")
	for _, variant := range []string{"original", "wrong opcode", "payload", "uses", "missing record", "missing IR", "wrong PC", "wrong origin", "origin slot", "origin auxiliary", "copy", "synthetic", "changed data", "changed type", "UTF16 data", "nil node", "opaque node", "duplicate producer", "node limit", "depth", "work", "canceled", "literal CHECKCAST", "wrong cast target", "wrong cast witness", "wrong cast CP kind"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["NullOperand.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			name := "get"
			if variant == "literal CHECKCAST" || variant == "wrong cast target" || variant == "wrong cast witness" || variant == "wrong cast CP kind" {
				name = "cast"
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == name {
					method = m
					for _, a := range m.Attributes {
						if ca, ok := a.(*CodeAttribute); ok {
							code = ca
						}
					}
				}
			}
			ir, fn, ok := d.nativeOriginalMethodSnapshot(method, code)
			if !ok {
				t.Fatal("original method snapshot")
			}
			records, ok := nativeAnonymousArgumentRecords(fn, nil)
			if !ok {
				t.Fatal("original operand records")
			}
			literal := values.NewOriginalNullLiteral(0)
			var operand values.JavaValue = literal
			origin := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: 0}
			depth := 0
			p := &nativeAnonymousArgumentTree{obj: obj, ir: ir, records: records, ctx: &class_context.ClassContext{}, seen: map[int]bool{}}
			switch variant {
			case "wrong opcode":
				p.ir.Instrs[0].Opcode = core.OP_ICONST_0
			case "payload":
				p.ir.Instrs[0].Data = []byte{0}
			case "uses":
				p.records[0].Uses = []ssabuild.Origin{{Kind: ssabuild.OriginParam, Slot: 1}}
			case "missing record":
				delete(p.records, 0)
			case "missing IR":
				p.ir = nil
			case "wrong PC":
				operand = values.NewOriginalNullLiteral(1)
			case "wrong origin":
				origin.Kind = ssabuild.OriginPhi
			case "origin slot":
				origin.Slot = 1
			case "origin auxiliary":
				origin.Aux = 1
			case "copy":
				copied := *literal
				operand = &copied
			case "synthetic":
				operand = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "changed data":
				literal.Data = "other"
			case "changed type":
				literal.JavaType = types.NewJavaClass("java.lang.String")
			case "UTF16 data":
				literal.Units = []uint16{'n', 'u', 'l', 'l'}
			case "nil node":
				operand = (*values.JavaLiteral)(nil)
			case "opaque node":
				operand = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			case "duplicate producer":
				p.seen[0] = true
			case "node limit":
				for i := 1; i <= 128; i++ {
					p.seen[i] = true
				}
			case "depth":
				depth = 64
			case "work":
				p.work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				p.work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				p.work = workbudget.New(ctx, workbudget.Limits{})
			case "literal CHECKCAST", "wrong cast target", "wrong cast witness", "wrong cast CP kind":
				// The independently compiled null cast has a physical CHECKCAST. Keep
				// its target and evaluation position; source admission cannot drop it.
				var cast methodir.Instr
				for _, ins := range ir.Instrs {
					if ins.Opcode == core.OP_CHECKCAST {
						cast = ins
					}
				}
				if cast.CPIndex == 0 {
					t.Fatal("fixture CHECKCAST")
				}
				origin.PC = cast.PC
				converted := values.NewOriginalCheckCast(literal, types.NewJavaClass("java.lang.String"), int(cast.PC))
				operand = converted
				if variant == "wrong cast target" {
					converted.TargetType = types.NewJavaClass("java.lang.Object")
				}
				if variant == "wrong cast witness" {
					converted.OriginalCheckCast = false
				}
				if variant == "wrong cast CP kind" {
					obj.ConstantPool[cast.CPIndex-1] = &ConstantIntegerInfo{Value: 1}
				}
			}
			got := p.operand(operand, origin, depth)
			if got != (variant == "original" || variant == "literal CHECKCAST") {
				t.Fatalf("original null producer=%v", got)
			}
			if variant == "literal CHECKCAST" && got && (len(p.order) != 2 || p.order[0] != 0 || p.order[1] != int(origin.PC)) {
				t.Fatal("lost original null cast order")
			}
			if variant == "original" && got && (len(p.order) != 1 || p.order[0] != 0 || !p.seen[0]) {
				t.Fatal("null omitted from original instruction order")
			}
		})
	}
}
