package rewriter

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The generated cases exercise every use context the widening proof must
// distinguish. A method that contains even one unsafe use must never authorize
// an Object/Number declaration. The test runs several deterministic rounds so
// adding a new value/statement shape cannot silently default to "safe".
func TestGeneratedWidenSafetyNeverAcceptsUnsafeUse(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	stringType := types.NewJavaClass("java.lang.String")
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	ctx := useTestContext(objectType)

	safeUses := []func(ref *values.JavaRef) statements.Statement{
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(values.NewOriginalInstanceOf(ref, stringType, 1))
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(&values.CastExpression{Value: ref, TargetType: stringType})
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(useTestStaticCall(ref, objectType))
		},
		func(ref *values.JavaRef) statements.Statement {
			return &statements.ReturnStatement{JavaValue: ref}
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewSynchronizedStatement(ref, nil)
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(values.NewBinaryExpression(
				ref, values.NewJavaLiteral("null", objectType), values.EQ, boolType))
		},
	}
	unsafeUses := []func(ref *values.JavaRef) statements.Statement{
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(useTestStaticCall(ref, stringType))
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(&values.TernaryExpression{
				Condition: values.NewJavaLiteral(true, boolType), TrueValue: ref, FalseValue: useTestString("y"),
			})
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(&values.ArrayLengthExpression{Array: ref})
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(&values.RefMember{
				Object: ref, Member: "value", JavaType: stringType,
			})
		},
		func(ref *values.JavaRef) statements.Statement {
			return statements.NewExpressionStatement(values.NewBinaryExpression(
				ref, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), values.MUL,
				types.NewJavaPrimer(types.JavaInteger)))
		},
	}

	for round := 0; round < 3; round++ {
		rng := rand.New(rand.NewSource(int64(0x5eed + round)))
		for caseIndex := 0; caseIndex < 200; caseIndex++ {
			ref := useTestRef(fmt.Sprintf("var%d", 100+caseIndex), "java.lang.String")
			sts := []statements.Statement{statements.NewAssignStatement(ref, useTestString("x"), true)}
			hasUnsafe := false
			if rng.Intn(2) == 0 {
				sts = append(sts, safeUses[rng.Intn(len(safeUses))](ref))
			}
			if rng.Intn(2) == 0 {
				sts = append(sts, unsafeUses[rng.Intn(len(unsafeUses))](ref))
				hasUnsafe = true
			}
			safety := analyzeWidenSafety(sts, ref, ctx)
			if hasUnsafe && (safety.object || safety.number) {
				t.Fatalf("round %d case %d: unsafe use accepted: %+v", round, caseIndex, safety)
			}
			if !hasUnsafe && (!safety.object || !safety.number) {
				t.Fatalf("round %d case %d: safe uses rejected: %+v", round, caseIndex, safety)
			}
			again := analyzeWidenSafety(sts, ref, ctx)
			if again != safety {
				t.Fatalf("round %d case %d: nondeterministic safety %+v vs %+v", round, caseIndex, safety, again)
			}
		}
	}
}
