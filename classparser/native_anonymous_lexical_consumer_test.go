package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousLexicalLookupBoundsCompleteHierarchy(t *testing.T) {
	files := nativeCompileClasses(t, anonymousReturnedLambdaBodyFixture)
	for _, shape := range []string{"diamond", "cycle", "breadth limit", "unknown declaration", "inherited rival"} {
		t.Run(shape, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["ReturnedOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original committed forest")
			}
			f := p.anonymousForest
			leaf := f.objects["ReturnedOwner$1$1"]
			declarations := map[string]*ClassObject{}
			makeInterface := func(name string) *ClassObject {
				o := NewClassObject()
				o.ThisClass = uint16(o.ConstantPoolManager.AddNewClassInfo(name))
				o.MajorVersion, o.AccessFlags = 52, 0x0601
				declarations[name] = o
				return o
			}
			addInterface := func(o *ClassObject, name string) {
				o.Interfaces = append(o.Interfaces, uint16(NewConstantPoolWithConstant(&o.ConstantPool).AddNewClassInfo(name)))
			}
			left, right, common := makeInterface("LookupLeft"), makeInterface("LookupRight"), makeInterface("LookupCommon")
			addInterface(leaf, left.GetClassName())
			addInterface(leaf, right.GetClassName())
			addInterface(left, common.GetClassName())
			addInterface(right, common.GetClassName())
			switch shape {
			case "cycle":
				addInterface(common, left.GetClassName())
			case "breadth limit":
				for i := 0; i < 129; i++ {
					o := makeInterface(fmt.Sprintf("LookupWide%d", i))
					addInterface(leaf, o.GetClassName())
				}
			case "unknown declaration":
				delete(declarations, common.GetClassName())
			case "inherited rival":
				common.Methods = []*MemberInfo{{AccessFlags: 0x0401, NameIndex: uint16(common.ConstantPoolManager.AddUtf8Info("identity")), DescriptorIndex: uint16(common.ConstantPoolManager.AddUtf8Info("()Ljava/lang/Object;"))}}
			}
			resolve := func(name string) (*ClassObject, bool) {
				if o := declarations[name]; o != nil {
					return o, true
				}
				return f.resolve(name)
			}
			got := nativeAnonymousLexicalConsumerLookup(f, leaf.GetClassName(), "ReturnedOwner$1", "identity", false, resolve, nil)
			if got != (shape == "diamond") {
				t.Fatalf("complete bounded lookup=%v", got)
			}
		})
	}
}

func TestNativeAnonymousAccessorDeclarationRequiresOriginalPacket(t *testing.T) {
	files := nativeCompileClasses(t, anonymousReturnedLambdaBodyFixture)
	for _, variant := range []string{"original", "copied object", "copied declaration", "missing getter", "wrong field", "wrong result", "wrong ordinal", "static flag", "ordinary flag", "changed code", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["ReturnedOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("complete original forest")
			}
			f := p.anonymousForest
			o := f.objects["ReturnedOwner$1"]
			var g *nativeMemberPrivateGetter
			for _, x := range p.getters {
				if x.owner == o.GetClassName() {
					g = x
				}
			}
			if g == nil {
				t.Fatal("original accessor")
			}
			m := g.method
			var work *workbudget.Budget
			switch variant {
			case "copied object":
				copy := *o
				o = &copy
			case "copied declaration":
				copy := *m
				m = &copy
			case "missing getter":
				delete(p.getters, nativeMemberGetterKey(g.owner, g.name, g.descriptor))
			case "wrong field":
				g.field = "token"
			case "wrong result":
				g.fieldDescriptor = "Ljava/lang/String;"
			case "wrong ordinal":
				g.ordinal += 100
			case "static flag":
				g.staticField = true
			case "ordinary flag":
				m.AccessFlags = 8
			case "changed code":
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						c.Code[0] = core.OP_ACONST_NULL
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousAccessorDeclaration(f, o, m, work); got != (variant == "original") {
				t.Fatalf("declaration admitted=%v", got)
			}
		})
	}
}

