package values

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type NewExpression struct {
	types.JavaType
	Length          []JavaValue
	ArgumentsGetter func() string
	Initializer     []JavaValue
	// ConstructorCall holds the invokespecial `<init>` call whose arguments this `new T(...)`
	// renders through ArgumentsGetter. The arguments live ONLY inside that closure (string
	// rendering), so without this back-reference any value-tree traversal — most importantly
	// RewriteVar's idReplaceMap ReplaceVar pass — cannot reach the argument SlotValues. A
	// constructor argument that is a freshly-bound local then keeps the stale slot-derived
	// `varN` name minted before renaming while its declaration is renamed, so the call site and
	// the declaration disagree and a colliding `varN` resolves to the wrong same-name variable
	// (commons-codec DaitchMokotoffSoundex: `new Rule(p, q, parts, ...)` instead of
	// `new Rule(p, q, r, ...)`). ReplaceVar recurses into ConstructorCall.Arguments only, never
	// its Object (which is this same NewExpression — recursing it would loop). Kill-switch:
	// JDEC_NO_CTOR_ARG_REPLACE=1.
	ConstructorCall *FunctionCallExpression
}

// ReplaceVar implements JavaValue.
func (n *NewExpression) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	for _, length := range n.Length {
		length.ReplaceVar(oldId, newId)
	}
	for _, initializer := range n.Initializer {
		initializer.ReplaceVar(oldId, newId)
	}
	if n.ConstructorCall != nil && os.Getenv("JDEC_NO_CTOR_ARG_REPLACE") == "" {
		for _, arg := range n.ConstructorCall.Arguments {
			if arg != nil {
				arg.ReplaceVar(oldId, newId)
			}
		}
	}
}

func NewNewArrayExpression(typ types.JavaType, length ...JavaValue) *NewExpression {
	return &NewExpression{
		JavaType: typ,
		Length:   length,
	}
}

// coerceInitializerLiteral renders an array-initializer element with the array's
// element type when that yields more faithful source. Today this only matters for
// boolean element types: a boolean[] initializer is filled by iconst_0/iconst_1,
// whose values carry an int type, so they must be rendered as false/true.
func coerceInitializerLiteral(v JavaValue, elemType types.JavaType, funcCtx *class_context.ClassContext) string {
	if lit, ok := v.(*JavaLiteral); ok {
		if elemType.String(funcCtx) == types.NewJavaPrimer(types.JavaBoolean).String(funcCtx) {
			if n, ok := lit.Data.(int); ok {
				if n == 0 {
					return "false"
				}
				return "true"
			}
		}
	}
	return v.String(funcCtx)
}

func NewNewExpression(typ types.JavaType) *NewExpression {
	return &NewExpression{
		JavaType: typ,
	}
}
func (n *NewExpression) Type() types.JavaType {
	return n.JavaType
}

func (n *NewExpression) String(funcCtx *class_context.ClassContext) string {
	if n.IsArray() {
		base := n.JavaType
		for base.IsArray() {
			base = base.ElementType()
		}
		s := fmt.Sprintf("new %s", base.String(funcCtx))
		// An explicit initializer (new T[]{...}) is incompatible with a sized dimension
		// (new T[3]{...} is a javac error); the literal supplies the length, so drop the
		// first numeric dimension and emit empty brackets per array dimension instead.
		if len(n.Initializer) != 0 {
			for i := 0; i < n.JavaType.ArrayDim(); i++ {
				s += "[]"
			}
			vsStr := []string{}
			for _, v := range n.Initializer {
				// Coerce int 0/1 literals to boolean false/true when the array element type is
				// boolean: iconst_0/iconst_1 fill a boolean[] but carry an int type, so without
				// this coercion the initializer renders `new boolean[]{1,1,1,1}`, which javac
				// rejects ("int cannot be converted to boolean").
				vsStr = append(vsStr, coerceInitializerLiteral(v, base, funcCtx))
			}
			s += fmt.Sprintf("{%s}", strings.Join(vsStr, ","))
			return s
		}
		for _, l := range n.Length {
			s += fmt.Sprintf("[%v]", l.(JavaValue).String(funcCtx))
		}
		for i := len(n.Length); i < n.JavaType.ArrayDim(); i++ {
			s += "[]"
		}
		return s
	}
	var args string
	if n.ArgumentsGetter != nil {
		args = n.ArgumentsGetter()
	}
	name := n.JavaType.String(funcCtx)
	if !strings.Contains(name, "<") {
		name += n.genericCtorDiamond(funcCtx)
	}
	return fmt.Sprintf("new %s(%s)", name, args)
}

// genericCtorDiamond returns "<>" when this `new T(...)` constructs a GENERIC jar-internal class and at
// least one constructor argument is a method reference or lambda (a LambdaFuncRef). The decompiler
// renders constructor instantiations of generic classes RAW (`new ObjectReaderImplFromString(...)`),
// which is normally only a harmless unchecked warning -- EXCEPT when an argument is a method reference
// or lambda: a raw instantiation erases the constructor's functional-interface parameter (e.g.
// `Function<String,T>` collapses to raw `Function`), so javac cannot type the method reference against
// the raw SAM and rejects it ("incompatible types: invalid method reference"). The canonical case is
// fastjson2 `new ObjectReaderImplFromString(Duration.class, Duration::parse)` and its URI/Charset/
// Pattern/ZoneOffset/ZoneId/TimeZone siblings. Emitting the diamond `<>` restores the source form: javac
// infers the class's type argument from the constructor arguments (`Class<Duration>` -> T=Duration),
// re-parameterizing the functional-interface parameter so the method reference binds. Gated to (a) a
// class the SiblingClassSig resolver confirms is generic and (b) a method-reference/lambda argument, so
// non-generic classes and ordinary raw instantiations (whose unchecked-warning behaviour is intentionally
// preserved) are never touched. Kill-switch: JDEC_CTOR_DIAMOND_OFF=1.
func (n *NewExpression) genericCtorDiamond(funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_CTOR_DIAMOND_OFF") != "" || funcCtx == nil || funcCtx.SiblingClassSig == nil || n.ConstructorCall == nil {
		return ""
	}
	jc, ok := n.JavaType.RawType().(*types.JavaClass)
	if !ok || jc == nil {
		return ""
	}
	hasLambdaArg := false
	for _, a := range n.ConstructorCall.Arguments {
		if cv, ok := UnpackSoltValue(a).(*CustomValue); ok && cv.Flag == "lambda" {
			hasLambdaArg = true
			break
		}
	}
	if !hasLambdaArg {
		return ""
	}
	classSig, _, ok := funcCtx.SiblingClassSig(strings.ReplaceAll(jc.Name, ".", "/"))
	if !ok || len(types.ClassFormalTypeParamNames(classSig)) == 0 {
		return ""
	}
	return "<>"
}

type JavaExpression struct {
	Values []JavaValue
	Op     string
	Typ    types.JavaType
}

// ReplaceVar implements JavaValue.
func (j *JavaExpression) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	for _, value := range j.Values {
		value.ReplaceVar(oldId, newId)
	}
}

func (j *JavaExpression) Type() types.JavaType {
	// A non-short-circuit boolean connective (& | ^) of two boolean operands is boolean-typed even
	// when its operands reach it as int-shaped `cond ? 1 : 0` ternaries (built by a later CFG pass,
	// after NewBinaryExpression already fixed j.Typ to int). Reporting boolean here lets a boolean
	// context (return/assign/condition) accept it; see String() for the matching rendering.
	if _, _, ok := j.boolConnectiveConds(); ok {
		return types.NewJavaPrimer(types.JavaBoolean)
	}
	return j.Typ
}

