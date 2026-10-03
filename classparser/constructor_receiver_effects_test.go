package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// These are original-bytecode proof tests, not text-shape assertions. In
// particular an alias of this must never become a receiver-free value, and an
// inherited Fieldref must resolve to the same storage as a moved capture.
func TestAdversarialConstructorReceiverEffectsAliasesStorageAndFailureBoundaries(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	source := `class EffectBase {int value;EffectBase(){value=3;}}
 class EffectAlias {int value;EffectAlias(int n){EffectAlias alias=this;alias.value=n+2;}}
 class EffectWide {long wide;double floating;EffectWide(long n,double d){long copy=n;wide=(copy<<3)^7L;floating=d/2.0;}}
 class EffectRead extends EffectBase {int copied;EffectRead(){EffectBase alias=this;copied=alias.value;}}
 class EffectOwnRead {int value;int copied;EffectOwnRead(){value=3;copied=value;}}
 class EffectOverwrite extends EffectBase {EffectOverwrite(){value=5;}}
 class EffectPublish {static Object escaped;EffectPublish(){Object alias=this;escaped=alias;}}
 class EffectHeapAlias {Object self;EffectHeapAlias(){self=this;}}
 class EffectDivide {int value;EffectDivide(int n){value=7/n;}}
 class EffectArray {int value;EffectArray(int[] n){value=n.length;}}
 class EffectOpaque {int value;EffectOpaque(){value=System.identityHashCode(this);}}
 class EffectBranch {int value;EffectBranch(int n){value=n==0?1:2;}}
 class EffectVolatile {volatile int value;EffectVolatile(){value=3;}}
 `
	path := filepath.Join(dir, "Effects.java")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, debug := range []string{"-g", "-g:none"} {
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, path).CombinedOutput(); err != nil {
			t.Fatalf("original: %v\n%s", err, out)
		}
		resolve := func(name string) ([]byte, bool) {
			raw, err := os.ReadFile(filepath.Join(dir, name+".class"))
			return raw, err == nil
		}
		dummy, _ := resolve("EffectAlias")
		obj, err := Parse(dummy)
		if err != nil {
			t.Fatal(err)
		}
		dumper := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve, FuncCtx: &class_context.ClassContext{}}
		dumper.FuncCtx.InvocationMetadata = dumper.buildInvocationMetadata()
		for _, tc := range []struct {
			name, desc, storage string
			want                bool
		}{
			{"EffectAlias", "(I)V", "", true}, {"EffectWide", "(JD)V", "", true}, {"EffectRead", "()V", "", true},
			{"EffectOwnRead", "()V", "", true}, {"EffectOwnRead", "()V", "EffectOwnRead", false},
			{"EffectRead", "()V", "EffectBase", false}, {"EffectOverwrite", "()V", "EffectBase", false},
			{"EffectPublish", "()V", "", false}, {"EffectHeapAlias", "()V", "", false},
			{"EffectDivide", "(I)V", "", false}, {"EffectArray", "([I)V", "", false},
			{"EffectOpaque", "()V", "", false}, {"EffectBranch", "(I)V", "", false}, {"EffectVolatile", "()V", "", false},
		} {
			t.Run(debug+"/"+tc.name+"/"+tc.storage, func(t *testing.T) {
				writes := map[string]bool{}
				if tc.storage != "" {
					writes[tc.storage+"\x00value\x00I"] = true
				}
				remaining := 512
				if got := dumper.constructorChainDoesNotObserve(tc.name, tc.desc, writes, map[string]bool{}, &remaining, 0); got != tc.want {
					t.Fatalf("proof=%v want=%v", got, tc.want)
				}
			})
		}
		raw, _ := resolve("EffectWide")
		wide, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var code *CodeAttribute
		for _, m := range wide.Methods {
			n, _ := wide.getUtf8(m.NameIndex)
			if n == "<init>" {
				for _, a := range m.Attributes {
					if v, ok := a.(*CodeAttribute); ok {
						code = v
					}
				}
			}
		}
		if code == nil {
			t.Fatal("missing original constructor")
		}
		decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(wide.ConstantPool, i) })
		if err := decoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		malformed := *code
		malformed.MaxStack = 0
		remaining := 512
		if dumper.constructorReceiverEffects(wide, &malformed, constructorMotionOps(decoder), "(JD)V", map[string]bool{}, map[string]bool{}, &remaining, 0) {
			t.Fatal("invalid declared stack capacity accepted")
		}
	}
}

func TestAdversarialConstructorMotionMemberRejectsMalformedReferences(t *testing.T) {
	op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_PUTFIELD}, Data: []byte{0, 1}}
	for _, pool := range [][]ConstantInfo{nil, {nil}, {&ConstantMethodrefInfo{}}, {&ConstantFieldrefInfo{}}, {&ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: 65535, NameAndTypeIndex: 1}}}} {
		if constructorMotionMember(&ClassObject{ConstantPool: pool}, op, core.OP_PUTFIELD) != nil {
			t.Fatal("malformed field reference accepted")
		}
	}
}
