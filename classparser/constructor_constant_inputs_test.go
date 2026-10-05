package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"testing"
)

func TestAdversarialConstructorConstantDomainPreservesUnknownAndObservationPaths(t *testing.T) {
	const source = `class InputGuard {int value;InputGuard(int n){if(n!=7)throw new IllegalArgumentException();value=n;}}
 class InputCopied {int value;InputCopied(int n){int copy=n;if(copy!=7)throw new IllegalArgumentException();value=copy;}}
 class InputChanged {int value;InputChanged(int n){n++;if(n!=7)throw new IllegalArgumentException();value=n;}}
 class InputWideChanged {int value;InputWideChanged(int n){n+=300;if(n!=7)throw new IllegalArgumentException();value=n;}}
 class InputDelegate extends InputGuard {InputDelegate(int n){super(n);}}
 class InputCleared {int value;InputCleared(int n,int other){n=other;if(n!=7)throw new IllegalArgumentException();value=n;}}
 class InputObserve {Object token,copy;InputObserve(int n){if(n==7)copy=token;}}
 class InputPublish {static Object saved;InputPublish(int n){if(n==7)saved=this;}}
 class InputLoop {int value;InputLoop(int n){while(n>0)n--;value=n;}}
 class InputCompare {int value;InputCompare(int n){if(n<=3)throw new IllegalArgumentException();value=n;}}
 class InputNegative {int value;InputNegative(int n){if(n<0)throw new IllegalArgumentException();value=n;}}
 class InputArithmetic {int value;InputArithmetic(int n){n=n*2;if(n!=14)throw new IllegalArgumentException();value=n;}}
 `
	known := func(n int32) constructorEffectValue {
		return constructorEffectValue{kind: 'I', knownInt: true, intWord: n}
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, source, debug)
			obj, err := Parse(files["InputGuard.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }, FuncCtx: &class_context.ClassContext{}}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			for _, tc := range []struct {
				name, desc string
				args       []constructorEffectValue
				want       bool
			}{
				{"InputGuard", "(I)V", nil, false}, {"InputGuard", "(I)V", []constructorEffectValue{known(7)}, true}, {"InputGuard", "(I)V", []constructorEffectValue{known(8)}, false},
				{"InputCopied", "(I)V", []constructorEffectValue{known(7)}, true},
				{"InputCompare", "(I)V", []constructorEffectValue{known(5)}, true}, {"InputCompare", "(I)V", []constructorEffectValue{known(2)}, false}, {"InputNegative", "(I)V", []constructorEffectValue{known(0)}, true}, {"InputNegative", "(I)V", []constructorEffectValue{known(-3)}, false}, {"InputChanged", "(I)V", []constructorEffectValue{known(6)}, true}, {"InputChanged", "(I)V", []constructorEffectValue{known(7)}, false},
				{"InputWideChanged", "(I)V", []constructorEffectValue{known(-293)}, true}, {"InputWideChanged", "(I)V", []constructorEffectValue{known(7)}, false},
				{"InputDelegate", "(I)V", []constructorEffectValue{known(7)}, true}, {"InputDelegate", "(I)V", nil, false},
				{"InputCleared", "(II)V", []constructorEffectValue{known(7), {kind: 'I'}}, false}, {"InputCleared", "(II)V", []constructorEffectValue{known(8), known(7)}, true},
				{"InputObserve", "(I)V", []constructorEffectValue{known(7)}, false}, {"InputObserve", "(I)V", []constructorEffectValue{known(8)}, true},
				{"InputPublish", "(I)V", []constructorEffectValue{known(7)}, false}, {"InputPublish", "(I)V", []constructorEffectValue{known(8)}, true}, {"InputPublish", "(I)V", nil, false},
				{"InputLoop", "(I)V", []constructorEffectValue{known(0)}, true}, {"InputLoop", "(I)V", []constructorEffectValue{known(1)}, false}, {"InputLoop", "(I)V", nil, false}, {"InputArithmetic", "(I)V", []constructorEffectValue{known(7)}, false},
				{"InputGuard", "(I)V", []constructorEffectValue{{kind: 'L'}}, false}, {"InputGuard", "(I)V", []constructorEffectValue{{kind: 'I', receiver: true}}, false},
				{"InputGuard", "(I)V", []constructorEffectValue{{kind: 'I', allocation: 1}}, false}, {"InputGuard", "(I)V", []constructorEffectValue{known(7), known(7)}, false},
				{"java/lang/Object", "()V", []constructorEffectValue{known(7)}, false},
			} {
				aliases := &constructorSelfStorageProof{}
				remaining := 512
				writes := map[string]bool{"InputObserve\x00token\x00Ljava/lang/Object;": true}
				got := d.constructorChainEffectsWithArguments(tc.name, tc.desc, writes, map[string]bool{}, &remaining, 0, aliases, tc.args) && aliases.closed()
				if got != tc.want {
					t.Fatalf("%s args=%+v proof=%v want=%v", tc.name, tc.args, got, tc.want)
				}
			}
		})
	}
}