// boolConnectiveConds reports whether this expression is `a & b`, `a | b` or `a ^ b` where BOTH
// operands are boolean (either already boolean-typed, or the int `cond ? 1 : 0` shape javac emits
// for a comparison feeding an integer bitwise op). It returns the two underlying boolean conditions.
// This recovers the original `cond1 & cond2` boolean connective instead of the int-typed
// `(c1?1:0) & (c2?1:0)`, which fails to compile where a boolean is required.
func (j *JavaExpression) boolConnectiveConds() (JavaValue, JavaValue, bool) {
	if len(j.Values) != 2 || (j.Op != AND && j.Op != OR && j.Op != XOR) {
		return nil, nil, false
	}
	c1, ok1 := boolOperandCondition(j.Values[0])
	c2, ok2 := boolOperandCondition(j.Values[1])
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	return c1, c2, true
}

// boolOperandCondition returns the boolean condition underlying a `cond ? 1 : 0` ternary, or the
// value itself when it is already boolean-typed (a comparison or a nested boolean connective).
func boolOperandCondition(v JavaValue) (JavaValue, bool) {
	u := UnpackSoltValue(v)
	if cond, ok := BoolTernaryCondition(u); ok {
		return cond, true
	}
	if isBooleanTyped(u) {
		return u, true
	}
	return nil, false
}

func (j *JavaExpression) String(funcCtx *class_context.ClassContext) string {
	if c1, c2, ok := j.boolConnectiveConds(); ok {
		return fmt.Sprintf("(%s) %s (%s)",
			SimplifyConditionValue(c1).String(funcCtx), j.Op, SimplifyConditionValue(c2).String(funcCtx))
	}
	vs := []string{}
	for _, value := range j.Values {
		vs = append(vs, value.String(funcCtx))
	}
	if len(vs) == 1 {
		return fmt.Sprintf("%s(%s)", j.Op, vs[0])
	}
	switch j.Op {
	case ADD:
		return fmt.Sprintf("(%s) + (%s)", vs[0], vs[1])
	case INC:
		return fmt.Sprintf("%s++", vs[0])
	case DEC:
		return fmt.Sprintf("%s--", vs[0])
	case GT, SUB:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	default:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	}
}

// UnaryMinusOperand renders v as the operand of a leading unary minus, wrapping it in parentheses
// whenever the bare "-"+v form would re-associate or merge tokens. The JVM emits `... ; ineg` for a
// negated sub-expression, so an arithmetic `-(a + b)` arrives as Neg(Add(a,b)); rendering it as
// "-" + "(a) + (b)" silently re-parses as "(-a) + b" (wrong value). It also guards "-" + "-x" /
// "-" + "+x" from fusing into the predecrement/increment tokens "--"/"-+". Simple operands
// (refs, literals, fields, calls, array loads) are left unparenthesised to keep output readable.
func UnaryMinusOperand(v JavaValue, funcCtx *class_context.ClassContext) string {
	s := v.String(funcCtx)
	needParen := false
	switch uv := UnpackSoltValue(v).(type) {
	case *JavaExpression:
		// A binary expression (two operands) binds looser than unary minus and must be wrapped.
		if len(uv.Values) >= 2 {
			needParen = true
		}
	case *TernaryExpression:
		needParen = true
	}
	if !needParen && (strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+")) {
		needParen = true
	}
	if needParen {
		return "(" + s + ")"
	}
	return s
}

// primerRawType returns the *types.JavaPrimer raw type of t, guarding against a nil JavaType
// (which incomplete stack simulation can produce) so callers never nil-dereference RawType().
func primerRawType(t types.JavaType) (*types.JavaPrimer, bool) {
	if t == nil {
		return nil, false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	return p, ok
}

func isBooleanTyped(v JavaValue) bool {
	if v == nil {
		return false
	}
	uv := UnpackSoltValue(v)
	if uv == nil {
		return false
	}
	t := uv.Type()
	if t == nil {
		return false
	}
	prim, ok := t.RawType().(*types.JavaPrimer)
	return ok && prim.Name == types.JavaBoolean
}

// resetTypeSafe resets v's type to t, but only when v already carries a non-nil JavaType.
// Incomplete stack simulation can leave a value with a nil Type(); skipping the reset there
// avoids a nil-dereference while leaving correctly-typed values unchanged.
func resetTypeSafe(v JavaValue, t types.JavaType) {
	if v == nil {
		return
	}
	if vt := v.Type(); vt != nil {
		vt.ResetType(t)
	}
}

// nonNilType returns the first non-nil candidate, falling back to int. Expression constructors use
// it so a nil result type (which incomplete stack simulation can yield for an operand) degrades to
// a sensible default instead of panicking at typ.Copy().
func nonNilType(candidates ...types.JavaType) types.JavaType {
	for _, c := range candidates {
		if c != nil {
			return c
		}
	}
	return types.NewJavaPrimer(types.JavaInteger)
}

func NewUnaryExpression(value1 JavaValue, op string, typ types.JavaType) *JavaExpression {
	if IsStrictBooleanOperator(op) {
		resetTypeSafe(value1, types.NewJavaPrimer(types.JavaBoolean))
	}
	return &JavaExpression{
		Values: []JavaValue{value1},
		Op:     op,
		Typ:    nonNilType(typ, value1.Type()).Copy(),
	}
}
func NewBinaryExpression(value1, value2 JavaValue, op string, typ types.JavaType) *JavaExpression {
	if IsStrictBooleanOperator(op) {
		resetTypeSafe(value1, types.NewJavaPrimer(types.JavaBoolean))
		resetTypeSafe(value2, types.NewJavaPrimer(types.JavaBoolean))
	} else if (op == AND || op == OR || op == XOR) && (isBooleanTyped(value1) || isBooleanTyped(value2)) {
		// &, |, ^ are shared between boolean logic and integer bitwise arithmetic. Decide by
		// the operands: if either side is already boolean (e.g. descriptor-typed parameters or
		// a negation), this is boolean logic, so align both sides to boolean. Otherwise leave
		// the operands as their inferred integer type.
		resetTypeSafe(value1, types.NewJavaPrimer(types.JavaBoolean))
		resetTypeSafe(value2, types.NewJavaPrimer(types.JavaBoolean))
		typ = types.NewJavaPrimer(types.JavaBoolean)
	}
	resultType := nonNilType(typ, value1.Type(), value2.Type()).Copy()
	resultType = promoteBinaryNumericResult(op, resultType)
	return &JavaExpression{
		Values: []JavaValue{value1, value2},
		Op:     op,
		Typ:    resultType,
	}
}

// promoteBinaryNumericResult applies JLS 5.6.2 binary numeric promotion to the result type of an
// arithmetic/bitwise/shift binary operator: when both operands are in the int computational category
// (byte/char/short/int) the operation is evaluated in int and yields int — never the narrower operand
// type. The bytecode confirms this (iadd/iand/ishl/... consume and produce the int stack category;
// there is NO byte/short/char arithmetic opcode), so reporting a narrow operand type for the result
// disagrees with javac. That disagreement defeated both the slot-merge in AssignVar (a `byte + 256`
// reassign looked byte-typed, matched the slot type, and never widened the slot to int) and the
// store/return narrowing-cast rendering (which keys off an int-typed value), producing uncompilable
// "possible lossy conversion from int to byte" stores (commons-codec Base32/Base64
// `byte b = in[i++]; if (b < 0) b = b + 256;`). The narrowing cast is reintroduced precisely at the
// byte/short/char-typed use sites (declaration/return/arg/array-store). Only a byte/char/short result
// is promoted: int stays int and long/float/double already carry the wider category via `typ`. The
// boolean &|^ case (handled just above) and comparison operators are left untouched. Kill-switch:
// JDEC_NO_BINNUM_PROMOTE=1.
func promoteBinaryNumericResult(op string, resultType types.JavaType) types.JavaType {
	switch op {
	case ADD, SUB, MUL, DIV, REM, AND, OR, XOR, SHL, SHR, USHR:
	default:
		return resultType
	}
	if os.Getenv("JDEC_NO_BINNUM_PROMOTE") != "" {
		return resultType
	}
	p, ok := resultType.RawType().(*types.JavaPrimer)
	if !ok {
		return resultType
	}
	switch p.Name {
	case types.JavaByte, types.JavaChar, types.JavaShort:
		return types.NewJavaPrimer(types.JavaInteger)
	}
	return resultType
}

type FunctionCallExpression struct {
	IsStatic     bool
	Object       JavaValue
	FunctionName string
	ClassName    string
	Arguments    []JavaValue
	FuncType     *types.JavaFuncType
	// IsSpecialInvoke marks a call decoded from invokespecial. For a non-constructor invokespecial
	// whose receiver is `this` and whose target class is NOT the current class, this is a `super.m()`
	// call (the only other invokespecial forms are constructors and private same-class calls). It must
	// render as `super.m()`, never `this.m()` -- the latter re-dispatches virtually to the overriding
	// method and recurses infinitely (e.g. guava CaseFormat constant-body `convert` -> StackOverflow).
	IsSpecialInvoke bool
}

// ReplaceVar implements JavaValue.
func (f *FunctionCallExpression) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if f.Object != nil {
		f.Object.ReplaceVar(oldId, newId)
	}
	for _, arg := range f.Arguments {
		arg.ReplaceVar(oldId, newId)
	}
}

