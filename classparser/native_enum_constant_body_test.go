package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

const nativeEnumConstantPacketFixture = `public class ConstantPacketOwner{public enum Mode{FIRST(7,0.25){public long apply(long x){return x+1;}},LAST(9,-0.0){public long apply(long x){return -x;}};final long step;final double bias;Mode(long step,double bias){this.step=step;this.bias=bias;}public abstract long apply(long x);}}`

// Mutate original compiled packets, not proposed source text. Each distinct
// counterexample removes a fact needed to erase the compiler constructor or
// to regenerate the constant class with its original owner and ordinal.
// Parse retains Code slices into its input: each case must own fresh bytes,
// otherwise an earlier bad opcode can make an unrelated later guard pass.
func TestNativeEnumConstantBodyRequiresCompleteOriginalPacket(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileSourceReleaseClasses(t, map[string]string{"ConstantPacketOwner.java": nativeEnumConstantPacketFixture}, debug, "8")
		variants := []string{"original", "nil parent", "nil resolver", "missing body", "foreign body", "wrong body flags", "wrong super", "wrong ordinal", "ordinary field", "interface", "duplicate ctor", "missing ctor", "ctor access", "ctor descriptor", "missing code", "duplicate code", "handler", "harmless nop", "extra abrupt effect", "receiver", "wide slot", "dummy", "super call", "stack", "locals", "missing bridge", "wrong bridge target", "unknown ctor attribute", "generic constructor signature", "missing parameters", "named parameter", "parameter flags", "parameter size", "opaque code metadata", "code type annotation", "unknown class attribute", "duplicate table", "missing self", "duplicate self", "named self", "wrong self flags", "duplicate enclosure", "source file size", "work", "memory", "canceled"}
		for _, variant := range variants {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				objects := map[string]*ClassObject{}
				for path, raw := range files {
					obj, e := Parse(bytes.Clone(raw))
					if e != nil {
						t.Fatal(e)
					}
					objects[path[:len(path)-6]] = obj
				}
				parent := objects["ConstantPacketOwner$Mode"]
				obj := objects["ConstantPacketOwner$Mode$1"]
				resolve := func(name string) (*ClassObject, bool) { v, ok := objects[name]; return v, ok }
				reader := NewClassObjectDumper(parent)
				allocations, e := reader.nativeEnumConstantInitializationsWithDeclarations(resolve)
				if e != nil {
					t.Fatal(e)
				}
				plan := allocations["FIRST"]
				bridges := reader.nativeConstructorAccessBridges()
				var ctor *MemberInfo
				var code *CodeAttribute
				var params *UnparsedAttribute
				var rows *InnerClassesAttribute
				var self *InnerClassInfo
				var enclosing AttributeInfo
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == "<init>" {
						ctor = m
						for _, a := range m.Attributes {
							switch a := a.(type) {
							case *CodeAttribute:
								code = a
							case *UnparsedAttribute:
								if a.Name == "MethodParameters" {
									params = a
								}
							}
						}
					}
				}
				for _, a := range obj.Attributes {
					switch a := a.(type) {
					case *InnerClassesAttribute:
						rows = a
						for _, row := range a.Classes {
							n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if n == obj.GetClassName() {
								self = row
							}
						}
					case *UnparsedAttribute:
						if a.Name == "EnclosingMethod" {
							enclosing = a
						}
					}
				}
				if ctor == nil || code == nil || params == nil || rows == nil || self == nil || enclosing == nil || len(bridges) != 1 {
					t.Fatalf("incomplete genuine fixture ctor=%v code=%v params=%v rows=%v self=%v enclosing=%v bridges=%d", ctor != nil, code != nil, params != nil, rows != nil, self != nil, enclosing != nil, len(bridges))
				}
				removeAttr := func(target AttributeInfo) {
					var out []AttributeInfo
					for _, a := range ctor.Attributes {
						if a != target {
							out = append(out, a)
						}
					}
					ctor.Attributes = out
				}
				var work *workbudget.Budget
				ordinal := 1
				switch variant {
				case "nil parent":
					parent = nil
				case "nil resolver":
					resolve = nil
				case "missing body":
					delete(objects, plan.allocatedClass)
				case "foreign body":
					objects[plan.allocatedClass] = objects["ConstantPacketOwner"]
				case "wrong body flags":
					obj.AccessFlags &^= 0x4000
				case "wrong super":
					obj.SuperClass = obj.ThisClass
				case "wrong ordinal":
					ordinal = 2
				case "ordinary field":
					obj.Fields = append(obj.Fields, &MemberInfo{})
				case "interface":
					obj.Interfaces = append(obj.Interfaces, obj.SuperClass)
				case "duplicate ctor":
					obj.Methods = append(obj.Methods, ctor)
				case "missing ctor":
					obj.Methods = obj.Methods[1:]
				case "ctor access":
					ctor.AccessFlags = 1
				case "ctor descriptor":
					ctor.DescriptorIndex = sourceBridgePoolString(t, obj, "(Ljava/lang/String;I)V")
				case "missing code":
					removeAttr(code)
				case "duplicate code":
					ctor.Attributes = append(ctor.Attributes, code)
				case "handler":
					code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: 1, HandlerPc: 0})
				case "harmless nop":
					code.Code = append([]byte{byte(core.OP_NOP)}, code.Code...)
				case "extra abrupt effect":
					code.Code = append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_ATHROW)}, code.Code...)
				case "receiver":
					code.Code[0] = byte(core.OP_ALOAD_1)
				case "wide slot":
					code.Code[3] = byte(core.OP_LLOAD_2)
				case "dummy":
					code.Code[len(code.Code)-5] = byte(core.OP_ICONST_0)
				case "super call":
					code.Code[len(code.Code)-4] = byte(core.OP_INVOKEVIRTUAL)
				case "stack":
					code.MaxStack++
				case "locals":
					code.MaxLocals++
				case "missing bridge":
					bridges = nil
				case "wrong bridge target":
					for _, b := range bridges {
						b.target = "(Ljava/lang/String;I)V"
					}
				case "unknown ctor attribute":
					ctor.Attributes = append(ctor.Attributes, &DeprecatedAttribute{})
				case "generic constructor signature":
					ctor.Attributes = append(ctor.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "(JD)V")})
				case "missing parameters":
					removeAttr(params)
				case "named parameter":
					i := sourceBridgePoolString(t, obj, "hidden")
					params.Info[1], params.Info[2] = byte(i>>8), byte(i)
				case "parameter flags":
					params.Info[3] = 0
				case "parameter size":
					params.Length++
				case "opaque code metadata":
					code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "Opaque"})
				case "code type annotation":
					code.Attributes = append(code.Attributes, &RuntimeVisibleTypeAnnotationsAttribute{})
				case "unknown class attribute":
					obj.Attributes = append(obj.Attributes, &DeprecatedAttribute{})
				case "duplicate table":
					obj.Attributes = append(obj.Attributes, rows)
				case "missing self":
					for i, row := range rows.Classes {
						if row == self {
							rows.Classes = append(rows.Classes[:i], rows.Classes[i+1:]...)
							break
						}
					}
				case "duplicate self":
					rows.Classes = append(rows.Classes, self)
				case "named self":
					self.InnerNameIndex = sourceBridgePoolString(t, obj, "Wrong")
				case "wrong self flags":
					self.InnerClassAccessFlags ^= 1
				case "duplicate enclosure":
					obj.Attributes = append(obj.Attributes, enclosing)
				case "source file size":
					obj.Attributes = append(obj.Attributes, &SourceFileAttribute{AttrLen: 3})
				case "work":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				got := nativeEnumConstantBodyProof(parent, plan, ordinal, bridges, resolve, work)
				if variant == "missing parameters" && got != nil && !got.legacyConstructorMetadata {
					t.Fatal("optional parameter metadata regeneration difference not reported")
				}
				if (got != nil) != (variant == "original" || variant == "harmless nop" || variant == "missing parameters") {
					t.Fatalf("constant constructor admission=%v", got != nil)
				}
			})
		}
	}
}

