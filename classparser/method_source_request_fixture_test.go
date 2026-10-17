package javaclassparser

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// This models the successful typed renderer return used by the existing
// source-consumption mutation matrices. Its key has no method-name meaning.
func registerMethodSourceBodyFixture(t *testing.T, dumper *ClassObjectDumper, name, descriptor string, captures int, body *dumpedMethods) string {
	t.Helper()
	request, known := dumper.newLambdaMethodSourceRequest(captures)
	if !known {
		t.Fatal("fixture lambda source request")
	}
	body.sourceRequest, body.sourceDumper, body.sourceObject = request, dumper, dumper.obj
	body.sourceCacheKey = fmt.Sprintf("fixture-lambda-request:%d", request.serial)
	dumper.dumpedMethodsSet[body.sourceCacheKey] = body
	if !dumper.recordLambdaMethodSourceBody(name, descriptor, request, body) {
		t.Fatal("fixture lambda source receipt")
	}
	return body.sourceCacheKey
}

func methodParameterNonsyntheticLambdaFixture() methodParameterNameCase {
	return methodParameterNameCase{
		name:           "nonsynthetic lambda implementation",
		declaration:    `class NamedOwner{static class Anchor{}static int impl(int capture,int left,int right){return 100*capture+10*left+right;}static java.util.function.IntBinaryOperator first(int captured){return(left,right)->impl(captured,left,right);}static java.util.function.IntUnaryOperator second(int captured,int fixed){return right->impl(captured,fixed,right);}static int read(int originalNamed){return originalNamed;}}`,
		check:          `if(NamedOwner.impl(7,2,3)!=723||NamedOwner.first(7).applyAsInt(2,3)!=723||NamedOwner.second(7,2).applyAsInt(3)!=723||NamedOwner.read(9)!=9)throw new AssertionError("two physical factory captures");java.lang.reflect.Method impl=NamedOwner.class.getDeclaredMethod("impl",int.class,int.class,int.class);if(impl.isSynthetic())throw new AssertionError("ordinary nonsynthetic implementation");String[]physical={"capture","left","right"};for(int i=0;i<3;i++)if(!impl.getParameters()[i].getName().equals(physical[i]))throw new AssertionError("physical ordinary names");`,
		parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true,
		mutate: func(t *testing.T, object *ClassObject) {
			methodParameterRetargetLambdaFactories(t, object, "(III)I", 2)
		},
	}
}

func methodParameterRetargetLambdaFactories(t *testing.T, object *ClassObject, targetDescriptor string, bootstrapRows int) {
	originalNames := methodParameterNamesIndependent(t, object.Bytes())
	originalCode := map[*MemberInfo][]byte{}
	for _, method := range object.Methods {
		originalCode[method] = append([]byte(nil), methodCodeBytes(method)...)
	}
	var implRef uint16
	for i, constant := range object.ConstantPool {
		ref, ok := constant.(*ConstantMethodrefInfo)
		if !ok {
			continue
		}
		name, descriptor := getNameAndType(object.ConstantPool, ref.NameAndTypeIndex)
		owner, known := sourceBridgeClassName(object, ref.ClassIndex)
		if known && owner == "NamedOwner" && name == "impl" && descriptor == targetDescriptor {
			if implRef != 0 {
				t.Fatal("duplicate authored ordinary implementation ref")
			}
			implRef = uint16(i + 1)
		}
	}
	if implRef == 0 {
		t.Fatal("missing authored ordinary implementation ref")
	}
	var handles []*ConstantMethodHandleInfo
	for _, attribute := range object.Attributes {
		table, ok := attribute.(*BootstrapMethodsAttribute)
		if !ok {
			continue
		}
		for _, row := range table.BootstrapMethods {
			if len(row.BootstrapArguments) != 3 {
				t.Fatal("authored LambdaMetafactory arguments")
			}
			handle, ok := object.ConstantPool[row.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
			if !ok || handle.ReferenceKind != 6 {
				t.Fatal("authored static implementation handle")
			}
			ref, ok := object.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo)
			if !ok {
				t.Fatal("authored implementation method ref")
			}
			name, descriptor := getNameAndType(object.ConstantPool, ref.NameAndTypeIndex)
			if !strings.HasPrefix(name, "lambda$") || descriptor != targetDescriptor {
				t.Fatalf("unexpected authored lambda helper %s%s", name, descriptor)
			}
			handles = append(handles, handle)
		}
	}
	if len(handles) != bootstrapRows {
		t.Fatalf("expected %d authored bootstrap rows, got %d", bootstrapRows, len(handles))
	}
	// javac may share an identical helper handle between factories.
	// Validate all rows before changing either reference.
	for _, handle := range handles {
		handle.ReferenceIndex = implRef
	}
	if !reflect.DeepEqual(originalNames, methodParameterNamesIndependent(t, object.Bytes())) {
		t.Fatal("authored bootstrap change modified MethodParameters")
	}
	for method, before := range originalCode {
		if !reflect.DeepEqual(before, methodCodeBytes(method)) {
			t.Fatal("authored bootstrap change modified original Code")
		}
	}
}

func methodParameterSameCaptureLambdaFixture(reverse bool) methodParameterNameCase {
	first := `static java.util.function.IntUnaryOperator first(int firstNamed){return value->impl(firstNamed,value);}`
	second := `static java.util.function.IntUnaryOperator second(int l0){return value->impl(l0,value);}`
	factories := first + second
	if reverse {
		factories = second + first
	}
	return methodParameterNameCase{
		name:           "same capture count, different enclosing names",
		declaration:    `class NamedOwner{static int impl(int captured,int value){return 10*captured+value;}` + factories + `static int read(int originalNamed){return originalNamed;}}`,
		check:          `if(NamedOwner.impl(7,2)!=72||NamedOwner.first(7).applyAsInt(2)!=72||NamedOwner.second(7).applyAsInt(2)!=72||NamedOwner.read(9)!=9)throw new AssertionError("same-count distinct source namespaces");if(!NamedOwner.class.getDeclaredMethod("first",int.class).getParameters()[0].getName().equals("firstNamed")||!NamedOwner.class.getDeclaredMethod("second",int.class).getParameters()[0].getName().equals("l0"))throw new AssertionError("physical factory declaration identity");`,
		parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true,
		mutate: func(t *testing.T, object *ClassObject) { methodParameterRetargetLambdaFactories(t, object, "(II)I", 1) },
	}
}

func methodSourceRequestSyntheticName(t *testing.T, d *ClassObjectDumper, descriptor string) string {
	t.Helper()
	name := ""
	for _, method := range d.obj.Methods {
		n, _ := sourceBridgeUTF8(d.obj, method.NameIndex)
		desc, _ := sourceBridgeUTF8(d.obj, method.DescriptorIndex)
		if isSyntheticMethod(method.AccessFlags) && desc == descriptor {
			if name != "" {
				t.Fatal("one compiler-deduplicated synthetic member required")
			}
			name = n
		}
	}
	if name == "" {
		t.Fatal("original synthetic implementation missing")
	}
	return name
}
