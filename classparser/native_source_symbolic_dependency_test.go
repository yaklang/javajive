package javaclassparser

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An unused symbolic member or method type is a distinct CP-root problem from
// a standalone class catalog. Keep declarations and every executed operand;
// discard only the removable method before the independent original JVM runs.
func TestNativeUnusedSymbolicConstantDoesNotCreateSourceCycleRoundTrip(t *testing.T) {
	const nodes = 66
	fixture, owners := wideSourceDependencyFixture(nodes, false)
	fixture = strings.Replace(fixture, "static final int initialized=GraphDriver.mark(0);", "static final int initialized=GraphDriver.mark(0);static Marker identity(Marker value){return value;}", 1)
	last := fmt.Sprintf("static final int initialized=GraphDriver.mark(%d);", nodes-1)
	fixture = strings.Replace(fixture, last, last+"static GraphScope0.Marker unused(){return GraphScope0.Marker.identity(null);}", 1)
	for _, kind := range []string{"member reference", "name and type", "method type"} {
		t.Run(kind, func(t *testing.T) {
			for _, prefix := range []string{"GraphScope", "IndependentDeclaration"} {
				t.Run(prefix, func(t *testing.T) {
					f := strings.ReplaceAll(fixture, "GraphScope", prefix)
					names := append([]string(nil), owners...)
					for i := range names {
						names[i] = strings.ReplaceAll(names[i], "GraphScope", prefix)
					}
					mutate := func(t *testing.T, files map[string][]byte) {
						name := fmt.Sprintf("%s%d$Marker.class", prefix, nodes-1)
						object, err := Parse(files[name])
						if err != nil {
							t.Fatal(err)
						}
						removed := false
						for i, method := range object.Methods {
							methodName, known := sourceBridgeUTF8(object, method.NameIndex)
							if known && methodName == "unused" {
								object.Methods = append(object.Methods[:i], object.Methods[i+1:]...)
								removed = true
								break
							}
						}
						if !removed {
							t.Fatal("removable original method missing")
						}
						if kind != "member reference" {
							// Remove only the now-unused symbolic entries, preserving
							// their slots as valid UTF8 constants. No live bytecode or
							// declaration descriptor refers to these CP entries.
							for i, constant := range object.ConstantPool {
								if member := nativeConstantMember(constant); member != nil {
									owner, known := sourceBridgeClassName(object, member.ClassIndex)
									if known && owner == prefix+"0$Marker" {
										object.ConstantPool[i] = NewUtf8FromString("removed unused member")
									}
								}
							}
							if kind == "method type" {
								for i, constant := range object.ConstantPool {
									if nt, ok := constant.(*ConstantNameAndTypeInfo); ok && nt != nil {
										desc, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
										if known && strings.Contains(desc, "L"+prefix+"0$Marker;") {
											object.ConstantPool[i] = &ConstantMethodTypeInfo{DescriptorIndex: nt.DescriptorIndex}
										}
									}
								}
							}
						}
						files[name] = object.Bytes()
					}
					testNativeIndependentMutatedFamilyFixture(t, f, names, "GraphDriver", "66:4:wide-graph:owners:identity:init:callback:failure\n", mutate, nativeLexicalExactSignatures)
				})
			}
		})
	}
}

