package javaclassparser

import (
	"strings"
	"testing"
)

const independentOpaqueBootstrapSource = `
public static java.lang.invoke.CallSite opaqueFactory(java.lang.invoke.MethodHandles.Lookup lookup,String name,java.lang.invoke.MethodType factory,java.lang.invoke.MethodType sam,java.lang.invoke.MethodHandle implementation,java.lang.invoke.MethodType instantiated)throws java.lang.invoke.LambdaConversionException{return java.lang.invoke.LambdaMetafactory.metafactory(lookup,name,factory,sam,implementation,instantiated);}
public static java.lang.invoke.CallSite opaqueLiteral(java.lang.invoke.MethodHandles.Lookup lookup,String name,java.lang.invoke.MethodType type){return new java.lang.invoke.ConstantCallSite(java.lang.invoke.MethodHandles.constant(String.class,"opaque-body").asType(type));}
`

// These parents still require a source completion decision after local metadata
// admission. A valid custom bootstrap delegates to the original metafactory;
// the unchanged original driver executes it, but the decompiler has no declared
// source protocol for that custom bootstrap. Known incompatible compiler
// metadata would now be an earlier, independent flat-layout refusal instead.
func compileIndependentOpaqueParentFixture(t *testing.T, compiler, source, debug string) map[string][]byte {
	t.Helper()
	source = strings.ReplaceAll(source, "static IntUnaryOperator unrelated(", independentOpaqueBootstrapSource+"static IntUnaryOperator unrelated(")
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, source, "CyclicA", debug, true)
	const descriptor = "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;"
	for _, name := range []string{"CyclicA", "CyclicB"} {
		object, err := Parse(files[name+".class"])
		if err != nil {
			t.Fatal(err)
		}
		pool := NewConstantPoolWithConstant(&object.ConstantPool)
		ref := uint16(pool.AddNewMethodInfo("CyclicA", "opaqueFactory", descriptor))
		handle := uint16(pool.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: ref}))
		patched := 0
		for _, attribute := range object.Attributes {
			if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
				for _, row := range table.BootstrapMethods {
					row.BootstrapMethodRef = handle
					patched++
				}
			}
		}
		if patched != 1 {
			t.Fatal("original fixture factory count", name, patched)
		}
		files[name+".class"] = object.Bytes()
	}
	return files
}

// A late body refusal needs a valid executable body that local compiler-role
// admission cannot decide. This zero-argument call site returns the same literal
// as the authored method while retaining the custom bootstrap linkage protocol.
func installIndependentOpaqueLiteralBody(t *testing.T, files map[string][]byte, owner, name string) map[string][]byte {
	t.Helper()
	object, err := Parse(files[owner+".class"])
	if err != nil {
		t.Fatal(err)
	}
	pool := NewConstantPoolWithConstant(&object.ConstantPool)
	pool.AddUtf8Info("BootstrapMethods")
	ref := uint16(pool.AddNewMethodInfo("CyclicA", "opaqueLiteral", "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;"))
	handle := uint16(pool.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: ref}))
	for _, attribute := range object.Attributes {
		if _, ok := attribute.(*BootstrapMethodsAttribute); ok {
			t.Fatal("late literal fixture already has bootstrap methods")
		}
	}
	object.Attributes = append(object.Attributes, &BootstrapMethodsAttribute{AttributeLength: 6, NumBootstrapMethods: 1, BootstrapMethods: []*BootstrapMethod{{BootstrapMethodRef: handle}}})
	nat := uint16(pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(pool.AddUtf8Info("opaqueValue")), DescriptorIndex: uint16(pool.AddUtf8Info("()Ljava/lang/String;"))}))
	dynamic := uint16(pool.AppendConstantInfo(&ConstantInvokeDynamicInfo{BootstrapMethodAttrIndex: 0, NameAndTypeIndex: nat}))
	patched := 0
	for _, method := range object.Methods {
		methodName, _ := sourceBridgeUTF8(object, method.NameIndex)
		if methodName != name {
			continue
		}
		for i, attribute := range method.Attributes {
			if _, ok := attribute.(*CodeAttribute); ok {
				method.Attributes[i] = &CodeAttribute{AttrLen: 18, MaxStack: 1, MaxLocals: 1, Code: []byte{0xba, byte(dynamic >> 8), byte(dynamic), 0, 0, 0xb0}}
				patched++
			}
		}
	}
	if patched != 1 {
		t.Fatal("late literal method count", patched)
	}
	files[owner+".class"] = object.Bytes()
	return files
}