func (f *FunctionCallExpression) Type() types.JavaType {
	// Phase 3 (generic.go) limited generic instantiation (Bug AH): the descriptor return type is
	// erased (Iterable<T>.iterator() -> raw Iterator, Iterator<T>.next() -> Object), so a slot typed
	// from such a call loses its element type and later uses fail to recompile (`Object cannot be
	// converted to T`, guava PairwiseEquivalence). When the receiver carries concrete type arguments
	// and the method is a known JDK container method, substitute the receiver's args into the JDK
	// signature to recover the instantiated return (Iterator<T>, then T). Kill-switch:
	// JDEC_GENERIC_INFER_OFF. Conservative: only the small provably-correct JDK table fires.
	if inst := f.instantiatedReturnType(); inst != nil {
		return inst
	}
	return f.FuncType.ReturnType
}

// instantiatedReturnType applies InstantiateJDKMethodReturn using the receiver's parameterized type,
// or returns nil to keep the erased descriptor return.
func (f *FunctionCallExpression) instantiatedReturnType() types.JavaType {
	if os.Getenv("JDEC_GENERIC_INFER_OFF") != "" || f.IsStatic || f.Object == nil {
		return nil
	}
	recv := f.Object.Type()
	pt, ok := types.AsParameterizedType(recv)
	if !ok {
		return nil
	}
	return types.InstantiateJDKMethodReturn(pt.RawClassName, f.FunctionName, len(f.Arguments), pt.TypeArgs)
}

// receiverParamTypeArgs returns the call receiver's raw class name and actual type arguments, taken
// from the receiver value's parameterized static type, or -- when the receiver is a same-class field
// whose getfield value carries only the ERASED descriptor type (raw `BiConsumer`) -- from the field's
// recorded generic Signature (funcCtx.FieldSignatures, e.g. `Ljava/util/function/BiConsumer<TT;TV;>;`).
// Returns ("", nil) when no parameterized receiver type is available. The field-signature fallback is
// independently gated by JDEC_GENERIC_PARAM_FIELD_OFF.
func (f *FunctionCallExpression) receiverParamTypeArgs(funcCtx *class_context.ClassContext) (string, []types.JavaType) {
	if f.Object == nil {
		return "", nil
	}
	if pt, ok := types.AsParameterizedType(f.Object.Type()); ok {
		return pt.RawClassName, pt.TypeArgs
	}
	if funcCtx == nil || os.Getenv("JDEC_GENERIC_PARAM_FIELD_OFF") != "" {
		return "", nil
	}
	// Same-class field receiver (`this.field`): recover type args from the field's generic Signature.
	rm, ok := UnpackSoltValue(f.Object).(*RefMember)
	if !ok {
		return "", nil
	}
	ref, ok := UnpackSoltValue(rm.Object).(*JavaRef)
	if !ok || !ref.IsThis {
		return "", nil
	}
	sig := funcCtx.FieldSignature(class_context.SafeIdentifier(rm.Member))
	if sig == "" {
		return "", nil
	}
	parsed := types.ParseSignature(sig)
	if parsed == nil {
		return "", nil
	}
	pt, ok := types.AsParameterizedType(parsed)
	if !ok {
		return "", nil
	}
	return pt.RawClassName, pt.TypeArgs
}

// instantiatedParamType applies InstantiateJDKMethodParam using the receiver's parameterized type to
// recover the generic type of the i-th parameter (which the descriptor erases to its bound), or nil
// to keep the erased descriptor parameter. Feeding the instantiated type back into ArgumentStrings
// lets the existing cast logic re-emit the source's `(V)`/`(T)` argument cast (the fastjson2
// BiConsumer.accept / Map.put generic-erasure blocker). The receiver may be a parameterized local /
// parameter / this, OR a same-class parameterized field (see receiverParamTypeArgs). Kill-switch
// JDEC_GENERIC_PARAM_INFER_OFF.
func (f *FunctionCallExpression) instantiatedParamType(i int, funcCtx *class_context.ClassContext) types.JavaType {
	if os.Getenv("JDEC_GENERIC_PARAM_INFER_OFF") != "" || f.IsStatic || f.Object == nil {
		return nil
	}
	raw, typeArgs := f.receiverParamTypeArgs(funcCtx)
	if raw == "" || len(typeArgs) == 0 {
		return nil
	}
	return types.InstantiateJDKMethodParam(raw, f.FunctionName, len(f.Arguments), i, typeArgs)
}

// sameClassMethodParamType recovers the i-th formal parameter type of a call to a same-class generic
// method on `this` (e.g. `this.tailSet(objVal)` where the current class declares
// `SortedSet<E> tailSet(E)`), so the descriptor-erased `(E)` argument cast can be re-emitted (guava
// Forwarding* / collection family: `Object cannot be converted to E/K/V/N/C`). It reads the callee's
// generic Signature from funcCtx.MethodSignatures (recorded per class in the dumper) and parses it on
// demand. SAFETY: returns the type ONLY when the formal is a bare CLASS-scope type variable -- never a
// method-scope `<T>` parameter (which is not in scope at the call site, so a `(T)` cast there would not
// compile) and never a concrete type (a real mismatch must not be blanket-cast). Kill-switch
// JDEC_GENERIC_SELFMETHOD_PARAM_OFF.
func (f *FunctionCallExpression) sameClassMethodParamType(i int, funcCtx *class_context.ClassContext) types.JavaType {
	if os.Getenv("JDEC_GENERIC_SELFMETHOD_PARAM_OFF") != "" || f.IsStatic || f.Object == nil || funcCtx == nil {
		return nil
	}
	// A `super.m()` call (invokespecial to a NON-current class) must not be treated as a same-class
	// call -- its signature lives in the superclass, not funcCtx.MethodSignatures. But a PRIVATE
	// same-class method is ALSO invokespecial (its target class IS the current class), and its
	// signature IS in funcCtx.MethodSignatures, so it must still be handled (guava AbstractBiMap
	// `this.updateInverseMap(k, b, objVal, v)` where param 3 is V -> needs `(V) objVal`). Focused
	// sub-switch JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF restores the legacy blanket-skip of invokespecial.
	if f.IsSpecialInvoke {
		if os.Getenv("JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF") != "" || !f.isCurrentClass(funcCtx) {
			return nil
		}
	}
	ref, ok := UnpackSoltValue(f.Object).(*JavaRef)
	if !ok || !ref.IsThis {
		return nil
	}
	sig := funcCtx.MethodSignature(f.FunctionName, len(f.Arguments))
	if sig == "" {
		return nil
	}
	_, params, _ := types.ParseMethodSignatureFull(sig, funcCtx)
	if i < 0 || i >= len(params) || params[i] == nil {
		return nil
	}
	raw := params[i].RawType()
	if raw == nil {
		return nil
	}
	jc, ok := raw.(*types.JavaClass)
	if !ok {
		return nil
	}
	// Only a class-scope type variable is denotable AND castable at the call site.
	if !funcCtx.IsTypeParam(jc.Name) {
		return nil
	}
	return params[i]
}

