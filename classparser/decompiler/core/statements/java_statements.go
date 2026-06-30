package statements

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type ConditionStatement struct {
	Condition values.JavaValue
	Neg       bool
	Callback  func(values.JavaValue)
	// TernaryChainArm mirrors OpCode.TernaryChainArm: this condition supplies a DISTINCT nested
	// ternary arm and therefore must not be folded into a short-circuit &&/|| by MergeIf.
	TernaryChainArm bool
}

// ReplaceVar implements Statement.
func (r *ConditionStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	r.Condition.ReplaceVar(oldId, newId)
}

func (r *ConditionStatement) String(funcCtx *class_context.ClassContext) string {
	// A ConditionStatement is an intermediate structuring placeholder that should be consumed by
	// IfRewriter before reaching output. If one leaks (a structuring gap at a complex merge point),
	// rendering it as bare `if cond` (no brackets, no body) produces INVALID Java that fails syntax
	// validation and stubs the ENTIRE method. As a last-resort safety net, render it as a valid
	// `if (cond){}` — syntactically parseable Java with an empty body — so the method degrades
	// gracefully (one branch is empty) instead of being fully stubbed.
	return fmt.Sprintf("if (%s){}", r.Condition.String(funcCtx))
}

// isBoolPrimer reports whether v carries a non-nil boolean primitive type. It guards against the
// nil Type() that incomplete stack simulation can produce, so the boolean-comparison folding below
// never nil-dereferences (which would panic the whole method into a stub).
func isBoolPrimer(v values.JavaValue) bool {
	if v == nil {
		return false
	}
	t := v.Type()
	if t == nil {
		return false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	return ok && p.Name == types.JavaBoolean
}

func NewConditionStatement(cmp values.JavaValue, op string) *ConditionStatement {
	if t := cmp.Type(); t != nil {
		t.ResetType(types.NewJavaPrimer(types.JavaBoolean))
	}
	if v, ok := cmp.(*values.JavaCompare); ok {
		if op == values.NEQ {
			if literal, ok := v.JavaValue2.(*values.JavaLiteral); ok {
				if isBoolPrimer(v.JavaValue1) {
					if literal.Data == 0 {
						return &ConditionStatement{
							Condition: v.JavaValue1,
						}
					}
					if literal.Data == 1 {
						return &ConditionStatement{
							Condition: values.NewUnaryExpression(v.JavaValue1, values.Not, types.NewJavaPrimer(types.JavaBoolean)),
						}
					}
				}
			}
		}
		if op == values.EQ {
			if literal, ok := v.JavaValue2.(*values.JavaLiteral); ok {
				if isBoolPrimer(v.JavaValue1) {
					if literal.Data == 0 {
						return &ConditionStatement{
							Condition: values.NewUnaryExpression(v.JavaValue1, values.Not, types.NewJavaPrimer(types.JavaBoolean)),
						}
					}
					if literal.Data == 1 {
						return &ConditionStatement{
							Condition: v.JavaValue1,
						}
					}
				}
			}
		}
		return &ConditionStatement{
			Condition: values.NewBinaryExpression(v.JavaValue1, v.JavaValue2, op, types.NewJavaPrimer(types.JavaBoolean)),
		}
	} else {
		return &ConditionStatement{
			Condition: cmp,
		}
	}
}

type ReturnStatement struct {
	JavaValue values.JavaValue
}

// ReplaceVar implements Statement.
func (r *ReturnStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if r.JavaValue != nil {
		r.JavaValue.ReplaceVar(oldId, newId)
	}
}

func (r *ReturnStatement) String(funcCtx *class_context.ClassContext) string {
	if r.JavaValue == nil {
		return "return"
	}
	expr := r.JavaValue.String(funcCtx)
	// Narrowing cast for char/byte/short return types: bytecode stores char/byte/short
	// literals as ints (bipush/sipush/iconst), so a method returning char whose body
	// returns `cond ? 102 : 101` renders int literals that javac rejects ("possible
	// lossy conversion from int to char"). When the declared return type is narrower than
	// int and the returned value is int-typed, wrap it in an explicit cast. This is a
	// pure rendering fix — the recompiled bytecode is behaviorally identical.
	if cast := narrowingReturnCast(funcCtx, r.JavaValue); cast != "" {
		return fmt.Sprintf("return (%s) (%s)", cast, expr)
	}
	// Type-variable return: when the method's recovered return type is a class-scope type
	// variable (e.g. T/K/V) but the returned value's static type is the erased bound/Object,
	// emit an explicit unchecked cast so the source recompiles. `return null` needs no cast
	// (null is assignable to any type variable).
	if expr != "null" {
		if cast := typeVarReturnCast(funcCtx, r.JavaValue); cast != "" {
			// When the cast target carries a NESTED parameterized argument (e.g.
			// `Function<Supplier<T>, T>`) and the returned value is a DIFFERENT class cast to that
			// interface (e.g. an enum singleton `SupplierFunctionImpl.INSTANCE` that concretely
			// implements `Function<Supplier<Object>, Object>`), the direct cast is rejected as
			// inconvertible: javac proves `Supplier<Object>` distinct from `Supplier<T>`. Guava's own
			// source casts via the directly-implemented intermediate interface; the byte-faithful
			// decompiler analogue is a raw-erasure bridge cast `(Target<..>) (RawTarget) (value)`,
			// which is legal (downcast to the raw supertype, then an unchecked widen). Bare type-arg
			// targets (`Predicate<T>`, `Converter<T, T>`) keep the single direct cast.
			if bridge := nestedGenericRawBridge(funcCtx, r.JavaValue, cast); bridge != "" {
				return fmt.Sprintf("return (%s) (%s) (%s)", cast, bridge, expr)
			}
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
		// Concrete reference return type with an Object-typed value (erased generic / null-only slot):
		// emit an explicit downcast so the source recompiles. See objectReturnDowncast.
		if cast := objectReturnDowncast(funcCtx, r.JavaValue); cast != "" {
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
	}
	return fmt.Sprintf("return %s", expr)
}

// nestedGenericRawBridge returns the raw-erasure type name to interpose as an intermediate cast
// (`(retStr) (bridge) (value)`) when a type-variable return cast to a target with NESTED generic
// arguments would otherwise be rejected as inconvertible. It triggers only when (1) retStr has a
// nested parameterization (more than one `<`, e.g. `Function<Supplier<T>, T>`) AND (2) the returned
// value's erased type differs from retStr's erasure (the value is a different class being cast to a
// parameterized supertype). Otherwise it returns "" and the single direct cast is used.
func nestedGenericRawBridge(funcCtx *class_context.ClassContext, v values.JavaValue, retStr string) string {
	if strings.Count(retStr, "<") <= 1 {
		return ""
	}
	if v == nil {
		return ""
	}
	vt := v.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	targetErasure := erasureName(retStr)
	if targetErasure == "" || erasureName(raw.String(funcCtx)) == targetErasure {
		return ""
	}
	return targetErasure
}

// typeVarReturnCast returns the type-variable name to cast the returned value to when the
// enclosing method's declared return type is a class-scope type variable (recovered from the
// method Signature, e.g. `()TT;`) but the returned value's static type is a DIFFERENT reference
// type (the erased bound, typically Object or the declared bound such as Comparable). Bytecode
// erases a type-variable return to its bound, so a local typed as that bound, when returned from
// a method whose return type Yak has correctly recovered to `T`, fails to compile ("incompatible
// types: Comparable cannot be converted to T"). An explicit `(T)` cast is an unchecked but
// behavior-preserving rendering fix, matching what CFR/Fernflower emit. Returns "" when the
// return type is not a type variable, the value is a primitive, or the value already renders as
// that type variable. Kill-switch JDEC_TYPEVAR_RET_CAST_OFF disables it.
func typeVarReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil {
		return ""
	}
	if os.Getenv("JDEC_TYPEVAR_RET_CAST_OFF") != "" {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	// Two distinct triggers:
	//   - bareTypeVar: the return type IS a type variable (T). Bytecode erases a type-variable return
	//     to its bound (Object/Comparable), so a value typed as that bound fails "Comparable cannot be
	//     converted to T"; an unchecked (T) cast fixes it. This is the original behavior.
	//   - a generic return type that MENTIONS a type variable (Converter<T, T>, Map<K, V>). Here a cast
	//     is needed ONLY when the returned value is itself PARAMETERIZED with arguments that differ
	//     from the recovered return type (e.g. `Converter.identity()` returns `IdentityConverter<?>` -
	//     captured to IdentityConverter<CAP> - into `Converter<T, T>`, a hard "cannot convert" error).
	//     A RAW or bare value (`new InnerNode()`, `this.putExisting(...)` whose call type is erased to
	//     the raw class) converts to the parameterized return type by UNCHECKED conversion and needs no
	//     cast; adding one there is gratuitous and was over-casting plain raw-subtype returns.
	bareTypeVar := funcCtx.IsTypeParam(retStr)
	// arrayTypeVar: the return type is an array whose element is a bare type variable (`E[]`, `T[][]`).
	// Bytecode erases it to its bound array (Object[]), so a value returning Object[] (typically
	// `Collection.toArray(E[])` in a `<E> E[] toArray(E[])` override) needs an unchecked `(E[])` cast.
	arrayTypeVar := !bareTypeVar && isArrayOfTypeParam(retStr, funcCtx.TypeParams)
	if !bareTypeVar && !arrayTypeVar && !mentionsTypeParam(retStr, funcCtx.TypeParams) {
		return ""
	}
	vt := v.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	// A type variable is always a reference type; a primitive value never needs (and cannot take)
	// this cast.
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	rawStr := raw.String(funcCtx)
	if rawStr == retStr {
		return ""
	}
	if arrayTypeVar {
		// `(E[]) value` is legal only from an array or Object value; guard against unrelated scalars.
		if !strings.HasSuffix(rawStr, "[]") && rawStr != "Object" && rawStr != "java.lang.Object" {
			return ""
		}
		return retStr
	}
	if !bareTypeVar {
		// Parameterized return type (Converter<T, T>): only certain returned-value forms carry a
		// RECOMPILED static type that CONFLICTS with the recovered return type and thus need a cast:
		//   - a field-access singleton (`IdentityConverter<?> INSTANCE` captures to
		//     IdentityConverter<CAP>, not assignable to Converter<T, T>);
		//   - `return this` when the declared return type is a DIFFERENT (super)type parameterized
		//     with a type variable -- e.g. enum `ObjectPredicate implements Predicate<Object>` whose
		//     `<T> Predicate<T> withNarrowedType()` does `return this`: `this` is Predicate<Object>,
		//     not Predicate<T>, so guava itself writes the unchecked `(Predicate<T>) this` cast.
		// Every other form is safe without a cast: a method call recompiles to its own recovered
		// generic signature, `return this` whose return type is the class's OWN type (InnerNode<K,V>
		// in class InnerNode) converts by unchecked conversion, and `new Raw(...)` / bare values
		// convert by unchecked conversion. Casting those only adds noise (the InnerNode over-cast
		// regression), so they are excluded.
		switch uv := values.UnpackSoltValue(v).(type) {
		case *values.JavaClassMember, *values.RefMember:
			// field access (static `Class.INSTANCE` or instance `this.field`) - may need the cast
		case *values.JavaRef:
			if !uv.IsThis {
				return ""
			}
			// `return this`: `this` has the class's OWN parameterization `C<ClassTypeParams>`. A cast
			// is needed when the declared return type is a type `this` is NOT assignable to:
			//   - a DIFFERENT erasure (a supertype/interface `C2<..>`): erasures differ -> fall through
			//     and cast (e.g. enum `ObjectPredicate implements Predicate<Object>` returning
			//     `Predicate<T>`).
			//   - the SAME class `C` but parameterized with DIFFERENT type arguments than the class's
			//     own parameters: the `cast()` reparameterization idiom
			//     `<N1 extends N> C<N1> cast() { return (C<N1>) this; }`. `this` is `C<N>`, NOT `C<N1>`,
			//     so guava itself writes the unchecked cast; the decompiler dropped it (the erased cast
			//     emits no bytecode). javac otherwise rejects "C<N> cannot be converted to C<N1>"
			//     (guava GraphBuilder/NetworkBuilder/ValueGraphBuilder/ElementOrder.cast()).
			// Same erasure AND the return type's args ARE exactly the class's own params (identity
			// `return this`, e.g. `InnerNode<K,V>` in class `InnerNode<K,V>`) converts by identity and
			// must NOT be cast (the InnerNode over-cast regression). Kill-switch
			// JDEC_THIS_REPARAM_CAST_OFF restores the legacy "no cast on any same-erasure return this".
			if erasureName(retStr) == erasureName(raw.String(funcCtx)) {
				if os.Getenv("JDEC_THIS_REPARAM_CAST_OFF") != "" {
					return ""
				}
				if returnArgsAreClassParams(retStr, funcCtx.ClassTypeParams) {
					return ""
				}
			}
		case *values.JavaClassValue:
			// A class literal `X.class` has static type `Class<X>` (X concrete), so returning it where the
			// declared return type is `Class<...T...>` (mentions a type variable) ALWAYS conflicts:
			// `Class<Integer>` is never `Class<T>`, so the source carried an unchecked `(Class<T>)` cast
			// (gson Primitives.wrap `<T> Class<T> wrap(...) { ... return Integer.class; }`). The sibling
			// `Integer.TYPE` (a field) already gets the cast via the JavaClassMember case; the class-literal
			// value-kind was simply missing here. Reaching this branch already guarantees the return type
			// mentions a type variable, and a literal can never be `T.class`, so this never over-casts.
			// Kill-switch JDEC_CLASSLIT_RET_CAST_OFF.
			if os.Getenv("JDEC_CLASSLIT_RET_CAST_OFF") != "" {
				return ""
			}
		case *values.NewExpression, *values.TernaryExpression:
			// A concrete NON-GENERIC subtype constructed (`new ObjectTypeAdapter(...)`) or selected by a
			// ternary (`cond ? this.val$adapter : null`, val$adapter a NumberTypeAdapter) and returned as
			// a generic supertype R<T> (`TypeAdapter<T>`): the subclass FIXES R's type argument, so
			// unchecked conversion does NOT apply and the source carried an `(R<T>)` cast (gson's
			// TypeAdapterFactory.create family). genericReturnSubtypeCastNeeded gates it tightly (erasure
			// must differ AND the value's class must be non-generic), so raw-generic subtypes
			// (`new ArrayList()` -> List<T>) and identity returns are NOT over-cast.
			jc, okjc := raw.(*types.JavaClass)
			if !okjc || !genericReturnSubtypeCastNeeded(funcCtx, jc, retStr) {
				return ""
			}
		case *values.FunctionCallExpression:
			// A same-class instance call `this.getTypeAdapter(...)` whose recovered generic RETURN type
			// is a WILDCARD parameterization of the SAME erasure as the declared return type R<T> --
			// gson `<T> TypeAdapter<T> create(...) { ... return this.getTypeAdapter(...); }` where
			// `getTypeAdapter` returns `TypeAdapter<?>`. The call site's FuncType.ReturnType is only the
			// ERASED descriptor (raw `TypeAdapter`), so the real generic return is recovered from the
			// sibling method's Signature (funcCtx.MethodSignature). javac captures the wildcard to a
			// fresh CAP#1 and rejects `TypeAdapter<CAP#1>` -> `TypeAdapter<T>`, so the source carried an
			// unchecked `(TypeAdapter<T>)` cast. Tightly gated (this-receiver, same-class method, return
			// is a wildcard parameterization of the SAME erasure) so an ordinary call recovering its OWN
			// concrete generic signature is never over-cast. Kill-switch JDEC_WILDCARD_RET_CAST_OFF.
			if os.Getenv("JDEC_WILDCARD_RET_CAST_OFF") != "" {
				return ""
			}
			if uv.IsStatic {
				return ""
			}
			rcv, okr := values.UnpackSoltValue(uv.Object).(*values.JavaRef)
			if !okr || !rcv.IsThis {
				return ""
			}
			sig := funcCtx.MethodSignature(uv.FunctionName, len(uv.Arguments))
			if sig == "" {
				return ""
			}
			_, _, ret := types.ParseMethodSignatureFull(sig, funcCtx)
			if ret == nil {
				return ""
			}
			retSig := ret.String(funcCtx)
			if !strings.Contains(retSig, "?") || erasureName(retSig) != erasureName(retStr) {
				return ""
			}
			// wildcard same-erasure return -> fall through to the `(R<T>)` cast below.
		default:
			return ""
		}
	}
	return retStr
}

// genericReturnSubtypeCastNeeded reports whether a `new X(...)` or ternary value, whose static type is
// the non-generic class jc, returned where the declared return type is a generic type parameterized by
// a type variable (R<T>), requires an explicit `(R<T>)` unchecked cast. It fires only when (a) jc's
// erasure DIFFERS from R's erasure (jc is a SUBTYPE, not the same raw type -- so the InnerNode
// `new InnerNode()` -> InnerNode<K,V> identity case is excluded) AND (b) jc is NON-GENERIC per its own
// class Signature (SiblingClassSig). A non-generic subclass FIXES the supertype's type argument
// (gson `ObjectTypeAdapter extends TypeAdapter<Object>` returned as `TypeAdapter<T>`), so unchecked
// conversion does NOT apply and javac rejects it without the cast; a RAW generic subtype
// (`new ArrayList()` -> List<T>) converts by unchecked conversion and must NOT be over-cast. The
// bytecode's areturn guarantees jc is assignable to R's erasure, so the cast is always legal (never
// inconvertible). Requires the cross-class resolver (jar-internal class); JDK / unknown classes are
// skipped conservatively. Kill-switch JDEC_GENERIC_RET_SUBTYPE_CAST_OFF.
func genericReturnSubtypeCastNeeded(funcCtx *class_context.ClassContext, jc *types.JavaClass, retStr string) bool {
	if os.Getenv("JDEC_GENERIC_RET_SUBTYPE_CAST_OFF") != "" {
		return false
	}
	if funcCtx == nil || funcCtx.SiblingClassSig == nil || jc == nil {
		return false
	}
	// (a) jc's erasure must differ from R's erasure: jc is a proper subtype, not the same raw type.
	if erasureName(retStr) == erasureName(jc.String(funcCtx)) {
		return false
	}
	// (b) jc must be NON-GENERIC (declares no formal type parameters): only then does returning it as
	// R<T> bypass unchecked conversion and require the cast. A jar-internal generic class used raw is
	// left to unchecked conversion (no over-cast); a JDK / unknown class (resolver miss) is skipped.
	classSig, _, ok := funcCtx.SiblingClassSig(strings.ReplaceAll(jc.Name, ".", "/"))
	if !ok {
		return false
	}
	return len(types.ClassFormalTypeParamNames(classSig)) == 0
}

// objectReturnDowncast returns the concrete reference return type to downcast a returned value to,
// when the enclosing method's declared return type is a concrete reference type (NOT a bare type
// variable -- that path is typeVarReturnCast) but the returned value's static type is the erased top
// type java.lang.Object. Bytecode erases many generic / null-only locals to Object: e.g. fastjson2
// `public static JSONObject parseObject(String)` has a null-branch slot `Object var3 = null; return
// var3;`, which javac rejects as "incompatible types: Object cannot be converted to JSONObject". An
// explicit downcast `(JSONObject) var3` is ALWAYS legal (Object is the supertype of every reference
// type) and behavior-preserving: the method contract (recovered return type) guarantees the runtime
// value is that type or null, exactly what an implicit checkcast would assert; this is what
// CFR/Fernflower emit. Conservative guards: fires only when the value is statically Object and the
// return type is a different, non-primitive, non-bare-type-variable reference type. Parameterized
// targets (`List<T>`) are allowed -- `(List<T>) obj` is a legal unchecked cast. Kill-switch
// JDEC_OBJECT_RET_DOWNCAST_OFF restores the legacy uncast emission.
func objectReturnDowncast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil {
		return ""
	}
	if os.Getenv("JDEC_OBJECT_RET_DOWNCAST_OFF") != "" {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	if retStr == "" || retStr == "Object" || retStr == "java.lang.Object" {
		return ""
	}
	// Bare type-variable returns (T) are handled by typeVarReturnCast; skip to avoid double-casting.
	if funcCtx.IsTypeParam(retStr) {
		return ""
	}
	// A primitive return type cannot take a reference value (and is handled by narrowingReturnCast).
	if rt := ft.ReturnType.RawType(); rt != nil {
		if _, isPrim := rt.(*types.JavaPrimer); isPrim {
			return ""
		}
	}
	vt := v.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	// Only the erased top type Object qualifies. Any other concrete source type that fails to convert
	// is a different root cause (a genuine mis-typing such as `String cannot be converted to X`) and
	// must NOT be blanket-cast here.
	rawStr := raw.String(funcCtx)
	if rawStr != "Object" && rawStr != "java.lang.Object" {
		return ""
	}
	return retStr
}

// typeVarFieldStoreCast returns the bare class-scope type variable (e.g. "K") to cast the stored
// value to when assigning into a SAME-CLASS field that is declared as that type variable but the
// stored value's static type is a DIFFERENT reference type (Object or the erased bound). Bytecode
// erases a type-variable field to its bound, so `this.key = objExpr` (objExpr typed Object, as a raw
// `keys[]` element read produces) fails "incompatible types: Object cannot be converted to K"; the
// source originally carried an explicit `(K)` cast (guava CompactHashMap.MapEntry). The cast is
// unchecked but behavior-preserving (the field erases to Object at runtime), matching CFR/Fernflower.
// Only same-class fields qualify because their declared type variable is only known for the class
// being rendered (funcCtx.FieldTypeVars). Returns "" for non-field LHS, foreign fields, non-type-var
// fields, primitive/nil values, null literals, or values already rendering as the type variable.
// Kill-switch JDEC_NO_TYPEVAR_FIELD_CAST disables it.
func typeVarFieldStoreCast(funcCtx *class_context.ClassContext, left values.JavaValue, value values.JavaValue) string {
	if funcCtx == nil || left == nil || value == nil {
		return ""
	}
	if os.Getenv("JDEC_NO_TYPEVAR_FIELD_CAST") != "" {
		return ""
	}
	if len(funcCtx.FieldTypeVars) == 0 {
		return ""
	}
	var fieldName string
	switch lv := left.(type) {
	case *values.RefMember:
		// instance field store `this.field` - the receiver must be `this`.
		ref, ok := values.UnpackSoltValue(lv.Object).(*values.JavaRef)
		if !ok || !ref.IsThis {
			return ""
		}
		fieldName = class_context.SafeIdentifier(lv.Member)
	case *values.JavaClassMember:
		// static field store `ClassName.field` into the class being rendered.
		if lv.Name != funcCtx.ClassName {
			return ""
		}
		fieldName = class_context.SafeIdentifier(lv.Member)
	default:
		return ""
	}
	tv := funcCtx.FieldTypeVar(fieldName)
	if tv == "" {
		return ""
	}
	if lit, ok := values.UnpackSoltValue(value).(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
		// null is assignable to any type variable without a cast.
		return ""
	}
	vt := value.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	// A type variable is a reference type; a primitive value never needs (and cannot take) the cast.
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	rawStr := raw.String(funcCtx)
	// Already rendered as the type variable: no cast needed.
	if rawStr == tv {
		return ""
	}
	// Array-of-type-variable field (`E[] rest`): the cast `(E[])` is legal only from an array or
	// Object value (e.g. `(Object[]) checkNotNull(var2)`); never from an unrelated scalar reference.
	if strings.HasSuffix(tv, "[]") && !strings.HasSuffix(rawStr, "[]") && rawStr != "Object" && rawStr != "java.lang.Object" {
		return ""
	}
	return tv
}

// wildcardFieldStoreCast returns the parameterized declared type (e.g. "Class<? super T>") to cast a
// value stored into a SAME-CLASS field whose generic Signature is a parameterization that MENTIONS a
// class type variable AND carries a wildcard (e.g. `Class<? super T> rawType`), when the stored value
// erases to the SAME raw type but is NOT already that exact parameterization -- gson TypeToken
// `this.rawType = $Gson$Types.getRawType(this.type)` where getRawType returns `Class<?>`. The call site
// carries only the erased descriptor (raw `Class`), so JavaJive cannot see the conflict, but javac uses
// getRawType's real signature `Class<?>` -> captures to `Class<CAP#1>`, not assignable to
// `Class<? super T>`; the source carried an unchecked `(Class<? super T>)` cast. The cast is unchecked
// but behavior-preserving (the field erases to its raw bound at runtime) and always legal because the
// value erases to the field's own raw type. Tightly gated to a wildcard-bearing, type-var-mentioning
// same-class field so a fully-concrete or non-type-var generic field is never over-cast. Kill-switch
// JDEC_WILDCARD_FIELD_CAST_OFF.
func wildcardFieldStoreCast(funcCtx *class_context.ClassContext, left, value values.JavaValue) string {
	if funcCtx == nil || left == nil || value == nil {
		return ""
	}
	if os.Getenv("JDEC_WILDCARD_FIELD_CAST_OFF") != "" {
		return ""
	}
	var fieldName string
	switch lv := left.(type) {
	case *values.RefMember:
		ref, ok := values.UnpackSoltValue(lv.Object).(*values.JavaRef)
		if !ok || !ref.IsThis {
			return ""
		}
		fieldName = class_context.SafeIdentifier(lv.Member)
	case *values.JavaClassMember:
		if lv.Name != funcCtx.ClassName {
			return ""
		}
		fieldName = class_context.SafeIdentifier(lv.Member)
	default:
		return ""
	}
	sig := funcCtx.FieldSignature(fieldName)
	if sig == "" {
		return ""
	}
	ft := types.ParseSignature(sig)
	if ft == nil {
		return ""
	}
	fieldTypeStr := ft.String(funcCtx)
	// Must be a parameterized type that mentions a class-scope type variable AND carries a wildcard
	// (the assignment-incompatible `X<?>` -> `X<? super T>` shape). A bare type-var field is handled by
	// typeVarFieldStoreCast; a fully-concrete generic field needs no unchecked cast.
	if !strings.Contains(fieldTypeStr, "<") || !strings.Contains(fieldTypeStr, "?") {
		return ""
	}
	if !mentionsTypeParam(fieldTypeStr, funcCtx.ClassTypeParams) {
		return ""
	}
	if lit, ok := values.UnpackSoltValue(value).(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
		return ""
	}
	vt := value.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	// The value must erase to the field's own raw type so the cast is legal (never inconvertible).
	vtStr := vt.String(funcCtx)
	if erasureName(vtStr) != erasureName(fieldTypeStr) {
		return ""
	}
	// Normally an already-exact parameterization (vtStr == fieldTypeStr) needs no cast. The ONE
	// exception is a ternary (conditional) value: it is a poly expression whose type is computed as the
	// merge/LUB of its two arms (TernaryExpression.Type -> MergeTypes), and an unresolved merge silently
	// keeps the FIRST arm's type. So `this.comparator = cond ? var1 : NATURAL_ORDER` reports var1's exact
	// `Comparator<? super K>` type, hiding that the other arm (`NATURAL_ORDER`, a `Comparator<Comparable>`)
	// is NOT assignment-compatible -> javac "bad type in conditional expression". An explicit
	// `(Comparator<? super K>)(...)` re-targets the whole conditional as a poly expression so BOTH arms are
	// checked against (and unchecked-converted to) the field type, which recompiles (gson LinkedTreeMap /
	// LinkedHashTreeMap NATURAL_ORDER). Casting a ternary to its own already-reported type is harmless when
	// both arms truly match, so this stays gated to ternary values only. Kill-switch is the parent's
	// JDEC_WILDCARD_FIELD_CAST_OFF.
	if vtStr == fieldTypeStr {
		if _, isTernary := values.UnpackSoltValue(value).(*values.TernaryExpression); !isTernary {
			return ""
		}
	}
	return fieldTypeStr
}

// erasureName strips a generic type string down to its raw/erased name: "Predicate<T>" -> "Predicate",
// "Map<K, V>" -> "Map", "CopyOnWriteHashMap$InnerNode<K, V>" -> "CopyOnWriteHashMap$InnerNode".
func erasureName(s string) string {
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// isArrayOfTypeParam reports whether s is an array (one or more `[]`) whose element type is a bare
// in-scope type variable (e.g. "E[]", "T[][]"). Used to extend the type-variable cast to array-typed
// returns and field stores, where bytecode erases the element to its bound (`E[]` -> `Object[]`).
func isArrayOfTypeParam(s string, typeParams []string) bool {
	if !strings.HasSuffix(s, "[]") {
		return false
	}
	base := s
	for strings.HasSuffix(base, "[]") {
		base = strings.TrimSuffix(base, "[]")
	}
	base = strings.TrimSpace(base)
	for _, tp := range typeParams {
		if tp != "" && tp == base {
			return true
		}
	}
	return false
}

// returnArgsAreClassParams reports whether retStr is the class's OWN parameterization, i.e. its
// top-level type-argument list equals classParams EXACTLY and in order. It is the guard that tells an
// identity `return this` (`InnerNode<K, V>` in class `InnerNode<K, V>`, args==[K,V]==classParams ->
// true, no cast) apart from the `cast()` reparameterization idiom (`C<N1>` in class `C<N>`,
// args==[N1]!=[N] -> false, needs the unchecked `(C<N1>) this` cast). retStr must already share its
// erasure with `this`'s class (checked by the caller). A raw retStr (no `<...>`) has no args and
// returns false (but the caller never reaches here for a raw return type, which can't reparameterize).
func returnArgsAreClassParams(retStr string, classParams []string) bool {
	open := strings.IndexByte(retStr, '<')
	if open < 0 || !strings.HasSuffix(retStr, ">") {
		return false
	}
	inner := retStr[open+1 : len(retStr)-1]
	args := splitTopLevelTypeArgs(inner)
	if len(args) != len(classParams) {
		return false
	}
	for i := range args {
		if strings.TrimSpace(args[i]) != classParams[i] {
			return false
		}
	}
	return true
}

// splitTopLevelTypeArgs splits a type-argument list on commas that are NOT nested inside angle
// brackets, so "N1" -> ["N1"], "N, E" -> ["N", "E"], "Map<K, V>, T" -> ["Map<K, V>", "T"].
func splitTopLevelTypeArgs(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

// mentionsTypeParam reports whether retStr contains any of the in-scope type-variable names as a
// whole identifier token (so type param "T" matches "Converter<T, T>" and bare "T" but never the "T"
// inside "String" or a class named "Tree"). Used to extend the type-variable return cast to generic
// return types whose type ARGUMENTS are type variables, not just bare-type-variable returns.
func mentionsTypeParam(retStr string, typeParams []string) bool {
	if len(typeParams) == 0 || retStr == "" {
		return false
	}
	isIdent := func(b byte) bool {
		return b == '_' || b == '$' ||
			(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
	}
	for _, tp := range typeParams {
		if tp == "" {
			continue
		}
		from := 0
		for {
			idx := strings.Index(retStr[from:], tp)
			if idx < 0 {
				break
			}
			start := from + idx
			end := start + len(tp)
			beforeOK := start == 0 || !isIdent(retStr[start-1])
			afterOK := end == len(retStr) || !isIdent(retStr[end])
			if beforeOK && afterOK {
				return true
			}
			from = start + 1
		}
	}
	return false
}

// narrowingReturnCast returns the cast type name ("char"/"byte"/"short") when the enclosing
// method's declared return type is a narrowing-of-int and the returned value is int-typed,
// otherwise "". The cast lets the emitted source recompile without "possible lossy
// conversion from int to char/byte/short" errors.
func narrowingReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(&class_context.ClassContext{})
	valStr := v.Type().RawType().String(&class_context.ClassContext{})
	if valStr != "int" {
		return ""
	}
	switch retStr {
	case "char", "byte", "short":
		return retStr
	}
	return ""
}

// narrowingInitCast returns the cast type name ('byte'/'char'/'short') when a local is declared
// with a narrowing-of-int slot type but its initializer is int-valued. Per JLS the initializer is
// always int-promoted for arithmetic/bitwise/shift expressions, so assigning it to a byte/char/short
// local without a cast is a 'possible lossy conversion' javac error (e.g. commons-codec
// PureJavaCrc32C: `byte x = (arr[i] ^ crc) & 255`). Wrapping the initializer in an explicit cast
// is a pure rendering fix — the recompiled bytecode is behaviorally identical.
// intCategoryWiderThan reports whether slot type a is `int` while initializer type b is one of the
// narrower int-category primitives (byte/char/short). It is used to choose the declared type of a
// local whose slot was unified to int (because a later store assigns an int-valued expression) but
// whose first/initializer value is a narrower type that widens to int implicitly. Only the widening
// to int is recognized (the only widening the slot-merge in AssignVar produces); boolean and the
// non-int categories are excluded by the underlying name checks.
func intCategoryWiderThan(a types.JavaType, b types.JavaType) bool {
	if a == nil || b == nil {
		return false
	}
	pa, oka := a.RawType().(*types.JavaPrimer)
	pb, okb := b.RawType().(*types.JavaPrimer)
	if !oka || !okb {
		return false
	}
	if pa.Name != types.JavaInteger {
		return false
	}
	switch pb.Name {
	case types.JavaByte, types.JavaChar, types.JavaShort:
		return true
	}
	return false
}

func narrowingInitCast(slotType types.JavaType, valueType types.JavaType) string {
	if slotType == nil || valueType == nil {
		return ""
	}
	slotStr := slotType.RawType().String(&class_context.ClassContext{})
	valStr := valueType.RawType().String(&class_context.ClassContext{})
	if valStr != "int" {
		return ""
	}
	switch slotStr {
	case "char", "byte", "short":
		return slotStr
	}
	return ""
}

func NewReturnStatement(value values.JavaValue) *ReturnStatement {
	return &ReturnStatement{
		JavaValue: value,
	}
}

type StackAssignStatement struct {
	Id        int
	JavaValue *values.JavaRef
}

// ReplaceVar implements Statement.
func (a *StackAssignStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.JavaValue.ReplaceVar(oldId, newId)
}

func (a *StackAssignStatement) String(funcCtx *class_context.ClassContext) string {
	return a.JavaValue.String(funcCtx)
}
func NewStackAssignStatement(id int, value *values.JavaRef) *StackAssignStatement {
	return &StackAssignStatement{
		Id:        id,
		JavaValue: value,
	}
}

type AssignStatement struct {
	LeftValue   values.JavaValue
	ArrayMember *values.JavaArrayMember
	JavaValue   values.JavaValue
	IsDeclare   bool
	IsFirst     bool
}

// ReplaceVar implements Statement.
func (a *AssignStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if a.LeftValue != nil {
		a.LeftValue.ReplaceVar(oldId, newId)
	}
	if a.ArrayMember != nil {
		a.ArrayMember.ReplaceVar(oldId, newId)
	}
	if a.JavaValue != nil {
		a.JavaValue.ReplaceVar(oldId, newId)
	}
}

// arrayStoreRHS renders the right-hand side of an array-element store. The JVM has no boolean type on
// the operand stack: `boolean[] a; a[i] = true;` compiles to iconst_1 + bastore, so the stored value
// reaches the decompiler as an int literal (1) whose static type is int, not boolean. Rendering it
// verbatim yields `a[i] = 1`, which javac rejects ("int cannot be converted to boolean"). When the
// array's element type is boolean and the value is an int literal, render it as true/false. bastore is
// shared with byte[] and the remaining primitive array stores all accept a fitting int constant, so
// boolean is the only element type that needs this coercion.
func arrayStoreRHS(member *values.JavaArrayMember, value values.JavaValue, funcCtx *class_context.ClassContext) string {
	if member != nil && value != nil {
		if elem := member.Type(); elem != nil && elem.String(funcCtx) == "boolean" {
			if lit, ok := value.(*values.JavaLiteral); ok {
				if iv, ok := lit.Data.(int); ok {
					if iv == 0 {
						return "false"
					}
					return "true"
				}
			}
		}
		// Narrowing cast for byte[]/char[]/short[] element stores. bastore/castore/sastore implicitly
		// truncate the int on the stack to the element width, but Java source assignment context (JLS
		// 5.2) forbids the implicit int->byte/char/short narrowing, so `arr[i] = intValue` is a
		// "possible lossy conversion" error (commons-codec QCodec `out[i] = b` where b is int-typed).
		// The explicit cast reproduces the truncation the opcode already performs, so it is
		// behaviorally identical; only int-typed values need it (a value already byte/char/short
		// widens to int and back without change, and long/float/double cannot reach these arrays).
		if member.Type() != nil {
			if cast := narrowingInitCast(member.Type(), value.Type()); cast != "" {
				return fmt.Sprintf("(%s) (%s)", cast, value.String(funcCtx))
			}
		}
	}
	return value.String(funcCtx)
}

func (a *AssignStatement) String(funcCtx *class_context.ClassContext) string {
	if a.IsDeclare {
		if a.LeftValue == nil {
			return values.EmptySlotValuePlaceholder
		}
		return fmt.Sprintf("%s %s", a.LeftValue.Type().String(funcCtx), a.LeftValue.String(funcCtx))
	}
	if a.ArrayMember != nil {
		if a.JavaValue == nil {
			return fmt.Sprintf("%s = %s", a.ArrayMember.String(funcCtx), values.EmptySlotValuePlaceholder)
		}
		return fmt.Sprintf("%s = %s", a.ArrayMember.String(funcCtx), arrayStoreRHS(a.ArrayMember, a.JavaValue, funcCtx))
	}
	if a.LeftValue == nil || a.JavaValue == nil {
		left := values.EmptySlotValuePlaceholder
		right := values.EmptySlotValuePlaceholder
		if a.LeftValue != nil {
			left = a.LeftValue.String(funcCtx)
		}
		if a.JavaValue != nil {
			right = a.JavaValue.String(funcCtx)
		}
		return fmt.Sprintf("%s = %s", left, right)
	}
	// Re-insert the `? 1 : 0` coercion when an intrinsically-boolean value (a `&`/`|`/`^` boolean
	// connective or a boolean ternary, NOT a bare ref) is assigned to an int target: javac elided it
	// because the boolean is already 0/1 on the stack (guava DoubleMath/LongMath, ImmutableSortedMap).
	rhsVal := values.CoerceIntAssignRHS(a.LeftValue.Type(), a.JavaValue)
	assign := fmt.Sprintf("%s = %s", a.LeftValue.String(funcCtx), rhsVal.String(funcCtx))
	if a.IsFirst {
		// For `T x = null`, the initializer's static type is java.lang.Object, but the
		// variable's declared type is its (possibly refined) ref type — using the initializer
		// type would emit `Object x = null` even after the slot adopted a concrete type, and
		// `return x` would then mismatch the method's return type. Prefer the variable type
		// for a null initializer; for every other case this is identical to the value type.
		declType := a.JavaValue.Type()
		if lit, ok := a.JavaValue.(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
			declType = a.LeftValue.Type()
		}
		// A class literal initializer (`Foo.class`) is a java.lang.Class object, but its
		// JavaValue.Type() reports the *referenced* class (Foo) to drive bare-name rendering and
		// static-call receivers (`Foo.class`, `Foo.parseInt(...)`). When captured into a local the
		// declared type must be java.lang.Class, not Foo, or later member reads (`c.getName()`,
		// `c.isPrimitive()`) fail to recompile ("cannot find symbol"). Declare it `Class`; raw Class
		// is assignment-compatible with `Foo.class` and always recompiles. Kill-switch:
		// JDEC_NO_CLASSLIT_SLOT_TYPE=1 (shared with the slot-typing guard in stack_simulation.go).
		if _, ok := values.UnpackSoltValue(a.JavaValue).(*values.JavaClassValue); ok && os.Getenv("JDEC_NO_CLASSLIT_SLOT_TYPE") == "" {
			declType = types.NewJavaClass("java.lang.Class")
		}
		// Either side's type can be nil under incomplete simulation; fall back to the other
		// side rather than dereferencing nil (which panicked the whole method into a stub).
		if declType == nil {
			declType = a.LeftValue.Type()
		}
		if declType == nil {
			declType = a.JavaValue.Type()
		}
		if declType == nil {
			// No recoverable declared type. Emit the placeholder so the dumper's safety net
			// degrades this method cleanly instead of crashing.
			return values.EmptySlotValuePlaceholder + " " + assign
		}
		if _, ok := declType.RawType().(*types.JavaMultiCatchType); ok {
			// A multi-catch union type is legal only inside `catch (A | B e)`. If the exception
			// value is hoisted into an ordinary local (`cause = e` after the catch), render it as
			// a common Throwable subtype so the declaration remains valid Java.
			declType = types.NewJavaClass("java.lang.Exception")
		}
		// When the slot's resolved type is a WIDER int-category primitive than the initializer's
		// type, declare with the slot type so the initializer widens implicitly. The slot gets
		// widened to int when a later reassignment stores an int-valued expression into a slot first
		// seen as byte/char/short (commons-codec QuotedPrintableCodec.getUnsignedOctet:
		// `int o = bytes[i]; if (o < 0) o = 256 + o;` — slot is int, the baload initializer is byte).
		// Without this the variable is declared `byte o = bytes[i]` and the `o = 256 + o` reassign is
		// a "possible lossy conversion from int to byte" error; casting the reassign to byte would be
		// SEMANTICALLY WRONG (it truncates 255 back to -1), so the correct fix is the wider int decl.
		if lt := a.LeftValue.Type(); intCategoryWiderThan(lt, declType) {
			declType = lt
		}
		// Narrowing cast for byte/char/short locals: JLS promotes these types to int in any
		// arithmetic/bitwise/shift expression, so `byte x = (arr[i] ^ crc) & 255` is int-valued at
		// the source level even though the slot is byte (commons-codec PureJavaCrc32C). When the
		// slot type is a narrowing-of-int and the initializer is int-typed, keep the slot type as
		// the declaration and wrap the initializer in an explicit cast — mirrors ReturnStatement.
		if cast := narrowingInitCast(a.LeftValue.Type(), declType); cast != "" {
			assign = fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
			declType = a.LeftValue.Type()
		}
		return declType.String(funcCtx) + " " + assign
	} else {
		// Narrowing REASSIGNMENT to a byte/char/short target (field/local) whose RHS is int-typed:
		// `this.quote = cond ? '\'' : '"'` compiles to a ternary pushing the char constants as ints
		// (the JVM stack has no char category) followed by `putfield ... C`, which truncates
		// implicitly, so no i2c opcode is emitted and the value reaches us int-typed. Rendered
		// verbatim that is `this.quote = cond ? 39 : 34`, which javac rejects in assignment context
		// (JLS 5.2: "possible lossy conversion from int to char"; a non-constant conditional is not a
		// constant expression, so constant-narrowing does not apply). The declare branch above and the
		// array-element store (arrayStoreRHS) already insert this cast; mirror them here so plain
		// reassignments to narrow primitive fields/locals recompile (fastjson2 JSONWriter.quote,
		// JSONReaderUTF8 char writes). The explicit cast reproduces exactly the truncation the store
		// opcode performs, so it is behaviorally identical. Values already typed char/byte/short (and
		// those carrying an i2c/i2b/i2s cast) report a non-int type, so narrowingInitCast returns ""
		// and they are untouched. Kill-switch JDEC_NO_NARROW_REASSIGN_CAST=1.
		if os.Getenv("JDEC_NO_NARROW_REASSIGN_CAST") == "" && a.LeftValue != nil && a.JavaValue != nil {
			if cast := narrowingInitCast(a.LeftValue.Type(), a.JavaValue.Type()); cast != "" {
				return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
			}
		}
		// Type-variable field store: `this.key = objExpr` where `key` is declared `K` needs an
		// explicit unchecked `(K)` cast (the field erases to its bound in bytecode). See
		// typeVarFieldStoreCast.
		if cast := typeVarFieldStoreCast(funcCtx, a.LeftValue, a.JavaValue); cast != "" {
			return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
		}
		// Wildcard-parameterized same-class field store: `this.rawType = call()` where rawType is
		// `Class<? super T>` and the call returns `Class<?>` needs an explicit unchecked
		// `(Class<? super T>)` cast (gson TypeToken). See wildcardFieldStoreCast.
		if cast := wildcardFieldStoreCast(funcCtx, a.LeftValue, a.JavaValue); cast != "" {
			return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
		}
		return assign
	}
}

type ForStatement struct {
	InitVar       Statement
	Condition     *ConditionStatement
	EndExp        Statement
	SubStatements []Statement
}

// ReplaceVar implements Statement.
func (f *ForStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	f.InitVar.ReplaceVar(oldId, newId)
	f.Condition.ReplaceVar(oldId, newId)
	f.EndExp.ReplaceVar(oldId, newId)
	for _, st := range f.SubStatements {
		st.ReplaceVar(oldId, newId)
	}
}

func NewForStatement(subStatements []Statement) *ForStatement {
	return &ForStatement{
		InitVar:       subStatements[0],
		Condition:     subStatements[1].(*ConditionStatement),
		EndExp:        subStatements[len(subStatements)-2],
		SubStatements: subStatements[2 : len(subStatements)-2],
	}
}
func (f *ForStatement) String(funcCtx *class_context.ClassContext) string {
	datas := []string{}
	datas = append(datas, f.InitVar.String(funcCtx))
	datas = append(datas, f.Condition.String(funcCtx))
	datas = append(datas, f.EndExp.String(funcCtx))
	statementStr := []string{}
	for _, statement := range f.SubStatements {
		statementStr = append(statementStr, statement.String(funcCtx))
	}
	s := fmt.Sprintf("for(%s; %s; %s) {\n%s\n}", datas[0], datas[1], datas[2], strings.Join(statementStr, "\n"))
	return s
}

func NewArrayMemberAssignStatement(m *values.JavaArrayMember, value values.JavaValue) *AssignStatement {
	return &AssignStatement{
		ArrayMember: m,
		JavaValue:   value,
	}
}

func NewDeclareStatement(leftVal values.JavaValue) *AssignStatement {
	return &AssignStatement{
		LeftValue: leftVal,
		IsDeclare: true,
	}
}
func NewAssignStatement(leftVal, value values.JavaValue, isFirst bool) *AssignStatement {
	if value == nil || leftVal == nil || value.Type() == nil || leftVal.Type() == nil {
		// Guard against nil values/types in malformed bytecode: rather than panicking
		// (which forces the whole method into a stub), create the assignment as-is.
		// The type merge is skipped when either side has no type.
	}

	if value.Type() != nil && leftVal.Type() != nil {
		value.Type().ResetType(leftVal.Type())
	}
	return &AssignStatement{
		LeftValue: leftVal,
		JavaValue: value,
		IsFirst:   isFirst,
	}
}

type IfStatement struct {
	Condition values.JavaValue
	IfBody    []Statement
	ElseBody  []Statement
}

func (g *IfStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	g.Condition.ReplaceVar(oldId, newId)
	for _, st := range g.IfBody {
		st.ReplaceVar(oldId, newId)
	}
	for _, st := range g.ElseBody {
		st.ReplaceVar(oldId, newId)
	}
}

func (g *IfStatement) String(funcCtx *class_context.ClassContext) string {
	getBody := func(sts []Statement) string {
		var res []string
		for _, st := range sts {
			res = append(res, st.String(funcCtx))
		}
		return strings.Join(res, "\n")
	}
	return fmt.Sprintf("if (%s){\n"+
		"%s\n"+
		"}else{\n"+
		"%s\n"+
		"}", g.Condition.String(funcCtx), getBody(g.IfBody), getBody(g.ElseBody))
}
func NewIfStatement(condition values.JavaValue, ifBody, elseBody []Statement) *IfStatement {
	return &IfStatement{
		Condition: condition,
		IfBody:    ifBody,
		ElseBody:  elseBody,
	}
}

type GOTOStatement struct {
	ToStatement int
}

// ReplaceVar implements Statement.
func (g *GOTOStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (g *GOTOStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("goto: %d", g.ToStatement)
}
func NewGOTOStatement() *GOTOStatement {
	return &GOTOStatement{}
}

type NewStatement struct {
	Class *types.JavaClass
}

// ReplaceVar implements Statement.
func (a *NewStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Class.ReplaceVar(oldId, newId)
}

func (a *NewStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("new %s()", a.Class.Name)
}

func NewNewStatement(class *types.JavaClass) *NewStatement {
	return &NewStatement{
		Class: class,
	}
}

type ExpressionStatement struct {
	Expression values.JavaValue
}

// ReplaceVar implements Statement.
func (a *ExpressionStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Expression.ReplaceVar(oldId, newId)
}

func (a *ExpressionStatement) String(funcCtx *class_context.ClassContext) string {
	return a.Expression.String(funcCtx)
}

func NewExpressionStatement(v values.JavaValue) *ExpressionStatement {
	return &ExpressionStatement{
		Expression: v,
	}
}

type CaseItem struct {
	IsDefault bool
	IntValue  int
	Body      []Statement
}

func NewCaseItem(v int, body []Statement) *CaseItem {
	return &CaseItem{
		Body:     body,
		IntValue: v,
	}
}

type SwitchStatement struct {
	Value values.JavaValue
	Cases []*CaseItem
}

// ReplaceVar implements Statement.
func (a *SwitchStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Value.ReplaceVar(oldId, newId)
	for _, c := range a.Cases {
		for _, st := range c.Body {
			st.ReplaceVar(oldId, newId)
		}
	}
}

func (a *SwitchStatement) String(funcCtx *class_context.ClassContext) string {
	casesStrs := []string{}
	for _, c := range a.Cases {
		if c.IsDefault {
			casesStrs = append(casesStrs, fmt.Sprintf("default:\n%s", StatementsString(c.Body, funcCtx)))
			continue
		}
		casesStrs = append(casesStrs, fmt.Sprintf("case %d:\n%s", c.IntValue, StatementsString(c.Body, funcCtx)))
	}
	return fmt.Sprintf("switch(%s) {\n%s\n}", a.Value.String(funcCtx), strings.Join(casesStrs, "\n"))
}

func NewSwitchStatement(value values.JavaValue, cases []*CaseItem) *SwitchStatement {
	return &SwitchStatement{
		Value: value,
		Cases: cases,
	}
}

const (
	MiddleSwitch   = "switch"
	MiddleTryStart = "tryStart"
)

type MiddleStatement struct {
	Data any
	Flag string
}

// ReplaceVar implements Statement.
func (a *MiddleStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (a *MiddleStatement) String(funcCtx *class_context.ClassContext) string {
	return a.Flag
}

func NewMiddleStatement(flag string, d any) *MiddleStatement {
	return &MiddleStatement{
		Flag: flag,
		Data: d,
	}
}

type SynchronizedStatement struct {
	Argument values.JavaValue
	Body     []Statement
}

// ReplaceVar implements Statement.
func (s *SynchronizedStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	s.Argument.ReplaceVar(oldId, newId)
	for _, st := range s.Body {
		st.ReplaceVar(oldId, newId)
	}
}

func NewSynchronizedStatement(val values.JavaValue, body []Statement) *SynchronizedStatement {
	return &SynchronizedStatement{Argument: val, Body: body}
}

func (s *SynchronizedStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("synchronized(%s) {\n%s\n}", s.Argument.String(funcCtx), StatementsString(s.Body, funcCtx))
}
