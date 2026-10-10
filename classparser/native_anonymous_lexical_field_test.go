package javaclassparser

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousInheritedFieldReopensOriginalBinding(t *testing.T) {
	files := nativeCompileClasses(t, hierarchyQualifiedAnonymousFixture)
	variants := []string{"original", "foreign object", "copied field", "wrong declaration owner", "private field", "static field", "generic field", "wrong consumer PC", "wrong field PC", "wrong owner", "wrong descriptor", "changed opcode", "methodref tag", "shadow field", "captured shadow", "unknown parent", "wrong receiver", "missing original receiver", "wrong operand PC", "opaque operand", "missing field witness", "wrong field witness", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["ConstructOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original forest")
			}
			f := p.anonymousForest
			o := f.objects["ConstructOwner$1$1"]
			var c *nativeAnonymousLexicalConsumer
			for _, entry := range f.consumers[o.GetClassName()]["read()I"] {
				if entry.declaredField != nil {
					if c != nil {
						t.Fatal("ambiguous fixture")
					}
					c = entry
				}
			}
			if c == nil {
				t.Fatal("original inherited field consumer")
			}
			leafD := z.nativeMemberReader(o)
			leafD.nativeAnonymousForest = f
			leafD.nativeMemberRoot = p
			ctx := &class_context.ClassContext{ClassName: o.GetClassName(), FunctionName: "read", CurrentMethodDesc: "()I"}
			ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(o.GetClassName()))
			ref.IsThis = true
			ref.IsParam = true
			ref.MarkOriginalParameter(0)
			if variant != "missing original receiver" {
				ref.MarkOriginalReceiver()
			}
			var path []*nativeMemberLexicalRead
			for read := c.read; read != nil; read = read.prior {
				path = append(path, read)
			}
			var operand values.JavaValue = ref
			for i := len(path) - 1; i >= 0; i-- {
				read := path[i]
				field := values.NewRefMember(operand, read.field, types.NewJavaClass(read.descriptor[1:len(read.descriptor)-1]))
				field.HasOriginPC = true
				field.OriginPC = read.pc
				operand = field
			}
			field := values.NewRefMember(operand, c.name, types.NewJavaPrimer(types.JavaInteger))
			field.HasOriginPC = true
			field.OriginPC = c.pc
			witness := &values.JavaClassMember{Name: c.owner, Member: c.name, Description: c.descriptor}
			if variant == "wrong field witness" {
				witness.Description = "J"
			}
			if variant != "missing field witness" {
				field.MarkOriginalFieldRead(witness, c.pc)
			}
			var code *CodeAttribute
			for _, method := range o.Methods {
				name, _ := sourceBridgeUTF8(o, method.NameIndex)
				if name == "read" {
					for _, attr := range method.Attributes {
						if ca, ok := attr.(*CodeAttribute); ok {
							code = ca
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original code")
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign object":
				copy := *o
				leafD.obj = &copy
			case "copied field":
				copy := *c.declaredField
				c.declaredField = &copy
			case "wrong declaration owner":
				c.declarationOwner = "WrongAncestor"
			case "private field":
				c.declaredField.AccessFlags |= 2
			case "static field":
				c.declaredField.AccessFlags |= 8
			case "generic field":
				c.declaredField.Attributes = append(c.declaredField.Attributes, &SignatureAttribute{})
			case "wrong consumer PC":
				c.pc++
			case "wrong field PC":
				c.read.pc++
			case "wrong owner":
				c.owner = "ConstructOwner"
			case "wrong descriptor":
				c.descriptor = "J"
			case "changed opcode":
				code.Code[c.pc] = core.OP_GETSTATIC
			case "methodref tag":
				index := int(code.Code[c.pc+1])<<8 | int(code.Code[c.pc+2])
				cp := o.ConstantPool[index-1].(*ConstantFieldrefInfo)
				o.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: cp.ConstantMemberrefInfo}
			case "shadow field":
				pool := NewConstantPoolWithConstant(&o.ConstantPool)
				o.Fields = append(o.Fields, &MemberInfo{NameIndex: uint16(pool.AddUtf8Info(c.name)), DescriptorIndex: uint16(pool.AddUtf8Info(c.descriptor))})
			case "captured shadow":
				f.units[o.GetClassName()].fields["val$"+c.name] = 1
			case "unknown parent":
				target := f.objects[c.owner]
				target.SuperClass = uint16(NewConstantPoolWithConstant(&target.ConstantPool).AddNewClassInfo("UnknownAncestor"))
			case "wrong receiver":
				ref.IsThis = false
			case "wrong operand PC":
				operand.(*values.RefMember).OriginPC++
			case "opaque operand":
				original := operand
				field.Object = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return original.Type() })
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				cx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(cx, workbudget.Limits{})
			}
			leafD.Work = work
			leafD.wireNativeAnonymousForestCaptures(ctx)
			text, known := ctx.SourceLexicalCapturedField(field, field.OriginPC, field.Member)
			if known != (variant == "original") || known && text != "slots" {
				t.Fatalf("source binding=%q,%v", text, known)
			}
			// A field's planning PC cannot grant permission to a virtual call.
			leafD.wireNativeAnonymousLexicalConsumers(ctx)
			call := &values.FunctionCallExpression{Object: operand, ClassName: c.owner, FunctionName: c.name, Descriptor: "()I", Kind: values.InvokeVirtual, HasOriginPC: true, OriginPC: c.pc}
			if _, known := ctx.SourceLexicalInvocationReceiver(call); known {
				t.Fatal("field certificate authorized a method call")
			}
		})
	}
}