func TestAdversarialConstructorIntFactsRejectWrongTagsWidthsAndWideLiterals(t *testing.T) {
	obj := &ClassObject{ConstantPool: []ConstantInfo{&ConstantIntegerInfo{Value: -2147483648}, &ConstantFloatInfo{Value: 7}}}
	for _, tc := range []struct {
		opcode int
		data   []byte
		wide   bool
		want   bool
		word   int32
	}{
		{core.OP_ICONST_M1, nil, false, true, -1}, {core.OP_BIPUSH, []byte{128}, false, true, -128}, {core.OP_SIPUSH, []byte{128, 0}, false, true, -32768}, {core.OP_LDC, []byte{1}, false, true, -2147483648}, {core.OP_LDC_W, []byte{0, 1}, false, true, -2147483648},
		{core.OP_BIPUSH, nil, false, false, 0}, {core.OP_BIPUSH, []byte{0, 7}, false, false, 0}, {core.OP_ICONST_1, []byte{0}, false, false, 0}, {core.OP_LDC, []byte{2}, false, false, 0}, {core.OP_LDC, []byte{0}, false, false, 0}, {core.OP_LDC2_W, []byte{0, 1}, false, false, 0}, {core.OP_ICONST_1, nil, true, false, 0},
	} {
		op := &core.OpCode{Instr: &core.Instruction{OpCode: tc.opcode}, Data: tc.data, IsWide: tc.wide}
		v, ok := constructorOriginalIntLiteral(obj, op)
		if ok != tc.want || ok && (!v.knownInt || v.kind != 'I' || v.intWord != tc.word) {
			t.Fatalf("op=%x data=%v wide=%v fact=%+v known=%v", tc.opcode, tc.data, tc.wide, v, ok)
		}
	}
	if _, ok := constructorKnownIntBranch(core.OP_IF_ICMPEQ, []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 7}, {kind: 'I'}}); ok {
		t.Fatal("unknown second operand pruned a branch")
	}
	if _, ok := constructorKnownIntBranch(core.OP_IFNULL, []constructorEffectValue{{kind: 'L', knownInt: true}}); ok {
		t.Fatal("reference branch specialized by integer domain")
	}
}

func TestAdversarialConstructorInputPacketNeverSpecializesForwardedParameters(t *testing.T) {
	const source = `class PacketParent{PacketParent(int n){}}class PacketFixed extends PacketParent{PacketFixed(){super(-129);}}class PacketForward extends PacketParent{PacketForward(int n){super(n);}}class PacketArithmetic extends PacketParent{PacketArithmetic(int n){super(n+1);}}class PacketSource{static int n;}class PacketRead extends PacketParent{PacketRead(){super(PacketSource.n);}}`
	files := nativeCompileDebugClasses(t, source, "none")
	for _, tc := range []struct {
		name          string
		params        []string
		known, packet bool
		word          int32
	}{
		{"PacketFixed", nil, true, true, -129}, {"PacketForward", []string{"I"}, false, true, 0}, {"PacketArithmetic", []string{"I"}, false, false, 0}, {"PacketRead", nil, false, false, 0},
	} {
		obj, err := Parse(files[tc.name+".class"])
		if err != nil {
			t.Fatal(err)
		}
		var code *CodeAttribute
		for _, m := range obj.Methods {
			name, _ := obj.getUtf8(m.NameIndex)
			if name == "<init>" {
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
		}
		if code == nil {
			t.Fatal("missing original constructor")
		}
		dec := core.NewDecompiler(code.Code, nil)
		if err := dec.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		ops := constructorMotionOps(dec)
		pc := -1
		for _, op := range ops {
			if op.Instr.OpCode == core.OP_INVOKESPECIAL {
				pc = int(op.CurrentOffset)
			}
		}
		facts, ok := constructorOriginalArgumentFacts(obj, ops, 0, pc, tc.params, constructorParameterSlots(tc.params))
		if ok != tc.packet || ok && (len(facts) != 1 || facts[0].knownInt != tc.known || tc.known && facts[0].intWord != tc.word) {
			t.Fatalf("%s facts=%+v packet=%v", tc.name, facts, ok)
		}
	}
}
