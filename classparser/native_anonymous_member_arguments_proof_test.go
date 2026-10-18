package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// These are valid authored bytecode variants, executed before their refusal is
// inspected. A receiver may be hidden for one constructor argument without
// gaining an expression role for another Object argument of the same call.
func TestNativeAnonymousMemberArgumentsDoNotExportHiddenReceiver(t *testing.T) {
	source := strings.Replace(anonymousMemberArgumentsFixture, "MemberArgsEffects.ref(1,first),MemberArgsEffects.wide(salt),MemberArgsEffects.ref(3,last)", "first,salt,last", 1)
	source += `
class MemberArgsReceiverDriver{public static void main(String[]args){Object first=new Object(),last=new Object();MemberArgsRoot root=MemberArgsRoot.create(null);MemberArgsRoot.Entry value=(MemberArgsRoot.Entry)root.make(first,17,last).get();if(value.first!=root||value.root!=root||value.last!=last||value.word!=17)throw new AssertionError("original receiver as ordinary argument");System.out.println("receiver:original:identity");}}
`
	base := nativeCompileClasses(t, source)
	for _, variant := range []string{"duplicate hidden value", "separate ordinary read"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for name, raw := range base {
				files[name] = bytes.Clone(raw)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["MemberArgsRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			family := d.planNativeMemberFamily()
			if family == nil || !d.planNativeMemberAnonymousScopes(family) {
				t.Fatal("original multiargument lexical forest")
			}
			forest := family.anonymousForest
			caller := forest.objects["MemberArgsRoot$1$1"]
			var receipt *nativeAnonymousMemberEnclosing
			for _, sites := range forest.memberEnclosing[caller.GetClassName()] {
				for _, candidate := range sites {
					if receipt != nil {
						t.Fatal("multiple original allocation receipts")
					}
					receipt = candidate
				}
			}
			if receipt == nil {
				t.Fatal("original multiargument consumer absent")
			}
			decoder := core.NewDecompiler(receipt.code.Code, func(index int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, index) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			changed := false
			for i, op := range ops {
				field := constructorMotionMember(caller, op, core.OP_GETFIELD)
				if field == nil || field.Member != "val$first" {
					continue
				}
				if i == 0 || ops[i-1].Instr.OpCode != core.OP_ALOAD_0 || int(op.CurrentOffset) != int(ops[i-1].CurrentOffset)+1 || len(op.Data) != 2 {
					t.Fatal("original ordinary argument producer")
				}
				pc := int(ops[i-1].CurrentOffset)
				if variant == "duplicate hidden value" {
					copy(receipt.code.Code[pc:pc+4], []byte{core.OP_DUP, core.OP_NOP, core.OP_NOP, core.OP_NOP})
				} else {
					index := binary.BigEndian.Uint16(receipt.code.Code[receipt.read.pc+1:])
					binary.BigEndian.PutUint16(receipt.code.Code[int(op.CurrentOffset)+1:], index)
				}
				changed = true
				break
			}
			if !changed {
				t.Fatal("ordinary argument was not mutated")
			}
			files[caller.GetClassName()+".class"] = caller.Bytes()
			nativeConstructorLambdaOriginalRun(t, files, "MemberArgsReceiverDriver", "receiver:original:identity\n")
			decoder = core.NewDecompiler(receipt.code.Code, func(index int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, index) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			fresh := nativeAnonymousMemberEnclosingProof(forest, caller, receipt.method, receipt.code, constructorMotionOps(decoder), receipt.read, receipt.child.owner, nil)
			// A separate GETFIELD leaves the first consumer closed. It must not
			// make the second read printable through a cached field-role grant.
			if (fresh != nil) != (variant == "separate ordinary read") {
				t.Fatal("one hidden receiver gained another argument use", variant)
			}
			freshArchive := nativeArchive(t, files)
			defer freshArchive.Close()
			freshRoot, err := Parse(files["MemberArgsRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			freshReader := freshArchive.nativeMemberReader(freshRoot)
			freshFamily := freshReader.planNativeMemberFamily()
			if freshFamily == nil || freshReader.planNativeMemberAnonymousScopes(freshFamily) {
				t.Fatal("unnameable ordinary argument was admitted", variant)
			}
		})
	}
}