func TestNativeEnumConstantArchiveRejectsUnownedExecutableTypes(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ConstantPacketOwner.java": nativeEnumConstantPacketFixture}, "none", "8")
	originalArchive := nativeArchive(t, files)
	defer originalArchive.Close()
	root, e := Parse(bytes.Clone(files["ConstantPacketOwner.class"]))
	if e != nil {
		t.Fatal(e)
	}
	p := originalArchive.nativeMemberReader(root).planNativeMemberFamily()
	if p == nil || len(p.enumConstants) != 2 {
		t.Fatal("genuine family ownership")
	}
	for _, variant := range []string{"original", "unused class reference", "foreign allocation", "class literal", "array class literal", "cast", "array cast", "instanceof", "array allocation", "multiarray allocation", "typed method", "typed field", "method type", "handle", "wrong call descriptor", "wrong allocation PC", "wrong invocation PC", "nil family", "nil index", "invalid index", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(bytes.Clone(files["ConstantPacketOwner.class"]))
			if e != nil {
				t.Fatal(e)
			}
			const body = "ConstantPacketOwner$Mode$1"
			poolClass := func(name string) uint16 {
				for i, c := range obj.ConstantPool {
					if c, ok := c.(*ConstantClassInfo); ok {
						n, _ := sourceBridgeUTF8(obj, c.NameIndex)
						if n == name {
							return uint16(i + 1)
						}
					}
				}
				nameIndex := sourceBridgePoolString(t, obj, name)
				obj.ConstantPool = append(obj.ConstantPool, &ConstantClassInfo{NameIndex: nameIndex})
				return uint16(len(obj.ConstantPool))
			}
			classIndex := poolClass(body)
			operand := []byte{byte(classIndex >> 8), byte(classIndex)}
			var ctor *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
					for _, a := range m.Attributes {
						if a, ok := a.(*CodeAttribute); ok {
							code = a
						}
					}
				}
			}
			if ctor == nil || code == nil {
				t.Fatal("original root constructor")
			}
			inject := func(packet []byte) {
				code.Code = append(append(append([]byte{}, code.Code[:len(code.Code)-1]...), packet...), byte(core.OP_RETURN))
				code.MaxStack = 2
				code.AttrLen += uint32(len(packet))
			}
			family := *p
			family.enumConstants = map[string]*nativeEnumConstantBody{}
			for n, b := range p.enumConstants {
				copy := *b
				family.enumConstants[n] = &copy
			}
			switch variant {
			case "unused class reference":
				poolClass("[L" + body + ";")
			case "foreign allocation":
				inject(append(append([]byte{byte(core.OP_NEW)}, operand...), byte(core.OP_POP)))
			case "class literal":
				inject(append(append([]byte{byte(core.OP_LDC_W)}, operand...), byte(core.OP_POP)))
			case "array class literal":
				i := poolClass("[L" + body + ";")
				inject([]byte{byte(core.OP_LDC_W), byte(i >> 8), byte(i), byte(core.OP_POP)})
			case "cast":
				inject(append(append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_CHECKCAST)}, operand...), byte(core.OP_POP)))
			case "array cast":
				i := poolClass("[L" + body + ";")
				inject([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_CHECKCAST), byte(i >> 8), byte(i), byte(core.OP_POP)})
			case "instanceof":
				inject(append(append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_INSTANCEOF)}, operand...), byte(core.OP_POP)))
			case "array allocation":
				inject(append(append([]byte{byte(core.OP_ICONST_0), byte(core.OP_ANEWARRAY)}, operand...), byte(core.OP_POP)))
			case "multiarray allocation":
				i := poolClass("[[L" + body + ";")
				inject([]byte{byte(core.OP_ICONST_0), byte(core.OP_ICONST_0), byte(core.OP_MULTIANEWARRAY), byte(i >> 8), byte(i), 2, byte(core.OP_POP)})
			case "typed method":
				ctor.DescriptorIndex = sourceBridgePoolString(t, obj, "(L"+body+";)V")
			case "typed field":
				obj.Fields = append(obj.Fields, &MemberInfo{NameIndex: sourceBridgePoolString(t, obj, "hidden"), DescriptorIndex: sourceBridgePoolString(t, obj, "L"+body+";")})
			case "method type":
				descriptorIndex := sourceBridgePoolString(t, obj, "(L"+body+";)V")
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodTypeInfo{DescriptorIndex: descriptorIndex})
			case "handle":
				nt := &ConstantNameAndTypeInfo{NameIndex: sourceBridgePoolString(t, obj, "apply"), DescriptorIndex: sourceBridgePoolString(t, obj, "(J)J")}
				obj.ConstantPool = append(obj.ConstantPool, nt)
				member := &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: classIndex, NameAndTypeIndex: uint16(len(obj.ConstantPool))}}
				obj.ConstantPool = append(obj.ConstantPool, member)
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 5, ReferenceIndex: uint16(len(obj.ConstantPool))})
			case "wrong call descriptor":
				family.enumConstants[body].descriptor = "(Ljava/lang/String;I)V"
			case "wrong allocation PC":
				family.enumConstants[body].plan.newPC++
			case "wrong invocation PC":
				family.enumConstants[body].plan.invokePC++
			}
			candidate := map[string][]byte{}
			for path, raw := range files {
				candidate[path] = raw
			}
			candidate["ConstantPacketOwner.class"] = obj.Bytes()
			z := nativeArchive(t, candidate)
			defer z.Close()
			index := z.originalMemberIndex()
			if !index.valid {
				t.Fatal("metadata-only mutated archive index must remain readable")
			}
			request := &family
			var work *workbudget.Budget
			switch variant {
			case "nil family":
				request = nil
			case "nil index":
				index = nil
			case "invalid index":
				index.valid = false
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := z.nativeEnumConstantsArchiveClosed(request, index, work)
			if got != (variant == "original" || variant == "unused class reference") {
				t.Fatalf("archive type ownership admission=%v", got)
			}
		})
	}
}
