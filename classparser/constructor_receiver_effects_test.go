package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// These are original-bytecode proof tests, not text-shape assertions. In
// particular an alias of this must never become a receiver-free value, and an
// inherited Fieldref must resolve to the same storage as a moved capture.
func TestAdversarialConstructorReceiverEffectsAliasesStorageAndFailureBoundaries(t *testing.T) {
	javac, java := t04Tools(t)
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
 class EffectPreBase {int value;EffectPreBase(int n){value=n;}}
 interface EffectPreProduce {int apply(int n);}
 class EffectPreCall extends EffectPreBase {EffectPreCall(EffectPreProduce op,int n){super(op.apply(n));}}
 class EffectPostCall extends EffectPreBase {EffectPostCall(EffectPreProduce op,int n){super(n);op.apply(n);}}
 class EffectPreVerifyDriver {public static void main(String[] args){try{new EffectPreCall(null,1);}catch(NullPointerException expected){System.out.println("null");}}}
 class EffectBranch {int value;EffectBranch(int n){value=n==0?1:2;}}
 class EffectEarlyReturn {int value;EffectEarlyReturn(int n){if(n<0){value=3;return;}value=n;}}
 class EffectBranchPublish {static Object escaped;int value;EffectBranchPublish(int n){if(n<0){Object alias=this;escaped=alias;}else{value=n;}}}
 class EffectBranchDivide {int value;EffectBranchDivide(int n){if(n<0){value=7/n;}else{value=n;}}}
 class EffectBranchRead {int value;int copied;EffectBranchRead(int n){value=n;if(n<0){copied=value;}else{copied=3;}}}
 class EffectLoop {int value;EffectLoop(int n){for(int i=0;i<n;i++){value+=i;}}}
 class EffectCompareWide {long value;EffectCompareWide(long n,double d){if(n>0&&d>0){value=n;}else{value=0;}}}
 class EffectAccessDriver {public static void main(String[] args){
 try{System.out.println("read:"+new EffectRead().copied);}catch(IllegalAccessError failure){System.out.println("read:IllegalAccessError");}
 try{new EffectOverwrite();System.out.println("write:ok");}catch(IllegalAccessError failure){System.out.println("write:IllegalAccessError");}
 }}
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
			{"EffectOpaque", "()V", "", false}, {"EffectBranch", "(I)V", "", true},
			{"EffectPreCall", "(LEffectPreProduce;I)V", "", true}, {"EffectPostCall", "(LEffectPreProduce;I)V", "", false},
			{"EffectEarlyReturn", "(I)V", "", true},
			{"EffectBranchPublish", "(I)V", "", false},
			{"EffectBranchDivide", "(I)V", "", false},
			{"EffectBranchRead", "(I)V", "", true},
			{"EffectBranchRead", "(I)V", "EffectBranchRead", false},
			{"EffectLoop", "(I)V", "", false},
			{"EffectCompareWide", "(JD)V", "", true}, {"EffectVolatile", "()V", "", false},
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
		preRaw, _ := resolve("EffectPreCall")
		if got := t04RunJava(t, java, dir, "EffectPreVerifyDriver"); got != "null\n" {
			t.Fatal("original pre-initialization null oracle", got)
		}
		pre, err := Parse(preRaw)
		if err != nil {
			t.Fatal(err)
		}
		var preCode *CodeAttribute
		for _, method := range pre.Methods {
			n, _ := pre.getUtf8(method.NameIndex)
			if n != "<init>" {
				continue
			}
			for _, a := range method.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					preCode = code
				}
			}
		}
		if preCode == nil {
			t.Fatal("missing original invocation code")
		}
		preDecoder := core.NewDecompiler(preCode.Code, func(i int) values.JavaValue { return GetValueFromCP(pre.ConstantPool, i) })
		if err := preDecoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, op := range constructorMotionOps(preDecoder) {
			if op.Instr.OpCode != core.OP_INVOKEINTERFACE {
				continue
			}
			found = true
			originalCode := append([]byte(nil), preCode.Code...)
			for _, operands := range [][2]byte{{0, 0}, {1, 0}, {255, 0}, {2, 1}} {
				preCode.Code = append([]byte(nil), originalCode...)
				pc := int(op.CurrentOffset)
				preCode.Code[pc+3], preCode.Code[pc+4] = operands[0], operands[1]
				if err := os.WriteFile(filepath.Join(dir, "EffectPreCall.class"), pre.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "EffectPreVerifyDriver").CombinedOutput()
				if err == nil || (!strings.Contains(string(output), "VerifyError") && !strings.Contains(string(output), "ClassFormatError")) {
					t.Fatalf("invalid interface operands %v were not rejected by original JVM: %v\n%s", operands, err, output)
				}
				remaining := 512
				if dumper.constructorChainDoesNotObserve("EffectPreCall", "(LEffectPreProduce;I)V", map[string]bool{}, map[string]bool{}, &remaining, 0) {
					t.Fatal("invalid interface operands certified", operands)
				}
			}
			break
		}
		if !found {
			t.Fatal("fixture lacks original interface invocation")
		}
		if err := os.WriteFile(filepath.Join(dir, "EffectPreCall.class"), preRaw, 0600); err != nil {
			t.Fatal(err)
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
		raw, _ = resolve("EffectBranch")
		branch, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var branchCode *CodeAttribute
		for _, m := range branch.Methods {
			n, _ := branch.getUtf8(m.NameIndex)
			if n == "<init>" {
				for _, a := range m.Attributes {
					if v, ok := a.(*CodeAttribute); ok {
						branchCode = v
					}
				}
			}
		}
		if branchCode == nil {
			t.Fatal("missing branch constructor")
		}
		decoder = core.NewDecompiler(branchCode.Code, func(i int) values.JavaValue { return GetValueFromCP(branch.ConstantPool, i) })
		if err := decoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		originalOps := constructorMotionOps(decoder)
		changed := false
		for index, op := range originalOps {
			if op.Instr.OpCode >= core.OP_IFEQ && op.Instr.OpCode <= core.OP_IFLE {
				changed = true
				for _, data := range [][]byte{{0, 0}, {0, 1}, {0x7f, 0xff}, {0xff, 0xff}} {
					badOps := append([]*core.OpCode(nil), originalOps...)
					bad := *op
					bad.Data = data
					badOps[index] = &bad
					remaining := 512
					if dumper.constructorReceiverEffects(branch, branchCode, badOps, "(I)V", map[string]bool{}, map[string]bool{}, &remaining, 0) {
						t.Fatalf("invalid/cyclic bytecode target accepted: %x", data)
					}
				}
				break
			}
		}
		if !changed {
			t.Fatal("fixture lacks original branch")
		}

		baseRaw, _ := resolve("EffectBase")
		for _, tc := range []struct {
			flags  uint16
			read   bool
			output string
		}{
			{2, false, "read:IllegalAccessError\nwrite:IllegalAccessError\n"},
			{16, true, "read:3\nwrite:IllegalAccessError\n"},
		} {
			base, err := Parse(baseRaw)
			if err != nil {
				t.Fatal(err)
			}
			base.Fields[0].AccessFlags = tc.flags
			if err := os.WriteFile(filepath.Join(dir, "EffectBase.class"), base.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if got := t04RunJava(t, java, dir, "EffectAccessDriver"); got != tc.output {
				t.Fatalf("original access oracle: %q want %q", got, tc.output)
			}
			remaining := 512
			if got := dumper.constructorChainDoesNotObserve("EffectRead", "()V", map[string]bool{}, map[string]bool{}, &remaining, 0); got != tc.read {
				t.Fatalf("ancestor access flags %x: read proof=%v", tc.flags, got)
			}
			remaining = 512
			if dumper.constructorChainDoesNotObserve("EffectOverwrite", "()V", map[string]bool{}, map[string]bool{}, &remaining, 0) {
				t.Fatalf("IllegalAccessError write accepted for motion: %x", tc.flags)
			}
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
	for _, data := range [][]byte{nil, {0, 1}, {0, 1, 1}, {0, 1, 1, 1}, {0, 1, 1, 0, 0}} {
		op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKEINTERFACE}, Data: data}
		if constructorMotionMember(&ClassObject{}, op, core.OP_INVOKEINTERFACE) != nil {
			t.Fatal("invalid interface invocation operand shape accepted")
		}
	}
}
