package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousMemberOuterRequiresExactConsumer(t *testing.T) {
	files := nativeCompileClasses(t, anonymousMemberOuterFixture)
	for _, variant := range []string{"original", "missing receipt", "wrong new", "wrong init", "foreign caller", "foreign method", "code replaced", "duplicate code", "changed bytes", "missing approval", "missing field role", "wrong field role", "borrowed lexical this", "missing forest", "failed group", "field flags", "static child", "abstract child", "local name shadow", "class formal shadow", "method formal shadow", "member name shadow", "wrong field pc", "foreign receiver", "wrong result", "field reference kind", "constructor reference kind", "late field mutation", "late formal shadow", "late member shadow", "late code mutation", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(bytes.Clone(files["OuterMemberRoot.class"]))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("sealed original family")
			}
			f := p.anonymousForest
			current := "OuterMemberRoot$1$1"
			caller := f.objects[current]
			var r *nativeAnonymousMemberEnclosing
			for _, sites := range f.memberEnclosing[current] {
				for _, candidate := range sites {
					if r != nil {
						t.Fatal("multiple packets")
					}
					r = candidate
				}
			}
			if r == nil {
				t.Fatal("member enclosing packet absent")
			}
			if f.reads[current][r.methodKey][r.read.pc] != nil || f.lexicalThis[current][r.methodKey][r.read.pc] {
				t.Fatal("anonymous THIS gained a general expression role")
			}
			plans, known := z.nativeMemberReader(caller).nativeMemberAllocations(p)
			if !known {
				t.Fatal("allocation plan")
			}
			plan := plans[r.methodKey][r.invokePC]
			if plan == nil || plan.anonymousMemberEnclosing != r {
				t.Fatal("consumer receipt not wired")
			}
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(current))
			receiver.IsThis = true
			field := values.NewRefMember(receiver, r.read.field, types.NewJavaClass(r.child.owner))
			field.HasOriginPC = true
			field.OriginPC = r.read.pc
			var operand any = field
			var work *workbudget.Budget
			mutated := false
			switch variant {
			case "missing receipt":
				delete(f.memberEnclosing[current][r.methodKey], r.newPC)
			case "wrong new":
				r.newPC++
			case "wrong init":
				r.invokePC++
			case "foreign caller":
				r.caller = root
			case "foreign method":
				copy := *r.method
				r.method = &copy
			case "code replaced":
				for i, a := range r.method.Attributes {
					if a == r.code {
						copy := *r.code
						r.method.Attributes[i] = &copy
					}
				}
			case "duplicate code":
				r.method.Attributes = append(r.method.Attributes, r.code)
			case "changed bytes":
				r.code.Code = bytes.Clone(r.code.Code)
				r.code.Code[0] = 0
			case "missing approval":
				delete(f.readPCs[current][r.methodKey], r.read.pc)
			case "missing field role":
				delete(f.units[current].fields, r.read.field)
			case "wrong field role":
				f.units[current].fields[r.read.field] = 1
			case "borrowed lexical this":
				f.lexicalThis[current][r.methodKey][r.read.pc] = true
			case "missing forest":
				p.anonymousForest = nil
			case "failed group":
				p.anonymousUnits[current].failed = true
			case "field flags":
				for _, ff := range caller.Fields {
					n, _ := sourceBridgeUTF8(caller, ff.NameIndex)
					if n == r.read.field {
						ff.AccessFlags &^= 0x10
					}
				}
			case "static child":
				r.child.static = true
			case "abstract child":
				r.child.object.AccessFlags |= 0x400
			case "local name shadow":
				cp := NewConstantPoolWithConstant(&caller.ConstantPool)
				caller.Attributes = append(caller.Attributes, &InnerClassesAttribute{Classes: []*InnerClassInfo{{InnerClassInfoIndex: uint16(cp.AddNewClassInfo(current + "$1")), InnerNameIndex: uint16(cp.AddUtf8Info(r.child.name))}}})
			case "class formal shadow":
				cp := NewConstantPoolWithConstant(&caller.ConstantPool)
				caller.Attributes = append(caller.Attributes, &SignatureAttribute{SignatureIndex: uint16(cp.AddUtf8Info("<" + r.child.name + ":Ljava/lang/Object;>Ljava/lang/Object;"))})
			case "method formal shadow":
				cp := NewConstantPoolWithConstant(&caller.ConstantPool)
				r.method.Attributes = append(r.method.Attributes, &SignatureAttribute{SignatureIndex: uint16(cp.AddUtf8Info("<" + r.child.name + ":Ljava/lang/Object;>" + r.methodKey[len("<init>"):]))})
			case "member name shadow":
				cp := NewConstantPoolWithConstant(&caller.ConstantPool)
				caller.Attributes = append(caller.Attributes, &InnerClassesAttribute{Classes: []*InnerClassInfo{{InnerClassInfoIndex: uint16(cp.AddNewClassInfo(r.child.object.GetClassName())), OuterClassInfoIndex: uint16(cp.AddNewClassInfo(current)), InnerNameIndex: uint16(cp.AddUtf8Info(r.child.name))}}})
			case "wrong field pc":
				field.OriginPC++
			case "foreign receiver":
				receiver.IsThis = false
			case "wrong result":
				field.JavaType = types.NewJavaClass("java.lang.Object")
			case "field reference kind":
				index := int(r.code.Code[r.read.pc+1])<<8 | int(r.code.Code[r.read.pc+2])
				ref := caller.ConstantPool[index-1].(*ConstantFieldrefInfo)
				caller.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "constructor reference kind":
				index := int(r.code.Code[r.invokePC+1])<<8 | int(r.code.Code[r.invokePC+2])
				ref := caller.ConstantPool[index-1].(*ConstantMethodrefInfo)
				caller.ConstantPool[index-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "late field mutation", "late formal shadow", "late member shadow", "late code mutation":
				originalResolve := f.resolve
				armed := true
				f.resolve = func(name string) (*ClassObject, bool) {
					o, known := originalResolve(name)
					if armed {
						armed = false
						mutated = true
						switch variant {
						case "late field mutation":
							for _, ff := range caller.Fields {
								n, _ := sourceBridgeUTF8(caller, ff.NameIndex)
								if n == r.read.field {
									ff.AccessFlags &^= 0x10
								}
							}
						case "late formal shadow":
							cp := NewConstantPoolWithConstant(&caller.ConstantPool)
							caller.Attributes = append(caller.Attributes, &SignatureAttribute{SignatureIndex: uint16(cp.AddUtf8Info("<" + r.child.name + ":Ljava/lang/Object;>Ljava/lang/Object;"))})
						case "late member shadow":
							cp := NewConstantPoolWithConstant(&caller.ConstantPool)
							caller.Attributes = append(caller.Attributes, &InnerClassesAttribute{Classes: []*InnerClassInfo{{InnerClassInfoIndex: uint16(cp.AddNewClassInfo(r.child.object.GetClassName())), OuterClassInfoIndex: uint16(cp.AddNewClassInfo(current)), InnerNameIndex: uint16(cp.AddUtf8Info(r.child.name))}}})
						case "late code mutation":
							r.method.Attributes = append(r.method.Attributes, r.code)
						}
					}
					return o, known
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeMemberLexicalEnclosingOperand(operand, plan, p, current, []statements.Statement{}, work)
			if (variant == "late field mutation" || variant == "late formal shadow" || variant == "late member shadow" || variant == "late code mutation") && !mutated {
				t.Fatal("did not reach provider mutation")
			}
			if got != (variant == "original") {
				t.Fatalf("consumer=%v want=%v", got, variant == "original")
			}
		})
	}
}

const anonymousMemberOuterFixture = `interface OuterMemberAction{Object get();}
abstract class OuterMemberRoot {
 class Entry {final OuterMemberRoot parent;Entry(){parent=OuterMemberRoot.this;}}
 abstract OuterMemberAction make();
 static OuterMemberRoot create(final Object token){return new OuterMemberRoot(){
  class Element extends Entry {final Object capture=token;}
  OuterMemberAction make(){return new OuterMemberAction(){
   final Entry entry=new Element();public Object get(){return entry;}
  };}
 };}
}`