// These are syntactic root witnesses, not a program-equivalence oracle. In
// particular, a descriptor-only type deliberately has no CONSTANT_Class.
// Real invocation, lambda and ownership behavior has separate JVM roundtrips.
func TestNativeSourceSymbolicDependenciesFollowTypedOriginalConsumers(t *testing.T) {
	files := nativeCompileClasses(t, `class SymbolicUseRoot{static void unused(){}}`)
	for _, scenario := range []string{
		"unused member", "unused name type", "unused method type", "unused handle", "unused dynamic",
		"getstatic", "putstatic", "getfield", "putfield", "invokevirtual", "invokespecial", "invokestatic", "invokeinterface", "interface static", "interface special", "unreachable invocation",
		"ldc method type", "ldc_w method type", "ldc method handle", "ldc_w method handle", "field handle", "interface handle", "invokedynamic", "ldc dynamic", "ldc_w dynamic", "ldc2_w dynamic",
		"bootstrap ref", "bootstrap method type", "bootstrap method handle", "bootstrap dynamic", "enclosing name type",
		"wrong field tag", "wrong virtual tag", "wrong interface tag", "wrong dynamic tag", "wrong handle target", "zero bootstrap", "wrong bootstrap tag", "wrong bootstrap argument", "invalid unused descriptor", "unknown metadata",
	} {
		t.Run(scenario, func(t *testing.T) {
			object, err := Parse(files["SymbolicUseRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			pool := NewConstantPoolWithConstant(&object.ConstantPool)
			appendConstant := func(c ConstantInfo) uint16 { return uint16(pool.AppendConstantInfo(c)) }
			utf := func(s string) uint16 { return uint16(pool.AddUtf8Info(s)) }
			const owner, parameter, result = "foreign/Operator", "descriptor/Input", "descriptor/Element"
			methodDescriptor := "(L" + parameter + ";)L" + result + ";"
			nat := appendConstant(&ConstantNameAndTypeInfo{NameIndex: utf("consume"), DescriptorIndex: utf(methodDescriptor)})
			member := ConstantMemberrefInfo{ClassIndex: uint16(pool.AddNewClassInfo(owner)), NameAndTypeIndex: nat}
			ref := appendConstant(&ConstantMethodrefInfo{ConstantMemberrefInfo: member})
			methodType := appendConstant(&ConstantMethodTypeInfo{DescriptorIndex: utf(methodDescriptor)})
			handle := appendConstant(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: ref})
			bootstrapRef := uint16(pool.AddNewMethodInfo("foreign/Bootstrap", "bootstrap", "()Ljava/lang/Object;"))
			bootstrapHandle := appendConstant(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: bootstrapRef})
			code := &CodeAttribute{MaxStack: 4, MaxLocals: 1, Code: []byte{core.OP_RETURN}}
			object.Attributes = nil
			object.Methods[1].Attributes = []AttributeInfo{code}
			operand := func(op byte, index uint16) { code.Code = []byte{op, byte(index >> 8), byte(index), core.OP_RETURN} }
			bootstrap := func(args ...uint16) {
				object.Attributes = append(object.Attributes, &BootstrapMethodsAttribute{BootstrapMethods: []*BootstrapMethod{{BootstrapMethodRef: bootstrapHandle, BootstrapArguments: args}}})
			}
			wantOwner, wantDescriptor, wantKnown := false, false, true
			switch scenario {
			case "unused member", "unused name type", "unused method type", "unused handle", "unused dynamic":
				if scenario == "unused dynamic" {
					appendConstant(&ConstantInvokeDynamicInfo{NameAndTypeIndex: nat})
				}
			case "getstatic", "putstatic", "getfield", "putfield", "field handle":
				fieldNat := appendConstant(&ConstantNameAndTypeInfo{NameIndex: utf("value"), DescriptorIndex: utf("L" + result + ";")})
				fieldRef := appendConstant(&ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: member.ClassIndex, NameAndTypeIndex: fieldNat}})
				if scenario == "field handle" {
					fieldHandle := appendConstant(&ConstantMethodHandleInfo{ReferenceKind: 2, ReferenceIndex: fieldRef})
					operand(core.OP_LDC_W, fieldHandle)
				} else {
					op := map[string]byte{"getstatic": core.OP_GETSTATIC, "putstatic": core.OP_PUTSTATIC, "getfield": core.OP_GETFIELD, "putfield": core.OP_PUTFIELD}[scenario]
					operand(op, fieldRef)
				}
				wantOwner = true
			case "invokevirtual", "invokespecial", "invokestatic", "unreachable invocation":
				op := map[string]byte{"invokevirtual": core.OP_INVOKEVIRTUAL, "invokespecial": core.OP_INVOKESPECIAL, "invokestatic": core.OP_INVOKESTATIC, "unreachable invocation": core.OP_INVOKESTATIC}[scenario]
				operand(op, ref)
				if scenario == "unreachable invocation" {
					code.Code = append([]byte{core.OP_RETURN}, code.Code...)
				}
				wantOwner, wantDescriptor = true, true
			case "invokeinterface", "interface static", "interface special", "interface handle":
				interfaceRef := appendConstant(&ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: member})
				switch scenario {
				case "invokeinterface":
					code.Code = []byte{core.OP_INVOKEINTERFACE, byte(interfaceRef >> 8), byte(interfaceRef), 2, 0, core.OP_RETURN}
				case "interface static":
					operand(core.OP_INVOKESTATIC, interfaceRef)
				case "interface special":
					operand(core.OP_INVOKESPECIAL, interfaceRef)
				case "interface handle":
					operand(core.OP_LDC_W, appendConstant(&ConstantMethodHandleInfo{ReferenceKind: 9, ReferenceIndex: interfaceRef}))
				}
				wantOwner, wantDescriptor = true, true
			case "ldc method type", "ldc_w method type":
				if scenario == "ldc method type" {
					code.Code = []byte{core.OP_LDC, byte(methodType), core.OP_RETURN}
				} else {
					operand(core.OP_LDC_W, methodType)
				}
				wantDescriptor = true
			case "ldc method handle", "ldc_w method handle":
				if scenario == "ldc method handle" {
					code.Code = []byte{core.OP_LDC, byte(handle), core.OP_RETURN}
				} else {
					operand(core.OP_LDC_W, handle)
				}
				wantOwner, wantDescriptor = true, true
			case "invokedynamic":
				dynamic := appendConstant(&ConstantInvokeDynamicInfo{NameAndTypeIndex: nat})
				bootstrap()
				code.Code = []byte{core.OP_INVOKEDYNAMIC, byte(dynamic >> 8), byte(dynamic), 0, 0, core.OP_RETURN}
				wantDescriptor = true
			case "ldc dynamic", "ldc_w dynamic", "ldc2_w dynamic", "bootstrap dynamic":
				dynamicNat := appendConstant(&ConstantNameAndTypeInfo{NameIndex: utf("constant"), DescriptorIndex: utf("L" + result + ";")})
				if scenario == "ldc2_w dynamic" {
					// Category-two condy must be a primitive J/D descriptor.
					dynamicNat = appendConstant(&ConstantNameAndTypeInfo{NameIndex: utf("wide"), DescriptorIndex: utf("J")})
				}
				dynamic := appendConstant(&ConstantDynamicInfo{NameAndTypeIndex: dynamicNat})
				bootstrap()
				switch scenario {
				case "ldc dynamic":
					code.Code = []byte{core.OP_LDC, byte(dynamic), core.OP_RETURN}
				case "ldc_w dynamic":
					operand(core.OP_LDC_W, dynamic)
				case "ldc2_w dynamic":
					operand(core.OP_LDC2_W, dynamic)
				case "bootstrap dynamic":
					object.Attributes = nil
					bootstrap(dynamic)
				}
			case "bootstrap ref":
				// The bootstrap handle itself is a root, without any callsite.
				bootstrapHandle = handle
				bootstrap()
				wantOwner, wantDescriptor = true, true
			case "bootstrap method type":
				bootstrap(methodType)
				wantDescriptor = true
			case "bootstrap method handle":
				bootstrap(handle)
				wantOwner, wantDescriptor = true, true
			case "enclosing name type":
				outer := uint16(pool.AddNewClassInfo("foreign/Outer"))
				object.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "EnclosingMethod", Info: []byte{byte(outer >> 8), byte(outer), byte(nat >> 8), byte(nat)}}}
				wantDescriptor = true
			case "wrong field tag", "wrong virtual tag", "wrong interface tag", "wrong dynamic tag", "wrong handle target", "zero bootstrap", "wrong bootstrap tag", "wrong bootstrap argument", "invalid unused descriptor":
				wantKnown = false
				switch scenario {
				case "wrong field tag":
					operand(core.OP_GETFIELD, ref)
				case "wrong virtual tag":
					operand(core.OP_INVOKEVIRTUAL, appendConstant(&ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: member}))
				case "wrong interface tag":
					code.Code = []byte{core.OP_INVOKEINTERFACE, byte(ref >> 8), byte(ref), 2, 0, core.OP_RETURN}
				case "wrong dynamic tag":
					code.Code = []byte{core.OP_INVOKEDYNAMIC, byte(ref >> 8), byte(ref), 0, 0, core.OP_RETURN}
				case "wrong handle target":
					object.ConstantPool[handle-1].(*ConstantMethodHandleInfo).ReferenceIndex = methodType
					operand(core.OP_LDC_W, handle)
				case "zero bootstrap":
					bootstrapHandle = 0
					bootstrap()
				case "wrong bootstrap tag":
					bootstrapHandle = ref
					bootstrap()
				case "wrong bootstrap argument":
					bootstrap(nat)
				case "invalid unused descriptor":
					appendConstant(&ConstantMethodTypeInfo{DescriptorIndex: utf("L" + result + ";")})
				}
			case "unknown metadata":
				object.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "OpaqueStructuralMetadata"}}
				wantOwner, wantDescriptor = true, true
			}
			originalBytes := object.Bytes()
			names, known := nativeMemberSourceBindingNames(object, nil)
			if !bytes.Equal(object.Bytes(), originalBytes) {
				t.Fatal("source dependency query modified original class bytes")
			}
			if known != wantKnown || !known && names != nil {
				t.Fatalf("consumer known=%v names=%v want=%v", known, names, wantKnown)
			}
			if !known {
				return
			}
			originalNames, originalKnown := nativeMemberDependencyNames(object, nil)
			if !originalKnown || !slices.Contains(originalNames, owner) || !slices.Contains(originalNames, parameter) || !slices.Contains(originalNames, result) {
				t.Fatal("source mask removed original archive/index dependencies", originalKnown, originalNames)
			}
			wantResult := wantDescriptor || wantOwner && (scenario == "getstatic" || scenario == "putstatic" || scenario == "getfield" || scenario == "putfield" || scenario == "field handle") || scenario == "ldc dynamic" || scenario == "ldc_w dynamic" || scenario == "bootstrap dynamic"
			if slices.Contains(names, owner) != wantOwner || slices.Contains(names, parameter) != wantDescriptor || slices.Contains(names, result) != wantResult {
				t.Fatalf("typed roots %q: %v; want owner=%v argument=%v result=%v", scenario, names, wantOwner, wantDescriptor, wantResult)
			}
		})
	}
}