// ctorWildcardArgCast returns the parameterized formal type to cast the i-th argument of a SAME-CLASS
// `this(...)` constructor self-call, when that formal is a wildcard parameterization MENTIONING a class
// type variable and the argument erases to the same raw type but is NOT that exact parameterization. The
// bytecode erases both to the raw class and emits no checkcast, dropping the source's unchecked cast; javac
// then rejects the call (gson LinkedTreeMap `this((Comparator<? super K>) NATURAL_ORDER)`, NATURAL_ORDER
// being `Comparator<Comparable>` -> "Comparator<Comparable> cannot be converted to Comparator<? super K>").
// This mirrors statements.wildcardFieldStoreCast for an argument position. Returns "" unless every gate
// passes. The constructor signature comes from funcCtx.ConstructorSignatures, which is recorded only when
// offset-safe (no synthetic this$0 parameter), so an inner-class `this(...)` self-call never mis-indexes.
// Kill-switch JDEC_CTOR_WILDCARD_CAST_OFF (the same switch that gates recording the signature).
func (f *FunctionCallExpression) ctorWildcardArgCast(i int, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_CTOR_WILDCARD_CAST_OFF") != "" || funcCtx == nil || f.FunctionName != "<init>" {
		return ""
	}
	// Only a `this(...)` self-call: its constructor is in the CURRENT class (signature recorded). A
	// `super(...)` call binds to the superclass constructor, whose signature is not in this table.
	if f.ClassName != funcCtx.ClassName {
		return ""
	}
	// MUST be an actual `this(...)` constructor self-call (receiver is `this`), NOT a `new CurrentClass(...)`
	// expression -- which is also an `<init>` invocation whose ClassName equals the current class but may
	// appear in a STATIC factory method where the class type variables are OUT OF SCOPE (a
	// `(Comparator<? super K>)` cast there fails "cannot find symbol: class K"; guava TreeMultimap.create,
	// Maps$FilteredEntrySortedMap). A `this(...)` self-call only ever runs inside an instance constructor,
	// where the class type parameters are always in scope, so the recovered cast is denotable.
	if ref, ok := UnpackSoltValue(f.Object).(*JavaRef); !ok || !ref.IsThis {
		return ""
	}
	if i < 0 || i >= len(f.Arguments) {
		return ""
	}
	sig := funcCtx.ConstructorSignature(len(f.Arguments))
	if sig == "" {
		return ""
	}
	_, params, _ := types.ParseMethodSignatureFull(sig, funcCtx)
	if i >= len(params) || params[i] == nil {
		return ""
	}
	paramTypeStr := params[i].String(funcCtx)
	// Must be a wildcard parameterization mentioning a class-scope type variable (`X<? ... K ...>`).
	if !strings.Contains(paramTypeStr, "<") || !strings.Contains(paramTypeStr, "?") {
		return ""
	}
	if !mentionsAnyTypeParamToken(paramTypeStr, funcCtx.ClassTypeParams) {
		return ""
	}
	arg := f.Arguments[i]
	if lit, ok := UnpackSoltValue(arg).(*JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
		return "" // null is assignable to any reference type without a cast
	}
	vt := arg.Type()
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
	// The argument must erase to the parameter's own raw type (cast always legal, never inconvertible)
	// AND differ from it in parameterization (an already-exact argument needs no cast).
	if erasureNameOf(vtStr) != erasureNameOf(paramTypeStr) || vtStr == paramTypeStr {
		return ""
	}
	return paramTypeStr
}

// erasureNameOf strips a generic type string down to its raw/erased name: "Comparator<? super K>" ->
// "Comparator". Mirrors statements.erasureName (kept local so the values package stays self-contained).
func erasureNameOf(s string) string {
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// mentionsAnyTypeParamToken reports whether s contains any of typeParams as a whole identifier token
// (so "K" matches in "Comparator<? super K>" but not inside "Komparator"). Empty typeParams -> false.
func mentionsAnyTypeParamToken(s string, typeParams []string) bool {
	if len(typeParams) == 0 {
		return false
	}
	set := make(map[string]bool, len(typeParams))
	for _, tp := range typeParams {
		if tp != "" {
			set[tp] = true
		}
	}
	var tok strings.Builder
	flush := func() bool {
		if tok.Len() == 0 {
			return false
		}
		name := tok.String()
		tok.Reset()
		return set[name]
	}
	for _, r := range s {
		if r == '_' || r == '$' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			tok.WriteRune(r)
			continue
		}
		if flush() {
			return true
		}
	}
	return flush()
}

// resolvedParamType is the unified cross-class generic resolver entry point (the root-cause
// generalization of instantiatedParamType's JDK table and sameClassMethodParamType's same-class /
// identity-one-level paths). It recovers the i-th formal parameter type of a call on a JAR-INTERNAL
// receiver by walking the receiver's generic supertype hierarchy with proper type-argument
// substitution (types.ResolveInstantiatedParamType), so it covers what the special cases miss: a
// non-`this` parameterized receiver (`var0.setCount(objVal)`, var0=`Multiset<E>`), a NON-identity
// supertype mapping (`Sub<X> implements Super<X,String>`), and a DEEP chain (method declared in an
// ancestor class/interface). It runs ADDITIVELY -- only when the JDK-table and same-class paths both
// declined -- so the output is a strict superset of the old behavior. Kill-switch
// JDEC_GENERIC_RESOLVE_OFF restores the special-case-only behavior. Returns nil to keep the erased
// descriptor parameter.
func (f *FunctionCallExpression) resolvedParamType(i int, funcCtx *class_context.ClassContext) types.JavaType {
	if os.Getenv("JDEC_GENERIC_RESOLVE_OFF") != "" || f.IsStatic || f.Object == nil || funcCtx == nil || funcCtx.SiblingClassSig == nil {
		return nil
	}
	// A `super.m()` invokespecial to a NON-current class binds to the supertype's declaration via a
	// different receiver type; leave it to the existing paths (mirrors sameClassMethodParamType).
	if f.IsSpecialInvoke && !f.isCurrentClass(funcCtx) {
		return nil
	}
	var recvRaw string
	var recvArgs []types.JavaType
	if ref, ok := UnpackSoltValue(f.Object).(*JavaRef); ok && ref.IsThis {
		// `this` receiver: start at the current class with an IDENTITY type-argument mapping (each class
		// formal mapped to itself), built from the authoritative class Signature so the count always
		// matches the walk's formals. The walk then ascends this class's generic supertypes (covering
		// non-identity edges and deep chains).
		if funcCtx.ClassSig == "" {
			return nil
		}
		formals := types.ClassFormalTypeParamNames(funcCtx.ClassSig)
		if len(formals) == 0 {
			return nil
		}
		recvRaw = funcCtx.ClassName
		recvArgs = make([]types.JavaType, len(formals))
		for idx, n := range formals {
			recvArgs[idx] = types.NewJavaClass(n)
		}
	} else {
		// Non-`this` receiver: recover its raw class + actual type args from the receiver value's
		// parameterized static type, or -- when the receiver is a same-class parameterized field whose
		// getfield value carries only the erased descriptor -- from the field's generic Signature. This
		// reuses the exact receiver-type recovery the JDK-table path uses (receiverParamTypeArgs), so a
		// parameterized local (`var0` of `Multiset<E>`) and a same-class field (`this.box` of `Box<E>`)
		// are both handled.
		recvRaw, recvArgs = f.receiverParamTypeArgs(funcCtx)
	}
	if recvRaw == "" || len(recvArgs) == 0 {
		return nil
	}
	return types.ResolveInstantiatedParamType(funcCtx, funcCtx.SiblingClassSig, recvRaw, recvArgs, f.FunctionName, len(f.Arguments), i)
}

