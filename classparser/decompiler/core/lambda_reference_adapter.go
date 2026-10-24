package core

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// LambdaReferenceAdapter preserves the reference checks performed at the entry
// of a raw SAM when its declaration metadata is unavailable. It describes
// bytecode adaptation, not guessed type arguments of a custom interface.
type LambdaReferenceAdapter struct {
	ErasedDescriptor       string
	InstantiatedDescriptor string
}

// FixedParameterTypes is a constraint from a completed original body-source
// receipt. It grants no factory, invocation, adaptation or ownership role.
// A later generic consumer must not retarget explicit source SAM formals.
type LambdaBodySource struct {
	Identity *values.LambdaSourceIdentity
	// ReferenceTarget marks a separately proved compiler-owned method reference.
	// It is not an arrow body and must not borrow SAM parameter-name receipts.
	ReferenceTarget types.JavaType
	// Static references retain a checked TypeName from the source proof. The
	// actual writer must reopen that namespace after final value-name binding.
	ReferenceOwner      string
	ReferenceQualifier  string
	Code                string
	FixedParameterTypes bool
	// These are original hidden-field ports proved by the owning anonymous
	// source renderer to denote enclosing lexical variables. Ordinary fields
	// never receive this permission. The core still checks eager factory,
	// operand, receiver, type and pending snapshot identities independently.
	LexicalFieldCaptures []*values.LambdaLexicalFieldCapture
	// RecordRendered observes this actual writer's completed packet after
	// capture substitution and target adaptation. It grants no body/factory
	// permission and must not render captured operands a second time.
	RecordRendered func(string)
	// Observes the same completed writer, including the exact bytes used for
	// each physical capture. A consumer can compose nested substitutions
	// without evaluating a captured operand or replaying its renderer.
	RecordRenderedWithCaptures func(LambdaSourceRender)
}

type LambdaCaptureRender struct {
	Index int
	Text  string
}

type LambdaSourceRender struct {
	Packet   string
	Captures []LambdaCaptureRender
	Complete bool
}

func dumpLambdaWithReferenceAdapter(d *Decompiler, impl *values.JavaClassMember, captured []values.JavaValue, raw types.JavaType, static []values.JavaValue, origin ...int) (string, error) {
	source, err := dumpLambdaSourceWithReferenceAdapter(d, impl, captured, raw, static, origin...)
	return source.Code, err
}

func dumpLambdaSourceWithReferenceAdapter(d *Decompiler, impl *values.JavaClassMember, captured []values.JavaValue, raw types.JavaType, static []values.JavaValue, origin ...int) (LambdaBodySource, error) {
	if d.originalLambdaDeclaration(impl) != LambdaDeclarationSyntheticBody {
		return LambdaBodySource{}, fmt.Errorf("lambda source body requires original synthetic declaration metadata")
	}
	var adapter *LambdaReferenceAdapter
	if len(static) >= 3 && !d.blockPartialFunctionalTarget {
		erased, actual := t19MethodTypeDesc(static[0]), t19MethodTypeDesc(static[2])
		unbox := UnboxingLambdaAdapterProven(impl.Description, erased, actual, len(captured), impl.RefKind)
		if unbox || (inferDeclaredLambdaTarget(d, raw, static[0], static[2]) == nil && ReferenceLambdaAdapterProven(impl.Description, erased, actual, len(captured), impl.RefKind)) {
			adapter = &LambdaReferenceAdapter{ErasedDescriptor: erased, InstantiatedDescriptor: actual}
		}
	}
	if len(origin) == 1 && d.DumpClassLambdaSourceAtOrigin != nil {
		return d.DumpClassLambdaSourceAtOrigin(impl.Member, impl.Description, utils.NewRootVariableId(), captured, adapter, origin[0])
	}
	if len(origin) == 1 && d.DumpClassLambdaMethodAtOrigin != nil {
		code, err := d.DumpClassLambdaMethodAtOrigin(impl.Member, impl.Description, utils.NewRootVariableId(), captured, adapter, origin[0])
		return LambdaBodySource{Code: code}, err
	}
	if adapter != nil && d.DumpClassLambdaMethodWithAdapter != nil {
		code, err := d.DumpClassLambdaMethodWithAdapter(impl.Member, impl.Description, utils.NewRootVariableId(), captured, adapter)
		return LambdaBodySource{Code: code}, err
	}
	code, err := d.DumpClassLambdaMethod(impl.Member, impl.Description, utils.NewRootVariableId(), captured)
	return LambdaBodySource{Code: code}, err
}