func TestNativeSourceDescriptorCacheKeepsEachDeclarationObligation(t *testing.T) {
	for _, scenario := range []string{"unused only", "field after unused", "method after unused", "array after unused", "second rooted symbol", "many rooted symbols", "wrong method role"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "Ldescriptor/Leaf;"
			if scenario == "method after unused" {
				desc = "(Ldescriptor/Leaf;)Ldescriptor/Result;"
			}
			object := &ClassObject{ConstantPool: []ConstantInfo{NewUtf8FromString(desc), &ConstantNameAndTypeInfo{DescriptorIndex: 1}}}
			mask := map[uint16]bool{}
			want, known := []string{"descriptor/Leaf"}, true
			switch scenario {
			case "unused only":
				want = nil
			case "field after unused":
				object.Fields = []*MemberInfo{{DescriptorIndex: 1}}
			case "method after unused":
				object.Methods = []*MemberInfo{{DescriptorIndex: 1}}
				want = append(want, "descriptor/Result")
			case "array after unused":
				object.ConstantPool[0] = NewUtf8FromString("[[" + desc)
				object.ConstantPool = append(object.ConstantPool, &ConstantClassInfo{NameIndex: 1})
			case "second rooted symbol":
				object.ConstantPool = append(object.ConstantPool, &ConstantNameAndTypeInfo{DescriptorIndex: 1})
				mask[3] = true
			case "many rooted symbols":
				for i := 0; i < 1024; i++ {
					object.ConstantPool = append(object.ConstantPool, &ConstantNameAndTypeInfo{DescriptorIndex: 1})
					mask[uint16(i+3)] = true
				}
			case "wrong method role":
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodTypeInfo{DescriptorIndex: 1})
				mask[3], known = true, false
			}
			work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: int64(len(object.ConstantPool) + 2*len(desc) + 40)})
			names, gotKnown := nativeMemberDependencyNamesWithConstantMasks(object, "", nil, mask, work)
			slices.Sort(names)
			if gotKnown != known || !known && names != nil || known && !slices.Equal(names, want) {
				t.Fatalf("descriptor obligation %q known=%v names=%v err=%v, want=%v %v", scenario, gotKnown, names, work.Err(), known, want)
			}
		})
	}
}