// isCurrentClass reports whether the call's target class is the class currently being rendered.
// Used to tell a private same-class invokespecial (`this.m()`) from a super call (`super.m()`).
func (f *FunctionCallExpression) isCurrentClass(funcCtx *class_context.ClassContext) bool {
	return funcCtx != nil && f.ClassName == funcCtx.ClassName
}

func (f *FunctionCallExpression) IsSupperConstructorInvoke(funcCtx *class_context.ClassContext) bool {
	if f.FunctionName == "<init>" && f.ClassName == funcCtx.SupperClassName {
		return true
	}
	return false
}
func (f *FunctionCallExpression) ArgumentString(funcCtx *class_context.ClassContext) string {
	return strings.Join(f.ArgumentStrings(funcCtx), ",")
}

// suppressTypeVarArgCast reports whether the synthesized argument cast `(expect)(arg)` should be
// dropped because the argument's static type is a bare in-scope type variable (Bug AH arg-side).
// A type-variable-typed value is erased to its bound in bytecode and is therefore pushed onto the
// stack WITHOUT a checkcast, so the descriptor parameter type (the erased bound, e.g. Comparable for
// `<C extends Comparable>`) differs from the argument's source type (the type variable C) purely
// through erasure - never through an explicit source cast (which would have changed the argument's
// static type away from the type variable). Emitting `(Comparable)(cVar)` is then a spurious upcast
// that javac rejects once it binds the call to the more specific generic signature
// ("incompatible types: Comparable cannot be converted to C"; same for Object->K/E/V/N/T). The cast
// is suppressed only when the EXPECTED parameter type is a concrete class (not itself a type
// variable), so a genuine `<X> m(X)`-style mismatch keeps its cast. Kill-switch:
// JDEC_NO_TYPEVAR_ARG_NOCAST=1.
func suppressTypeVarArgCast(funcCtx *class_context.ClassContext, argRaw, expectRaw *types.JavaClass) bool {
	if os.Getenv("JDEC_NO_TYPEVAR_ARG_NOCAST") != "" {
		return false
	}
	if funcCtx == nil || argRaw == nil || expectRaw == nil {
		return false
	}
	if !funcCtx.IsTypeParam(argRaw.Name) {
		return false
	}
	if funcCtx.IsTypeParam(expectRaw.Name) {
		return false
	}
	return true
}

// typeVarArrayArgCast reports whether the i-th argument needs a synthesized `(T)` cast because a
// generic resolver recovered the formal as a denotable, in-scope type VARIABLE while the argument is a
// reference ARRAY. The normal class-vs-class arg-cast branch cannot reach this case: an array type's
// RawType() is *JavaArrayType, not *JavaClass, so the (ok1 && ok2) gate is false and the array argument
// renders bare. A reference array is never directly assignable to a bare type variable in source (it is
// a widening to the variable's Object bound, which javac re-checks against the variable), so the source
// carried an unchecked `(T)` cast; re-emitting it is behaviour-preserving (the bytecode already stored
// the array into the erased-to-Object parameter). Tightly gated:
//   - ok1: the formal was recovered as a *JavaClass (a bare type-variable reference like `T`/`E`), not
//     an array/parameterized/concrete type (an array-typed formal would mean T was instantiated to an
//     array and the bare argument is already correct);
//   - resolvedGeneric: only when one of the generic resolvers recovered the instantiated parameter
//     (never on a raw descriptor Object param, which would over-cast plain Object[] args);
//   - the recovered name is a type parameter in the CURRENT scope (so `(T)` is denotable and compiles);
//   - the argument's static type is an array.
//
// Kill-switch: JDEC_TYPEVAR_ARRAY_ARG_CAST_OFF=1.
func (f *FunctionCallExpression) typeVarArrayArgCast(ok1, resolvedGeneric bool, expect *types.JavaClass, arg JavaValue, funcCtx *class_context.ClassContext) bool {
	if os.Getenv("JDEC_TYPEVAR_ARRAY_ARG_CAST_OFF") != "" {
		return false
	}
	if !ok1 || !resolvedGeneric || expect == nil || funcCtx == nil || arg == nil {
		return false
	}
	if expect.Name == "java.lang.Object" || !funcCtx.IsTypeParam(expect.Name) {
		return false
	}
	at := arg.Type()
	return at != nil && at.IsArray()
}

// calleeParamIsErasedTypeVar reports whether the called method's i-th formal parameter is a TYPE
// VARIABLE in the callee's own generic Signature (so the JVM descriptor erased it to merely that
// variable's bound). Synthesizing a cast to that erased bound is a no-op UPCAST (the argument flowed
// into the parameter in bytecode, so it is already assignable to the bound) that DESTROYS javac's
// call-site type inference: e.g. for `<C extends Comparable> Range<C> closed(C, C)` the decompiler
// would emit `Range.closed((Comparable)Integer.valueOf(x), ...)`, javac then infers C=Comparable and
// `ContiguousSet.create(Range<C>, DiscreteDomain<Integer>)` no longer applies ("method create cannot
// be applied"). Returning true lets ArgumentStrings DROP the cast so the precise argument type
// (Integer) drives inference. Only consulted when no generic resolver recovered a concrete/denotable
// instantiated parameter type (those casts are wanted). Restricted to JAR-INTERNAL callees whose
// Signature is available via SiblingClassSig (JDK callees stay on the descriptor). Kill-switch:
// JDEC_NO_ERASED_TYPEVAR_NOCAST=1.
func (f *FunctionCallExpression) calleeParamIsErasedTypeVar(i int, funcCtx *class_context.ClassContext) bool {
	if os.Getenv("JDEC_NO_ERASED_TYPEVAR_NOCAST") != "" || funcCtx == nil || funcCtx.SiblingClassSig == nil {
		return false
	}
	internal := strings.ReplaceAll(f.ClassName, ".", "/")
	classSig, methodSigs, ok := funcCtx.SiblingClassSig(internal)
	if !ok || methodSigs == nil {
		return false
	}
	sig := methodSigs[class_context.MethodSigKey(f.FunctionName, len(f.Arguments))]
	if sig == "" {
		return false
	}
	_, params, _ := types.ParseMethodSignatureFull(sig, funcCtx)
	if i < 0 || i >= len(params) || params[i] == nil {
		return false
	}
	raw, ok := params[i].RawType().(*types.JavaClass)
	if !ok {
		return false
	}
	// parseSigType emits a `TC;` type-variable reference as JavaClass{Name:"C"}. It is a real type
	// variable (not a concrete class that merely shares the name) iff it is declared as a formal type
	// parameter of the callee method itself or of its declaring class.
	name := raw.Name
	for _, n := range types.MethodFormalTypeParamNames(sig) {
		if n == name {
			return true
		}
	}
	for _, n := range types.ClassFormalTypeParamNames(classSig) {
		if n == name {
			return true
		}
	}
	return false
}

