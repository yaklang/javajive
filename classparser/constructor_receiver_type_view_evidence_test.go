package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestConstructorReceiverTypeViewsRequireOriginalInitializedBody(t *testing.T) {
	files := nativeCompileClasses(t, `interface TypeEvidenceMarker {}
 class TypeEvidence {int result;TypeEvidence(Object input){Object alias=(TypeEvidenceMarker)this;result=alias==this?3:5;}}
 class TypeEvidenceIndependent {int result;TypeEvidenceIndependent(Object input){Object alias=(TypeEvidenceMarker)input;result=3;}}`)
	for _, variant := range []string{"original", "forged receiver load", "opcode view", "cast offset", "wide cast", "omitted return", "extra instruction", "changed bytes", "uninitialized this", "uninitialized allocation", "wrong category", "short stack", "chain budget", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			name := "TypeEvidence"
			if variant == "forged receiver load" {
				name = "TypeEvidenceIndependent"
			}
			obj, e := Parse(files[name+".class"])
			if e != nil {
				t.Fatal(e)
			}
			var body *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							if body != nil {
								t.Fatal("duplicate original constructor body")
							}
							body = code
						}
					}
				}
			}
			if body == nil {
				t.Fatal("missing original constructor body")
			}
			code := *body
			code.Code = append([]byte(nil), body.Code...)
			decode := func() []*core.OpCode {
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if e := decoder.ParseOpcode(); e != nil {
					t.Fatal(e)
				}
				return constructorMotionOps(decoder)
			}
			ops := decode()
			cast := -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_CHECKCAST {
					cast = i
					break
				}
			}
			if cast < 1 {
				t.Fatal("missing physical checked receiver view")
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }, FuncCtx: &class_context.ClassContext{}, constructorReceiverFinalizerSilent: true}
			d.options.TargetSourceVersion = 8
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			remaining := 512
			switch variant {
			case "forged receiver load":
				copy := *ops[cast-1].Instr
				copy.OpCode = core.OP_ALOAD_0
				ops[cast-1].Instr = &copy
			case "opcode view":
				copy := *ops[cast].Instr
				copy.OpCode = core.OP_INSTANCEOF
				ops[cast].Instr = &copy
			case "cast offset":
				ops[cast].CurrentOffset++
			case "wide cast":
				ops[cast].IsWide = true
			case "omitted return":
				ops = ops[:len(ops)-1]
			case "extra instruction":
				ops = append(ops, &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_RETURN}, CurrentOffset: 65534})
			case "changed bytes":
				code.Code[int(ops[cast].CurrentOffset)] = core.OP_NOP
			case "uninitialized this", "uninitialized allocation", "wrong category":
				prefix := []byte{core.OP_ALOAD_0}
				if variant == "uninitialized allocation" {
					prefix = []byte{core.OP_NEW, byte(obj.ThisClass >> 8), byte(obj.ThisClass), core.OP_DUP}
				}
				if variant == "wrong category" {
					prefix = []byte{core.OP_ICONST_0}
				}
				prefix = append(prefix, byte(core.OP_CHECKCAST), ops[cast].Data[0], ops[cast].Data[1], core.OP_POP)
				code.Code = append(prefix, code.Code...)
				ops = decode()
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
			if got := d.constructorReceiverEffects(obj, &code, ops, "(Ljava/lang/Object;)V", map[string]bool{}, map[string]bool{}, &remaining, 0); got != (variant == "original") {
				t.Fatalf("original initialized view=%v", got)
			}
		})
	}
}

// A resolver callback can change caller-owned inputs between two type views.
// A receipt from before that observation must not certify the later view.
func TestConstructorReceiverTypeViewsRecheckAcrossProviderObservations(t *testing.T) {
	files := nativeCompileClasses(t, `interface TypeEpochMarker {}
 class TypeEpochParent {int state;}
 class TypeEpoch extends TypeEpochParent {void run(){Object first=(TypeEpochMarker)this;state=1;Object second=(TypeEpochMarker)this;}}`)
	for _, variant := range []string{"unchanged", "changed original bytes", "changed prior opcode view", "inconsistent provider"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["TypeEpoch.class"])
			if err != nil {
				t.Fatal(err)
			}
			var body *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "run" {
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							body = code
						}
					}
				}
			}
			if body == nil {
				t.Fatal("missing original method")
			}
			code := *body
			code.Code = append([]byte(nil), body.Code...)
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			casts := 0
			for _, op := range ops {
				if op.Instr.OpCode == core.OP_CHECKCAST {
					casts++
				}
			}
			if casts != 2 || ops[0].Instr.OpCode != core.OP_ALOAD_0 {
				t.Fatal("missing original two-view boundary")
			}
			evidence := &constructorProfileEvidence{}
			calls := 0
			d := &ClassObjectDumper{obj: obj, FuncCtx: &class_context.ClassContext{}, constructorReceiverFinalizerSilent: true, constructorProfileEvidence: evidence}
			d.foldSiblingResolver = func(n string) ([]byte, bool) {
				evidence.providerObserved()
				if n == "TypeEpochParent" {
					calls++
					switch variant {
					case "changed original bytes":
						code.Code[0] = core.OP_ALOAD_1
					case "changed prior opcode view":
						copy := *ops[0].Instr
						copy.OpCode = core.OP_ALOAD_1
						ops[0].Instr = &copy
					case "inconsistent provider":
						evidence.inconsistent = true
					}
				}
				b, ok := files[n+".class"]
				return b, ok
			}
			remaining := 512
			got := d.constructorReceiverBodyEffectsWithStorage(obj, &code, ops, "()V", map[string]bool{}, map[string]bool{}, &remaining, 0, &constructorSelfStorageProof{freshReceiver: true}, true)
			if calls != 1 {
				t.Fatalf("provider boundary was not traversed exactly once: %d", calls)
			}
			if got != (variant == "unchanged") {
				t.Fatalf("view proof across mutable provider=%v", got)
			}
		})
	}
}