// Exact wrapper-to-primitive input adaptation belongs at SAM entry, before
// implementation effects. No numeric widening or guessed generic bound is
// supported. The implementation's unchanged result needs no extra check.
func UnboxingLambdaAdapterProven(impl, erased, actual string, captures int, kind uint8) bool {
	i, iok := directSamTokens(impl, false)
	e, eok := directSamTokens(erased, false)
	a, aok := directSamTokens(actual, false)
	if !iok || !eok || !aok || captures < 0 || len(a) < 2 || len(a) > 256 || len(i) > 256 || len(e) != len(a) || len(i) < len(a) || i[len(i)-1] != a[len(a)-1] {
		return false
	}
	prefix := len(i) - len(a)
	switch kind {
	case RefInvokeStatic:
		if prefix != captures {
			return false
		}
	case RefInvokeVirtual, RefInvokeSpecial, RefInvokeInterface:
		if captures == 0 || prefix != captures-1 {
			return false
		}
	default:
		return false
	}
	wrappers := map[string]string{"Z": "Ljava/lang/Boolean;", "B": "Ljava/lang/Byte;", "C": "Ljava/lang/Character;", "S": "Ljava/lang/Short;", "I": "Ljava/lang/Integer;", "J": "Ljava/lang/Long;", "F": "Ljava/lang/Float;", "D": "Ljava/lang/Double;"}
	changed := false
	for n := 0; n < len(a)-1; n++ {
		if i[prefix+n] == a[n] {
			if e[n] != a[n] && !(e[n] == "Ljava/lang/Object;" && (strings.HasPrefix(a[n], "L") || strings.HasPrefix(a[n], "["))) {
				return false
			}
			continue
		}
		if wrappers[i[prefix+n]] != a[n] || (e[n] != a[n] && e[n] != "Ljava/lang/Object;") {
			return false
		}
		changed = true
	}
	result := a[len(a)-1]
	return changed && (e[len(e)-1] == result || (e[len(e)-1] == "Ljava/lang/Object;" && (strings.HasPrefix(result, "L") || strings.HasPrefix(result, "["))))
}

// Require exact trailing implementation parameters and return type. Primitive
// boxing/unboxing and narrowed returns require a different adaptation proof.
// Reference checks run in SAM argument order before any body effects, including
// checks on arguments that the implementation never reads.
func ReferenceLambdaAdapterProven(impl, erased, actual string, captures int, kind uint8) bool {
	i, iok := directSamTokens(impl, false)
	e, eok := directSamTokens(erased, false)
	a, aok := directSamTokens(actual, false)
	if !iok || !eok || !aok || captures < 0 || len(a) < 2 || len(a) > 256 || len(i) > 256 || len(e) != len(a) || len(i) < len(a) || i[len(i)-1] != a[len(a)-1] {
		return false
	}
	prefix := len(i) - len(a)
	switch kind {
	case RefInvokeStatic:
		if prefix != captures {
			return false
		}
	case RefInvokeVirtual, RefInvokeSpecial, RefInvokeInterface:
		if captures == 0 || prefix != captures-1 {
			return false
		}
	default:
		return false
	}
	reference := func(s string) bool { return strings.HasPrefix(s, "L") || strings.HasPrefix(s, "[") }
	changed := false
	for n := 0; n < len(a)-1; n++ {
		if i[prefix+n] != a[n] {
			return false
		}
		if !reference(a[n]) || !reference(e[n]) {
			if a[n] != e[n] {
				return false // only unchanged primitives; no boxing or widening
			}
			continue
		}
		if e[n] != a[n] && e[n] != "Ljava/lang/Object;" {
			return false // stronger bounds need a hierarchy/adaptation proof
		}
		changed = changed || e[n] != a[n]
	}
	return changed && (e[len(e)-1] == a[len(a)-1] || (e[len(e)-1] == "Ljava/lang/Object;" && reference(a[len(a)-1])))
}
