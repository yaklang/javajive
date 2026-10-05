package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorAbruptExitRequiresClosedReceiverEffects(t *testing.T) {
	const source = `final class AbruptSafe {Object capture;AbruptSafe(RuntimeException e,int n){if(n<0)throw e;}}
 final class AbruptIndependentPublication {Object capture;static Object saved,fresh;static long bits;AbruptIndependentPublication(RuntimeException e,int n){saved=e;fresh=new RuntimeException();bits=123456789L;if(n<0)throw e;}}
 final class AbruptFresh {Object capture;AbruptFresh(RuntimeException e,int n){if(n<0)throw new RuntimeException();}}
 class AbruptOpen {Object capture;AbruptOpen(RuntimeException e,int n){if(n<0)throw e;}}
 final class AbruptOwnFinalizer {Object capture;AbruptOwnFinalizer(RuntimeException e,int n){if(n<0)throw e;}protected void finalize(){System.out.println(capture);}}
 class AbruptFinalizerBase {protected void finalize(){System.out.println(this);}}
 final class AbruptInheritedFinalizer extends AbruptFinalizerBase {Object capture;AbruptInheritedFinalizer(RuntimeException e,int n){if(n<0)throw e;}}
 final class AbruptPublish {Object capture;static Object saved;AbruptPublish(RuntimeException e,int n){if(n<0){saved=this;throw e;}}}
 final class AbruptRead {Object capture;static Object seen;AbruptRead(RuntimeException e,int n){if(n<0){seen=capture;throw e;}}}
 final class AbruptOverwrite {Object capture;AbruptOverwrite(RuntimeException e,int n){if(n<0){capture=null;throw e;}}}
 final class AbruptCatch {Object capture;AbruptCatch(RuntimeException e,int n){try{if(n<0)throw e;}catch(RuntimeException x){System.out.println(capture);}}}
 final class AbruptAlias {Object capture,self;static Object saved;AbruptAlias(RuntimeException e,int n){self=this;if(n<0){saved=self;throw e;}}}
`
	files := nativeCompileClasses(t, source)
	resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
	for _, tc := range []struct {
		name string
		safe bool
	}{{"AbruptSafe", true}, {"AbruptIndependentPublication", true}, {"AbruptFresh", true}, {"AbruptOpen", false}, {"AbruptOwnFinalizer", false}, {"AbruptInheritedFinalizer", false}, {"AbruptPublish", false}, {"AbruptRead", false}, {"AbruptOverwrite", false}, {"AbruptCatch", false}, {"AbruptAlias", false}} {
		t.Run(tc.name, func(t *testing.T) {
			obj, err := Parse(files[tc.name+".class"])
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
			d.options.TargetSourceVersion = 8
			writes := map[string]bool{tc.name + "\x00capture\x00Ljava/lang/Object;": true}
			if got := d.constructorCaptureChainDoesNotObserve(tc.name, "(Ljava/lang/RuntimeException;I)V", writes); got != tc.safe {
				t.Fatalf("capture motion=%v want=%v", got, tc.safe)
			}
		})
	}
	obj, err := Parse(files["AbruptSafe.class"])
	if err != nil {
		t.Fatal(err)
	}
	var originalCode *CodeAttribute
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n == "<init>" {
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					originalCode = c
				}
			}
		}
	}
	if originalCode == nil {
		t.Fatal("missing original constructor")
	}
	// Each mutant starts with the physical original code. A mutable opcode view
	// cannot add a throw certificate; invalid operands/aliases cannot close it.
	for _, variant := range []string{"closed", "unclosed finalizer", "receiver operand", "wrong category", "extra stack", "missing operand", "nonoriginal terminal", "wide terminal", "terminal operand", "chain budget"} {
		t.Run(variant, func(t *testing.T) {
			code := *originalCode
			code.Code = append([]byte(nil), originalCode.Code...)
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			throwAt := -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_ATHROW {
					throwAt = i
				}
			}
			if throwAt < 1 || ops[throwAt-1].Instr.OpCode != core.OP_ALOAD_1 {
				t.Fatal("original throw packet")
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve, FuncCtx: &class_context.ClassContext{}, constructorReceiverFinalizerSilent: true}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			remaining := 512
			switch variant {
			case "unclosed finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "receiver operand", "wrong category", "extra stack", "missing operand":
				pc := int(ops[throwAt-1].CurrentOffset)
				prefix := append([]byte(nil), code.Code[:pc]...)
				suffix := append([]byte(nil), code.Code[pc+1:]...)
				if variant == "receiver operand" {
					prefix = append(prefix, core.OP_ALOAD_0)
				}
				if variant == "wrong category" {
					prefix = append(prefix, core.OP_ILOAD_2)
				}
				if variant == "extra stack" {
					prefix = append(prefix, core.OP_ALOAD_1, core.OP_DUP)
				}
				code.Code = append(prefix, suffix...)
				// Keep the original conditional's RETURN destination valid. A
				// rejection must come from the malformed throw packet, rather
				// than accidentally from a broken branch or MaxStack limit.
				delta := len(code.Code) - len(originalCode.Code)
				for _, op := range ops {
					if op.Instr.OpCode >= core.OP_IFEQ && op.Instr.OpCode <= core.OP_IFLE {
						pc := int(op.CurrentOffset)
						old := int(int16(binary.BigEndian.Uint16(code.Code[pc+1 : pc+3])))
						binary.BigEndian.PutUint16(code.Code[pc+1:pc+3], uint16(int16(old+delta)))
					}
				}
				if variant == "extra stack" {
					code.MaxStack = 2
				}
				decoder = core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				ops = constructorMotionOps(decoder)
			case "nonoriginal terminal":
				code.Code[int(ops[throwAt].CurrentOffset)] = core.OP_RETURN
			case "wide terminal":
				ops[throwAt].IsWide = true
			case "terminal operand":
				ops[throwAt].Data = []byte{0}
			case "chain budget":
				remaining = 1
			}
			got := d.constructorReceiverEffects(obj, &code, ops, "(Ljava/lang/RuntimeException;I)V", map[string]bool{}, map[string]bool{}, &remaining, 0)
			if got != (variant == "closed") {
				t.Fatalf("abrupt proof=%v", got)
			}
		})
	}
	for _, variant := range []string{"cancelled", "graph budget"} {
		t.Run(variant, func(t *testing.T) {
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
			d.options.TargetSourceVersion = 8
			if variant == "cancelled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			} else {
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if d.constructorCaptureChainDoesNotObserve("AbruptSafe", "(Ljava/lang/RuntimeException;I)V", map[string]bool{}) {
				t.Fatal("abrupt proof bypassed request bounds")
			}
		})
	}
	for _, tc := range []struct{ name, variant string }{{"AbruptFresh", "uninitialized throw"}, {"AbruptIndependentPublication", "closed"}, {"AbruptIndependentPublication", "receiver publication"}, {"AbruptIndependentPublication", "wrong category"}, {"AbruptIndependentPublication", "uninitialized publication"}, {"AbruptIndependentPublication", "different original field"}, {"AbruptIndependentPublication", "nonoriginal store"}, {"AbruptIndependentPublication", "wide store"}, {"AbruptIndependentPublication", "unclosed finalizer"}} {
		t.Run(tc.name+"/"+tc.variant, func(t *testing.T) {
			obj, err := Parse(files[tc.name+".class"])
			if err != nil {
				t.Fatal(err)
			}
			var original *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "<init>" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							original = c
						}
					}
				}
			}
			if original == nil {
				t.Fatal("missing original code")
			}
			code := *original
			code.Code = append([]byte(nil), original.Code...)
			decode := func() []*core.OpCode {
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				return constructorMotionOps(decoder)
			}
			ops := decode()
			store := -1
			allocationInit := -1
			duplicate := -1
			for i, op := range ops {
				if member := constructorMotionMember(obj, op, core.OP_PUTSTATIC); member != nil && member.Member == "saved" {
					store = i
				}
				if member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); member != nil && member.Name == "java/lang/RuntimeException" && member.Member == "<init>" {
					allocationInit = i
				}
				if op.Instr.OpCode == core.OP_DUP {
					duplicate = i
				}
			}
			if allocationInit < 0 || duplicate < 0 {
				t.Fatal("missing original allocation")
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve, FuncCtx: &class_context.ClassContext{}, constructorReceiverFinalizerSilent: true}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			switch tc.variant {
			case "unclosed finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "uninitialized throw", "uninitialized publication":
				// Equal-width physical substitutions keep every branch target
				// intact, exposing a single live NEW token to the terminal/store.
				code.Code[int(ops[duplicate].CurrentOffset)] = core.OP_NOP
				pc := int(ops[allocationInit].CurrentOffset)
				for j := 0; j < 3; j++ {
					code.Code[pc+j] = core.OP_NOP
				}
				ops = decode()
			case "receiver publication", "wrong category":
				if store < 1 || ops[store-1].Instr.OpCode != core.OP_ALOAD_1 {
					t.Fatal("original publication packet")
				}
				pc := int(ops[store-1].CurrentOffset)
				code.Code[pc] = core.OP_ALOAD_0
				if tc.variant == "wrong category" {
					code.Code[pc] = core.OP_ILOAD_2
				}
				ops = decode()
			case "different original field":
				if store < 0 {
					t.Fatal("original store")
				}
				changed := false
				for _, op := range ops {
					if member := constructorMotionMember(obj, op, core.OP_PUTSTATIC); member != nil && member.Member == "bits" {
						ops[store].Data = append([]byte(nil), op.Data...)
						changed = true
					}
				}
				if !changed {
					t.Fatal("second original field")
				}
			case "nonoriginal store":
				if store < 0 {
					t.Fatal("original store")
				}
				code.Code[int(ops[store].CurrentOffset)] = core.OP_GETSTATIC
			case "wide store":
				if store < 0 {
					t.Fatal("original store")
				}
				ops[store].IsWide = true
			}
			remaining := 512
			got := d.constructorReceiverEffects(obj, &code, ops, "(Ljava/lang/RuntimeException;I)V", map[string]bool{}, map[string]bool{}, &remaining, 0)
			if got != (tc.variant == "closed") {
				t.Fatalf("independent effect proof=%v", got)
			}
		})
	}
}
