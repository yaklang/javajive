package javaclassparser

import (
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestSourceBridgeForwarderRequiresCompleteOriginalProtocol(t *testing.T) {
	source := `class HiddenProtocolParent<E extends Comparable<E>> {
 public boolean accept(E value){return value!=null;}
 public long wide(long a,double b,E[] values,int n,float f){return a+(long)b+n+(long)f;}
 public E[] array(E[] values){return values;}
 public void nothing(E value){}
 public double real(double d,long l,E value){return d+l;}
 public int integer(int i,E value){return i;}
}public class ProtocolBridgeOwner<T extends Comparable<T>> extends HiddenProtocolParent<T>{}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ProtocolBridgeOwner.java": source}, "none", "8")
	variants := []string{"original", "cached", "nonpublic owner", "missing bridge flag", "missing synthetic flag", "static", "synchronized", "final", "abstract", "no code", "duplicate code", "signature", "nil attribute", "extra effect", "wrong receiver", "wrong argument slot", "wrong invoke kind", "wrong return", "wrong stack", "wrong locals", "handler", "bad CP member", "wrong CP member tag", "wrong CP name tag", "wrong CP descriptor tag", "wrong direct parent", "duplicate declaration", "duplicate unrelated declaration", "nil method tail", "bad method tail", "parameter count", "parameter name", "parameter flags", "parameter length", "duplicate parameter metadata", "unknown parent", "public parent", "private target", "static target", "bridge target", "final target", "duplicate target", "missing target", "wrong signature erasure", "unknown parent bound", "work", "memory", "canceled", "cached cancellation"}
	for _, methodName := range []string{"accept", "wide", "array", "nothing", "real", "integer"} {
		t.Run(methodName, func(t *testing.T) {
			cases := variants
			if methodName != "accept" {
				cases = []string{"original", "cached"}
			}
			for _, variant := range cases {
				t.Run(variant, func(t *testing.T) {
					obj, e := Parse(append([]byte(nil), files["ProtocolBridgeOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					parent, e := Parse(append([]byte(nil), files["HiddenProtocolParent.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					var m, target *MemberInfo
					var code *CodeAttribute
					var parameters *UnparsedAttribute
					for _, candidate := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, candidate.NameIndex)
						if n == methodName && candidate.AccessFlags == 0x1041 {
							m = candidate
							for _, a := range m.Attributes {
								switch x := a.(type) {
								case *CodeAttribute:
									code = x
								case *UnparsedAttribute:
									if x.Name == "MethodParameters" {
										parameters = x
									}
								}
							}
						}
					}
					for _, candidate := range parent.Methods {
						n, _ := sourceBridgeUTF8(parent, candidate.NameIndex)
						if n == methodName {
							target = candidate
						}
					}
					if m == nil || code == nil || target == nil {
						t.Fatal("independently compiled original bridge")
					}
					desc, known := sourceBridgeUTF8(obj, m.DescriptorIndex)
					if !known {
						t.Fatal("original descriptor")
					}
					at := -1
					for i := 0; i+3 < len(code.Code); i++ {
						if code.Code[i] == core.OP_INVOKESPECIAL {
							at = i
							break
						}
					}
					if at < 0 {
						t.Fatal("original invokespecial")
					}
					cp := NewConstantPoolWithConstant(&obj.ConstantPool)
					index := binary.BigEndian.Uint16(code.Code[at+1 : at+3])
					ref := cp.IndexInfo(int(index)).(*ConstantMethodrefInfo)
					nt := cp.IndexInfo(int(ref.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
					d := NewClassObjectDumper(obj)
					missing := false
					switch variant {
					case "nonpublic owner":
						obj.AccessFlags &^= 1
					case "missing bridge flag":
						m.AccessFlags &^= 0x40
					case "missing synthetic flag":
						m.AccessFlags &^= 0x1000
					case "static":
						m.AccessFlags |= 8
					case "synchronized":
						m.AccessFlags |= 0x20
					case "final":
						m.AccessFlags |= 0x10
					case "abstract":
						m.AccessFlags |= 0x400
					case "no code":
						m.Attributes = nil
					case "duplicate code":
						m.Attributes = append(m.Attributes, code)
					case "signature":
						m.Attributes = append(m.Attributes, &SignatureAttribute{})
					case "nil attribute":
						m.Attributes = append(m.Attributes, nil)
					case "extra effect":
						code.Code = append([]byte{core.OP_NOP}, code.Code...)
					case "wrong receiver":
						code.Code[0] = core.OP_ACONST_NULL
					case "wrong argument slot":
						code.Code[1] = core.OP_ALOAD_0
					case "wrong invoke kind":
						code.Code[at] = core.OP_INVOKEVIRTUAL
					case "wrong return":
						code.Code[len(code.Code)-1] = core.OP_ARETURN
					case "wrong stack":
						code.MaxStack++
					case "wrong locals":
						code.MaxLocals++
					case "handler":
						code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code)), HandlerPc: 0}}
					case "bad CP member":
						code.Code[at+1], code.Code[at+2] = 0, 0
					case "wrong CP member tag":
						obj.ConstantPool[index-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
					case "wrong CP name tag":
						obj.ConstantPool = append(obj.ConstantPool, &ConstantStringInfo{StringIndex: nt.NameIndex})
						nt.NameIndex = uint16(len(obj.ConstantPool))
					case "wrong CP descriptor tag":
						obj.ConstantPool = append(obj.ConstantPool, &ConstantStringInfo{StringIndex: nt.DescriptorIndex})
						nt.DescriptorIndex = uint16(len(obj.ConstantPool))
					case "wrong direct parent":
						obj.SuperClass = obj.ThisClass
					case "duplicate declaration":
						obj.Methods = append(obj.Methods, m)
					case "duplicate unrelated declaration":
						obj.Methods = append(obj.Methods, obj.Methods[0])
					case "nil method tail":
						obj.Methods = append(obj.Methods, nil)
					case "bad method tail":
						obj.Methods = append(obj.Methods, &MemberInfo{})
					case "parameter count", "parameter name", "parameter flags", "parameter length", "duplicate parameter metadata":
						if parameters == nil {
							t.Fatal("modern compiler did not provide synthetic parameter metadata")
						}
						switch variant {
						case "parameter count":
							parameters.Info[0]++
						case "parameter name":
							parameters.Info[2] = 1
						case "parameter flags":
							parameters.Info[3], parameters.Info[4] = 0, 0
						case "parameter length":
							parameters.Length++
						case "duplicate parameter metadata":
							m.Attributes = append(m.Attributes, parameters)
						}
					case "unknown parent":
						missing = true
					case "public parent":
						parent.AccessFlags |= 1
					case "private target":
						target.AccessFlags = 2
					case "static target":
						target.AccessFlags |= 8
					case "bridge target":
						target.AccessFlags |= 0x1040
					case "final target":
						target.AccessFlags |= 0x10
					case "duplicate target":
						parent.Methods = append(parent.Methods, target)
					case "missing target":
						for i, candidate := range parent.Methods {
							if candidate == target {
								parent.Methods = append(parent.Methods[:i], parent.Methods[i+1:]...)
								break
							}
						}
					case "wrong signature erasure":
						nativeReplaceOriginalSignature(t, parent, target.Attributes, "(Ljava/lang/Object;)Z")
					case "unknown parent bound":
						nativeReplaceOriginalSignature(t, parent, parent.Attributes, "<E:TU;>Ljava/lang/Object;")
					case "work":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					parentBytes := parent.Bytes()
					d.foldSiblingResolver = func(n string) ([]byte, bool) { return parentBytes, n == "HiddenProtocolParent" && !missing }
					query := d.buildSourceBridgeTargets()
					good := variant == "original" || variant == "cached" || variant == "cached cancellation"
					for pass := 0; pass < 2; pass++ {
						name, ok := query("ProtocolBridgeOwner", methodName, desc)
						if ok != good || ok && name != "HiddenProtocolParent" {
							t.Fatalf("pass=%d source target=%q/%v expected=%v", pass, name, ok, good)
						}
					}
					if variant == "cached cancellation" {
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
						if _, ok := query("ProtocolBridgeOwner", methodName, desc); ok {
							t.Fatal("canceled query reused cached permission")
						}
					}
					if _, ok := query("ProtocolBridgeOwner", methodName, "()Ljava/lang/Object;"); ok {
						t.Fatal("foreign descriptor borrowed capability")
					}
					// The query does not rewrite physical method declarations or descriptors.
					if original, _ := sourceBridgeUTF8(obj, m.DescriptorIndex); original != desc {
						t.Fatal("source view rewrote physical descriptor")
					}
				})
			}
		})
	}
}
