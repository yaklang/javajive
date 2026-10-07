package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNativeSourceArrayAssignmentVisitsEveryActualOperandAndNoAbsentLocalTarget(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	for _, variant := range []string{"valid", "local target also present", "declaration", "missing array", "missing index", "missing rhs", "typed nil array", "typed nil index", "typed nil rhs", "missing all targets"} {
		t.Run(variant, func(t *testing.T) {
			array := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaArrayType(integer))
			index, rhs := values.NewJavaLiteral(0, integer), values.NewJavaLiteral(7, integer)
			target := values.NewJavaArrayMember(array, index)
			store := statements.NewArrayMemberAssignStatement(target, rhs)
			switch variant {
			case "local target also present":
				store.LeftValue = array
			case "declaration":
				store.IsDeclare = true
			case "missing array":
				target.Object = nil
			case "missing index":
				target.Index = nil
			case "missing rhs":
				store.JavaValue = nil
			case "typed nil array":
				target.Object = (*values.JavaRef)(nil)
			case "typed nil index":
				target.Index = (*values.JavaLiteral)(nil)
			case "typed nil rhs":
				store.JavaValue = (*values.JavaLiteral)(nil)
			case "missing all targets":
				store.ArrayMember = nil
			}
			for _, visitor := range []func(statements.Statement) ([]values.JavaValue, [][]statements.Statement, bool){catchSourceChildren, nativeSourceNameChildren} {
				roots, children, known := visitor(store)
				if known != (variant == "valid") {
					t.Fatalf("variant %s known=%v", variant, known)
				}
				if known && (len(roots) != 2 || roots[0] != target || roots[1] != rhs || len(children) != 0) {
					t.Fatalf("actual operands lost: %#v", roots)
				}
			}
		})
	}
}

// Enumerate the independent write-set oracle: replacing any syntactic operand
// with a write to the captured ID must fail. Reads/element mutation retain the
// local binding. Opaque and cyclic expressions remain unproved.
func TestNativeArrayStoresPreserveCaptureIdentityWithoutHidingOperandWrites(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	for mask := 0; mask < 8; mask++ {
		t.Run(string(rune('0'+mask)), func(t *testing.T) {
			captured := values.NewJavaRef(coreutils.NewRootVariableId(), nil, integer)
			captured.IsParam = true
			array := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaArrayType(integer))
			array.IsParam = true
			field := "val$kept"
			child := &nativeAnonymousClass{descriptor: "(I)V", method: "make(I[I)Ljava/lang/Object;", fields: map[string]int{field: 0}}
			family := &nativeAnonymousFamily{owner: "ArrayProof", children: map[string]*nativeAnonymousClass{"ArrayProof$1": child}}
			c := &ClassObjectDumper{nativeAnonymousRoot: family, FuncCtx: &class_context.ClassContext{ClassName: "ArrayProof", FunctionName: "make", CurrentMethodDesc: child.method[len("make"):]}}
			var object, index, rhs values.JavaValue = array, values.NewJavaLiteral(0, integer), captured
			write := func() values.JavaValue {
				return &values.AssignmentExpression{Target: captured, Value: values.NewJavaLiteral(1, integer)}
			}
			if mask&1 != 0 {
				object = write()
			}
			if mask&2 != 0 {
				index = write()
			}
			if mask&4 != 0 {
				rhs = write()
			}
			// The array-object write models declaration stability only; JVM type
			// validity is a separate obligation and cannot grant this proof.
			store := statements.NewArrayMemberAssignStatement(values.NewJavaArrayMember(object, index), rhs)
			call := &values.FunctionCallExpression{ClassName: "ArrayProof$1", FunctionName: "<init>", Descriptor: "(I)V", Arguments: []values.JavaValue{captured}, OriginPC: 8, HasOriginPC: true}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("ArrayProof$1"), ConstructorCall: call, OriginPC: 4, HasOriginPC: true}
			body := []statements.Statement{store, statements.NewReturnStatement(allocation)}
			c.prepareNativeCaptureBindings(body, []values.JavaValue{captured, array})
			if family.failed != (mask != 0) {
				t.Fatalf("write mask=%d failed=%v", mask, family.failed)
			}
			if mask == 0 && (c.FuncCtx.SourceCaptureStable == nil || !c.FuncCtx.SourceCaptureStable(8, captured.Id)) {
				t.Fatal("lost original call-site identity")
			}
		})
	}
}

func TestNativeArrayStoreUnknownOperandsCannotProveUnwrittenBinding(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	for _, operand := range []string{"array", "index", "rhs"} {
		for _, invalid := range []string{"opaque", "cycle"} {
			t.Run(operand+"/"+invalid, func(t *testing.T) {
				ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, integer)
				array := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaArrayType(integer))
				var object, index, rhs values.JavaValue = array, values.NewJavaLiteral(0, integer), ref
				var unknown values.JavaValue = &values.CustomValue{}
				if invalid == "cycle" {
					cycle := &values.JavaExpression{}
					cycle.Values = []values.JavaValue{cycle}
					unknown = cycle
				}
				switch operand {
				case "array":
					object = unknown
				case "index":
					index = unknown
				case "rhs":
					rhs = unknown
				}
				// Build the malformed graph directly: constructor/type inference
				// is outside this source-visitor refusal test's obligation.
				body := []statements.Statement{statements.NewArrayMemberAssignStatement(&values.JavaArrayMember{Object: object, Index: index}, rhs)}
				for _, visitor := range []func(statements.Statement) ([]values.JavaValue, [][]statements.Statement, bool){catchSourceChildren, nativeSourceNameChildren} {
					remaining := 100
					if sourceParameterUnwritten(body, ref, &remaining, visitor) {
						t.Fatal("unknown array operand granted declaration stability")
					}
				}
			})
		}
	}
}
