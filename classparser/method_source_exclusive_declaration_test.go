package javaclassparser

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// These are newly authored classfile protocols. javac compiles the reviewed
// source and unchanged Driver; the authoring step below gives the implementation
// an observable synthetic declaration and makes it the actual metafactory
// target. The original JVM must accept that program before source is requested.
func TestAdversarialMethodSourceSyntheticFlagDoesNotConsumeDeclaration(t *testing.T) {
	for _, access := range []string{"public", "private"} {
		for _, name := range []string{"impl", "lambda$authored", "alpha$implementation"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(access+"/"+name+"/"+debug, func(t *testing.T) {
					wrapper, invoke := "", name
					flags := uint16(0x1009)
					if access == "private" {
						flags = 0x100a
						wrapper, invoke = `static int call(int captured,int value){return `+name+`(captured,value);}`, "call"
					}
					methodParameterNameRoundTrip(t, methodParameterNameCase{
						name:           access + " synthetic declaration " + name,
						declaration:    `class NamedOwner{` + access + ` static int ` + name + `(int captured,int value){return 10*captured+value;}` + wrapper + `static java.util.function.IntUnaryOperator first(int captured){return value->` + name + `(captured,value);}static int read(int originalNamed){return originalNamed;}}`,
						check:          `if(NamedOwner.` + invoke + `(7,2)!=72||NamedOwner.first(7).applyAsInt(2)!=72)throw new AssertionError("original invocation");java.lang.reflect.Method m=NamedOwner.class.getDeclaredMethod("` + name + `",int.class,int.class);if(!m.isSynthetic()||m.getModifiers()!=` + fmt.Sprint(flags) + `)throw new AssertionError("synthetic declaration");if(m.getParameters()[0].isNamePresent())throw new AssertionError("synthetic physical names");`,
						parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true,
						refuseSource: true,
						mutate: func(t *testing.T, object *ClassObject) {
							authorObservableSyntheticLambdaDeclaration(t, object, name, flags)
						},
					}, debug)
				})
			}
		}
	}
}

func authorObservableSyntheticLambdaDeclaration(t *testing.T, object *ClassObject, name string, flags uint16) {
	t.Helper()
	var implementation *MemberInfo
	var targetRef uint16
	before := map[*MemberInfo][]byte{}
	for _, method := range object.Methods {
		before[method] = append([]byte(nil), methodCodeBytes(method)...)
		n, _ := sourceBridgeUTF8(object, method.NameIndex)
		d, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		if n == name && d == "(II)I" {
			if implementation != nil {
				t.Fatal("one authored target")
			}
			implementation = method
		}
	}
	for index, constant := range object.ConstantPool {
		ref, ok := constant.(*ConstantMethodrefInfo)
		if !ok {
			continue
		}
		owner, known := sourceBridgeClassName(object, ref.ClassIndex)
		n, d := getNameAndType(object.ConstantPool, ref.NameAndTypeIndex)
		if known && owner == "NamedOwner" && n == name && d == "(II)I" {
			if targetRef != 0 {
				t.Fatal("one authored target reference")
			}
			targetRef = uint16(index + 1)
		}
	}
	if implementation == nil || targetRef == 0 || implementation.AccessFlags|0x1000 != flags {
		t.Fatal("authored original target")
	}
	var helper string
	for _, attribute := range object.Attributes {
		if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
			if len(table.BootstrapMethods) != 1 || table.NumBootstrapMethods != 1 {
				t.Fatal("one authored factory row")
			}
			row := table.BootstrapMethods[0]
			if len(row.BootstrapArguments) != 3 || row.NumBootstrapArguments != 3 {
				t.Fatal("authored factory arity")
			}
			handle, ok := object.ConstantPool[row.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
			if !ok || handle.ReferenceKind != 6 {
				t.Fatal("authored factory static handle")
			}
			ref, ok := object.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo)
			if !ok {
				t.Fatal("authored original helper reference")
			}
			n, d := getNameAndType(object.ConstantPool, ref.NameAndTypeIndex)
			if helper != "" || !strings.HasPrefix(n, "lambda$first$") || d != "(II)I" {
				t.Fatal("authored original helper")
			}
			helper = n
			handle.ReferenceIndex = targetRef
		}
	}
	if helper == "" {
		t.Fatal("authored factory missing")
	}
	implementation.AccessFlags = flags
	var attributes []AttributeInfo
	removedMP := 0
	for _, attribute := range implementation.Attributes {
		if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
			removedMP++
		} else {
			attributes = append(attributes, attribute)
		}
	}
	if removedMP != 1 {
		t.Fatal("one authored synthetic parameter table")
	}
	implementation.Attributes = attributes
	var methods []*MemberInfo
	removedHelper := 0
	for _, method := range object.Methods {
		n, _ := sourceBridgeUTF8(object, method.NameIndex)
		if n == helper {
			removedHelper++
			continue
		}
		if !reflect.DeepEqual(before[method], methodCodeBytes(method)) {
			t.Fatal("authoring changed retained original Code")
		}
		methods = append(methods, method)
	}
	if removedHelper != 1 {
		t.Fatal("one unreferenced authored compiler helper")
	}
	object.Methods = methods
}