func TestNativeSourceLocalDescriptorsUseRuntimeArrayGrammar(t *testing.T) {
	files := nativeCompileClasses(t, `class LocalDescriptorRoot{static void unused(){}}`)
	for _, dimensions := range []int{0, 1, 128, 129, 254, 255, 256} {
		t.Run(fmt.Sprint(dimensions), func(t *testing.T) {
			object, err := Parse(files["LocalDescriptorRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			pool := NewConstantPoolWithConstant(&object.ConstantPool)
			index := uint16(pool.AddUtf8Info(strings.Repeat("[", dimensions) + "Ldescriptor/LocalLeaf;"))
			// The descriptor index is a UTF8-only local use, not a class
			// constant, declaration or generic signature root.
			info := []byte{0, 1, 0, 0, 0, 1, 0, 1, byte(index >> 8), byte(index), 0, 0}
			object.Methods[1].Attributes = []AttributeInfo{&CodeAttribute{Code: []byte{core.OP_RETURN}, Attributes: []AttributeInfo{&UnparsedAttribute{Name: "LocalVariableTable", Info: info}}}}
			names, known := nativeMemberSourceBindingNames(object, nil)
			if known != (dimensions <= 255) || !known && names != nil || known && !slices.Contains(names, "descriptor/LocalLeaf") {
				t.Fatalf("local dimensions %d known=%v names=%v", dimensions, known, names)
			}
		})
	}
}