func TestNativeAnonymousConsumerSourceRequiresOriginalOwnershipAndInterval(t *testing.T) {
	files := nativeCompileClasses(t, anonymousReturnedLambdaBodyFixture)
	for _, operation := range []string{"field", "call"} {
		t.Run(operation, func(t *testing.T) {
			for _, variant := range []string{"original", "foreign object", "foreign forest", "foreign group", "wrong consumer PC", "wrong field PC", "wrong receiver", "missing getter", "shadow declaration", "captured local shadow", "unknown superclass", "changed opcode", "static method", "missing original receiver", "ordinary parameter zero", "opaque operand", "wrong operand PC", "budget", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					root, _ := Parse(files["ReturnedOwner.class"])
					d := z.nativeMemberReader(root)
					p := d.planNativeMemberFamily()
					if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
						t.Fatal("original forest")
					}
					f := p.anonymousForest
					o := f.objects["ReturnedOwner$1$1"]
					key := "read()Ljava/lang/Object;"
					if operation == "call" {
						key = "owner()Ljava/lang/Object;"
					}
					var consumer *nativeAnonymousLexicalConsumer
					for _, x := range f.consumers[o.GetClassName()][key] {
						consumer = x
					}
					if consumer == nil {
						t.Fatal("original consumer")
					}
					leafD := z.nativeMemberReader(o)
					leafD.nativeAnonymousForest = f
					leafD.nativeMemberRoot = p
					methodName := "read"
					if operation == "call" {
						methodName = "owner"
					}
					ctx := &class_context.ClassContext{ClassName: o.GetClassName(), FunctionName: methodName, CurrentMethodDesc: "()Ljava/lang/Object;"}
					ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(o.GetClassName()))
					ref.IsThis = true
					ref.IsParam = true
					ref.MarkOriginalParameter(0)
					if variant != "missing original receiver" && variant != "ordinary parameter zero" {
						ref.MarkOriginalReceiver()
					}
					if variant == "ordinary parameter zero" {
						ref.MarkOriginalParameter(0)
					}
					var path []*nativeMemberLexicalRead
					for r := consumer.read; r != nil; r = r.prior {
						path = append(path, r)
					}
					var operand values.JavaValue = ref
					for i := len(path) - 1; i >= 0; i-- {
						r := path[i]
						v := values.NewRefMember(operand, r.field, types.NewJavaClass(r.descriptor[1:len(r.descriptor)-1]))
						v.HasOriginPC = true
						v.OriginPC = r.pc
						operand = v
					}
					var method *MemberInfo
					var code *CodeAttribute
					for _, m := range o.Methods {
						n, _ := sourceBridgeUTF8(o, m.NameIndex)
						if n == methodName {
							method = m
							for _, a := range m.Attributes {
								if c, ok := a.(*CodeAttribute); ok {
									code = c
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
					case "foreign forest":
						p.anonymousForest = &nativeAnonymousForest{}
					case "foreign group":
						f.groups[p.anonymousUnits[o.GetClassName()].owner] = &nativeAnonymousFamily{}
					case "wrong consumer PC":
						consumer.pc++
					case "wrong field PC":
						consumer.read.pc++
					case "wrong receiver":
						ref.IsThis = false
					case "missing getter":
						if consumer.getter != nil {
							delete(p.getters, nativeMemberGetterKey(consumer.getter.owner, consumer.getter.name, consumer.getter.descriptor))
						} else {
							delete(f.units, consumer.owner)
						}
					case "shadow declaration":
						pool := NewConstantPoolWithConstant(&o.ConstantPool)
						if operation == "field" {
							o.Fields = append(o.Fields, &MemberInfo{NameIndex: uint16(pool.AddUtf8Info(consumer.getter.field)), DescriptorIndex: uint16(pool.AddUtf8Info("Ljava/lang/Object;"))})
						} else {
							o.Methods = append(o.Methods, &MemberInfo{NameIndex: uint16(pool.AddUtf8Info(consumer.name)), DescriptorIndex: uint16(pool.AddUtf8Info(consumer.descriptor))})
						}
					case "captured local shadow":
						name := consumer.name
						if consumer.getter != nil {
							name = consumer.getter.field
						}
						f.units[o.GetClassName()].fields["val$"+name] = 1
					case "unknown superclass":
						o.SuperClass = uint16(NewConstantPoolWithConstant(&o.ConstantPool).AddNewClassInfo("UnknownLookupParent"))
					case "changed opcode":
						code.Code[consumer.pc] = core.OP_INVOKEINTERFACE
					case "static method":
						method.AccessFlags |= 8
					case "opaque operand":
						original := operand
						operand = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return original.Type() })
					case "wrong operand PC":
						operand.(*values.RefMember).OriginPC++
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						cx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(cx, workbudget.Limits{})
					}
					physical := nativeAnonymousLexicalConsumerSourceClosed(leafD, consumer, ctx, work)
					operandOK := nativeAnonymousLexicalConsumerOperand(operand, consumer, ctx, work)
					if got := physical && operandOK; got != (variant == "original" || variant == "captured local shadow" && operation == "call") {
						t.Fatalf("physical=%v operand=%v", physical, operandOK)
					}
				})
			}
		})
	}
}

