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
		// An instance call on a NON-`this` receiver (a field/local of a jar-internal class) whose recovered
		// generic return is a WILDCARD parameterization of the SAME erasure as a type-variable-mentioning
		// declared return (`Class<A> getType() { return this.mapping.getAnnotationType(); }`, where
		// getAnnotationType() returns `Class<? extends Annotation>`): javac captures the wildcard to CAP#1
		// and rejects `Class<CAP#1>` -> `Class<A>`, so the source carried an unchecked `(Class<A>)` cast.
		// typeVarReturnCast's own wildcard branch only covers this-receiver same-class calls; this handles
		// the cross-receiver case via the sibling resolver. See crossRecvWildcardReturnCast.
		if cast := crossRecvWildcardReturnCast(funcCtx, r.JavaValue); cast != "" {
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
		// A `Class.forName(...)` return into a type-variable-mentioning `Class<...>` declared return: the
		// JDK signature is `Class<?> forName(String)`, so javac captures the wildcard to CAP#1 and rejects
		// `Class<CAP#1>` -> `Class<ObjectInstantiator<T>>`; the source carried an unchecked
		// `(Class<ObjectInstantiator<T>>)` cast. See classForNameReturnCast (spring objenesis
		// DelegatingToExoticInstantiator.instantiatorClass).
		if cast := classForNameReturnCast(funcCtx, r.JavaValue); cast != "" {
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
		// Concrete reference return type with an Object-typed value (erased generic / null-only slot):
		// emit an explicit downcast so the source recompiles. See objectReturnDowncast.
		if cast := objectReturnDowncast(funcCtx, r.JavaValue); cast != "" {
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
		// Parameterized return type (`Entry<E>`) whose value erases to the same raw class with an
		// erased/Object type argument: wrap in an unchecked parameterization cast. See parameterizedReturnCast.
		if cast := parameterizedReturnCast(funcCtx, r.JavaValue); cast != "" {
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
		// A returned `this.field` read whose RECOVERED real generic type is a same-erasure but
		// WILDCARD-parameterized type (`Comparator<? super E>`), pinned by the declared return type to a
		// concrete parameterization (`Comparator<Object>`): the field read renders raw so no cast is
		// emitted, but javac types it from the field's true declaration and rejects
		// "Comparator<CAP#1> cannot be converted to Comparator<Object>". A wildcard-source same-erasure
		// cast is an unchecked conversion (legal). See inheritedFieldReturnCast.
		if cast := inheritedFieldReturnCast(funcCtx, r.JavaValue); cast != "" {
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
		// A method-call value (a synthetic singleton accessor `Cut$AboveAll.access$100()`) whose static
		// type is a NON-GENERIC jar-internal SUBTYPE returned where the declared return type is a
		// type-variable-parameterized type of a DIFFERENT erasure (`Cut<C>`): the subtype fixes the
		// supertype's type argument AND (being final) cannot take a direct `(Cut<C>)` cast, so the
		// raw-erasure bridge `(Cut<C>) (Cut) value` is the only legal form. See
		// genericSubtypeReturnRawBridge.
		if bridge, target := genericSubtypeReturnRawBridge(funcCtx, r.JavaValue); target != "" {
			return fmt.Sprintf("return (%s) (%s) (%s)", target, bridge, expr)
		}
		// A `recv.m(...)` value whose recovered GENERIC return shares the declared return's raw erasure
		// but carries DIFFERENT (invariant-incompatible) type arguments -- e.g. guava
		// `Map<K,List<V>> asMap(ListMultimap<K,V> var0) { return var0.asMap(); }` where
		// `ListMultimap<K,V>.asMap()` truly returns `Map<K,Collection<V>>`. A direct `(Map<K,List<V>>)`
		// cast is inconvertible (both fully parameterized, distinct); only the raw-erasure bridge
		// `(Map<K,List<V>>) (Map) value` compiles. See parameterizedReturnRawBridge.
		if bridge, target := parameterizedReturnRawBridge(funcCtx, r.JavaValue); target != "" {
			return fmt.Sprintf("return (%s) (%s) (%s)", target, bridge, expr)
		}
		// A CHAINED instance call whose value erases to `X<Object>` (a generic factory receiver that
		// defaulted its type variable to Object because target typing cannot flow through the outer call,
		// e.g. `Collections.emptyList().iterator()` -> Iterator<Object>) returned where the declared type
		// is a same-erasure `X<Concrete>` (`Iterator<Attribute>`, jsoup Attributes.iterator): a DIRECT
		// `(X<Concrete>)` cast is inconvertible (both parameterized, distinct), only the raw-erasure bridge
		// `(X<Concrete>) (X) value` compiles. See erasedGenericChainReturnRawBridge.
		if bridge, target := erasedGenericChainReturnRawBridge(funcCtx, r.JavaValue); target != "" {
			return fmt.Sprintf("return (%s) (%s) (%s)", target, bridge, expr)
		}
		// A value whose static type is a NON-GENERIC subtype of a CONCRETE parameterized return type
		// (`Map<String,Object>`) that fixes the supertype's type arguments to a DIFFERENT parameterization
		// (`Properties`/`AbstractEnvironment$1` are `Map<Object,Object>`/`Map<String,String>`): the bare
		// return fails ("Properties cannot be converted to Map<String,Object>"), and a DIRECT
		// `(Map<String,Object>)` cast is inconvertible (invariant, distinct args). The source carried a raw
		// `(Map)` cast (the erased checkcast is a no-op the bytecode drops). See
		// concreteParamReturnSubtypeRawCast (spring AbstractEnvironment getSystemProperties/getSystemEnvironment).
		if raw := concreteParamReturnSubtypeRawCast(funcCtx, r.JavaValue); raw != "" {
			return fmt.Sprintf("return (%s) (%s)", raw, expr)
		}
	}
	return fmt.Sprintf("return %s", expr)
}

// erasedGenericChainReturnRawBridge handles a returned CHAINED instance call whose static type erases to
// `X<Object>` returned where the declared return type is a same-erasure `X<...>` carrying at least one
// CONCRETE (non-Object) type argument. The canonical case is a generic factory used as a call receiver:
// `return Collections.emptyList().iterator();` types as `Iterator<Object>` (emptyList's type variable
// defaults to Object because Java target typing does not flow through the outer `.iterator()` call),
// while the method returns `Iterator<Attribute>` (jsoup Attributes.iterator). A direct
// `(Iterator<Attribute>)` cast of an `Iterator<Object>` is inconvertible; the raw-erasure bridge
// `(Iterator<Attribute>) (Iterator) value` is the only legal form (downcast to the raw supertype, then an
// unchecked widen), and is behavior-identical for the erased value. Returns (rawErasure, retStr) or
// ("",""). Tightly gated so it never fires on a bare poly factory (which infers from the return target on
// its own) or a value that is not fully erased to X<Object>: since an `X<Object>`-from-chained-call value
// returned into `X<Concrete>` never compiles as-is, a match is always a genuine, safe repair. Kill-switch
// JDEC_ERASED_GENERIC_CHAIN_RET_BRIDGE_OFF.
func erasedGenericChainReturnRawBridge(funcCtx *class_context.ClassContext, v values.JavaValue) (string, string) {
	if funcCtx == nil || v == nil || os.Getenv("JDEC_ERASED_GENERIC_CHAIN_RET_BRIDGE_OFF") != "" {
		return "", ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return "", ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	retArgs, ok := topLevelTypeArgs(retStr)
	if !ok || len(retArgs) == 0 {
		return "", ""
	}
	// The return target must carry at least one CONCRETE (non-Object, non-type-variable, non-wildcard)
	// argument — the only shape the other return-cast helpers deliberately skip. A pure `X<E>` / `X<?>` /
	// `X<Object>` target is handled by parameterizedReturnCast or needs no cast.
	hasConcrete := false
	for _, ta := range retArgs {
		if ta == "Object" || ta == "java.lang.Object" || strings.HasPrefix(ta, "?") || funcCtx.IsTypeParam(ta) {
			continue
		}
		hasConcrete = true
		break
	}
	if !hasConcrete {
		return "", ""
	}
	// Value must be a CHAINED instance call: a FunctionCallExpression whose receiver is itself a value
	// expression (not a static/class-qualified factory that would infer from the return target).
	call, ok := values.UnpackSoltValue(v).(*values.FunctionCallExpression)
	if !ok || call == nil || call.IsStatic || call.Object == nil {
		return "", ""
	}
	if _, recvIsClass := values.UnpackSoltValue(call.Object).(*values.JavaClassValue); recvIsClass {
		return "", ""
	}
	vt := v.Type()
	if vt == nil {
		return "", ""
	}
	if _, isPrim := vt.RawType().(*types.JavaPrimer); isPrim {
		return "", ""
	}
	vStr := vt.String(funcCtx)
	if vStr == retStr || erasureName(vStr) != erasureName(retStr) {
		return "", ""
	}
	// The value must be RAW (no type args) or fully erased to `X<Object>` — i.e. it carries no concrete
	// parameterization of its own. A partially-concrete value (`X<String>`) is a different (real) mismatch
	// that must not be blanket-bridged here.
	if valArgs, okv := topLevelTypeArgs(vStr); okv {
		for _, va := range valArgs {
			if va != "Object" && va != "java.lang.Object" {
				return "", ""
			}
		}
	}
	return erasureName(retStr), retStr
}

// parameterizedReturnRawBridge handles a `recv.m(...)` return value whose recovered GENERIC return type
// shares the enclosing method's declared return-type erasure but carries DIFFERENT type arguments, an
// invariant mismatch a DIRECT cast cannot bridge. The decompiler sees the call's type as the raw
// descriptor erasure (jar-internal returns are not instantiated), so it would emit a bare
// `return recv.m()` that javac rejects once it resolves the callee's TRUE generic return (guava
// Multimaps.asMap(ListMultimap/SetMultimap/SortedSetMultimap): declared `Map<K,{List,Set,SortedSet}<V>>`
// but `Multimap.asMap()` returns `Map<K,Collection<V>>`; the source's erased `(Map<K,X<V>>) (Map<K,?>)`
// double cast is dropped). We recover the callee's instantiated return via the sibling hierarchy walk,
// and when it shares the declared erasure but differs in args, emit the raw-erasure bridge
// `(RetType) (RawErasure) value`. The areturn verifier guarantees `value <: RawErasure`, so the bridge
// cast is always legal. Returns (rawErasure, retStr) or ("",""). Kill-switch
// JDEC_PARAM_RETURN_RAW_BRIDGE_OFF.
func parameterizedReturnRawBridge(funcCtx *class_context.ClassContext, v values.JavaValue) (string, string) {
	if funcCtx == nil || v == nil {
		return "", ""
	}
	if os.Getenv("JDEC_PARAM_RETURN_RAW_BRIDGE_OFF") != "" {
		return "", ""
	}
	if funcCtx.SiblingClassSig == nil {
		return "", ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return "", ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	// Declared return must be a parameterization mentioning a type variable (`Map<K, List<V>>`) but not
	// itself a bare type variable (that path is typeVarReturnCast).
	if !strings.Contains(retStr, "<") || funcCtx.IsTypeParam(retStr) {
		return "", ""
	}
	if !mentionsTypeParam(retStr, funcCtx.TypeParams) {
		return "", ""
	}
	// Only an instance-method call on a parameterized receiver can carry a hidden covariant/erased return.
	call, ok := values.UnpackSoltValue(v).(*values.FunctionCallExpression)
	if !ok || call.IsStatic || call.Object == nil {
		return "", ""
	}
	pt, ok := types.AsParameterizedType(call.Object.Type())
	if !ok || pt.RawClassName == "" {
		return "", ""
	}
	ret := types.ResolveInstantiatedReturnType(funcCtx, funcCtx.SiblingClassSig, pt.RawClassName, pt.TypeArgs, call.FunctionName, len(call.Arguments))
	if ret == nil {
		return "", ""
	}
	valRetStr := ret.String(funcCtx)
	// Same raw erasure but DIFFERENT type args -> a direct cast is inconvertible; bridge is required.
	// Identical strings (no mismatch) or different erasure (handled by other helpers) do not qualify.
	if valRetStr == retStr || erasureName(valRetStr) != erasureName(retStr) {
		return "", ""
	}
	return erasureName(retStr), retStr
}

// classForNameReturnCast returns the declared return type to interpose as
// `return (Class<ObjectInstantiator<T>>) value` when the returned value is a `Class.forName(...)` static
// call and the enclosing method's declared return is a `Class<...>` parameterization that MENTIONS an
// in-scope type variable. The JDK signature is `Class<?> forName(String)` (and its 3-arg overload), so
// javac captures the wildcard to a fresh CAP#1 and rejects `Class<CAP#1>` -> `Class<ObjectInstantiator<T>>`;
// the source carried an unchecked `(Class<...>)` cast (the erased checkcast is a no-op the bytecode drops).
// A `Class<?>` -> `Class<X>` cast is ALWAYS a legal unchecked conversion (capture), never inconvertible, so
// this is safe. Restricted to the exact `Class.forName` shape (a wildcard-returning JDK method that the
// decompiler renders raw) so it never touches a poly factory whose return javac independently infers to a
// concrete parameterization (the guava `ImmutableMap.of()` / "incompatible bounds" family). Kill-switch
// JDEC_CLASS_FORNAME_RET_CAST_OFF.
func classForNameReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil || os.Getenv("JDEC_CLASS_FORNAME_RET_CAST_OFF") != "" {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	// Declared return must be a `Class<...>` parameterization mentioning a type variable (`Class<A>`,
	// `Class<ObjectInstantiator<T>>`), not a bare type variable.
	if !strings.Contains(retStr, "<") || funcCtx.IsTypeParam(retStr) {
		return ""
	}
	if erasureName(retStr) != "Class" && erasureName(retStr) != "java.lang.Class" {
		return ""
	}
	if !mentionsTypeParam(retStr, funcCtx.TypeParams) {
		return ""
	}
	call, ok := values.UnpackSoltValue(v).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.IsStatic || call.FunctionName != "forName" {
		return ""
	}
	// The receiver must be java.lang.Class (the static `Class.forName`), not some other `forName`.
	recvIsClass := false
	if call.ClassName == "java/lang/Class" || call.ClassName == "java.lang.Class" {
		recvIsClass = true
	} else if call.Object != nil {
		if ot := call.Object.Type(); ot != nil {
			if jc, okc := ot.RawType().(*types.JavaClass); okc && jc != nil && jc.Name == "java.lang.Class" {
				recvIsClass = true
			}
		}
	}
	if !recvIsClass {
		return ""
	}
	return retStr
}

// crossRecvWildcardReturnCast returns the declared return type to interpose as `return (Class<A>) value`
// when the returned value is an instance call on a NON-`this` receiver (a field/local of a jar-internal
// class) whose recovered generic return type is a WILDCARD parameterization of the SAME erasure as a
// type-variable-mentioning declared return. The canonical case is spring
// `TypeMappedAnnotation.getType(): Class<A> { return this.mapping.getAnnotationType(); }`, where
// AnnotationTypeMapping.getAnnotationType() returns `Class<? extends Annotation>`: javac captures the
// wildcard to a fresh CAP#1 and rejects `Class<CAP#1>` -> `Class<A>`, so the source carried an unchecked
// `(Class<A>)` cast (the erased checkcast is a no-op the bytecode drops). typeVarReturnCast's own
// wildcard branch is deliberately restricted to this-receiver same-class calls (funcCtx.MethodSignature);
// this recovers the callee's return through the cross-class sibling resolver
// (ResolveInstantiatedReturnType), so a call on a field/local receiver of a different class is covered.
// The wildcard-source same-erasure cast is an unchecked conversion javac accepts. Returns the cast target
// or "". Kill-switch JDEC_CROSS_RECV_WILDCARD_RET_CAST_OFF.
func crossRecvWildcardReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil || os.Getenv("JDEC_CROSS_RECV_WILDCARD_RET_CAST_OFF") != "" {
		return ""
	}
	if funcCtx.SiblingClassSig == nil {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	// Return must be a parameterization that MENTIONS an in-scope type variable (`Class<A>`) but is not
	// itself a bare type variable (that is typeVarReturnCast's own bare path).
	if !strings.Contains(retStr, "<") || funcCtx.IsTypeParam(retStr) {
		return ""
	}
	if !mentionsTypeParam(retStr, funcCtx.TypeParams) {
		return ""
	}
	call, ok := values.UnpackSoltValue(v).(*values.FunctionCallExpression)
	if !ok || call == nil || call.IsStatic || call.Object == nil {
		return ""
	}
	// The this-receiver same-class case is handled by typeVarReturnCast; skip a bare `this` receiver to
	// avoid a double cast. A `this.field` receiver (RefMember) or a local/param receiver is handled here.
	if rcv, okr := values.UnpackSoltValue(call.Object).(*values.JavaRef); okr && rcv.IsThis {
		return ""
	}
	// Recover the receiver's raw class and (possibly empty) type arguments.
	var recvRaw string
	var recvArgs []types.JavaType
	recvType := call.Object.Type()
	if recvType == nil {
		return ""
	}
	if pt, okp := types.AsParameterizedType(recvType); okp && pt.RawClassName != "" {
		recvRaw = pt.RawClassName
		recvArgs = pt.TypeArgs
	} else if jc, okc := recvType.RawType().(*types.JavaClass); okc && jc != nil {
		recvRaw = jc.Name
	} else {
		return ""
	}
	ret := types.ResolveInstantiatedReturnType(funcCtx, funcCtx.SiblingClassSig, recvRaw, recvArgs, call.FunctionName, len(call.Arguments))
	if ret == nil {
		return ""
	}
	retSig := ret.String(funcCtx)
	// The recovered return must be a WILDCARD parameterization of the SAME erasure as the declared return
	// (`Class<? extends Annotation>` vs `Class<A>`): only then is the bare return uncompilable and the
	// unchecked same-erasure cast both needed and legal.
	if !strings.Contains(retSig, "?") || erasureName(retSig) != erasureName(retStr) {
		return ""
	}
	return retStr
}

// typeVarLocalDeclName recovers the type-variable name a FIRST-declared local should be typed as when
// its initializer is a `recv.m(...)` call whose TRUE generic return type is a bare in-scope type
// variable T, but whose descriptor erasure (the value's static type) is T's erased bound. The store
// keeps only the erased value, so the decompiler declares `Comparable var3 = domain.next(...)`; javac,
// resolving `DiscreteDomain<C>.next(C) -> C`, then rejects later uses of var3 in a `C`-typed context
// (guava Cut$AboveValue.withLowerBoundType: `belowValue(var3)` flows into a `Cut<C>` return, so the
// conditional `next == null ? Cut.belowAll() : belowValue(var3)` is "bad type in conditional
// expression"). Declaring the local at T matches the original source (`C next = domain.next(...)`);
// javac itself re-derives the RHS as T (it performs real generic inference on the call), so NO RHS cast
// is needed and no semantics change -- T's erasure is exactly the bound the value already carries.
// Returns "" unless the initializer is an instance call on a jar-internal parameterized receiver (so
// the sibling hierarchy walk can recover the instantiated return) and the recovered return is a bare
// in-scope type variable distinct from the current (erased) declaration type. Kill-switch
// JDEC_TYPEVAR_LOCAL_DECL_OFF.
func typeVarLocalDeclName(funcCtx *class_context.ClassContext, left values.JavaValue, value values.JavaValue, declType types.JavaType) string {
	if funcCtx == nil || value == nil || declType == nil {
		return ""
	}
	if os.Getenv("JDEC_TYPEVAR_LOCAL_DECL_OFF") != "" {
		return ""
	}
	if funcCtx.SiblingClassSig == nil {
		return ""
	}
	// Only a genuine local (JavaRef, non-`this`) declaration -- never a field or synthetic slot.
	ref, ok := values.UnpackSoltValue(left).(*values.JavaRef)
	if !ok || ref.IsThis {
		return ""
	}
	// The current declared type must be a reference type that is NOT already a type variable.
	declStr := declType.String(funcCtx)
	if funcCtx.IsTypeParam(declStr) {
		return ""
	}
	if raw := declType.RawType(); raw != nil {
		if _, isPrim := raw.(*types.JavaPrimer); isPrim {
			return ""
		}
	}
	// The initializer must be an instance call on a jar-internal receiver; a wrapping cast (which would
	// change the static type) unpacks to a non-call and bails.
	call, ok := values.UnpackSoltValue(value).(*values.FunctionCallExpression)
	if !ok || call.IsStatic || call.Object == nil {
		return ""
	}
	var recvRaw string
	var recvArgs []types.JavaType
	if pt, ok := types.AsParameterizedType(call.Object.Type()); ok && pt.RawClassName != "" {
		recvRaw = pt.RawClassName
		recvArgs = pt.TypeArgs
	} else if r, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef); ok && r.IsThis && len(funcCtx.ClassTypeParams) > 0 {
		// A `this.m(...)` call reads `this` as the RAW current class (its value type carries no type
		// arguments), so reconstruct the class's OWN parameterization (`CurrentClass<ClassTypeParams>`).
		// The sibling walk resolves the current class from the jar, letting a same-class generic method
		// whose declared return is a class type variable (guava Cut$AboveValue.leastValueAbove -> `C`)
		// be recovered (guava Cut$AboveValue.canonical: `C var2 = this.leastValueAbove(...)`).
		recvRaw = funcCtx.ClassName
		recvArgs = make([]types.JavaType, len(funcCtx.ClassTypeParams))
		for i, p := range funcCtx.ClassTypeParams {
			recvArgs[i] = types.NewJavaClass(p)
		}
	} else {
		return ""
	}
	// If any recovered receiver type argument is a type variable this class RAW-ERASES at render
	// (a flattened inner class whose enclosing K/V are stripped from its override-method parameters,
	// so the receiver `Entry<K,V> var2` prints as raw `Entry var2`), then the call is an UNCHECKED
	// invocation on a raw receiver: javac erases its return type to the bound (Object), so a bare
	// `V var4 = var2.getValue()` is uncompilable ("Object cannot be converted to V"). Bail here so the
	// erased `Object` declaration is kept; the downstream `return (V)(var4)` supplies the unchecked cast.
	for _, a := range recvArgs {
		if a == nil {
			continue
		}
		if jc, ok := a.RawType().(*types.JavaClass); ok && funcCtx.RawEraseTypeVar(jc.Name) {
			return ""
		}
	}
	params, ret := types.ResolveInstantiatedSignature(funcCtx, funcCtx.SiblingClassSig, recvRaw, recvArgs, call.FunctionName, len(call.Arguments))
	if ret == nil {
		return ""
	}
	// A raw argument fed to a PARAMETERIZED formal (a bare `ReferenceEntry`/`Map` into a
	// `ReferenceEntry<K,V>`/`Map<K,V>` parameter) makes the call an UNCHECKED invocation (JLS
	// 15.12.2.6), which erases the method's return type to its erasure -- so javac types the result as
	// the bound (Object), NOT the type variable, and a `V var = call()` declaration is uncompilable
	// (`Object cannot be converted to V`, guava LocalCache$Segment.getLiveValue,
	// fastjson2 ObjectReaderNoneDefaultConstructor.createInstanceNoneDefaultConstructor). Bail so the
	// erased declaration is kept in that case.
	if uncheckedInvocation(funcCtx, params, call) {
		return ""
	}
	retStr := ret.String(funcCtx)
	// The recovered return must be a bare in-scope type variable, and it must differ from the current
	// (erased) declaration type. Its erasure being that bound is guaranteed by the descriptor store.
	if !funcCtx.IsTypeParam(retStr) || retStr == declStr {
		return ""
	}
	return retStr
}

// uncheckedInvocation reports whether any argument fed to a PARAMETERIZED formal parameter is a RAW
// (bare, non-parameterized) reference type -- the exact condition under which javac applies unchecked
// conversion and erases the invocation's return type to its erasure (JLS 15.12.2.6). params are the
// callee's instantiated formal parameter types (from ResolveInstantiatedSignature); a formal that is
// not itself parameterized never triggers this, and a parameterized / type-variable / primitive / null
// argument is a checked conversion. Handles both jar-internal and JDK raw generics uniformly because the
// signal is the formal being parameterized, not the argument class being known-generic.
func uncheckedInvocation(funcCtx *class_context.ClassContext, params []types.JavaType, call *values.FunctionCallExpression) bool {
	for i, p := range params {
		if p == nil || i >= len(call.Arguments) {
			continue
		}
		pt, ok := types.AsParameterizedType(p)
		if !ok || len(pt.TypeArgs) == 0 {
			continue // formal not parameterized -> this argument cannot force unchecked
		}
		arg := call.Arguments[i]
		if arg == nil {
			continue
		}
		if lit, ok := values.UnpackSoltValue(arg).(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
			continue // null is assignable to any parameterized type -> checked
		}
		at := arg.Type()
		if at == nil {
			continue
		}
		if _, isParam := types.AsParameterizedType(at); isParam {
			continue // parameterized argument -> checked conversion
		}
		jc, ok := at.RawType().(*types.JavaClass)
		if !ok {
			continue // primitive / array -> not a raw generic
		}
		if funcCtx.IsTypeParam(jc.Name) {
			continue // a bare type variable argument (e.g. `(C) x`) -> checked
		}
		return true // raw reference argument into a parameterized formal -> unchecked invocation
	}
	return false
}

// genericSubtypeReturnRawBridge handles a METHOD-CALL value (typically a synthetic singleton accessor
// like `Cut$AboveAll.access$100()`) whose static type is a NON-GENERIC jar-internal SUBTYPE, returned
// where the declared return type is a parameterized type of a DIFFERENT erasure that mentions a type
// variable (`Cut<C>`). typeVarReturnCast deliberately bails on static calls (its FunctionCallExpression
// branch is for this-receiver wildcard returns), and genericReturnSubtypeCastNeeded is only wired for
// `new X(...)` / ternary values. Such a subtype FIXES the supertype's type argument
// (`AboveAll extends Cut<Comparable<?>>`) and, being final, cannot even take a direct `(Cut<C>)` cast
// (inconvertible: its only Cut-supertype is Cut<Comparable<?>>); unchecked conversion likewise does not
// apply. The raw-erasure bridge `(Cut<C>) (Cut) value` is the only legal form, and the areturn verifier
// guarantees `value <: Cut`, so `(Cut) value` can never be inconvertible. Returns (rawErasure,
// targetRetStr) or ("",""). Kill-switch JDEC_GENERIC_SUBTYPE_RET_BRIDGE_OFF.
func genericSubtypeReturnRawBridge(funcCtx *class_context.ClassContext, v values.JavaValue) (string, string) {
	if funcCtx == nil || v == nil {
		return "", ""
	}
	if os.Getenv("JDEC_GENERIC_SUBTYPE_RET_BRIDGE_OFF") != "" {
		return "", ""
	}
	// Only method-call values reach this helper unhandled (New/ternary go through typeVarReturnCast's
	// genericReturnSubtypeCastNeeded wiring; field singletons go through its JavaClassMember/RefMember
	// case). Restricting to calls prevents any double-cast with the earlier chain.
	if _, ok := values.UnpackSoltValue(v).(*values.FunctionCallExpression); !ok {
		return "", ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return "", ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	// Return type must be a parameterization that MENTIONS a type variable (`Cut<C>` / `Map<K, V>`) but
	// is not itself a bare type variable (that path is typeVarReturnCast).
	if !strings.Contains(retStr, "<") || funcCtx.IsTypeParam(retStr) {
		return "", ""
	}
	if !mentionsTypeParam(retStr, funcCtx.TypeParams) {
		return "", ""
	}
	vt := v.Type()
	if vt == nil {
		return "", ""
	}
	jc, ok := vt.RawType().(*types.JavaClass)
	if !ok || jc == nil {
		return "", ""
	}
	// Reuse the non-generic-subtype-of-parameterized-return check (jc non-generic, different erasure,
	// resolver-confirmed jar-internal, areturn-legal).
	if !genericReturnSubtypeCastNeeded(funcCtx, jc, retStr) {
		return "", ""
	}
	// Returns (bridge=raw erasure, target=parameterized return type) to match the call-site's
	// `(target) (bridge) (expr)` rendering.
	return erasureName(retStr), retStr
}

// nestedGenericRawBridge returns the raw-erasure type name to interpose as an intermediate cast
// (`(retStr) (bridge) (value)`) when a type-variable return cast to a target with NESTED generic
// arguments would otherwise be rejected as inconvertible. It triggers only when (1) retStr has a
// nested parameterization (more than one `<`, e.g. `Function<Supplier<T>, T>`) AND (2) the returned
// value's erased type differs from retStr's erasure (the value is a different class being cast to a
// parameterized supertype). Otherwise it returns "" and the single direct cast is used.
func nestedGenericRawBridge(funcCtx *class_context.ClassContext, v values.JavaValue, retStr string) string {
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
	if targetErasure == "" {
		return ""
	}
	valErasure := erasureName(raw.String(funcCtx))
	// Case A (original): the value is a DIFFERENT class being cast to a NESTED parameterized supertype
	// (`Function<Supplier<T>, T>`). A direct cast is inconvertible; bridge via the target's raw erasure.
	if valErasure != targetErasure {
		if strings.Count(retStr, "<") <= 1 {
			return ""
		}
		return targetErasure
	}
	// Case B: SAME erasure, but the value is a FIELD SINGLETON whose declared parameterization is
	// provably distinct from the target's -- e.g. guava Range.ALL (declared `Range<Comparable>`) returned
	// as `Range<C>`, or the CROSS-CLASS `Range$RangeLexOrdering.INSTANCE` (declared `Ordering<Range<?>>`)
	// returned as `Ordering<Range<C>>`. javac rejects the DIRECT cast (invariant, distinct args), and guava
	// bridges via the raw type `(Range<C>) (Range) ALL` / `(Ordering<Range<C>>) (Ordering) INSTANCE`. The
	// decompiler's VALUE type for the field read is the erased raw class, so the mismatch is invisible from
	// the value type -- it is recovered from the field's own generic Signature. Restricted to field-access
	// values so the this-reparam / new / ternary cases typeVarReturnCast handles with a legal direct cast
	// are never touched, and to fields whose declared type carries NON-bare-wildcard top-level args (a bare
	// `X<?>` converts to `X<C>` by the unchecked conversion the direct cast already allows -- no bridge).
	// Kill-switch JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF.
	if os.Getenv("JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF") != "" {
		return ""
	}
	var sig string
	if fieldName := sameClassFieldName(funcCtx, v); fieldName != "" {
		sig = funcCtx.FieldSignature(fieldName)
	} else if os.Getenv("JDEC_XCLASS_FIELD_RET_BRIDGE_OFF") == "" {
		// A CROSS-CLASS static field singleton (`OtherClass.INSTANCE`): its generic Signature lives in the
		// declaring class, recovered via SiblingFieldSig (the same cross-class field resolver that powers
		// inherited-field receiver typing). guava Range.rangeLexOrdering `Range$RangeLexOrdering.INSTANCE`.
		sig = crossClassStaticFieldSig(funcCtx, v)
	}
	if sig == "" {
		return ""
	}
	parsed := types.ParseSignature(sig)
	if parsed == nil {
		return ""
	}
	declStr := parsed.String(funcCtx)
	if erasureName(declStr) != targetErasure || declStr == retStr {
		return ""
	}
	// The declared field type must carry CONCRETE top-level type arguments (a top-level bare wildcard
	// `X<?>` converts to `X<C>` by an unchecked conversion the DIRECT cast already allows -- no bridge).
	args, ok := topLevelTypeArgs(declStr)
	if !ok || len(args) == 0 {
		return ""
	}
	for _, a := range args {
		if a == "?" || strings.HasPrefix(a, "? ") {
			return ""
		}
	}
	return targetErasure
}

// sameClassFieldName returns the field name when v is a read of a field DECLARED in the current class
// (a static `ThisClass.FIELD` access or an instance `this.field` access), or "" otherwise. Used to
// recover the field's declared generic Signature for cast decisions the erased value type hides.
func sameClassFieldName(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	switch lv := values.UnpackSoltValue(v).(type) {
	case *values.JavaClassMember:
		if lv.Name != funcCtx.ClassName {
			return ""
		}
		return class_context.SafeIdentifier(lv.Member)
	case *values.RefMember:
		ref, ok := values.UnpackSoltValue(lv.Object).(*values.JavaRef)
		if !ok || !ref.IsThis {
			return ""
		}
		return class_context.SafeIdentifier(lv.Member)
	}
	return ""
}

// crossClassStaticFieldSig returns the raw generic Signature of a CROSS-CLASS static field read
// (`OtherClass.FIELD`, OtherClass != the class being rendered), recovered from the declaring class's
// bytes via SiblingFieldSig, or "" when v is not such a read, the class is JDK/external (no bytes in the
// jar), or the field carries no generic Signature. It complements sameClassFieldName (current-class
// FieldSignatures) so a same-erasure return raw-bridge can be decided for a foreign field singleton
// whose declared parameterization the erased value type hides (guava Range.rangeLexOrdering's
// `Range$RangeLexOrdering.INSTANCE`, declared `Ordering<Range<?>>`). The lookup uses the RAW JVM field
// name (JavaClassMember.Member, matching buildSiblingFieldSig's name key) and the owner's binary
// internal name (dotted Name -> slash form; the inner-class `$` is part of the binary name and kept).
func crossClassStaticFieldSig(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || funcCtx.SiblingFieldSig == nil {
		return ""
	}
	jcm, ok := values.UnpackSoltValue(v).(*values.JavaClassMember)
	if !ok || jcm.Name == "" || jcm.Name == funcCtx.ClassName {
		return "" // same-class field is handled by sameClassFieldName; only cross-class here
	}
	sig, ok := funcCtx.SiblingFieldSig(strings.ReplaceAll(jcm.Name, ".", "/"), jcm.Member)
	if !ok {
		return ""
	}
	return sig
}

// ternaryArmNeedsTypeVarCast reports whether a conditional expression returned into a bare type-variable
// target needs a wrapping `(T)` cast: true when at least one non-null arm's static type is NOT the type
// variable T (so javac's poly-typing of that arm against T fails), and every arm type is known. A null
// literal arm is skipped (null is assignable to any type variable). When BOTH arms are already T (or
// null) the conditional types cleanly at T and needs no cast, so this returns false. Any arm with an
// unknown (nil) type returns false, so an ambiguous shape never gets a speculative cast. See the
// ternary branch in typeVarReturnCast.
func ternaryArmNeedsTypeVarCast(t *values.TernaryExpression, typeVar string, funcCtx *class_context.ClassContext) bool {
	if t == nil {
		return false
	}
	needCast := false
	for _, arm := range []values.JavaValue{t.TrueValue, t.FalseValue} {
		a := values.UnpackSoltValue(arm)
		if a == nil {
			return false
		}
		if lit, ok := a.(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
			continue // null is assignable to any type variable -> no constraint
		}
		at := a.Type()
		if at == nil {
			return false // unknown arm type -> do not risk an added cast
		}
		if at.String(funcCtx) != typeVar {
			needCast = true
		}
	}
	return needCast
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
	// A conditional expression returned into a BARE type-variable target is a POLY expression: javac
	// types each arm against T, so an arm whose static type is a non-T reference (typically Object, from
	// a value whose generic type the decompiler lost) fails "bad type in conditional expression" /
	// "Object cannot be converted to T". The ternary's OWN Type() is the arm LUB, which MergeTypes can
	// collapse to T (the other arm being genuinely T), masking the mismatch -- so the `rawStr == retStr`
	// guard below would skip the cast. Wrapping the whole ternary `(T)(cond ? a : b)` makes it a
	// STANDALONE (non-poly) expression typed at the arm LUB, then an unchecked `(T)` cast: legal and
	// behavior-preserving (the bytecode already areturns the erased value, no checkcast). guava
	// ConfigurableValueGraph.edgeValueOrDefault_internal `return (val == null) ? dflt : val;` where
	// `val` was read Object off a raw GraphConnections.get(). Only fires when at least one non-null arm
	// is NOT already the type variable (both-arms-T identity is left alone). Kill-switch
	// JDEC_TERNARY_TYPEVAR_RET_CAST_OFF.
	if bareTypeVar && os.Getenv("JDEC_TERNARY_TYPEVAR_RET_CAST_OFF") == "" {
		if tern, ok := values.UnpackSoltValue(v).(*values.TernaryExpression); ok &&
			ternaryArmNeedsTypeVarCast(tern, retStr, funcCtx) {
			return retStr
		}
	}
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

// jdkNonGenericParamSubtypes are JDK reference types that are NON-generic yet a proper subtype of a
// generic collection interface, so returning one where the declared return type is a CONCRETE
// parameterization of that interface (`Map<String,Object>`) never converts implicitly: their supertype
// instantiation is fixed and differs (`Properties` is `Map<Object,Object>`). Such a return needs the raw
// `(Map)` cast the source carried. Kept to the tiny provably-correct set; a generic JDK subtype
// (`ArrayList` -> `List<E>`) converts by unchecked conversion and MUST NOT appear here.
var jdkNonGenericParamSubtypes = map[string]bool{
	"java.util.Properties": true,
}

// concreteParamReturnSubtypeRawCast returns the RAW erasure name (`Map`) to interpose as
// `return (Map) (value)` when the enclosing method's declared return type is a CONCRETE parameterized
// type (`Map<String,Object>`, mentions no in-scope type variable) and the returned value's static type
// is a NON-GENERIC subtype of that return's erasure whose supertype instantiation is therefore fixed and
// distinct. Two provably-safe value shapes qualify:
//   - a jar-internal non-generic subtype (`new AbstractEnvironment$1(this)`), confirmed by the
//     cross-class resolver via genericReturnSubtypeCastNeeded (0 formal params, different erasure); or
//   - a whitelisted non-generic JDK subtype (`System.getProperties()` -> `Properties`).
//
// The bytecode areturn guarantees the value is assignable to the return's erasure, so the raw `(Map)`
// cast is always legal (never inconvertible), and raw `Map` -> `Map<String,Object>` is an unchecked
// conversion javac accepts. A generic subtype (`ArrayList` -> `List<E>`, or a jar-internal generic
// class) converts implicitly and is deliberately excluded so no covariant return is over-cast.
// Kill-switch JDEC_CONCRETE_PARAM_RET_SUBTYPE_RAW_CAST_OFF.
func concreteParamReturnSubtypeRawCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil || os.Getenv("JDEC_CONCRETE_PARAM_RET_SUBTYPE_RAW_CAST_OFF") != "" {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	// Declared return must be a CONCRETE parameterization: contains `<`, is not itself a type variable,
	// and mentions NO in-scope type variable (a type-variable-parameterized return is typeVarReturnCast's
	// domain and its cast form differs). It must carry at least one concrete top-level arg.
	if !strings.Contains(retStr, "<") || funcCtx.IsTypeParam(retStr) {
		return ""
	}
	if mentionsTypeParam(retStr, funcCtx.TypeParams) {
		return ""
	}
	retArgs, ok := topLevelTypeArgs(retStr)
	if !ok || len(retArgs) == 0 {
		return ""
	}
	hasConcrete := false
	for _, ta := range retArgs {
		if ta == "Object" || ta == "java.lang.Object" || strings.HasPrefix(ta, "?") {
			continue
		}
		hasConcrete = true
		break
	}
	if !hasConcrete {
		return ""
	}
	erasureR := erasureName(retStr)
	if erasureR == "" || erasureR == "Object" || erasureR == "java.lang.Object" {
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
	jc, ok := raw.(*types.JavaClass)
	if !ok || jc == nil {
		return ""
	}
	// The value's erasure must be a PROPER subtype (different raw class) of the return's erasure: a
	// same-erasure value is a different (arg-mismatch) root cause handled elsewhere.
	if erasureName(jc.String(funcCtx)) == erasureR {
		return ""
	}
	// Shape 1: jar-internal non-generic subtype (fixed supertype args). genericReturnSubtypeCastNeeded
	// confirms erasure differs, the resolver sees the class, and it declares no formal type params.
	if genericReturnSubtypeCastNeeded(funcCtx, jc, retStr) {
		return erasureR
	}
	// Shape 2: a whitelisted non-generic JDK subtype. Must be a proven (bridged) subtype of the return's
	// erasure so the raw cast is a legal downcast/identity.
	vFQN := jc.Name
	if jdkNonGenericParamSubtypes[vFQN] {
		if pt, okp := types.AsParameterizedType(ft.ReturnType); okp && pt != nil {
			if types.IsReferenceSubtypeBridged(vFQN, pt.RawClassName, funcCtx.SiblingSuperTypes) {
				return erasureR
			}
		}
	}
	return ""
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

// topLevelTypeArgs splits a rendered generic type string's OUTERMOST `<...>` into its top-level type
// arguments (nesting-aware, so `Map<K, List<V>>` yields ["K", "List<V>"]). ok=false when s is not a
// parameterized type (no balanced trailing `<...>`).
func topLevelTypeArgs(s string) ([]string, bool) {
	lt := strings.IndexByte(s, '<')
	if lt < 0 || !strings.HasSuffix(s, ">") {
		return nil, false
	}
	inner := s[lt+1 : len(s)-1]
	var args []string
	depth, start := 0, 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(inner[start:]))
	return args, true
}

// typeVarIsUnbounded reports whether name is a type variable in scope whose SOLE bound is Object (the
// canonical bare `<E>`). It looks in the enclosing method's formal type parameters first (which shadow
// the class's), then the class's. ClassFormalTypeParamBounds omits any variable whose only bound is
// Object, so "declared here AND absent from the bounds map" == unbounded. A bounded variable
// (`C extends Comparable`), or one not declared in either scope, returns false -- used to gate an
// unchecked `X<Object>` -> `X<E>` parameterization cast, which is only legal for unbounded E.
func typeVarIsUnbounded(name string, funcCtx *class_context.ClassContext) bool {
	if funcCtx == nil || name == "" {
		return false
	}
	for _, n := range types.MethodFormalTypeParamNames(funcCtx.CurrentMethodSig) {
		if n == name {
			_, bounded := types.ClassFormalTypeParamBounds(funcCtx.CurrentMethodSig, funcCtx)[name]
			return !bounded
		}
	}
	for _, n := range types.ClassFormalTypeParamNames(funcCtx.ClassSig) {
		if n == name {
			_, bounded := types.ClassFormalTypeParamBounds(funcCtx.ClassSig, funcCtx)[name]
			return !bounded
		}
	}
	// Injected (flattened inner-class) type variable: not declared in ClassSig, but in scope via
	// TypeParams. Its boundedness lives in InjectedTypeParamBounds (only non-Object bounds are recorded,
	// mirroring the rendered header), so "in scope AND absent from that map" == the rendered bare `<E>`.
	if funcCtx.IsTypeParam(name) {
		if _, bounded := funcCtx.InjectedTypeParamBounds[name]; bounded {
			return false
		}
		return true
	}
	return false
}

// parameterizedReturnCastValueEligible reports whether a returned value has a shape for which an
// unchecked `X<Object>` -> `X<E>` wrapper is safe to add. It EXCLUDES exactly the poly expressions that
// javac would type from the return target on their own (so wrapping them is at best redundant and at
// worst breaks inference / injects a reference to an unresolved dependency type -- spring
// `return (Mono<T>)(x.map(l -> ...))`):
//   - a bare lambda / method-reference return (the return type itself is the lambda's target);
//   - a zero-argument generic factory call (`of()` / `emptyIterator()`);
//   - a call with a lambda / method-reference argument (`x.map(l -> ...)`);
//   - a bare `this` return: `this` is ALWAYS the class's own self-parameterization `C<N>`, which either
//     equals the return type (identity `C<N> self() { return this; }` -- no cast) or is a reparameterization
//     `<N1 extends N> C<N1> cast() { return this; }` already handled by typeVarReturnCast. `this`'s value
//     type renders raw (`C`) so the same-erasure guard would otherwise mis-fire an unwanted `(C<N>)(this)`.
//
// Everything else -- a plain variable / field / cast (`return (Multiset<E>) var0`), a ternary (arms are
// LUB'd to `X<Object>`, which the return target cannot re-drive per-arm), and an ordinary call whose type
// variable is pinned by its value arguments (`Iterables.filter(it, Predicates.instanceOf(cls))`) -- is a
// case where the erasure+unbounded guards in parameterizedReturnCast already make the cast both legal and
// needed. See parameterizedReturnCast.
func parameterizedReturnCastValueEligible(v values.JavaValue) bool {
	v = values.UnpackSoltValue(v)
	if ref, ok := v.(*values.JavaRef); ok && ref != nil && ref.IsThis {
		return false
	}
	if cv, ok := v.(*values.CustomValue); ok && cv.Flag == "lambda" {
		return false
	}
	if call, ok := v.(*values.FunctionCallExpression); ok {
		if len(call.Arguments) == 0 {
			return false
		}
		for _, a := range call.Arguments {
			if cv, ok := values.UnpackSoltValue(a).(*values.CustomValue); ok && cv.Flag == "lambda" {
				return false
			}
		}
	}
	return true
}

// parameterizedReturnCast returns the enclosing method's declared return type string to wrap the
// returned value in an explicit unchecked cast, when that return type is a PARAMETERIZED type whose
// EVERY top-level type argument is an in-scope type variable (or a wildcard) -- e.g. `Multiset$Entry<E>`
// -- but the returned value's static type erases to the SAME raw class carrying a DIFFERENT (erased /
// Object) type argument. This is the "return target-type inference" gap: guava iterators do
// `return Multisets.immutableEntry(var2, n)` where the callee `<E> Entry<E> immutableEntry(E, int)`
// would have inferred E from the enclosing `Entry<E>` return type, but var2 was read as Object off a
// raw iterator, so javac reports "inference variable E has incompatible bounds: E, Object". Wrapping
// the whole call `(Entry<E>) (Multisets.immutableEntry(var2, n))` removes the return-target inference
// constraint (the call now infers E=Object standalone) and the same-erasure `Entry<Object>` -> `Entry<E>`
// unchecked cast is legal (a concrete type argument would be provably distinct = inconvertible, so those
// are excluded). Behavior-preserving, matches CFR/Fernflower. Ordered AFTER objectReturnDowncast (whose
// Object-erased value it never overlaps, since Object's erasure differs from the parameterized target).
// Kill-switch JDEC_PARAM_RETURN_CAST_OFF.
func parameterizedReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil {
		return ""
	}
	if os.Getenv("JDEC_PARAM_RETURN_CAST_OFF") != "" {
		return ""
	}
	// Only wrap a returned value whose type variable CANNOT be recovered by javac's own return-target
	// inference (otherwise the cast is at best redundant and at worst BREAKS a working poly expression --
	// wrapping strips target typing, and if the return-type erasure is an unresolved dependency type the
	// added reference becomes a fresh "cannot find symbol", e.g. spring `return (Mono<T>)(x.map(l -> ...))`
	// where reactor Mono is off the classpath). Eligible shapes:
	//   - a ternary: its arms are LUB'd to `X<Object>`, which the return target cannot re-drive per-arm;
	//   - a method call PINNED to Object by a concrete argument (guava `immutableEntry(objVal, n)`): the
	//     argument forces the type var to Object, so target inference genuinely fails without the cast.
	// A zero-argument factory (`of()` / `emptyIterator()`) or a call with a lambda/method-ref argument is a
	// poly expression that infers correctly from the return target on its own -- excluded.
	if !parameterizedReturnCastValueEligible(v) {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	args, ok := topLevelTypeArgs(retStr)
	if !ok || len(args) == 0 {
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
	vStr := vt.String(funcCtx)
	if vStr == retStr {
		return "" // value already renders exactly as the return type; no cast needed.
	}
	// Same raw erasure guarantees the parameterization cast targets the value's own raw class (a prereq
	// for it not being an unrelated-class inconvertible cast).
	if erasureName(vStr) != erasureName(retStr) {
		return ""
	}
	// Per-position cast-legality gate. A same-erasure `X<va...>` -> `X<ta...>` cast is only emitted when
	// EVERY target arg ta is legal from the value's corresponding arg va, per javac's cast-conversion
	// rules (getting this wrong turns a recompile error into a different inconvertible-types error):
	//   - ta is an UNBOUNDED bare type variable (sole Object bound): always legal (`X<anything>` -> `X<E>`
	//     is an unchecked widening). A BOUNDED var (`C extends Comparable`) is inconvertible from Object
	//     and is EXCLUDED (leaves `ContiguousSet<C>` etc. to their own working target-type inference).
	//   - ta is a WILDCARD (`?` / `? extends X` / `? super X`): legal only when va is itself the unbounded
	//     wildcard `?` OR the value is raw (both admit an unchecked cast to any same-erasure wildcard --
	//     guava `TypeToken<? super T> boundAsSuperclass(){ return of(bound); }` where `of` returns
	//     `TypeToken<?>`). A CONCRETE va (`Object` / `String`) -> `? extends X` would be provably distinct
	//     = inconvertible, so that combination bails.
	//   - any other ta (a concrete `List<V>`, `Comparator<Object>`): bail.
	valArgs, valParam := topLevelTypeArgs(vStr)
	for i, ta := range args {
		if typeVarIsUnbounded(ta, funcCtx) {
			continue
		}
		if strings.HasPrefix(ta, "?") {
			if !valParam { // value is raw -> cast to any wildcard parameterization is unchecked-legal.
				continue
			}
			if i < len(valArgs) && valArgs[i] == "?" {
				continue
			}
		}
		return ""
	}
	return retStr
}

// inheritedFieldReturnCast returns the enclosing method's declared parameterized return type string to
// wrap a returned `this.field` read in an unchecked cast, when the field's RECOVERED real generic type
// shares the return type's raw erasure but carries a top-level WILDCARD argument that the return type
// pins to a concrete parameterization. The decompiler renders a plain / inherited field read as its raw
// erasure (it is not instantiated at the read site), so it emits a bare `return this.field` that javac
// rejects once it resolves the field's TRUE declared generic type: guava RegularImmutableSortedSet<E>
// `Comparator<Object> unsafeComparator() { return this.comparator; }` where `comparator` is inherited
// from ImmutableSortedSet as `Comparator<? super E>` -- javac reports "Comparator<CAP#1> cannot be
// converted to Comparator<Object>". The field's real type is recovered by the same cross-class hierarchy
// walk that powers inherited-field receiver typing (values.RecoverThisFieldInstantiatedType). Restricted
// to a WILDCARD-parameterized source: a `X<? ...>` -> same-erasure `X<concrete>` cast is an unchecked
// conversion (always legal), whereas a fully-CONCRETE source (`List<Integer>` -> `List<String>`) would
// be inconvertible and is excluded. Complements parameterizedReturnCast (which handles a type-variable /
// wildcard RETURN target and bails on a concrete one like `Comparator<Object>`). Kill-switch
// JDEC_INHERITED_FIELD_RET_CAST_OFF.
func inheritedFieldReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil {
		return ""
	}
	if os.Getenv("JDEC_INHERITED_FIELD_RET_CAST_OFF") != "" {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	if retArgs, ok := topLevelTypeArgs(retStr); !ok || len(retArgs) == 0 {
		return ""
	}
	fieldType := values.RecoverThisFieldInstantiatedType(funcCtx, v)
	if fieldType == nil {
		return ""
	}
	realStr := fieldType.String(funcCtx)
	if realStr == retStr {
		return "" // field already renders exactly as the return type; no cast needed.
	}
	// Same raw erasure is required, else the cast would target an unrelated class (inconvertible).
	if erasureName(realStr) != erasureName(retStr) {
		return ""
	}
	// The field's real type must carry a top-level WILDCARD argument (`?` / `? super X` / `? extends X`).
	// A wildcard-parameterized source casts to any same-erasure parameterization by an unchecked
	// conversion; a fully concrete source would be provably distinct = inconvertible.
	realArgs, ok := topLevelTypeArgs(realStr)
	if !ok {
		return ""
	}
	hasWildcard := false
	for _, a := range realArgs {
		if strings.HasPrefix(a, "?") {
			hasWildcard = true
			break
		}
	}
	if !hasWildcard {
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

// parameterizedFieldStoreRawCast returns the RAW erased name of a SAME-CLASS field's declared type
// (e.g. "Iterable") to wrap the stored value in an unchecked raw cast, when the field's generic
// Signature is a FULLY-CONCRETE parameterization (no wildcard) `X<A>` and the stored value's static type
// is `X<B>` -- SAME raw erasure, DIFFERENT type arguments. That is an invariant mismatch javac rejects
// ("Iterable<RangeMapEntry<K,V>> cannot be converted to Iterable<Entry<Range<K>,V>>"); the source
// carried an explicit raw cast (guava TreeRangeMap$AsMapOfRanges `this.entryIterable = (Iterable)
// entryIterable`, needed once the ctor param is recovered as `Iterable<RangeMapEntry<K,V>>` rather than
// erased). A raw cast to the value's OWN erasure is ALWAYS legal and the raw -> parameterized field
// assignment is an unchecked conversion, so this can only ever FIX a mismatch, never introduce one. A
// wildcard-bearing field is left to wildcardFieldStoreCast; a bare type-var field to
// typeVarFieldStoreCast; a raw or already-identical value needs no cast. Kill-switch
// JDEC_PARAM_FIELD_RAW_CAST_OFF.
func parameterizedFieldStoreRawCast(funcCtx *class_context.ClassContext, left, value values.JavaValue) string {
	if funcCtx == nil || left == nil || value == nil {
		return ""
	}
	if os.Getenv("JDEC_PARAM_FIELD_RAW_CAST_OFF") != "" {
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
	// Field must be a FULLY-CONCRETE parameterization: has `<...>` but NO wildcard (wildcard fields are
	// wildcardFieldStoreCast's job; a bare type-var field is typeVarFieldStoreCast's).
	if !strings.Contains(fieldTypeStr, "<") || strings.Contains(fieldTypeStr, "?") {
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
	vtStr := vt.String(funcCtx)
	// `this` reads as the RAW enclosing class (its value type carries no type arguments), but javac types
	// it as the class's OWN parameterization `ThisClass<ownParams>`. A `this.field = this` where the field
	// is `ThisClass<swappedOrOtherArgs>` (e.g. guava RegularImmutableBiMap's `inverse` field declared
	// `RegularImmutableBiMap<V, K>` assigned `this` of type `RegularImmutableBiMap<K, V>`) is the same
	// invariant mismatch, just hidden behind the raw `this` value type. Reconstruct `this`'s own
	// parameterization so the same-raw-different-args logic below fires. Kill-switch is the parent's
	// JDEC_PARAM_FIELD_RAW_CAST_OFF.
	if ref, ok := values.UnpackSoltValue(value).(*values.JavaRef); ok && ref.IsThis {
		if !strings.Contains(vtStr, "<") && len(funcCtx.ClassTypeParams) > 0 {
			vtStr = vtStr + "<" + strings.Join(funcCtx.ClassTypeParams, ", ") + ">"
		}
	}
	// Value must be a PARAMETERIZED type of the SAME raw erasure (a raw value assigns fine unchecked; a
	// different erasure is an unrelated type handled elsewhere) but with a DIFFERENT parameterization
	// (identical needs no cast) -- exactly the invariant `X<B>` -> `X<A>` mismatch.
	if !strings.Contains(vtStr, "<") {
		return ""
	}
	if erasureName(vtStr) != erasureName(fieldTypeStr) {
		return ""
	}
	if vtStr == fieldTypeStr {
		return ""
	}
	return erasureName(fieldTypeStr)
}

// wildcardReturnFieldStoreCast returns a SAME-CLASS field's declared parameterized type string to wrap a
// stored instance-call value in an unchecked cast, when the call's RECOVERED instantiated return type
// shares the field's raw erasure but carries a top-level WILDCARD argument that the field pins to a
// concrete parameterization. The call's return renders as its raw descriptor erasure (a jar-internal
// generic return is not instantiated at the call site), so `this.field = recv.m()` emits no cast, but
// javac types the call from the callee's TRUE generic return and rejects the invariant mismatch: guava
// ImmutableSortedMap$SerializedForm ctor `this.comparator = var1.comparator()` where `var1` is
// `ImmutableSortedMap<?,?>` and `ImmutableSortedMap.comparator()` returns `Comparator<? super K>`
// (captured `Comparator<CAP#1>`), assigned into the field `Comparator<Object>` -- javac reports
// "Comparator<CAP#1> cannot be converted to Comparator<Object>". The call's instantiated return is
// recovered via the sibling hierarchy walk (types.ResolveInstantiatedReturnType), the same machinery
// parameterizedReturnRawBridge uses. A wildcard-source same-erasure cast is an unchecked conversion
// (always legal); a fully-concrete source (`List<Integer>` -> `List<String>`) would be inconvertible and
// is excluded by the wildcard gate. Complements the two existing field-store casts, which never see this
// shape: wildcardFieldStoreCast needs a WILDCARD-bearing field (this one is concrete),
// parameterizedFieldStoreRawCast needs the VALUE to already RENDER parameterized (this call renders raw).
// Kill-switch JDEC_WILDCARD_RETURN_FIELD_CAST_OFF.
func wildcardReturnFieldStoreCast(funcCtx *class_context.ClassContext, left, value values.JavaValue) string {
	if funcCtx == nil || left == nil || value == nil {
		return ""
	}
	if os.Getenv("JDEC_WILDCARD_RETURN_FIELD_CAST_OFF") != "" {
		return ""
	}
	if funcCtx.SiblingClassSig == nil {
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
	// Field must be a FULLY-CONCRETE parameterization: has `<...>` but NO wildcard (a wildcard field is
	// wildcardFieldStoreCast's job, which already handles a raw-rendered value into a wildcard field).
	if !strings.Contains(fieldTypeStr, "<") || strings.Contains(fieldTypeStr, "?") {
		return ""
	}
	// Value must be an instance call on a jar-internal parameterized receiver, whose true instantiated
	// return the sibling hierarchy walk can recover.
	call, ok := values.UnpackSoltValue(value).(*values.FunctionCallExpression)
	if !ok || call.IsStatic || call.Object == nil {
		return ""
	}
	pt, ok := types.AsParameterizedType(call.Object.Type())
	if !ok || pt.RawClassName == "" {
		return ""
	}
	ret := types.ResolveInstantiatedReturnType(funcCtx, funcCtx.SiblingClassSig, pt.RawClassName, pt.TypeArgs, call.FunctionName, len(call.Arguments))
	if ret == nil {
		return ""
	}
	valRetStr := ret.String(funcCtx)
	if valRetStr == fieldTypeStr || erasureName(valRetStr) != erasureName(fieldTypeStr) {
		return ""
	}
	// The recovered return must carry a top-level WILDCARD (unchecked-castable to any same-erasure
	// concrete parameterization); a fully-concrete source would be provably distinct = inconvertible.
	retArgs, ok := topLevelTypeArgs(valRetStr)
	if !ok {
		return ""
	}
	for _, a := range retArgs {
		if strings.HasPrefix(a, "?") {
			return fieldTypeStr
		}
	}
	return ""
}

// subtypeValueFieldStoreCast returns a SAME-CLASS field's declared parameterized type string to wrap a
// stored value in an unchecked cast, when the value's ERASURE is a PROPER (jar-internal) SUBTYPE of the
// field's erasure and the field's top-level type arguments are all TYPE VARIABLES / wildcards. guava
// EndpointPairIterator<N> ctor `this.successorIterator = ImmutableSet.of().iterator()`: the field is
// `Iterator<N>`, but `ImmutableSet.of().iterator()` returns `UnmodifiableIterator<Object>` (the
// zero-arg factory `of()` infers `<Object>` with no target-typing across the `.iterator()` chain), so
// javac reports "UnmodifiableIterator<Object> cannot be converted to Iterator<N>". The decompiler
// renders the value's receiver raw, so no cast is emitted. Casting ANY subtype of `Iterator` to
// `Iterator<N>` (N a type variable) is UNCONDITIONALLY legal -- an unchecked widening to the supertype
// interface whose type argument is a variable is never "provably distinct", regardless of the value's
// real inferred argument -- which is exactly why this stays safe where a same-erasure nested cast would
// not. Restricted to a PROPER subtype (same-erasure is parameterizedFieldStoreRawCast's job) proven via
// the jar-internal supertype walk (funcCtx.SiblingSuperTypes / IsSubtypeVia; a JDK-only relation is not
// provable and declines), and to a field whose args are all type-variables/wildcards (a concrete arg
// could be provably distinct from the subtype value's inferred arg). Kill-switch
// JDEC_SUBTYPE_FIELD_STORE_CAST_OFF.
func subtypeValueFieldStoreCast(funcCtx *class_context.ClassContext, left, value values.JavaValue) string {
	if funcCtx == nil || left == nil || value == nil {
		return ""
	}
	if os.Getenv("JDEC_SUBTYPE_FIELD_STORE_CAST_OFF") != "" || funcCtx.SiblingSuperTypes == nil {
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
	if !strings.Contains(fieldTypeStr, "<") {
		return ""
	}
	// Every top-level field type argument must be an UNBOUNDED type variable or a wildcard, so the cast
	// `(X<..>)(subtypeValue)` is never provably distinct from the value's real (unknown) inferred args.
	// A BOUNDED variable (`C extends Comparable`) must be EXCLUDED: `Iterator<Object>` -> `Iterator<C>`
	// is then provably distinct (C can never be Object) = inconvertible, AND the assignment-context
	// target-typing that made the original compile (`this.elemItr = Iterators.emptyIterator()` infers
	// T=C) would be DESTROYED by wrapping the value in a cast (a cast is not an inference target). Only
	// an unbounded variable both keeps the cast legal and is a case where the chained/broken-inference
	// value genuinely needs it (guava EndpointPairIterator `N extends Object`).
	fargs, ok := topLevelTypeArgs(fieldTypeStr)
	if !ok || len(fargs) == 0 {
		return ""
	}
	for _, fa := range fargs {
		if strings.HasPrefix(fa, "?") || typeVarIsUnbounded(fa, funcCtx) {
			continue
		}
		return ""
	}
	if lit, ok := values.UnpackSoltValue(value).(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
		return ""
	}
	vt := value.Type()
	if vt == nil {
		return ""
	}
	if _, isPrim := vt.RawType().(*types.JavaPrimer); isPrim {
		return ""
	}
	// Field raw FQN: the field is parameterized, so take the parameterized type's raw class name.
	fpt, ok := types.AsParameterizedType(ft)
	if !ok || fpt.RawClassName == "" {
		return ""
	}
	fieldFQN := fpt.RawClassName
	// Value raw FQN: a raw class value yields it directly; a parameterized value via its raw class name.
	valFQN, ok := types.ClassFQNOf(vt)
	if !ok {
		if vpt, okp := types.AsParameterizedType(vt); okp {
			valFQN = vpt.RawClassName
		}
	}
	if valFQN == "" {
		return ""
	}
	if valFQN == fieldFQN { // same erasure -> parameterizedFieldStoreRawCast / wildcardReturnFieldStoreCast
		return ""
	}
	if !types.IsSubtypeVia(valFQN, fieldFQN, funcCtx.SiblingSuperTypes) {
		return ""
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
// typeVarArrayElementStoreCast returns the bare class-scope type variable to cast the RHS of a
// SAME-CLASS type-variable-array ELEMENT store -- `this.buffer[i] = objExpr` / `this.values[r][c] =
// objExpr` where the field is declared `T[]` / `V[][]`. The array's component type is the type
// variable (T / V), but the value flowing in is the erased `Object` (a raw `Iterator.next()`,
// `Map.Entry.getValue()`, or another erased field/array read), so the source carried an unchecked
// `(T)` cast that the `aastore` opcode erased to a no-op (T erases to its Object bound). Without it
// javac -- re-checking against the declared `T[]` -- rejects `Object cannot be converted to T` (guava
// TopKSelector `buffer[i]`, HashBiMap `keys[i]`/`values[i]`, DenseImmutableTable `values[r][c]`,
// ImmutableSortedMultiset$SerializedForm `elements[i]`). This is the array-element analogue of
// typeVarFieldStoreCast (whole-field store) and the LHS counterpart of expression.go's
// typeVarArrayArgCast (call argument).
//
// The element type variable is read from the field's recorded generic Signature (FieldTypeVar, e.g.
// `keys` -> `K[]`) rather than the value's possibly-erased Type(), so it matches EXACTLY the array
// field declaration the dumper emits. Tightly gated:
//   - the LHS is a (possibly nested) array-element access whose base is a same-class field (`this.f`
//     or `CurrentClass.f`); a non-field array base (a local) is out of scope;
//   - the index dimension count equals the field's array depth (a PARTIAL index leaves an array type
//     variable `V[]`, not a bare `V` -- storing a whole sub-array is a different, array-typed
//     assignment and must not take a scalar `(V)` cast);
//   - the resulting component is a denotable class-scope type variable (FieldTypeVar already gates on
//     IsTypeParam at recording time);
//   - the value is a non-null reference whose rendered type is not already that type variable (a null
//     literal and an already-T value need no cast; a primitive cannot reach an aastore).
//
// Kill-switch: JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF=1.
func typeVarArrayElementStoreCast(funcCtx *class_context.ClassContext, member *values.JavaArrayMember, value values.JavaValue) string {
	if os.Getenv("JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF") != "" {
		return ""
	}
	if funcCtx == nil || member == nil || value == nil || len(funcCtx.FieldTypeVars) == 0 {
		return ""
	}
	// Unwrap nested array-element accesses, counting index depth, to reach the array base.
	depth := 0
	var base values.JavaValue = member
	for {
		am, ok := values.UnpackSoltValue(base).(*values.JavaArrayMember)
		if !ok {
			break
		}
		depth++
		base = am.Object
	}
	if depth == 0 {
		return ""
	}
	// Base must be a same-class field: `this.field` or `CurrentClass.field`.
	var fieldName string
	switch lv := values.UnpackSoltValue(base).(type) {
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
	tv := funcCtx.FieldTypeVar(fieldName) // e.g. "K[]", "V[][]"
	if tv == "" {
		return ""
	}
	tvDepth := strings.Count(tv, "[]")
	if tvDepth == 0 || depth != tvDepth {
		// scalar type-var field (no element store) or partial index (still an array) -- skip.
		return ""
	}
	elem := strings.TrimSuffix(tv, strings.Repeat("[]", tvDepth))
	if elem == "" || !funcCtx.IsTypeParam(elem) {
		return ""
	}
	// null is assignable to any type variable without a cast.
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
	// A type variable is a reference type; a primitive value never needs (and cannot take) the cast.
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	// Already rendered as the type variable: no cast needed.
	if raw.String(funcCtx) == elem {
		return ""
	}
	return elem
}

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
			// A non-literal int RHS into a boolean[] element (`boolArr[i] = cond ? 1 : 0`, spring ASM
			// ClassReader.readTypeAnnotationTarget / AttributeMethods.<init>) is a boolean materialized as
			// an int; Java rejects the implicit int->boolean store. Retype the 0/1 diamond to boolean and
			// fold it (`cond ? 1 : 0` -> `cond`). CoerceBooleanAssignRHS no-ops on already-boolean values.
			if coerced := values.CoerceBooleanAssignRHS(elem, value, funcCtx); coerced != value {
				return coerced.String(funcCtx)
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
		// Type-variable-array element store (`this.keys[i] = objExpr` where keys is `K[]`) needs an
		// unchecked `(K)` cast: the aastore erased the source cast to a no-op. See
		// typeVarArrayElementStoreCast.
		if cast := typeVarArrayElementStoreCast(funcCtx, member, value); cast != "" {
			return fmt.Sprintf("(%s) (%s)", cast, value.String(funcCtx))
		}
	}
	return value.String(funcCtx)
}

// ternaryHasClassLiteralArm reports whether either arm of the ternary is a class literal (`Foo.class`,
// a JavaClassValue). Such an arm's Type() reports the referenced class rather than java.lang.Class, so
// the ternary's arm-merge can under-type to the arms' LUB; the declaration path uses this to prefer the
// slot ref's (correct) resolved type. See AssignStatement.String.
func ternaryHasClassLiteralArm(tern *values.TernaryExpression) bool {
	if tern == nil {
		return false
	}
	if _, ok := values.UnpackSoltValue(tern.TrueValue).(*values.JavaClassValue); ok {
		return true
	}
	if _, ok := values.UnpackSoltValue(tern.FalseValue).(*values.JavaClassValue); ok {
		return true
	}
	return false
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
	rhsVal := values.CoerceIntAssignRHS(a.LeftValue.Type(), a.JavaValue, funcCtx)
	rhsStr := rhsVal.String(funcCtx)
	// A lambda / method-reference REASSIGNED into a slot whose declared type is the RAW form of the
	// target functional interface (the slot was first declared from a raw getfield, so it never adopted
	// the lambda's parameterized type) needs an explicit cast to its own instantiated type, else the
	// explicitly-typed lambda parameters / method reference cannot bind to the raw SAM. Applied only on
	// reassignment: on a first declaration the slot adopts the lambda's parameterized type directly. No-op
	// for the common (correctly parameterized) target. See values.LambdaAssignFunctionalCast.
	if !a.IsFirst {
		if cast := values.LambdaAssignFunctionalCast(a.LeftValue, a.JavaValue, funcCtx); cast != "" {
			rhsStr = fmt.Sprintf("(%s)(%s)", cast, rhsStr)
		}
	}
	assign := fmt.Sprintf("%s = %s", a.LeftValue.String(funcCtx), rhsStr)
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
		// A ternary with a class-literal arm (`cond ? Foo.class : classField`) is a java.lang.Class
		// value, but the arm's JavaValue.Type() reports the *referenced* class (Foo) to drive bare
		// `Foo.class` rendering, so the arm-merge collapses to the arms' LUB (Object for a
		// Class-object-vs-Object.class pair) and the RHS ternary reports `Object`. The slot ref, by
		// contrast, was minted from the fresh arm-merge and already resolved to the correct `Class`.
		// Prefer the (narrower, authoritative) ref type here so `var.getModifiers()/getName()` recompile
		// (spring-core cglib Enhancer.generateClass). Guarded to only NARROW toward the ref when the ref
		// is a subtype of the RHS type, so it never widens away from a precise RHS. Shares the
		// class-literal kill-switch JDEC_NO_CLASSLIT_SLOT_TYPE.
		if os.Getenv("JDEC_NO_CLASSLIT_SLOT_TYPE") == "" {
			if tern, ok := values.UnpackSoltValue(a.JavaValue).(*values.TernaryExpression); ok && ternaryHasClassLiteralArm(tern) {
				// The slot ref (LeftValue) is minted from the FRESH arm-merge (class-literal arm counted
				// as java.lang.Class) and is the authoritative resolved slot type, whereas the ternary's
				// cached Type() can be the stale over-wide arms-LUB (Object). Prefer the ref type; it is
				// always assignment-compatible with the RHS java.lang.Class value.
				if lt := a.LeftValue.Type(); lt != nil {
					declType = lt
				}
			}
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
		// A local whose initializer is a jar-internal generic call returning a bare type variable T (but
		// erased to T's bound in the descriptor) must be declared at T, not the erased bound, or later
		// T-typed uses fail to compile. javac re-derives the RHS as T, so no RHS cast is needed. See
		// typeVarLocalDeclName (guava Cut$AboveValue: `C var3 = domain.next(...)`).
		if tv := typeVarLocalDeclName(funcCtx, a.LeftValue, a.JavaValue, declType); tv != "" {
			return tv + " " + assign
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
		// Same-erasure invariant field-store mismatch (`X<B>` value into concrete `X<A>` field): the
		// source carried a raw `(X)` cast that bytecode erased. See parameterizedFieldStoreRawCast.
		if cast := parameterizedFieldStoreRawCast(funcCtx, a.LeftValue, a.JavaValue); cast != "" {
			return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
		}
		// Concrete `X<A>` field assigned a raw-rendered call whose RECOVERED instantiated return is a
		// same-erasure WILDCARD parameterization (`this.comparator = var1.comparator()` -> the callee
		// truly returns `Comparator<? super K>`): wrap in `(X<A>)`. See wildcardReturnFieldStoreCast.
		if cast := wildcardReturnFieldStoreCast(funcCtx, a.LeftValue, a.JavaValue); cast != "" {
			return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
		}
		// Proper-subtype value into a type-variable-parameterized field (`this.successorIterator =
		// ImmutableSet.of().iterator()` -> field `Iterator<N>`, value a subtype `UnmodifiableIterator`):
		// wrap in `(X<typevars>)`. See subtypeValueFieldStoreCast.
		if cast := subtypeValueFieldStoreCast(funcCtx, a.LeftValue, a.JavaValue); cast != "" {
			return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
		}
		// A LOCAL variable declared as an invariant parameterization mentioning a type variable
		// (`Class<T> var1`) REASSIGNED from a method call of the SAME erasure whose true generic return
		// is assignment-incompatible (`var1 = var1.getSuperclass()`, where getSuperclass() returns
		// `Class<? super T>` -> capture, NOT assignable to `Class<T>`): the source carried a raw
		// `(Class)` cast that bytecode erased. See parameterizedLocalReassignRawCast (objenesis
		// SerializationInstantiatorHelper / PercSerializationInstantiator).
		if cast := parameterizedLocalReassignRawCast(funcCtx, a.LeftValue, a.JavaValue); cast != "" {
			return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
		}
		return assign
	}
}

// parameterizedLocalReassignRawCast returns the raw erasure (`Class`) to cast the RHS of a LOCAL-variable
// REASSIGNMENT when the local is declared as an INVARIANT parameterization mentioning an in-scope type
// variable (`Class<T> var1`, no wildcard) and the reassigned value is a METHOD CALL of the SAME erasure
// but a DIFFERENT rendering -- the exact shape javac rejects once it resolves the callee's true generic
// return. The canonical hit is objenesis's `Class<? super T> result = type; result = result.getSuperclass();`
// which the decompiler types the local at `Class<T>` (from the initial `= type` store) while
// `Class.getSuperclass()` truly returns `Class<? super T>` (captured to `Class<CAP super T>`), giving
// "Class<CAP#1> cannot be converted to Class<T>". The source's raw `(Class)` cast erases the value to the
// raw type, which then unchecked-converts to `Class<T>` -- legal and behavior-identical (the slot erases
// to the raw type at runtime). Restricted to a JavaRef local (never `this`, never a field -- those have
// their own field-store helpers), an invariant (`<...>` without `?`) type-variable-mentioning declared
// type, and a method-call value of the same erasure whose rendering differs; a same-string (already
// matching) reassignment is skipped. A raw or same-erasure value returned bare into this shape never
// compiles cleanly, so a match is always a genuine repair. Kill-switch JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF.
func parameterizedLocalReassignRawCast(funcCtx *class_context.ClassContext, left, value values.JavaValue) string {
	if funcCtx == nil || left == nil || value == nil {
		return ""
	}
	if os.Getenv("JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF") != "" {
		return ""
	}
	// LHS must be a genuine local (JavaRef, non-`this`), not a field or synthetic slot.
	ref, ok := values.UnpackSoltValue(left).(*values.JavaRef)
	if !ok || ref.IsThis {
		return ""
	}
	lt := left.Type()
	if lt == nil {
		return ""
	}
	ltStr := lt.String(funcCtx)
	// Declared type must be an INVARIANT parameterization (`<...>` without a wildcard) that mentions an
	// in-scope type variable -- the `Class<T>` shape whose invariance rejects a `Class<? super T>` value.
	if !strings.Contains(ltStr, "<") || strings.Contains(ltStr, "?") {
		return ""
	}
	if !mentionsTypeParam(ltStr, funcCtx.TypeParams) {
		return ""
	}
	// The reassigned value must be a method CALL (a plain copy `var1 = var2` shares the declared type and
	// needs no cast; a call is where a hidden covariant/wildcard return hides).
	call, ok := values.UnpackSoltValue(value).(*values.FunctionCallExpression)
	if !ok || call == nil || call.Object == nil {
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
	vtStr := vt.String(funcCtx)
	// Same erasure (a genuine same-raw-type reassignment, not an unrelated type) but a different
	// rendering (an already-matching value needs no cast).
	if erasureName(vtStr) != erasureName(ltStr) || vtStr == ltStr {
		return ""
	}
	return erasureName(ltStr)
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