// jdkCalleeParamIsErasedTypeVar is the JDK companion to calleeParamIsErasedTypeVar, for the small set of
// JDK generic methods whose formal parameter is the class's own type variable erased to a NON-Object
// bound (so the existing java.lang.Object guard does not already skip the synthesized cast). The
// canonical case is java.lang.Enum<E>.compareTo(E): its descriptor erases E to the bound `java.lang.Enum`,
// so the arg-cast logic upcasts the concrete enum constant to raw `Enum` -- breaking compareTo's real
// signature compareTo(E) and yielding javac "Enum cannot be converted to <ConcreteEnum>" (guava
// AbstractService / ServiceManager `state().compareTo(State.RUNNING)`). The argument is the same concrete
// enum that flowed into the parameter in bytecode, so it is already assignable; dropping the cast is
// behaviour-preserving and lets javac infer E. JDK callees are invisible to SiblingClassSig, hence this
// descriptor-keyed companion. Kill-switch: JDEC_ENUM_COMPARETO_NOCAST_OFF=1.
func jdkCalleeParamIsErasedTypeVar(method string, paramIndex, argc int, paramType types.JavaType) bool {
	if os.Getenv("JDEC_ENUM_COMPARETO_NOCAST_OFF") != "" {
		return false
	}
	if method != "compareTo" || argc != 1 || paramIndex != 0 || paramType == nil {
		return false
	}
	raw, ok := paramType.RawType().(*types.JavaClass)
	return ok && raw.Name == "java.lang.Enum"
}

// classLiteralArgToClassParam reports whether arg is a class literal `X.class` being passed to a
// `java.lang.Class` parameter. A class literal is a `Class<X>` value, but JavaClassValue.Type() reports
// the REPRESENTED class X (so a static call on the literal renders `X.method()` rather than
// `Class.method()`), which makes the arg-cast mismatch check above see "X != Class" and wrap it as
// `(Class)(X.class)`. That raw cast is not merely redundant -- it ERASES the generic and collapses the
// callee's type inference: e.g. `new ObjectReaderImplFromString((Class)(Duration.class), Duration::parse)`
// turns the `<T> (Class<T>, Function<String,T>)` constructor into a raw invocation, so the
// `Function<String,T>` parameter degrades to raw `Function` and javac rejects the method reference
// ("incompatible types: invalid method reference"). A class literal already satisfies any `Class<...>`
// parameter it was passed to in bytecode (the bytecode type-checked it), and dropping the cast lets javac
// infer the callee's type argument from the literal. Tightly gated to a class-literal argument against a
// java.lang.Class parameter so no other argument cast is affected. Kill-switch:
// JDEC_CLASSLIT_ARG_NOCAST_OFF=1.
func classLiteralArgToClassParam(arg JavaValue, expect *types.JavaClass) bool {
	if os.Getenv("JDEC_CLASSLIT_ARG_NOCAST_OFF") != "" {
		return false
	}
	if expect == nil || expect.Name != "java.lang.Class" {
		return false
	}
	_, ok := UnpackSoltValue(arg).(*JavaClassValue)
	return ok
}

func (f *FunctionCallExpression) ArgumentStrings(funcCtx *class_context.ClassContext) []string {
	// Varargs spread (type-variable component): reconstruct `m(a, b)` from the javac-materialized
	// `m(new Object[]{a, b})` so a generic varargs callee's type variable is inferred from the element
	// types instead of being pinned to Object (see varargsTypeVarSpread). The leading fixed arguments
	// keep the normal per-argument cast logic; the spread elements render plainly (no synthetic cast --
	// that is the whole point, letting javac infer the callee's type variable).
	if elems, fixed, ok := f.varargsTypeVarSpread(funcCtx); ok {
		paramStrs := make([]string, 0, fixed+len(elems))
		for i := 0; i < fixed; i++ {
			paramStrs = append(paramStrs, f.renderArgAt(i, funcCtx))
		}
		for _, e := range elems {
			paramStrs = append(paramStrs, renderPlainArg(e, funcCtx))
		}
		return paramStrs
	}
	paramStrs := make([]string, 0, len(f.Arguments))
	for i := range f.Arguments {
		paramStrs = append(paramStrs, f.renderArgAt(i, funcCtx))
	}
	return paramStrs
}

// renderArgAt renders the i-th call argument, applying the generic-erasure parameter-type recovery and
// the synthesized argument cast (`(V)`/`(T)`/primitive) that reproduce the original source. Factored
// out of ArgumentStrings so the varargs-spread path can reuse it for the leading fixed arguments.
func (f *FunctionCallExpression) renderArgAt(i int, funcCtx *class_context.ClassContext) string {
	arg := f.Arguments[i]
	// A same-class `this(...)` constructor self-call whose i-th formal is a wildcard parameterization
	// mentioning a class type variable, fed an argument that erases to the same raw type but a different
	// parameterization, lost the source's unchecked cast to generic erasure -- re-add it (gson
	// LinkedTreeMap / LinkedHashTreeMap `this((Comparator<? super K>) NATURAL_ORDER)`).
	if cast := f.ctorWildcardArgCast(i, funcCtx); cast != "" {
		return fmt.Sprintf("(%s)(%s)", cast, arg.String(funcCtx))
	}
	argType := f.FuncType.ParamTypes[i]
	// Recover the generic parameter type the descriptor erased (e.g. BiConsumer<T,V>.accept's
	// param erases to Object): the source carried a `(V)` cast on the argument, so feed the
	// instantiated type back into the mismatch check below to re-emit it. resolvedGeneric records
	// that one of the resolvers recovered a concrete/denotable type, so the erased-type-var cast
	// SUPPRESSION below (calleeParamIsErasedTypeVar) is skipped -- a recovered cast is wanted.
	resolvedGeneric := false
	if inst := f.instantiatedParamType(i, funcCtx); inst != nil {
		argType = inst
		resolvedGeneric = true
	} else if inst := f.sameClassMethodParamType(i, funcCtx); inst != nil {
		argType = inst
		resolvedGeneric = true
	} else if inst := f.resolvedParamType(i, funcCtx); inst != nil {
		// Additive cross-class generic resolver: fires only when the JDK-table and same-class paths
		// declined, covering non-this / non-identity / deep-chain jar-internal receivers.
		argType = inst
		resolvedGeneric = true
	}
	// Incomplete stack simulation can leave an argument with a nil Type(); a parameter type
	// can likewise be nil for a malformed descriptor. Guard each RawType() behind a nil check
	// so a missing type degrades the per-argument cast logic to a no-op (rendering the argument
	// as-is) instead of nil-dereferencing and panicking the whole method into a stub.
	var expectClassType *types.JavaClass
	var atcClassType *types.JavaClass
	var ok1, ok2 bool
	if argType != nil {
		expectClassType, ok1 = argType.RawType().(*types.JavaClass)
	}
	if at := arg.Type(); at != nil {
		atcClassType, ok2 = at.RawType().(*types.JavaClass)
	}
	if ok1 && ok2 && expectClassType.Name != atcClassType.Name {
		if expectClassType.Name != "java.lang.Object" && !suppressTypeVarArgCast(funcCtx, atcClassType, expectClassType) &&
			!(!resolvedGeneric && f.calleeParamIsErasedTypeVar(i, funcCtx)) &&
			!(!resolvedGeneric && jdkCalleeParamIsErasedTypeVar(f.FunctionName, i, len(f.Arguments), argType)) &&
			!classLiteralArgToClassParam(arg, expectClassType) {
			argStr := arg.String(funcCtx)
			argTypeStr := argType.String(funcCtx)
			arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
			}, func() types.JavaType {
				return argType
			})
		}
	} else if f.typeVarArrayArgCast(ok1, resolvedGeneric, expectClassType, arg, funcCtx) {
		// The resolver recovered the formal as a denotable type variable T (in scope here) but the
		// ARGUMENT is a reference ARRAY -- the (ok1 && ok2) class-vs-class branch above never fires
		// because JavaArrayType.RawType() is not a *JavaClass. A reference array is never directly
		// assignable to a bare type variable, so the source carried an unchecked `(T)` cast that
		// erased to a no-op (the array already satisfied T's Object bound in bytecode); re-emit it.
		// fastjson2 CSVReader.readLineObjectAll: `consumer.accept((T) values)` where `values` is the
		// `Object[]` returned by readLineValues and `consumer` is `Consumer<T>`.
		argStr := arg.String(funcCtx)
		argTypeStr := argType.String(funcCtx)
		arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
		}, func() types.JavaType {
			return argType
		})
	} else if expectPrim, okp := primerRawType(argType); okp {
		if expectPrim.Name == types.JavaBoolean {
			// The JVM has no boolean opcodes: a boolean argument is pushed as an int
			// constant (iconst_0/iconst_1). Java forbids int->boolean conversion, so values
			// flowing into a boolean parameter must render with boolean literals, including
			// ternary trees like `cond ? 1 : 0`.
			arg = coerceBooleanArgument(arg)
		} else if actualPrim, oka := primerRawType(arg.Type()); oka &&
			actualPrim.Name != types.JavaBoolean && actualPrim.Name != expectPrim.Name {
			// The JVM descriptor pins the EXACT primitive parameter type, but byte/short/char/int
			// all share the int stack category and convert between each other without an opcode, so
			// the argument's static type frequently disagrees with the parameter type. Two failure
			// modes follow if the cast is dropped:
			//   - narrowing (int -> byte/short/char): illegal in invocation context (JLS 5.3),
			//     "possible lossy conversion from int to char";
			//   - widening that changes overloading (char/byte/short -> int): source picks a
			//     DIFFERENT overload than the bytecode (e.g. StringBuilder.append(char) instead of
			//     append(int)), silently changing behavior.
			// Emitting an explicit cast to the descriptor's parameter type reproduces the original
			// invocation exactly. (long/float/double mismatches already carry an i2l/i2d/... opcode
			// that makes the argument type match, so this fires only for the int-category gap.)
			argStr := arg.String(funcCtx)
			argTypeStr := expectPrim.Name
			arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
			}, func() types.JavaType {
				return argType
			})
		}
	}
	return renderPlainArg(arg, funcCtx)
}

