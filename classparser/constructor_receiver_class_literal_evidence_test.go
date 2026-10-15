package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeConstructorClassLiteralRequiresOriginalResolutionPacket(t *testing.T) {
	files := nativeCompileClasses(t, `class MirrorInput{Class<?> kind;MirrorInput(){kind=MirrorTarget.class;}}class MirrorTarget{}`)
	for _, variant := range []string{"original", "missing symbolic target", "reference array", "primitive array", "bad array", "empty class", "wrong name kind", "nil class constant", "old version", "opcode view", "wide opcode", "changed original byte", "omitted opcode", "bad offset", "wrong load width", "open finalizer", "preinit open finalizer", "short stack", "chain budget", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["MirrorInput.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			var body *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							body = c
						}
					}
				}
			}
			if body == nil {
				t.Fatal("original constructor")
			}
			code := *body
			code.Code = append([]byte(nil), body.Code...)
			dec := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if e = dec.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			ops := constructorMotionOps(dec)
			var op *core.OpCode
			for _, v := range ops {
				if v.Instr.OpCode == core.OP_LDC || v.Instr.OpCode == core.OP_LDC_W {
					op = v
					break
				}
			}
			if op == nil {
				t.Fatal("original literal")
			}
			index := int(op.Data[0])
			if len(op.Data) == 2 {
				index = int(core.Convert2bytesToInt(op.Data))
			}
			cp := obj.ConstantPool[index-1].(*ConstantClassInfo)
			d := NewClassObjectDumper(obj)
			d.FuncCtx = &class_context.ClassContext{}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			d.constructorReceiverFinalizerSilent = true
			remaining := 512
			switch variant {
			case "missing symbolic target":
				cp.NameIndex = sourceBridgePoolString(t, obj, "MissingMirrorTarget")
			case "reference array":
				cp.NameIndex = sourceBridgePoolString(t, obj, "[[LMirrorTarget;")
			case "primitive array":
				cp.NameIndex = sourceBridgePoolString(t, obj, "[[J")
			case "bad array":
				cp.NameIndex = sourceBridgePoolString(t, obj, "[V")
			case "empty class":
				cp.NameIndex = sourceBridgePoolString(t, obj, "")
			case "wrong name kind":
				cp.NameIndex = obj.ThisClass
			case "nil class constant":
				obj.ConstantPool[index-1] = (*ConstantClassInfo)(nil)
			case "old version":
				obj.MajorVersion = 48
			case "opcode view":
				copy := *op.Instr
				copy.OpCode = core.OP_LDC_W
				op.Instr = &copy
			case "wide opcode":
				op.IsWide = true
			case "changed original byte":
				code.Code[int(op.CurrentOffset)] = core.OP_NOP
			case "omitted opcode":
				ops = append(ops[:1], ops[2:]...)
			case "bad offset":
				op.CurrentOffset++
			case "wrong load width":
				copy := *op.Instr
				copy.OpCode = core.OP_LDC2_W
				op.Instr = &copy
			case "open finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "preinit open finalizer":
				d.constructorReceiverFinalizerSilent = false
				var init *core.OpCode
				for _, candidate := range ops {
					if candidate.Instr.OpCode == core.OP_INVOKESPECIAL {
						init = candidate
						break
					}
				}
				if init == nil {
					t.Fatal("original Object initialization")
				}
				code.Code = []byte{byte(core.OP_LDC), byte(index), byte(core.OP_POP), byte(core.OP_ALOAD_0), byte(core.OP_INVOKESPECIAL), init.Data[0], init.Data[1], byte(core.OP_RETURN)}
				decoder := core.NewDecompiler(code.Code, nil)
				if e = decoder.ParseOpcode(); e != nil {
					t.Fatal(e)
				}
				ops = constructorMotionOps(decoder)

			case "short stack":
				code.MaxStack = 0
			case "chain budget":
				remaining = len(ops)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.constructorReceiverEffects(obj, &code, ops, "()V", map[string]bool{}, map[string]bool{}, &remaining, 0)
			want := variant == "preinit open finalizer" || variant == "original" || variant == "missing symbolic target" || strings.HasSuffix(variant, " array") && variant != "bad array"
			if got != want {
				t.Fatalf("symbolic class literal admitted=%v want=%v", got, want)
			}
		})
	}
}
