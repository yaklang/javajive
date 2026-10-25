package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialMemberConstructorGraphRequiresClosedCapturePaths(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original javac8 required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, `class GraphOwner<T> {class Member {
 final java.util.List<T> value;
 Member(Object v,long n){this((String)null,n);}
 Member(String v,long n){this((java.util.List<T>)null,n);}
 Member(java.util.List<T> v,long n){value=v;}
 }}`, "GraphOwner", "none", false)
	const owner = "GraphOwner$Member"
	const first = "(LGraphOwner;Ljava/lang/Object;J)V"
	const middle = "(LGraphOwner;Ljava/lang/String;J)V"
	const terminal = "(LGraphOwner;Ljava/util/List;J)V"
	for _, variant := range []string{"control", "fresh", "reordered declarations", "cycle", "missing terminal", "duplicate constructor", "nil member", "wrong owner argument", "null owner", "wrong target owner", "missing target", "suffix effect", "handler", "insufficient stack", "insufficient locals", "wrong descriptor", "wrong flags", "nonfinal capture", "missing capture marker", "future version", "preview", "work", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files[owner+".class"]))
			methods := map[string]*MemberInfo{}
			for _, m := range obj.Methods {
				if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "<init>" {
					desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
					methods[desc] = m
				}
			}
			if len(methods) != 3 || methods[first] == nil || methods[middle] == nil || methods[terminal] == nil {
				t.Fatal("three distinct original constructor ports")
			}
			forward := methods[middle]
			code := constructorLoweringCode(forward)
			decoder := core.NewDecompiler(code.Code, nil)
			err := decoder.ParseOpcode()
			ops := constructorMotionOps(decoder)
			if err != nil || len(ops) < 4 || code.Code[0] != 0x2a || code.Code[1] != 0x2b {
				t.Fatalf("original this/outer operand prefix: error=%v code=%x ops=%d", err, code.Code, len(ops))
			}
			retarget := func(targetOwner, desc string) {
				t.Helper()
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_INVOKESPECIAL {
						at := int(op.CurrentOffset)
						index := obj.ConstantPoolManager.AddNewMethodInfo(targetOwner, "<init>", desc)
						binary.BigEndian.PutUint16(code.Code[at+1:at+3], uint16(index))
						return
					}
				}
				t.Fatal("original delegation absent")
			}
			var work *workbudget.Budget
			switch variant {
			case "reordered declarations":
				obj.Methods[0], obj.Methods[2] = obj.Methods[2], obj.Methods[0]
			case "cycle":
				retarget(owner, first)
			case "missing terminal":
				obj.Methods = []*MemberInfo{methods[first], methods[middle]}
			case "duplicate constructor":
				obj.Methods = append(obj.Methods, forward)
			case "nil member":
				obj.Methods = append(obj.Methods, nil)
			case "wrong owner argument":
				code.Code[1] = 0x2c
			case "null owner":
				code.Code[1] = 0x01
			case "wrong target owner":
				retarget("ForeignOwner", terminal)
			case "missing target":
				retarget(owner, "(LGraphOwner;Ljava/lang/CharSequence;J)V")
			case "suffix effect":
				code.Code = append(bytes.Clone(code.Code[:len(code.Code)-1]), 0x03, 0x57, 0xb1)
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
			case "insufficient stack":
				code.MaxStack = 1
			case "insufficient locals":
				code.MaxLocals = 2
			case "wrong descriptor":
				forward.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("(LGraphOwner;Ljava/lang/String;I)V"))
			case "wrong flags":
				forward.AccessFlags |= 8
			case "nonfinal capture", "missing capture marker":
				for _, f := range obj.Fields {
					if f.AccessFlags&syntheticMethodFlag != 0 {
						if variant == "nonfinal capture" {
							f.AccessFlags &^= 0x10
						} else {
							f.AccessFlags &^= syntheticMethodFlag
						}
					}
				}
			case "future version":
				obj.MajorVersion = 66
			case "preview":
				obj.MinorVersion = 65535
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			var before []byte
			if variant != "nil member" {
				before = bytes.Clone(obj.Bytes())
			}
			proof := memberScopeConstructorGraph(obj, "GraphOwner", variant != "fresh", work, nil)
			want := variant == "control" || variant == "fresh" || variant == "reordered declarations"
			if (proof != nil) != want {
				t.Fatalf("closed graph=%v want=%v", proof != nil, want)
			}
			if want && (len(proof) != 3 || proof[first].forward == nil || proof[middle].forward == nil || proof[terminal].capture == nil) {
				t.Fatal("lost edge or terminal capture")
			}
			if before != nil && !bytes.Equal(before, obj.Bytes()) {
				t.Fatal("read-only proof mutated input")
			}
		})
	}
}

func TestAdversarialMemberConstructorCompletionRetainsFreshCodeAndExactEdges(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, memberConstructorChainScopeFixture, "none", []string{"SuffixOwner$Member"}, "50:constructor:chain:scope:identity:overflow\n")
	base := artifact.source.jar.sourceConstructorLowering
	var err error
	fresh, err = base.restoreConstructorBindingSignatures(original, fresh, nil)
	if err != nil {
		t.Fatal("preceding constructor signature completion", err)
	}
	for _, variant := range []string{"control", "new null cast", "different wide argument", "missing target", "changed access", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var forward *MemberInfo
			for _, m := range obj.Methods {
				if d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex); d == "(LSuffixOwner;J)V" {
					forward = m
				}
			}
			if forward == nil {
				t.Fatal("forwarding constructor")
			}
			code := constructorLoweringCode(forward)
			ops, ok := nativeEnumMethodOps(obj, forward, nil)
			if !ok {
				t.Fatal("fresh instructions")
			}
			switch variant {
			case "new null cast":
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_ACONST_NULL {
						at := int(op.CurrentOffset) + 1
						idx := obj.ConstantPoolManager.AddNewClassInfo("java/util/List")
						next := append([]byte{}, code.Code[:at]...)
						next = append(next, 0xc0, byte(idx>>8), byte(idx))
						code.Code = append(next, code.Code[at:]...)
						code.AttrLen += 3
						break
					}
				}
			case "different wide argument":
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_LLOAD_2 {
						code.Code[op.CurrentOffset] = 0x09
					}
				}
			case "missing target":
				obj.Methods = []*MemberInfo{forward}
			case "changed access":
				forward.AccessFlags ^= 1
			}
			input := obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("counterexample must reach semantic proof", err)
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			saved := bytes.Clone(input)
			out, err := base.restoreMemberGenericScope(original, input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("caller input changed")
			}
			if variant != "control" {
				if err == nil || out != nil {
					t.Fatal("changed proof premises accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			finished := mustParseLowering(t, out)
			for i, m := range obj.Methods {
				if !reflect.DeepEqual(constructorLoweringCode(m), constructorLoweringCode(finished.Methods[i])) {
					t.Fatal("metadata completion changed fresh Code")
				}
			}
		})
	}
}
