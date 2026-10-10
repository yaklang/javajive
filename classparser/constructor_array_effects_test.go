package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorIndependentArrayEffectsKeepReceiverAndFailureBoundaries(t *testing.T) {
	const fixture = `final class ArraySafe {int result;ArraySafe(int[] a,int n){a[0]+=n;result=a.length+a[0];}}
 final class ArrayReference {Object result;ArrayReference(Object[] a,Object n){a[0]=n;result=a[0];}}
 class ArrayOpen {int result;ArrayOpen(int[] a,int n){result=a.length+a[0];}}
 final class ArrayFinalize {int result;ArrayFinalize(int[] a,int n){result=a.length;}protected void finalize(){System.out.println(result);}}
 final class ArrayPublish {ArrayPublish(Object[] a,int n){a[0]=this;}}
 final class ArrayAlias {Object self;ArrayAlias(Object[] a,int n){Object alias=this;a[0]=alias;}}
 final class ArrayHeapAlias {Object self;ArrayHeapAlias(Object[] a,int n){self=this;a[0]=self;}}
 final class ArrayMovedRead {Object[] capture;Object result;ArrayMovedRead(Object[] a,int n){result=capture[n];}}
 final class ArrayCaught {int result;ArrayCaught(int[] a,int n){try{result=a[n];}catch(RuntimeException e){result=9;}}}
 final class DivideSafe {int i;long l;DivideSafe(int n,long m){i=7/n;l=9L%m;}}
 class DivideOpen {int i;DivideOpen(int n){i=7/n;}}
`
	files := nativeCompileClasses(t, fixture)
	resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
	for _, tc := range []struct {
		name, desc string
		safe       bool
	}{{"ArraySafe", "([II)V", true}, {"ArrayReference", "([Ljava/lang/Object;Ljava/lang/Object;)V", true}, {"ArrayOpen", "([II)V", false}, {"ArrayFinalize", "([II)V", false}, {"ArrayPublish", "([Ljava/lang/Object;I)V", false}, {"ArrayAlias", "([Ljava/lang/Object;I)V", false}, {"ArrayHeapAlias", "([Ljava/lang/Object;I)V", false}, {"ArrayMovedRead", "([Ljava/lang/Object;I)V", false}, {"ArrayCaught", "([II)V", false}, {"DivideSafe", "(IJ)V", true}, {"DivideOpen", "(I)V", false}} {
		t.Run(tc.name, func(t *testing.T) {
			obj, err := Parse(files[tc.name+".class"])
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
			d.options.TargetSourceVersion = 8
			writes := map[string]bool{tc.name + "\x00capture\x00[Ljava/lang/Object;": true}
			if got := d.constructorCaptureChainDoesNotObserve(tc.name, tc.desc, writes); got != tc.safe {
				t.Fatalf("independent array effect=%v want=%v", got, tc.safe)
			}
		})
	}
	obj, err := Parse(files["ArraySafe.class"])
	if err != nil {
		t.Fatal(err)
	}
	var original *CodeAttribute
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n == "<init>" {
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					original = c
				}
			}
		}
	}
	if original == nil {
		t.Fatal("missing original constructor")
	}
	for _, variant := range []string{"closed", "receiver array", "scalar array", "reference index", "reference stored integer", "unclosed finalizer", "nonoriginal length", "length operand", "wide load", "chain budget"} {
		t.Run(variant, func(t *testing.T) {
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
			load, index, length, store := -1, -1, -1, -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_ALOAD_1 && load < 0 {
					load = i
				}
				if op.Instr.OpCode == core.OP_ICONST_0 && index < 0 {
					index = i
				}
				if op.Instr.OpCode == core.OP_ARRAYLENGTH {
					length = i
				}
				if op.Instr.OpCode == core.OP_IASTORE {
					store = i
				}
			}
			if load < 0 || index < 0 || length < 0 || store < 1 {
				t.Fatal("missing original array packets")
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve, FuncCtx: &class_context.ClassContext{}, constructorReceiverFinalizerSilent: true}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			remaining := 512
			switch variant {
			case "receiver array":
				code.Code[int(ops[load].CurrentOffset)] = core.OP_ALOAD_0
				ops = decode()
			case "scalar array":
				code.Code[int(ops[load].CurrentOffset)] = core.OP_ILOAD_2
				ops = decode()
			case "reference index":
				code.Code[int(ops[index].CurrentOffset)] = core.OP_ALOAD_1
				ops = decode()
			case "reference stored integer":
				// Replace the int addition with an equal-width packet that leaves
				// an array value on top of the valid original array/index pair.
				if store < 3 || ops[store-1].Instr.OpCode != core.OP_IADD || ops[store-3].Instr.OpCode != core.OP_IALOAD {
					t.Fatal("original integer update")
				}
				code.Code[int(ops[store-3].CurrentOffset)] = core.OP_POP2
				code.Code[int(ops[store-1].CurrentOffset)] = core.OP_NOP
				pc := int(ops[store-2].CurrentOffset)
				code.Code[pc] = core.OP_ALOAD_1
				ops = decode()
			case "unclosed finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "nonoriginal length":
				code.Code[int(ops[length].CurrentOffset)] = core.OP_NOP
			case "length operand":
				ops[length].Data = []byte{0}
			case "wide load":
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_IALOAD {
						op.IsWide = true
					}
				}
			case "chain budget":
				remaining = 1
			}
			got := d.constructorReceiverEffects(obj, &code, ops, "([II)V", map[string]bool{}, map[string]bool{}, &remaining, 0)
			if got != (variant == "closed") {
				t.Fatalf("array packet proof=%v", got)
			}
		})
	}
}