// renderPlainArg renders a value as a call argument with no synthesized cast, falling back to the raw
// variable id when String() yields empty (an incompletely simulated ref).
func renderPlainArg(arg JavaValue, funcCtx *class_context.ClassContext) string {
	argStr := arg.String(funcCtx)
	if argStr == "" {
		if ref, ok := arg.(*JavaRef); ok && ref != nil && ref.Id != nil {
			argStr = ref.Id.String()
		}
	}
	return argStr
}

// varargsTypeVarSpread detects the javac varargs-call idiom on a generic method whose varargs COMPONENT
// is a TYPE VARIABLE, returning the array literal's element values to spread plus the count of leading
// fixed (non-varargs) arguments. The bytecode for `m(a, b)` to `<T> R m(T... xs)` materializes a fresh
// `new Object[]{a, b}` passed as the single trailing array argument; rendering that array faithfully
// PINS the callee's T to Object (the array's erased element type), which then conflicts with the call's
// required instantiation (e.g. `UnmodifiableIterator<N> = Iterators.forArray(new Object[]{nodeU,nodeV})`
// -> "inference variable T has incompatible bounds: Object, N"). Spreading the literal back to `m(a, b)`
// lets javac infer T from the element types, reproducing the original source (this is what CFR /
// Vineflower emit). RESTRICTED to the type-variable-component case -- the only one that mis-infers;
// plain `Object...`/`String...` varargs neither mis-infer nor are safe to spread blindly (a lone array
// element passed to `Object...` would change meaning) -- and to JAR-INTERNAL callees whose generic
// Signature is available (SiblingClassSig). SAFE because GENERIC ARRAY CREATION IS ILLEGAL in Java, so
// a fresh `new Object[]{...}` reaching a type-variable array parameter can only be a varargs pack, never
// a hand-written array argument to a non-varargs `T[]` parameter. Kill-switch JDEC_VARARGS_SPREAD_OFF.
func (f *FunctionCallExpression) varargsTypeVarSpread(funcCtx *class_context.ClassContext) ([]JavaValue, int, bool) {
	if os.Getenv("JDEC_VARARGS_SPREAD_OFF") != "" || funcCtx == nil || funcCtx.SiblingClassSig == nil {
		return nil, 0, false
	}
	n := len(f.Arguments)
	if n == 0 {
		return nil, 0, false
	}
	// Last actual argument must be a FRESH 1-D array literal (new X[]{...}); a sized/empty `new X[len]`
	// (no initializer) carries no spreadable elements and is left as-is.
	ne, ok := UnpackSoltValue(f.Arguments[n-1]).(*NewExpression)
	if !ok || !ne.IsArray() || ne.Initializer == nil || ne.JavaType.ArrayDim() != 1 {
		return nil, 0, false
	}
	// Callee's last formal parameter (from its generic Signature) must be a 1-D array whose element is a
	// type variable declared by the callee method or its declaring class.
	internal := strings.ReplaceAll(f.ClassName, ".", "/")
	classSig, methodSigs, ok := funcCtx.SiblingClassSig(internal)
	if !ok || methodSigs == nil {
		return nil, 0, false
	}
	sig := methodSigs[class_context.MethodSigKey(f.FunctionName, n)]
	if sig == "" {
		return nil, 0, false
	}
	_, params, _ := types.ParseMethodSignatureFull(sig, funcCtx)
	if len(params) != n || params[n-1] == nil || !params[n-1].IsArray() || params[n-1].ArrayDim() != 1 {
		return nil, 0, false
	}
	elem := params[n-1].ElementType()
	if elem == nil {
		return nil, 0, false
	}
	raw, ok := elem.RawType().(*types.JavaClass)
	if !ok {
		return nil, 0, false
	}
	name := raw.Name
	isFormal := false
	for _, tn := range types.MethodFormalTypeParamNames(sig) {
		if tn == name {
			isFormal = true
			break
		}
	}
	if !isFormal {
		for _, tn := range types.ClassFormalTypeParamNames(classSig) {
			if tn == name {
				isFormal = true
				break
			}
		}
	}
	if !isFormal {
		return nil, 0, false
	}
	return ne.Initializer, n - 1, true
}

// polymorphicSignatureCastType reports the explicit cast a signature-polymorphic MethodHandle call
// needs at SOURCE level. MethodHandle.invoke / invokeExact are declared `Object invoke(Object...)`
// with @PolymorphicSignature: javac synthesizes a per-call-site descriptor (e.g.
// `invokeExact:()Ljava/util/function/BiFunction;`) so the bytecode return type is the REAL one, but
// the SOURCE-apparent return type is always Object - the original source therefore carries an explicit
// `(BiFunction) handle.invokeExact()` cast. The decompiler reads the call's return type from the
// (real) descriptor, so it declares `BiFunction var = handle.invokeExact()` with no cast and javac
// rejects it ("incompatible types: Object cannot be converted to BiFunction" - fastjson2
// JSONReader$BigIntegerCreator / JdbcSupport / DoubleToDecimal, all `LambdaMetafactory...getTarget()
// .invokeExact()`). Re-emit the cast to the descriptor return type. Object/void returns need no cast.
// Kill-switch: JDEC_NO_POLYSIG_CAST=1.
func (f *FunctionCallExpression) polymorphicSignatureCastType(funcCtx *class_context.ClassContext) (string, bool) {
	if os.Getenv("JDEC_NO_POLYSIG_CAST") != "" {
		return "", false
	}
	if f.FunctionName != "invoke" && f.FunctionName != "invokeExact" {
		return "", false
	}
	if f.ClassName != "java.lang.invoke.MethodHandle" {
		return "", false
	}
	if f.FuncType == nil || f.FuncType.ReturnType == nil {
		return "", false
	}
	rt := f.FuncType.ReturnType.String(funcCtx)
	switch rt {
	case "", "void", "Object", "java.lang.Object":
		return "", false
	}
	return rt, true
}

func (f *FunctionCallExpression) String(funcCtx *class_context.ClassContext) string {
	if castType, ok := f.polymorphicSignatureCastType(funcCtx); ok {
		return fmt.Sprintf("(%s)(%s)", castType, f.renderCall(funcCtx))
	}
	return f.renderCall(funcCtx)
}

