package javaclassparser

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestAdversarialMethodParameterNamesDeduplicatedLambdaNamespace(t *testing.T) {
	methodParameterNameRoundTrip(t, methodParameterNameCase{
		name:           "javac deduplicated lambda source namespaces",
		declaration:    `class NamedOwner{static int impl(int captured,int value){return 10*captured+value;}static java.util.function.IntUnaryOperator first(int firstNamed){return value->impl(firstNamed,value);}static java.util.function.IntUnaryOperator second(int l0){return value->impl(l0,value);}static int read(int originalNamed){return originalNamed;}}`,
		check:          `if(NamedOwner.first(7).applyAsInt(2)!=72||NamedOwner.second(7).applyAsInt(2)!=72)throw new AssertionError("distinct source namespaces");`,
		parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true,
		mutate: func(t *testing.T, object *ClassObject) {
			var implementation uint16
			rows := 0
			for _, attribute := range object.Attributes {
				if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
					for _, row := range table.BootstrapMethods {
						handle, ok := object.ConstantPool[row.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
						if !ok || implementation != 0 && implementation != handle.ReferenceIndex {
							t.Fatal("fixture must use one compiler-deduplicated implementation")
						}
						ref, ok := object.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo)
						if !ok {
							t.Fatal("original helper ref")
						}
						name, descriptor := getNameAndType(object.ConstantPool, ref.NameAndTypeIndex)
						if !strings.HasPrefix(name, "lambda$") || descriptor != "(II)I" {
							t.Fatal("original helper descriptor")
						}
						implementation = handle.ReferenceIndex
						rows++
					}
				}
			}
			if rows != 1 {
				t.Fatal("one compiler-deduplicated bootstrap row required")
			}
			var dynamic uint16
			callers := 0
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name != "first" && name != "second" {
					continue
				}
				code := methodCodeBytes(method)
				if len(code) != 7 || code[0] != 0x1a || code[1] != 0xba || code[4] != 0 || code[5] != 0 || code[6] != 0xb0 {
					t.Fatalf("original physical factory %s: %x", name, code)
				}
				index := binary.BigEndian.Uint16(code[2:4])
				if dynamic != 0 && dynamic != index {
					t.Fatal("factories must share the original dynamic constant")
				}
				dynamic = index
				callers++
			}
			if callers != 2 {
				t.Fatal("two physical callers required")
			}
			// Deliberately no mutation: this is the compiler's original protocol.
		},
	}, "none")
}
