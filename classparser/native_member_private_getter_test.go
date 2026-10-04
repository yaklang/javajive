package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

const nativeMemberGetterFixture = `class GetterOwner{private Object token;GetterOwner(Object x){token=x;}class Layer{private long number=7;class Leaf{Object token(){return GetterOwner.this.token;}long number(){return Layer.this.number;}}Leaf leaf(){return new Leaf();}}Layer layer(){return new Layer();}}`

func TestNativeMemberPrivateGetterRequiresPureOriginalDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberGetterFixture)
	for _, variant := range []string{"original", "public", "instance", "non synthetic", "wrong name", "foreign receiver descriptor", "wrong stack", "wrong locals", "extra code", "effectful opcode", "wrong return", "handler", "opaque code metadata", "constant field", "public field", "static field", "duplicate code", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, _ := Parse(files["GetterOwner.class"])
			var getter *MemberInfo
			var code *CodeAttribute
			var field *MemberInfo
			for _, m := range obj.Methods {
				n, _ := obj.getUtf8(m.NameIndex)
				if n == "access$000" {
					getter = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			for _, f := range obj.Fields {
				n, _ := obj.getUtf8(f.NameIndex)
				if n == "token" {
					field = f
				}
			}
			if getter == nil || code == nil || field == nil {
				t.Fatal("original getter")
			}
			var work *workbudget.Budget
			switch variant {
			case "public":
				getter.AccessFlags |= 1
			case "instance":
				getter.AccessFlags &^= 8
			case "non synthetic":
				getter.AccessFlags &^= 0x1000
			case "wrong name":
				obj.ConstantPool[getter.NameIndex-1].(*ConstantUtf8Info).Value = "access$001"
			case "foreign receiver descriptor":
				obj.ConstantPool[getter.DescriptorIndex-1].(*ConstantUtf8Info).Value = "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "wrong stack":
				code.MaxStack++
			case "wrong locals":
				code.MaxLocals++
			case "extra code":
				code.Code = append([]byte{0}, code.Code...)
			case "effectful opcode":
				code.Code[1] = 0xb5
			case "wrong return":
				code.Code[4] = 0xad
			case "handler":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
			case "opaque code metadata":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "Opaque", Info: []byte{0}})
			case "constant field":
				field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
			case "public field":
				field.AccessFlags &^= 2
			case "static field":
				field.AccessFlags |= 8
			case "duplicate code":
				getter.Attributes = append(getter.Attributes, code)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberPrivateGetterProof(obj, getter, work); (got != nil) != (variant == "original") {
				t.Fatalf("getter proof %#v", got)
			}
		})
	}
}