func (f *FunctionCallExpression) renderCall(funcCtx *class_context.ClassContext) string {
	paramStrs := f.ArgumentStrings(funcCtx)
	if f.FunctionName == "<init>" {
		if f.ClassName == funcCtx.ClassName {
			return fmt.Sprintf("%s(%s)", f.Object.String(funcCtx), strings.Join(paramStrs, ","))
		} else if f.ClassName == funcCtx.SupperClassName {
			return fmt.Sprintf("super(%s)", strings.Join(paramStrs, ","))
		}
	}
	functionName := class_context.SafeIdentifier(f.FunctionName)

	// A non-constructor invokespecial whose receiver is `this` and whose target is a DIFFERENT class
	// (the superclass / an ancestor, never the current class which would be a private same-class call)
	// is a `super.method(...)` call. Rendering it as `this.method(...)` re-dispatches virtually to the
	// overriding method and recurses forever (guava CaseFormat constant bodies' `super.convert(...)`).
	if f.IsSpecialInvoke && f.FunctionName != "<init>" && f.ClassName != "" && !f.isCurrentClass(funcCtx) {
		if ref, ok := UnpackSoltValue(f.Object).(*JavaRef); ok && ref != nil && ref.IsThis {
			return fmt.Sprintf("super.%s(%s)", functionName, strings.Join(paramStrs, ","))
		}
	}

	if v, ok := f.Object.(*JavaClassValue); ok {
		if classType, ok2 := v.Type().RawType().(*types.JavaClass); ok2 && classType.Name == funcCtx.ClassName && f.IsStatic {
			// Unqualified static call to a method of the current class (foo() instead of Foo.foo()).
			// Only valid for static dispatch; an instance call on the current class's own class
			// literal (Foo.class.getName()) must keep the `Foo.class` receiver, so it falls through.
			return fmt.Sprintf("%s(%s)", functionName, strings.Join(paramStrs, ","))
		}
		if f.IsStatic {
			// Static method invocation: the receiver is a type reference, so render the bare type
			// name (Integer.parseInt(...)). JavaClassValue.String() now yields the Class-object
			// literal form `Integer.class`, which is correct for value/instance-receiver positions
			// but wrong here, so bypass it via Type().
			return fmt.Sprintf("%s.%s(%s)", v.Type().String(funcCtx), functionName, strings.Join(paramStrs, ","))
		}
	}
	obj := UnpackSoltValue(f.Object)
	if cv, ok := obj.(*CustomValue); ok && cv.Flag == "lambda" {
		// A lambda / method reference inlined directly as a call receiver has no target type of
		// its own - `(() -> x).get()` does not compile. Supply one by casting to the functional
		// interface the value carries: `((Supplier)(() -> x)).get()`.
		return fmt.Sprintf("((%s)(%s)).%s(%s)", cv.Type().String(funcCtx), cv.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	}
	switch obj.(type) {
	case *JavaExpression, *TernaryExpression, *SlotValue:
		return fmt.Sprintf("(%s).%s(%s)", f.Object.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	default:
		return fmt.Sprintf("%s.%s(%s)", f.Object.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	}
}

func coerceBooleanArgument(arg JavaValue) JavaValue {
	switch v := UnpackSoltValue(arg).(type) {
	case *JavaLiteral:
		if prim, ok := primerRawType(v.Type()); ok && prim.Name == types.JavaInteger {
			if iv, ok := v.Data.(int); ok && (iv == 0 || iv == 1) {
				return NewJavaLiteral(iv, types.NewJavaPrimer(types.JavaBoolean))
			}
		}
		return arg
	case *TernaryExpression:
		if v == nil {
			return arg
		}
		coerced := NewTernaryExpression(v.Condition, coerceBooleanArgument(v.TrueValue), coerceBooleanArgument(v.FalseValue))
		coerced.ConditionFromOp = v.ConditionFromOp
		return coerced
	}
	// Any OTHER int-typed value reaching a boolean parameter is a boolean held as an int (the JVM has
	// no boolean storage: a boolean local is stored/reloaded with istore/iload, and javac materializes
	// a boolean value via iconst_0/iconst_1). Java forbids the implicit int->boolean conversion, so a
	// plain `int` local/expression flowing into a boolean parameter fails to recompile ("incompatible
	// types: int cannot be converted to boolean"). Render an explicit `(v) != (0)`, which is the exact
	// boolean meaning of the 0/1 int. Values already typed boolean (comparisons, predicate calls,
	// boolean refs) keep their boolean type, so they are left untouched and we never emit an illegal
	// `(a > b) != (0)`.
	if at := arg.Type(); at != nil {
		if prim, ok := primerRawType(at); ok && prim.Name == types.JavaInteger {
			inner := arg
			return NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("(%s) != (0)", inner.String(funcCtx))
			}, func() types.JavaType {
				return types.NewJavaPrimer(types.JavaBoolean)
			})
		}
	}
	return arg
}

// IntrinsicBooleanValue reports whether v is boolean BY CONSTRUCTION -- a non-short-circuit boolean
// connective `a & b` / `a | b` / `a ^ b` (boolConnectiveConds), or a ternary whose BOTH arms are
// boolean -- as opposed to a bare variable/slot that merely carries a boolean static type (which may be
// a slot mistyped boolean while actually holding an int). Used to decide when an `int x = <boolean>`
// assignment must re-insert the `? 1 : 0` coercion javac elided.
func IntrinsicBooleanValue(v JavaValue) bool {
	switch u := UnpackSoltValue(v).(type) {
	case *JavaExpression:
		_, _, ok := u.boolConnectiveConds()
		return ok
	case *TernaryExpression:
		return u != nil && isBooleanTyped(u.TrueValue) && isBooleanTyped(u.FalseValue)
	}
	return false
}

// CoerceIntAssignRHS wraps rhs as `(rhs) ? (1) : (0)` when an INTRINSICALLY-boolean value is assigned to
// an int-typed target. javac compiles `int x = c1 & c2;` (and other non-short-circuit boolean
// connectives / boolean ternaries originally written `cond ? 1 : 0`) by leaving the boolean -- already
// 0/1 on the operand stack -- without a branch, so the decompiler recovers a boolean RHS that javac then
// rejects ("boolean cannot be converted to int", guava DoubleMath/LongMath log2, ImmutableSortedMap
// copyOfInternal). Re-inserting the explicit `? 1 : 0` restores a compilable, behaviourally-identical
// form. It fires ONLY on IntrinsicBooleanValue (never a bare boolean-typed ref/slot), so a slot mistyped
// boolean but holding an int (LocalCache$Segment `this.count = var13`) is left untouched -- avoiding a
// silent miscompilation. Kill-switch: JDEC_BOOL_TO_INT_COERCE_OFF=1.
func CoerceIntAssignRHS(leftType types.JavaType, rhs JavaValue) JavaValue {
	if rhs == nil || leftType == nil {
		return rhs
	}
	if os.Getenv("JDEC_BOOL_TO_INT_COERCE_OFF") == "1" {
		return rhs
	}
	prim, ok := leftType.RawType().(*types.JavaPrimer)
	if !ok || prim.Name != types.JavaInteger {
		return rhs
	}
	if !IntrinsicBooleanValue(rhs) {
		return rhs
	}
	inner := rhs
	return NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return fmt.Sprintf("(%s) ? (1) : (0)", inner.String(funcCtx))
	}, func() types.JavaType {
		return types.NewJavaPrimer(types.JavaInteger)
	})
}

func NewFunctionCallExpression(object JavaValue, methodMember *JavaClassMember, funcType *types.JavaFuncType) *FunctionCallExpression {
	return &FunctionCallExpression{
		FuncType:     funcType,
		Object:       object,
		FunctionName: methodMember.Member,
		ClassName:    methodMember.Name,
	}
}
