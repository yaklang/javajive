package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeRootMemberAnonymousRequiresExactEnclosingReceiver(t *testing.T) {
	files := nativeCompileDebugClasses(t, nativeRootMemberAnonymousFixture, "none")
	for _, variant := range []string{"original", "missing family", "foreign family", "missing parent", "static parent", "foreign parent owner", "missing capture", "wrong root slot", "missing target constructor", "extra computation", "handler", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["RootAnonMemberOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			family := z.nativeMemberReader(root).planNativeMemberFamily()
			if family == nil {
				t.Fatal("original named family")
			}
			obj, err := Parse(append([]byte(nil), files["RootAnonMemberOwner$1.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			owner, method, known := originalAnonymousOwner(obj)
			if !known {
				t.Fatal("original anonymous owner")
			}
			before := nativeAnonymousConstructorWithinMembers(obj, owner, method, nil, family)
			if before == nil || before.memberSuper == nil || before.memberEnclosingReadPC != -1 {
				t.Fatal("original plain enclosing transfer")
			}
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "<init>" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original constructor")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			captureEnd := -1
			for i, op := range ops {
				if field := constructorMotionMember(obj, op, core.OP_PUTFIELD); field != nil {
					if _, known := before.capturePCs[field.Member]; known {
						captureEnd = i
					}
				}
			}
			if captureEnd < 0 || captureEnd+2 >= len(ops) {
				t.Fatal("original enclosing load")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing family":
				family = nil
			case "foreign family":
				family.owner = "Foreign"
			case "missing parent":
				delete(family.children, obj.GetSupperClassName())
			case "static parent":
				family.children[obj.GetSupperClassName()].static = true
			case "foreign parent owner":
				family.children[obj.GetSupperClassName()].owner = "Foreign"
			case "missing capture":
				for _, f := range obj.Fields {
					name, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if name == "this$0" {
						f.AccessFlags &^= 0x1000
					}
				}
			case "wrong root slot":
				code.Code[ops[captureEnd+2].CurrentOffset] = byte(core.OP_ALOAD_0)
			case "missing target constructor":
				family.children[obj.GetSupperClassName()].constructors = nil
			case "extra computation":
				offset := int(ops[captureEnd+2].CurrentOffset)
				body := append([]byte(nil), code.Code[:offset]...)
				body = append(body, byte(core.OP_ICONST_0), byte(core.OP_POP))
				body = append(body, code.Code[offset:]...)
				code.Code = body
			case "handler":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: uint16(before.superPC), EndPc: uint16(len(code.Code) - 1), HandlerPc: uint16(len(code.Code) - 1)})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeAnonymousConstructorWithinMembers(obj, owner, method, work, family)
			if (got != nil) != (variant == "original") {
				t.Fatalf("enclosing receiver admitted=%v", got != nil)
			}
		})
	}
}

func TestNativeRootMemberAnonymousRefusesForeignQualifiedOuter(t *testing.T) {
	fixture := strings.Replace(nativeRootMemberAnonymousFixture, `if(observed!=RootAnonMemberOwner.this)throw new AssertionError("captured enclosing receiver before SUPER callback");`, "", 1)
	fixture = strings.Replace(fixture, "Object owner(){return RootAnonMemberOwner.this;}Object capture(){return null;}", "Object declaredOwner(){return RootAnonMemberOwner.this;}Object owner(){return RootAnonMemberOwner.this;}Object capture(){return null;}", 1)
	fixture = strings.Replace(fixture, "Parent make(Object seed,Object kept,long number)", "Parent make(RootAnonMemberOwner other,Object seed,Object kept,long number)", 1)
	fixture = strings.Replace(fixture, "return new Parent(number,seed)", "return other.new Parent(number,seed)", 1)
	fixture = strings.ReplaceAll(fixture, "o.make(token,value,n)", "o.make(o==a?b:a,token,value,n)")
	fixture = strings.ReplaceAll(fixture, "a.make(null,token,0)", "a.make(b,null,token,0)")
	fixture = strings.Replace(fixture, "p.owner()!=o||", "p.declaredOwner()!=(o==a?b:a)||p.owner()!=o||", 1)
	files := nativeCompileClasses(t, fixture)
	_, java := t04Tools(t)
	original := t.TempDir()
	for n, raw := range files {
		if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "RootAnonMemberDriver"); got != "12:root:member:anonymous:identity:callback:checked\n" {
		t.Fatalf("valid original qualified outer=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	source, err := z.ReadFile("RootAnonMemberOwner.class")
	if err != nil || !strings.Contains(string(source), "new RootAnonMemberOwner$1(") {
		t.Fatalf("foreign enclosing operand was suppressed without qualified source proof: %v\n%s", err, source)
	}
}