func TestNativeAnonymousFieldDeclarationBoundsAndAmbiguity(t *testing.T) {
	for _, shape := range []string{"inherited", "own hides parent", "shared diamond", "ambiguous diamond", "cycle", "unknown", "breadth limit", "depth limit", "wrong resolver name", "duplicate", "descriptor mismatch", "generic", "budget", "memory", "canceled"} {
		t.Run(shape, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			makeObject := func(name string) *ClassObject {
				o := NewClassObject()
				o.ThisClass = uint16(o.ConstantPoolManager.AddNewClassInfo(name))
				objects[name] = o
				return o
			}
			root, parent := makeObject("FieldLeaf"), makeObject("FieldBase")
			root.SuperClass = uint16(root.ConstantPoolManager.AddNewClassInfo(parent.GetClassName()))
			field := &MemberInfo{AccessFlags: 1, NameIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("value")), DescriptorIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("I"))}
			parent.Fields = []*MemberInfo{field}
			addInterface := func(o *ClassObject, name string) {
				o.Interfaces = append(o.Interfaces, uint16(o.ConstantPoolManager.AddNewClassInfo(name)))
			}
			var work *workbudget.Budget
			switch shape {
			case "own hides parent":
				root.Fields = []*MemberInfo{{AccessFlags: 1, NameIndex: uint16(root.ConstantPoolManager.AddUtf8Info("value")), DescriptorIndex: uint16(root.ConstantPoolManager.AddUtf8Info("I"))}}
			case "shared diamond", "ambiguous diamond":
				left, right := makeObject("FieldLeft"), makeObject("FieldRight")
				addInterface(root, left.GetClassName())
				addInterface(root, right.GetClassName())
				addInterface(left, parent.GetClassName())
				addInterface(right, parent.GetClassName())
				root.SuperClass = 0
				if shape == "ambiguous diamond" {
					right.Fields = []*MemberInfo{{AccessFlags: 1, NameIndex: uint16(right.ConstantPoolManager.AddUtf8Info("value")), DescriptorIndex: uint16(right.ConstantPoolManager.AddUtf8Info("I"))}}
				}
			case "cycle":
				parent.Fields = nil
				parent.SuperClass = uint16(parent.ConstantPoolManager.AddNewClassInfo(root.GetClassName()))
			case "unknown":
				delete(objects, parent.GetClassName())
			case "breadth limit":
				for i := 0; i < 129; i++ {
					o := makeObject(fmt.Sprintf("FieldWide%d", i))
					addInterface(root, o.GetClassName())
				}
			case "depth limit":
				p := root
				for i := 0; i < 65; i++ {
					o := makeObject(fmt.Sprintf("FieldDeep%d", i))
					p.SuperClass = uint16(p.ConstantPoolManager.AddNewClassInfo(o.GetClassName()))
					p = o
				}
				p.Fields = []*MemberInfo{{AccessFlags: 1, NameIndex: uint16(p.ConstantPoolManager.AddUtf8Info("value")), DescriptorIndex: uint16(p.ConstantPoolManager.AddUtf8Info("I"))}}
			case "wrong resolver name":
				objects[parent.GetClassName()] = root
			case "duplicate":
				copy := *field
				parent.Fields = append(parent.Fields, &copy)
			case "descriptor mismatch":
				field.DescriptorIndex = uint16(parent.ConstantPoolManager.AddUtf8Info("J"))
			case "generic":
				field.Attributes = []AttributeInfo{&SignatureAttribute{}}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				cx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(cx, workbudget.Limits{})
			}
			resolve := func(name string) (*ClassObject, bool) { o := objects[name]; return o, o != nil }
			owner, got, known := nativeAnonymousLexicalFieldDeclaration(root, "value", "I", resolve, work)
			want := shape == "inherited" || shape == "own hides parent" || shape == "shared diamond"
			if known != want {
				t.Fatalf("declaration admitted=%v", known)
			}
			if known {
				expectedOwner, expectedField := parent, field
				if shape == "own hides parent" {
					expectedOwner, expectedField = root, root.Fields[0]
				}
				if owner != expectedOwner || got != expectedField {
					t.Fatal("resolved different original declaration")
				}
			}
		})
	}
}