func TestNativeAnonymousFailedForestRestoresAccessorRegistration(t *testing.T) {
	files := nativeCompileClasses(t, anonymousReturnedLambdaBodyFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	root, _ := Parse(files["ReturnedOwner.class"])
	d := z.nativeMemberReader(root)
	p := d.planNativeMemberFamily()
	if p == nil || len(p.getters) == 0 {
		t.Fatal("original named accessor registration")
	}
	prior := map[string]*nativeMemberPrivateGetter{}
	for k, g := range p.getters {
		prior[k] = g
	}
	o, err := Parse(files["ReturnedOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	pool := NewConstantPoolWithConstant(&o.ConstantPool)
	o.Methods = append(o.Methods, &MemberInfo{AccessFlags: 1, NameIndex: uint16(pool.AddUtf8Info("escape")), DescriptorIndex: uint16(pool.AddUtf8Info("(LReturnedOwner$1;)V")), Attributes: []AttributeInfo{&CodeAttribute{MaxLocals: 2, Code: []byte{core.OP_RETURN}}}})
	changed := o.Bytes()
	original := d.foldSiblingResolver
	d.foldSiblingResolver = func(name string) ([]byte, bool) {
		if name == "ReturnedOwner$1" {
			return changed, true
		}
		return original(name)
	}
	if d.planNativeAnonymousLexicalForest(p) != nil {
		t.Fatal("ordinary anonymous parameter admitted")
	}
	if len(prior) != len(p.getters) {
		t.Fatal("failed forest leaked staged accessors")
	}
	for k, g := range prior {
		if p.getters[k] != g {
			t.Fatal("failed forest changed original accessor identity")
		}
	}
}

func TestNativeAnonymousAccessorNameTypeRequiresExclusivePhysicalReferences(t *testing.T) {
	files := nativeCompileClasses(t, anonymousReturnedLambdaBodyFixture)
	for _, variant := range []string{"original", "copied object", "unused name type", "field alias", "method handle", "dynamic alias", "invoke dynamic alias", "foreign owner alias", "zero index", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["ReturnedOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original forest")
			}
			f := p.anonymousForest
			o := f.objects["ReturnedOwner$1$1"]
			index, refIndex := 0, 0
			var ref *ConstantMethodrefInfo
			for i, c := range o.ConstantPool {
				if m, ok := c.(*ConstantMethodrefInfo); ok && m != nil {
					owner, _ := sourceBridgeClassName(o, m.ClassIndex)
					if owner == "ReturnedOwner$1" {
						nt, ok := o.ConstantPool[m.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						if !ok {
							continue
						}
						n, _ := sourceBridgeUTF8(o, nt.NameIndex)
						if n == "access$100" {
							index = int(m.NameAndTypeIndex)
							refIndex = i + 1
							ref = m
						}
					}
				}
			}
			if ref == nil {
				t.Fatal("original physical getter reference")
			}
			var work *workbudget.Budget
			switch variant {
			case "copied object":
				copy := *o
				o = &copy
			case "unused name type":
				copy := *o.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				o.ConstantPool = append(o.ConstantPool, &copy)
				index = len(o.ConstantPool)
			case "field alias":
				o.ConstantPool = append(o.ConstantPool, &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo})
			case "method handle":
				o.ConstantPool = append(o.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: uint16(refIndex)})
			case "dynamic alias":
				o.ConstantPool = append(o.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "invoke dynamic alias":
				o.ConstantPool = append(o.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "foreign owner alias":
				copy := *ref
				copy.ClassIndex = uint16(NewConstantPoolWithConstant(&o.ConstantPool).AddNewClassInfo("ForeignAccessorOwner"))
				o.ConstantPool = append(o.ConstantPool, &copy)
			case "zero index":
				index = 0
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousAccessorNameType(f, o, index, work); got != (variant == "original") {
				t.Fatalf("physical name type admitted=%v", got)
			}
		})
	}
}
