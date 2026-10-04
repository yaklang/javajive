package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestPrimitiveFunctionalTypeChecksEntireSAM(t *testing.T) {
	for _, tc := range []struct {
		name, descriptor string
		valid            bool
	}{
		{"ToIntFunction", "(Ljava/lang/String;)I", true},
		{"ToLongFunction", "(Ljava/nio/ByteBuffer;)J", true},
		{"ToDoubleFunction", "(Ljava/lang/Double;)D", true},
		{"ToIntBiFunction", "(Ljava/lang/String;Ljava/lang/Integer;)I", true},
		{"ToLongBiFunction", "(Ljava/lang/String;Ljava/lang/Integer;)J", true},
		{"ToDoubleBiFunction", "(Ljava/lang/String;Ljava/lang/Integer;)D", true},
		{"IntFunction", "(I)[Ljava/lang/String;", true},
		{"LongFunction", "(J)Ljava/lang/String;", true},
		{"DoubleFunction", "(D)Ljava/lang/String;", true},
		{"ObjIntConsumer", "(Ljava/lang/StringBuilder;I)V", true},
		{"ObjLongConsumer", "(Ljava/lang/StringBuilder;J)V", true},
		{"ObjDoubleConsumer", "(Ljava/lang/StringBuilder;D)V", true},
		{"ToLongFunction", "(Ljava/lang/String;)I", false},
		{"ToLongFunction", "(J)J", false},
		{"ToLongFunction", "(Ljava/lang/String;Ljava/lang/String;)J", false},
		{"ToLongBiFunction", "(Ljava/lang/String;)J", false},
		{"IntFunction", "(I)V", false},
		{"IntFunction", "(J)Ljava/lang/String;", false},
		{"ObjDoubleConsumer", "(Ljava/lang/String;D)D", false},
		{"Unknown", "(Ljava/lang/String;)J", false},
	} {
		t.Run(tc.name+tc.descriptor, func(t *testing.T) {
			method, err := types.ParseMethodDescriptor(tc.descriptor)
			if err != nil {
				t.Fatal(err)
			}
			got := inferPrimitiveFunctionalType("java.util.function."+tc.name, method.FunctionType())
			if (got != nil) != tc.valid {
				t.Fatalf("inferred=%v", got)
			}
		})
	}
}