func TestNativeAnonymousInheritedAccessorRequiresDeclarationAndPacket(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, hierarchyQualifiedProtectedSources(), "none", "8")
	for _, variant := range []string{"original", "copied field", "wrong declaration owner", "private declaration", "static declaration", "generic declaration", "unknown ancestor", "methodref tag", "changed accessor code", "inherited role mutation", "lexical shadow", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["client/ConstructOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original protected accessor forest")
			}
			f := p.anonymousForest
			o := f.objects["client/ConstructOwner$1$1"]
			var c *nativeAnonymousLexicalConsumer
			for _, entry := range f.consumers[o.GetClassName()]["read()I"] {
				if entry.getter != nil && entry.getter.inheritedField {
					c = entry
				}
			}
			if c == nil || c.declaredField == nil {
				t.Fatal("original inherited accessor declaration")
			}
			leafD := z.nativeMemberReader(o)
			leafD.nativeAnonymousForest = f
			leafD.nativeMemberRoot = p
			ctx := &class_context.ClassContext{ClassName: o.GetClassName(), FunctionName: "read", CurrentMethodDesc: "()I"}
			owner := f.objects[c.owner]
			var work *workbudget.Budget
			switch variant {
			case "copied field":
				copy := *c.declaredField
				c.declaredField = &copy
			case "wrong declaration owner":
				c.declarationOwner = "other/Base"
			case "private declaration":
				c.declaredField.AccessFlags |= 2
			case "static declaration":
				c.declaredField.AccessFlags |= 8
			case "generic declaration":
				c.declaredField.Attributes = append(c.declaredField.Attributes, &SignatureAttribute{})
			case "unknown ancestor":
				owner.SuperClass = uint16(NewConstantPoolWithConstant(&owner.ConstantPool).AddNewClassInfo("UnknownParent"))
			case "methodref tag", "changed accessor code":
				for _, attr := range c.getter.method.Attributes {
					if code, ok := attr.(*CodeAttribute); ok {
						if variant == "changed accessor code" {
							code.Code[0] = core.OP_ACONST_NULL
						} else {
							index := int(code.Code[2])<<8 | int(code.Code[3])
							cp := owner.ConstantPool[index-1].(*ConstantFieldrefInfo)
							owner.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: cp.ConstantMemberrefInfo}
						}
					}
				}
			case "inherited role mutation":
				c.getter.inheritedField = false
			case "lexical shadow":
				pool := NewConstantPoolWithConstant(&o.ConstantPool)
				o.Fields = append(o.Fields, &MemberInfo{NameIndex: uint16(pool.AddUtf8Info(c.getter.field)), DescriptorIndex: uint16(pool.AddUtf8Info(c.getter.fieldDescriptor))})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				cx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(cx, workbudget.Limits{})
			}
			if got := nativeAnonymousLexicalConsumerSourceClosed(leafD, c, ctx, work); got != (variant == "original") {
				t.Fatalf("accessor consumption admitted=%v", got)
			}
		})
	}
}