func TestNativeMemberPrivateGetterRequiresClosedArchiveReferences(t *testing.T) {
	testNativeMemberPrivateGetterArchiveReferences(t, nativeMemberGetterFixture, "GetterOwner", "GetterOwner", "GetterOwner$Layer$Leaf", "token", false)
}
func TestNativeMemberAnonymousPrivateGetterRequiresClosedArchiveReferences(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousNestedContextFixture("member-context-depth"), "final Object token;final long seed;", "private final Object token;private final long seed;", 1)
	testNativeMemberPrivateGetterArchiveReferences(t, fixture, "NestedOwner", "NestedOwner$Layer$Middle", "NestedOwner$Layer$Middle$1$1", "origin", true)
}
func testNativeMemberPrivateGetterArchiveReferences(t *testing.T, fixture, rootName, getterOwner, user, methodName string, anonymous bool) {
	files := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "foreign user", "method handle", "interface reference", "unused getter", "dead reference", "wrong opcode", "own caller", "uncommitted anonymous forest", "foreign anonymous forest", "missing anonymous parent", "failed anonymous group", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files[rootName+".class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || anonymous && !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("member family")
			}
			original := z.originalMemberIndex()
			index := &nativeMemberIndex{valid: original.valid, getterUsers: map[string]map[string]bool{}, getterHandles: map[string]bool{}, getterInvalidReferences: map[string]bool{}}
			for key, users := range original.getterUsers {
				index.getterUsers[key] = map[string]bool{}
				for user := range users {
					index.getterUsers[key][user] = true
				}
			}
			key := nativeMemberGetterKey(getterOwner, "access$000", "(L"+getterOwner+";)Ljava/lang/Object;")
			var work *workbudget.Budget
			switch variant {
			case "foreign user":
				index.getterUsers[key]["Foreign"] = true
			case "method handle":
				index.getterHandles[key] = true
			case "interface reference":
				index.getterInvalidReferences[key] = true
			case "unused getter":
				delete(index.getterUsers, key)
			case "dead reference":
				index.getterUsers[key][rootName+"$Layer"] = true
			case "own caller":
				index.getterUsers[key][getterOwner] = true
			case "wrong opcode":
				object := p.lexicalObjects[user]
				if anonymous {
					object = p.anonymousForest.objects[user]
				}
				for _, m := range object.Methods {
					n, _ := sourceBridgeUTF8(object, m.NameIndex)
					if n == methodName {
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								for i, op := range c.Code {
									if op == 0xb8 {
										c.Code[i] = 0xb6
										break
									}
								}
							}
						}
					}
				}
			case "uncommitted anonymous forest", "foreign anonymous forest", "missing anonymous parent", "failed anonymous group":
				if !anonymous {
					index.getterUsers[key]["Foreign"] = true
					break
				}
				switch variant {
				case "uncommitted anonymous forest":
					p.anonymousForest = nil
				case "foreign anonymous forest":
					clone := *p.anonymousForest
					p.anonymousForest = &clone
				case "missing anonymous parent":
					delete(p.anonymousForest.units, "NestedOwner$Layer$Middle$1")
				case "failed anonymous group":
					p.anonymousUnits[user].failed = true
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberPrivateGetterReferencesClosed(p, index, work); got != (variant == "original") {
				t.Fatalf("reference closure %v", got)
			}
		})
	}
}

func TestNativeMemberPrivateGetterRequiresFinalSourceOrdinalOrder(t *testing.T) {
	first := &nativeMemberPrivateGetter{owner: "Owner", field: "a", ordinal: 0}
	second := &nativeMemberPrivateGetter{owner: "Owner$Layer", field: "b", ordinal: 100}
	good := `x/*jdec-owned-getter:0:Owner:a*/.a;y/*jdec-owned-getter:100:Owner$Layer:b*/.b;`
	for _, variant := range []string{"original", "repeated", "quoted", "line comment", "reversed", "missing", "foreign", "bad ordinal", "unclosed string", "unclosed comment", "failed family", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := &nativeMemberFamily{getters: map[string]*nativeMemberPrivateGetter{"a": first, "b": second}}
			source := good
			var work *workbudget.Budget
			accept := variant == "original" || variant == "repeated" || variant == "quoted" || variant == "line comment"
			switch variant {
			case "repeated":
				source += good
			case "quoted":
				source = `"/*jdec-owned-getter:100:Owner$Layer:b*/\\\"";` + source
			case "line comment":
				source = "// /*jdec-owned-getter:100:Owner$Layer:b*/\n" + source
			case "reversed":
				source = `y/*jdec-owned-getter:100:Owner$Layer:b*/.b;x/*jdec-owned-getter:0:Owner:a*/.a;`
			case "missing":
				source = `x/*jdec-owned-getter:0:Owner:a*/.a;`
			case "foreign":
				source += `/*jdec-owned-getter:200:Foreign:c*/`
			case "bad ordinal":
				source = strings.Replace(source, "getter:0:", "getter:00:", 1)
			case "unclosed string":
				source += `"`
			case "unclosed comment":
				source += `/*`
			case "failed family":
				p.failed = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberPrivateGetterSourceClosed(p, source, work); got != accept {
				t.Fatalf("source closure %v", got)
			}
		})
	}
}
