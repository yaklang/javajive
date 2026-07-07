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
		// `field != genericCall()` incomparable-generics repair: when one operand is a same-class field
		// whose declared generic type is `X<typevar>` and the other is a same-raw method call that javac
		// infers standalone (a generic method with a free return type var, e.g. Cut.aboveAll() inferring
		// `Cut<Comparable>`), the == / != operands are mutually incomparable (`Cut<C>` vs `Cut<Comparable>`).
		// A raw cast on the call operand restores comparability and is always safe on ==. See
		// incomparableGenericFieldVsCallCast. Kill-switch JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF.
		if j.Op == EQ || j.Op == NEQ {
			// Boolean-vs-int-ternary comparison collapse (spring-core ASM MethodVisitor.visitMethodInsn,
			// MethodWriter.putAttributes): javac materializes a boolean sub-expression that feeds an int
			// comparison as `cond ? 1 : 0`, and a boolean local/param compared against it renders
			// `(boolVar) != ((cond) ? (1) : (0))`, which javac rejects ("incomparable types: boolean and
			// int"). Since the `? 1 : 0` (resp. `? 0 : 1`) ternary IS the int form of the boolean `cond`
			// (resp. `!cond`), the comparison is exactly `boolVar <op> cond`. Collapse it. Only fires when
			// one operand is boolean-typed and the other is a literal-0/1 boolean-materialization ternary,
			// so it can only fix, never alter, semantics. Kill-switch: JDEC_BOOL_INT_TERNARY_CMP_OFF=1.
			if collapsed, ok := boolVsIntTernaryCollapse(j.Values[0], j.Values[1], j.Op, funcCtx); ok {
				return collapsed
			}
			if raw, idx := j.incomparableGenericFieldVsCallCast(funcCtx); raw != "" {
				vs[idx] = fmt.Sprintf("(%s)(%s)", raw, vs[idx])
			}
		}
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	}
}

// boolVsIntTernaryCollapse handles a `==`/`!=` comparison where one operand is boolean-typed and the
// other is a boolean-materialization ternary `cond ? 1 : 0` (or the inverse `cond ? 0 : 1`). javac
// emits that ternary when a boolean sub-expression is forced into an int comparison, and the resulting
// `(boolVar) != ((cond) ? (1) : (0))` is rejected as "incomparable types: boolean and int". The ternary
// is precisely the int encoding of the boolean `cond` (resp. `!cond`), so the comparison equals
// `boolVar <op> cond` (resp. `boolVar <op> !cond`). On a match it returns the collapsed, compilable
// rendering. Kill-switch: JDEC_BOOL_INT_TERNARY_CMP_OFF=1.
func boolVsIntTernaryCollapse(a, b JavaValue, op string, funcCtx *class_context.ClassContext) (string, bool) {
	if os.Getenv("JDEC_BOOL_INT_TERNARY_CMP_OFF") != "" {
		return "", false
	}
	tryOrder := func(boolCand, ternCand JavaValue) (string, bool) {
		if !isBooleanTyped(boolCand) {
			return "", false
		}
		cond, ok := boolMaterializationCondition(ternCand, funcCtx)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("(%s) %s (%s)", boolCand.String(funcCtx), op, cond.String(funcCtx)), true
	}
	if s, ok := tryOrder(a, b); ok {
		return s, true
	}
	if s, ok := tryOrder(b, a); ok {
		return s, true
	}
	return "", false
}

// boolMaterializationCondition returns the underlying boolean condition of a boolean-materialization
// ternary, else (nil,false). The simple shapes are `cond ? 1 : 0` (=> cond) and `cond ? 0 : 1`
// (=> !cond); the nested short-circuit shapes (`cond1 ? (cond2 ? 1 : 0) : 0` == `cond1 && cond2`, etc.)
// are handled generically by boolReduce, which folds a literal-0/1-leaf ternary tree back into the
// original &&/|| boolean connective. Only ternaries that reduce to a genuinely boolean-typed value
// qualify (a value ternary over non-0/1 arms does not reduce and is rejected).
func boolMaterializationCondition(v JavaValue, funcCtx *class_context.ClassContext) (JavaValue, bool) {
	t, ok := UnpackSoltValue(v).(*TernaryExpression)
	if !ok || t == nil || t.Condition == nil || t.TrueValue == nil || t.FalseValue == nil {
		return nil, false
	}
	// The materialization leaves are int literals 0/1 (the JVM has no boolean stack category), so
	// boolReduce -- which only folds boolean-typed leaves -- leaves them untouched. coerceBooleanArgument
	// retypes each 0/1 literal leaf to boolean (recursing into nested ternary arms) without touching the
	// conditions, after which boolReduce collapses the tree into the original &&/||/! connective. A
	// genuine value ternary (non-0/1 arms) is not coerced and does not reduce, so it is rejected.
	reduced := boolReduce(coerceBooleanArgument(t), funcCtx)
	if isBooleanTyped(reduced) {
		return reduced, true
	}
	return nil, false
}

// incomparableGenericFieldVsCallCast detects the `field <op> genericCall()` (op ∈ {==, !=}) shape where
// one operand is a SAME-CLASS field whose declared generic type is `X<..typevar..>` and the OTHER is a
// method call of the SAME raw erasure. In source such a call is a poly expression, but in a bare
// == / != there is NO target type, so javac infers the method's free return type variable to its bound
// (guava's `<C extends Comparable<?>> Cut<C> aboveAll()` -> `Cut<Comparable>`), which is incomparable
// with the field's `Cut<C>` ("incomparable types: Cut<C> and Cut<Comparable>"). The original source used
// an explicit type witness (`Cut.<C>aboveAll()`) that bytecode erases. A raw cast to the shared erasure
// on the call operand restores comparability and is ALWAYS safe on == / != (raw and parameterized of the
// same type are mutually comparable), so it can only fix, never break, a comparison. Returns the raw
// erasure name to cast to and the index (0 or 1) of the operand to wrap, or ("", -1). Kill-switch
// JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF.
func (j *JavaExpression) incomparableGenericFieldVsCallCast(funcCtx *class_context.ClassContext) (string, int) {
	if os.Getenv("JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF") != "" {
		return "", -1
	}
	if funcCtx == nil || len(j.Values) != 2 {
		return "", -1
	}
	a := UnpackSoltValue(j.Values[0])
	b := UnpackSoltValue(j.Values[1])
	if raw := genericFieldVsCallRaw(funcCtx, a, b); raw != "" {
		return raw, 1
	}
	if raw := genericFieldVsCallRaw(funcCtx, b, a); raw != "" {
		return raw, 0
	}
	return "", -1
}

// genericFieldVsCallRaw returns the shared raw erasure name when fieldCand is a same-class field with a
// `X<typevar>` declared generic signature and callCand is a method call of that same raw erasure X, else
// "". See incomparableGenericFieldVsCallCast.
func genericFieldVsCallRaw(funcCtx *class_context.ClassContext, fieldCand, callCand JavaValue) string {
	call, ok := callCand.(*FunctionCallExpression)
	if !ok {
		return ""
	}
	var fieldName string
	switch lv := fieldCand.(type) {
	case *RefMember:
		ref, ok := UnpackSoltValue(lv.Object).(*JavaRef)
		if !ok || !ref.IsThis {
			return ""
		}
		fieldName = class_context.SafeIdentifier(lv.Member)
	case *JavaClassMember:
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
	// Field must be a parameterization carrying an IN-SCOPE type variable (`X<C>` / `Map<K, V>`); a
	// fully-concrete `X<String>` or wildcard `X<?>` is not the free-inference mismatch this repairs.
	if !hasTopLevelTypeParamArg(funcCtx, fieldTypeStr) {
		return ""
	}
	fieldRaw := erasureNameLocal(fieldTypeStr)
	// The call operand's static type must share that raw erasure (same class, comparable at all).
	callRaw := ""
	if ct := call.Type(); ct != nil {
		callRaw = erasureNameLocal(ct.String(funcCtx))
	}
	if callRaw == "" || callRaw != fieldRaw {
		return ""
	}
	return fieldRaw
}

// hasTopLevelTypeParamArg reports whether the outermost type-argument list of a rendered generic type
// string (e.g. "Cut<C>", "Map<K, V>", "Entry<Range<K>, V>") contains at least one bare argument that is
// an in-scope type variable. Nested arguments are split at depth 0 only.
func hasTopLevelTypeParamArg(funcCtx *class_context.ClassContext, s string) bool {
	open := strings.IndexByte(s, '<')
	if open < 0 || !strings.HasSuffix(s, ">") {
		return false
	}
	inner := s[open+1 : len(s)-1]
	depth := 0
	start := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				if funcCtx.IsTypeParam(strings.TrimSpace(inner[start:i])) {
					return true
				}
				start = i + 1
			}
		}
	}
	return funcCtx.IsTypeParam(strings.TrimSpace(inner[start:]))
}

// erasureNameLocal strips a rendered generic type string to its raw name ("Cut<C>" -> "Cut"). It is the
// values-package twin of the statements-package erasureName helper.
func erasureNameLocal(s string) string {
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
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
	// Descriptor is the raw JVM method descriptor of the call target (e.g.
	// `(Ljava/lang/Object;Ljava/lang/Iterable;)Lcom/google/common/collect/ImmutableMultimap$Builder;`),
	// copied from the resolved constant-pool member. It DISAMBIGUATES same-arity overloads that the
	// arity-keyed MethodSignatures table drops as ambiguous (guava Builder `putAll(K, Iterable)` vs the
	// varargs `putAll(K, V...)`; `add(E)` vs `add(E...)`), enabling a precise descriptor-keyed same-class
	// signature lookup (sameClassMethodParamType). Empty for synthesized calls with no pool member.
	Descriptor string
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
	if funcCtx == nil {
		return "", nil
	}
	// Receiver whose static type is a bare type variable with a PARAMETERIZED bound (`C var1` where the
	// enclosing method / class declares `<C extends Collection<? super E>>`): the value type is only `C`,
	// but the bound `Collection<? super E>` carries the receiver type args downstream receiver/param
	// resolution needs (guava FluentIterable.copyInto `var1.add(objVal)`, Multimaps.invertFrom
	// `var1.put(objV, objK)`). Recover the bound's raw class + type args. Method-scope bounds take
	// precedence over class-scope (an inner `<C>` shadows a class `C`). Kill-switch
	// JDEC_TYPEVAR_BOUND_RECV_OFF.
	if os.Getenv("JDEC_TYPEVAR_BOUND_RECV_OFF") == "" {
		if ot := f.Object.Type(); ot != nil {
			if jc, ok := ot.RawType().(*types.JavaClass); ok && jc != nil && funcCtx.IsTypeParam(jc.Name) {
				for _, sig := range []string{funcCtx.CurrentMethodSig, funcCtx.ClassSig} {
					if sig == "" {
						continue
					}
					bounds := types.FormalTypeParamBounds(sig)
					if bounds == nil {
						continue
					}
					if b, ok := bounds[jc.Name]; ok {
						if pt, ok := types.AsParameterizedType(b); ok {
							return pt.RawClassName, pt.TypeArgs
						}
					}
				}
			}
		}
	}
	// Same-class `this.method()` receiver: recover type args from the callee method's generic RETURN
	// signature. The receiver VALUE is the inner call's descriptor return, erased to the raw class
	// (jar-internal returns are not instantiated), so its Type() carries no type args -- but the
	// method's Signature does (e.g. `Multiset<E> multiset()` -> the value is raw `Multiset`, the
	// signature says `Multiset<E>`). Recovering it lets a downstream param resolver re-emit the erased
	// `(E)` argument cast (guava Multisets$EntrySet `this.multiset().setCount(objVal, cnt, 0)` ->
	// `Object cannot be converted to E`). Only a same-class `this.m()` call qualifies: its signature is
	// in funcCtx.MethodSignatures, and its return type args are class-scope variables denotable at the
	// call site. A super.m()/overloaded miss yields "" and is skipped. Kill-switch:
	// JDEC_GENERIC_PARAM_RECV_METHOD_OFF.
	if os.Getenv("JDEC_GENERIC_PARAM_RECV_METHOD_OFF") == "" {
		if inner, ok := UnpackSoltValue(f.Object).(*FunctionCallExpression); ok && !inner.IsStatic && inner.Object != nil {
			if iref, ok := UnpackSoltValue(inner.Object).(*JavaRef); ok && iref.IsThis {
				if sig := funcCtx.MethodSignature(inner.FunctionName, len(inner.Arguments)); sig != "" {
					if _, _, ret := types.ParseMethodSignatureFull(sig, funcCtx); ret != nil {
						if pt, ok := types.AsParameterizedType(ret); ok {
							return pt.RawClassName, pt.TypeArgs
						}
					}
				}
			}
		}
	}
	if os.Getenv("JDEC_GENERIC_PARAM_FIELD_OFF") != "" {
		return "", nil
	}
	// Same-class field receiver (`this.field`): recover type args from the field's generic Signature;
	// an INHERITED field (declared in a superclass) is recovered via the cross-class hierarchy walk.
	if pt, ok := types.AsParameterizedType(RecoverThisFieldInstantiatedType(funcCtx, f.Object)); ok {
		return pt.RawClassName, pt.TypeArgs
	}
	return "", nil
}

// RecoverThisFieldInstantiatedType recovers the real, instantiated generic type of a `this.field` read
// (fieldRead is the field-access value, a RefMember whose object is `this`). It returns the field's
// same-class generic Signature when present (`funcCtx.FieldSignature`), else -- for an INHERITED field
// whose Signature lives in a superclass and is therefore absent from the current class's
// FieldSignatures -- it walks the class hierarchy with type-argument substitution
// (types.ResolveInstantiatedFieldType). `this` starts at the current class with an IDENTITY
// type-argument mapping (each class formal -> itself, built from the authoritative class Signature), so
// the recovered field type's args are the current class's own in-scope variables (denotable at the use
// site). Canonical cases: guava RegularContiguousSet<C> `this.domain` -> `DiscreteDomain<C>` (feeds
// receiverParamTypeArgs) and RegularImmutableSortedSet<E> `this.comparator` -> `Comparator<? super E>`
// (feeds the inherited-field return cast). Returns nil when fieldRead is not a `this.field` read, the
// field has no recoverable generic Signature, or the hierarchy leaves the jar. Kill-switch
// JDEC_INHERITED_FIELD_SIG_OFF gates the inherited (cross-class) leg only; the same-class leg is always on.
func RecoverThisFieldInstantiatedType(funcCtx *class_context.ClassContext, fieldRead JavaValue) types.JavaType {
	if funcCtx == nil || fieldRead == nil {
		return nil
	}
	rm, ok := UnpackSoltValue(fieldRead).(*RefMember)
	if !ok {
		return nil
	}
	ref, ok := UnpackSoltValue(rm.Object).(*JavaRef)
	if !ok || !ref.IsThis {
		return nil
	}
	fieldName := class_context.SafeIdentifier(rm.Member)
	if sig := funcCtx.FieldSignature(fieldName); sig != "" {
		return types.ParseSignature(sig)
	}
	if os.Getenv("JDEC_INHERITED_FIELD_SIG_OFF") == "" && funcCtx.SiblingClassSig != nil &&
		funcCtx.SiblingFieldSig != nil && funcCtx.ClassSig != "" && funcCtx.ClassName != "" {
		formals := types.ClassFormalTypeParamNames(funcCtx.ClassSig)
		if len(formals) > 0 {
			recvArgs := make([]types.JavaType, len(formals))
			for i, n := range formals {
				recvArgs[i] = types.NewJavaClass(n)
			}
			return types.ResolveInstantiatedFieldType(funcCtx, funcCtx.SiblingClassSig, funcCtx.SiblingFieldSig, funcCtx.ClassName, recvArgs, fieldName)
		}
	}
	return nil
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
		// The arity path abandons same-arity overloads as ambiguous; fall back to the call's EXACT
		// descriptor, which is unique in the JVM, to still recover the erased argument cast (guava
		// Builder `putAll(K, Iterable)` vs varargs `putAll(K, V...)`; `add(E)` vs `add(E...)`). Empty
		// when the descriptor-keyed table is disabled (JDEC_SAMECLASS_DESC_SIG_OFF) or the call has no
		// pool descriptor, so this stays a pure additive fallback.
		sig = funcCtx.MethodSignatureByDesc(f.FunctionName, f.Descriptor)
	}
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

// thisCtorTypeVarArgCast returns the class-scope type-variable NAME to cast the i-th argument of a
// SAME-CLASS `this(...)` constructor self-call to, or "" when no cast is needed. It is the bare-type-variable
// analogue of ctorWildcardArgCast: it fires when the target constructor's i-th formal is a BARE class type
// variable (`T`, recovered from the recorded constructor Signature) and the argument's static type is the
// erased bound (typically Object, e.g. a synthesized `new Object()`) rather than that type variable. The
// source passed a `(T)`-cast value (`this(name, (T) new Object())`); the bytecode erased it to Object and
// emitted no checkcast, so the decompiler renders a bare `this(name, new Object())` that javac rejects
// against the ctor's `(String, T)` signature ("Object cannot be converted to T"; spring PropertySource).
// Re-emitting the `(T)` unchecked cast makes it recompile. A `this(...)` self-call only runs inside an
// instance constructor, where the class type parameters are always in scope, so the cast is denotable.
// Kill-switch JDEC_THIS_CTOR_TYPEVAR_ARG_OFF.
func (f *FunctionCallExpression) thisCtorTypeVarArgCast(i int, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_THIS_CTOR_TYPEVAR_ARG_OFF") != "" || funcCtx == nil || f.FunctionName != "<init>" {
		return ""
	}
	// Only a `this(...)` self-call: constructor is in the CURRENT class (signature recorded), receiver is
	// `this` (never a `new CurrentClass(...)` in a static factory where the class type vars are out of scope).
	if f.ClassName != funcCtx.ClassName {
		return ""
	}
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
	// The formal must be a BARE class-scope type variable (`T`), not a parameterization (wildcard
	// parameterizations are ctorWildcardArgCast's domain) and not a concrete type.
	if strings.Contains(paramTypeStr, "<") || !funcCtx.IsTypeParam(paramTypeStr) {
		return ""
	}
	arg := f.Arguments[i]
	if lit, ok := UnpackSoltValue(arg).(*JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
		return "" // null is assignable to any type variable without a cast
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
	// Skip when the argument is already the target type variable (no erasure happened, no cast needed).
	if jc, ok := raw.(*types.JavaClass); ok && jc != nil && jc.Name == paramTypeStr {
		return ""
	}
	return paramTypeStr
}

// comparatorRawArgCast returns the raw `Comparator` cast string for the i-th argument when the call is a
// JDK sort/search static (Arrays.sort / Arrays.binarySearch / Collections.sort / Collections.binarySearch)
// and the i-th DESCRIPTOR parameter is java.util.Comparator. The array/list companion argument's element
// type is frequently erased to Object (the source's `(K[])` array cast erases to `(Object[])`), so a
// `Comparator<? super K>` argument no longer satisfies the method's `Comparator<? super T>` capture and
// javac rejects the call ("no suitable method for sort(Object[], Comparator<CAP#1>)"; guava
// ImmutableSortedMap$Builder.build / ImmutableSortedMultiset$Builder / ImmutableList.sortedCopyOf). A raw
// `Comparator` cast turns the call into an unchecked (behaviour-preserving) invocation that resolves; it
// is always compilation-safe even when the companion array was NOT erased (a redundant raw cast still
// binds to sort(T[], Comparator<? super T>) with an unchecked warning, never an error), so it never
// regresses a currently-compiling call. Identifying the Comparator position by the stable descriptor
// parameter type (not the argument's own possibly-erased static type) makes detection robust. Kill-switch
// JDEC_COMPARATOR_RAW_ARG_OFF.
func (f *FunctionCallExpression) comparatorRawArgCast(i int, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_COMPARATOR_RAW_ARG_OFF") != "" || !f.IsStatic {
		return ""
	}
	if f.ClassName != "java.util.Arrays" && f.ClassName != "java.util.Collections" {
		return ""
	}
	if f.FunctionName != "sort" && f.FunctionName != "binarySearch" {
		return ""
	}
	if f.FuncType == nil || i < 0 || i >= len(f.FuncType.ParamTypes) {
		return ""
	}
	pt := f.FuncType.ParamTypes[i]
	if pt == nil {
		return ""
	}
	jc, ok := pt.RawType().(*types.JavaClass)
	if !ok || jc.Name != "java.util.Comparator" {
		return ""
	}
	// NEVER raw-cast a lambda / method-reference argument: a raw `(Comparator)` target erases the SAM to
	// compare(Object, Object), so the lambda's parameters are inferred as Object and a body that uses them
	// at a more specific type fails ("cannot find symbol" / "incompatible types"). Only a plain value
	// (a field / variable Comparator, e.g. `this.comparator`) is safe to widen to raw. fastjson2 passes
	// lambdas to Arrays.sort, so this exclusion is load-bearing for zero cross-jar regression.
	if cv, ok := UnpackSoltValue(f.Arguments[i]).(*CustomValue); ok && cv.Flag == "lambda" {
		return ""
	}
	return types.NewJavaClass("java.util.Comparator").String(funcCtx)
}

// comparatorRawReceiverCast reports whether a `recv.compare(a, b)` call must render its RECEIVER as a raw
// `((Comparator)(recv))` cast. The receiver is a `Comparator<?>` / `Comparator<? super X>` /
// `Comparator<? extends X>` (a wildcard parameterization), so `compare` binds to `compare(CAP, CAP)`; the
// arguments -- read from a raw container or an Object[] -- are Object, which the capture rejects
// ("Object cannot be converted to CAP#1"; guava ImmutableSortedSet.unsafeCompare `var0.compare(v1, v2)`
// with `var0: Comparator<?>`, TreeMultiset `this.comparator().compare(...)`, ImmutableSortedMap$Builder
// `this.comparator.compare(...)`). Widening the receiver to raw `Comparator` makes it an unchecked,
// behaviour-preserving `compare(Object, Object)` that accepts the Object arguments. The three receiver
// shapes (parameterized local/param value, `this.field`, `this.method()`) are exactly what
// receiverParamTypeArgs already recovers. A lambda receiver is excluded (it is rendered with its own
// functional-interface cast in renderCall, and raw would erase its inferred parameter types). The
// existing non-wildcard `Comparator<E>.compare` path (InstantiateJDKMethodParam) already casts the
// ARGUMENTS to `(E)`, so this fires ONLY for the wildcard receiver it declines. Kill-switch
// JDEC_COMPARATOR_RAW_RECV_OFF.
func (f *FunctionCallExpression) comparatorRawReceiverCast(funcCtx *class_context.ClassContext) bool {
	if os.Getenv("JDEC_COMPARATOR_RAW_RECV_OFF") != "" || f.IsStatic || f.Object == nil {
		return false
	}
	if f.FunctionName != "compare" || len(f.Arguments) != 2 {
		return false
	}
	// A lambda / method-reference receiver is handled by the dedicated CustomValue branch in renderCall.
	if cv, ok := UnpackSoltValue(f.Object).(*CustomValue); ok && cv.Flag == "lambda" {
		return false
	}
	raw, typeArgs := f.receiverParamTypeArgs(funcCtx)
	if raw != "java.util.Comparator" || len(typeArgs) != 1 {
		return false
	}
	return types.IsWildcardType(typeArgs[0])
}

// collectionAddWildcardReceiverRawCast reports whether a `recv.add(x)` / `recv.offer(x)` call must render
// its RECEIVER as a raw `((Collection)(recv))` cast (rendered to the receiver's own raw class). The
// receiver is a wildcard `Collection<? super E>` (the JDK "consumer" collection shape), so add/offer bind
// to `add(CAP)` and reject an Object argument read from a raw container / Object[] ("Object cannot be
// converted to CAP#1"; guava Queues.drainUninterruptibly `var1.add(var7)`, FluentIterable.copyInto
// `var1.add(var3.next())`, both with `var1: Collection<? super E>`). Widening the receiver to its raw
// class makes it an unchecked, behaviour-preserving `add(Object)`. This mirrors comparatorRawReceiverCast
// (a wildcard receiver whose SOLE value parameter is the captured element type variable). It fires only
// for the JDK Iterable family, for a wildcard-parameterized receiver, and never for a lambda receiver.
// Returns the raw class name to cast to, or "". Kill-switch JDEC_COLLECTION_ADD_RAW_RECV_OFF.
func (f *FunctionCallExpression) collectionAddWildcardReceiverRawCast(funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_COLLECTION_ADD_RAW_RECV_OFF") != "" || f.IsStatic || f.Object == nil {
		return ""
	}
	if f.FunctionName != "add" && f.FunctionName != "offer" {
		return ""
	}
	if len(f.Arguments) != 1 {
		return ""
	}
	if cv, ok := UnpackSoltValue(f.Object).(*CustomValue); ok && cv.Flag == "lambda" {
		return ""
	}
	raw, typeArgs := f.receiverParamTypeArgs(funcCtx)
	if !types.IsJDKIterableFamily(raw) || len(typeArgs) != 1 {
		return ""
	}
	if !types.IsWildcardType(typeArgs[0]) {
		return ""
	}
	return raw
}

// wildcardConsumerReceiverMethods maps a SINGLE-type-parameter "consumer" interface (a value is fed IN to
// the sole type-parameter position) to the method(s) that consume it and their argument count. A wildcard-
// parameterized receiver `X<? super Y>` binds such a method to a capture and rejects the (erased) argument
// ("Y cannot be converted to CAP#N"); a raw `((X)(recv))` cast makes it an unchecked, behaviour-preserving
// call. Only single-type-parameter interfaces qualify so the raw cast erases exactly the one captured
// parameter (Comparator/Collection have dedicated helpers with different semantics). Covers the guava/JDK
// predicate/consumer family (guava Maps$FilteredEntryMap / $FilteredMapValues `predicate.apply(entry)` on
// `Predicate<? super Entry<K,V>>`).
var wildcardConsumerReceiverMethods = map[string]map[string]int{
	"com.google.common.base.Predicate": {"apply": 1},
	"java.util.function.Predicate":     {"test": 1},
	"java.util.function.Consumer":      {"accept": 1},
}

// wildcardConsumerReceiverRawCast reports whether a `recv.m(x)` call on a wildcard-parameterized
// single-type-parameter consumer interface (see wildcardConsumerReceiverMethods) must render its RECEIVER
// as a raw `((X)(recv))` cast. The receiver is e.g. `Predicate<? super Entry<K,V>>`, so `apply` binds to
// `apply(CAP)` and rejects an argument read at the erased type ("Entry cannot be converted to CAP#N"; guava
// Maps$FilteredEntryMap:29 `var1.apply(var5)`, $FilteredMapValues `this.predicate.apply(var3)`). Widening
// the receiver to its raw class makes it an unchecked, behaviour-preserving call. Mirrors
// collectionAddWildcardReceiverRawCast / comparatorRawReceiverCast. Never fires for a lambda receiver.
// Returns the raw class name to cast to, or "". Kill-switch JDEC_WILDCARD_CONSUMER_RECV_OFF.
func (f *FunctionCallExpression) wildcardConsumerReceiverRawCast(funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_WILDCARD_CONSUMER_RECV_OFF") != "" || f.IsStatic || f.Object == nil {
		return ""
	}
	if cv, ok := UnpackSoltValue(f.Object).(*CustomValue); ok && cv.Flag == "lambda" {
		return ""
	}
	raw, typeArgs := f.receiverParamTypeArgs(funcCtx)
	methods, ok := wildcardConsumerReceiverMethods[raw]
	if !ok {
		return ""
	}
	argc, ok := methods[f.FunctionName]
	if !ok || len(f.Arguments) != argc {
		return ""
	}
	if len(typeArgs) != 1 || !types.IsWildcardType(typeArgs[0]) {
		return ""
	}
	return raw
}

// superCtorTypeVarArgCast returns the type-variable name to cast the i-th argument of a `super(...)` call
// to, or "" when no cast is needed. It fires when the SUPERCLASS constructor's i-th parameter is a bare
// type variable (recovered from the super ctor's generic Signature via SiblingCtorSig) that the subclass
// binds to one of its OWN in-scope type variables (via the `extends Super<...>` clause), and the argument's
// static type is the erased bound rather than that type variable. The source passed a type-variable-typed
// value with no cast (`super(graph, node)` where node is N); the bytecode erased the value to Object/the
// bound and emitted no checkcast, so the decompiler renders a bare `super(..., objVal)` that javac rejects
// against the super ctor's `(..., N)` signature ("Object cannot be converted to N/CAP#1"; guava graph
// IncidentEdgeSet subclasses, RegularContiguousSet anonymous iterators). Re-emitting the erased `(N)` cast
// makes it recompile (unchecked, behaviour-preserving). Kill-switch JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF.
func (f *FunctionCallExpression) superCtorTypeVarArgCast(i int, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF") != "" {
		return ""
	}
	if funcCtx == nil || funcCtx.SiblingCtorSig == nil || funcCtx.ClassSig == "" {
		return ""
	}
	// Only a super(...) constructor call: invokespecial <init> on the direct superclass.
	if f.FunctionName != "<init>" || !f.IsSpecialInvoke || f.ClassName == "" || f.ClassName != funcCtx.SupperClassName {
		return ""
	}
	if i < 0 || i >= len(f.Arguments) {
		return ""
	}
	superInternal := strings.ReplaceAll(f.ClassName, ".", "/")
	ctorSig, ok := funcCtx.SiblingCtorSig(superInternal, len(f.Arguments))
	if !ok || ctorSig == "" {
		return ""
	}
	_, params, _ := types.ParseMethodSignatureFull(ctorSig, funcCtx)
	if i >= len(params) || params[i] == nil {
		return ""
	}
	pjc, ok := params[i].RawType().(*types.JavaClass)
	if !ok || pjc == nil {
		return ""
	}
	// Map the super ctor param's type-variable name (super's formal name) to the subclass's own type var.
	mapped := f.mapSuperFormalToSubclassTypeVar(pjc.Name, funcCtx)
	if mapped == "" || !funcCtx.IsTypeParam(mapped) {
		return ""
	}
	// Skip when the argument is already the target type variable (no erasure happened).
	if at := UnpackSoltValue(f.Arguments[i]).Type(); at != nil {
		if ajc, ok := at.RawType().(*types.JavaClass); ok && ajc != nil && ajc.Name == mapped {
			return ""
		}
	}
	return mapped
}

// mapSuperFormalToSubclassTypeVar maps a superclass formal type-variable NAME (as it appears in the super
// ctor Signature, e.g. IncidentEdgeSet's `N`) to the subclass's OWN in-scope type variable that the
// subclass binds it to through its `extends Super<...>` clause (e.g. subclass `Sub<N> extends
// IncidentEdgeSet<N>` binds super-N to sub-N). Returns "" when the name is not a super formal, the super
// signature is unavailable, or the bound argument is not a bare subclass type variable.
func (f *FunctionCallExpression) mapSuperFormalToSubclassTypeVar(superFormalName string, funcCtx *class_context.ClassContext) string {
	superInternal := strings.ReplaceAll(f.ClassName, ".", "/")
	if funcCtx.SiblingClassSig == nil {
		return ""
	}
	superSig, _, ok := funcCtx.SiblingClassSig(superInternal)
	if !ok || superSig == "" {
		return ""
	}
	superFormalNames := types.ClassFormalTypeParamNames(superSig)
	idx := -1
	for k, n := range superFormalNames {
		if n == superFormalName {
			idx = k
			break
		}
	}
	if idx < 0 {
		return ""
	}
	sup, ifaces := types.ParseClassSignatureSupers(funcCtx.ClassSig)
	cands := append([]types.JavaType{sup}, ifaces...)
	for _, c := range cands {
		if c == nil {
			continue
		}
		pt, ok := types.AsParameterizedType(c)
		if !ok || pt.RawClassName != f.ClassName {
			continue
		}
		if idx >= len(pt.TypeArgs) || pt.TypeArgs[idx] == nil {
			return ""
		}
		if ajc, ok := pt.TypeArgs[idx].RawType().(*types.JavaClass); ok && ajc != nil {
			return ajc.Name
		}
		return ""
	}
	return ""
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
	// A `super.m()` invokespecial to a NON-current class binds to the SUPERTYPE's declaration. Recover
	// the superclass's PARAMETERIZED type from the current class Signature's extends clause (e.g. guava
	// MutableClassToInstanceMap$1<B> extends ForwardingMapEntry<Class<? extends B>, B>) and resolve the
	// param there: setValue(V) -> V maps to B, so `super.setValue(objArg)` re-emits the source's `(B)`
	// cast (`Object cannot be converted to B`). The receiver is `this`; starting the hierarchy walk at
	// the DIRECT super with its actual args correctly binds to the super declaration (skipping the
	// current class's override) and, via ResolveInstantiatedParamType, follows deeper chains too.
	// Kill-switch JDEC_SUPER_PARAM_RESOLVE_OFF restores the legacy skip.
	if f.IsSpecialInvoke && !f.isCurrentClass(funcCtx) {
		if os.Getenv("JDEC_SUPER_PARAM_RESOLVE_OFF") != "" || funcCtx.ClassSig == "" {
			return nil
		}
		ref, ok := UnpackSoltValue(f.Object).(*JavaRef)
		if !ok || !ref.IsThis {
			return nil
		}
		sup, _ := types.ParseClassSignatureSupers(funcCtx.ClassSig)
		pt, ok := types.AsParameterizedType(sup)
		if !ok {
			return nil
		}
		return types.ResolveInstantiatedParamType(funcCtx, funcCtx.SiblingClassSig, pt.RawClassName, pt.TypeArgs, f.FunctionName, len(f.Arguments), i)
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

// genericMethodWitnessArgParamType recovers the instantiated type of the i-th argument of a SAME-CLASS
// call to a GENERIC method whose i-th formal is a bare METHOD-scope type variable (e.g. the `N` in
// `<N> ... connectionsOf(Graph<N> graph, N node)`), by INFERRING that type variable from a "witness"
// argument -- another formal `SomeClass<...N...>` whose actual argument carries a concrete type argument
// at N's position. The witness pins N to a caller-scope type (guava ImmutableGraph.getNodeConnections
// `connectionsOf(var0, var3)` / Graphs `reachableNodes(var0, var3)`: `var0` is `Graph<N>` so N binds to
// the caller's N, but `var3` was read as `Object` from a raw `nodes().iterator()` -> "Object cannot be
// converted to N"). Neither sameClassMethodParamType (class-scope vars only, skips static) nor
// resolvedParamType (instance receivers only) covers this method-scope, witness-inferred case.
//
// The recovered type is only returned when it is a type variable IN SCOPE at the call site
// (funcCtx.IsTypeParam), so the emitted `(N)` cast is always denotable, and only for a SAME-CLASS callee
// whose generic Signature is available (arity- or descriptor-keyed). Kill-switch
// JDEC_GENERIC_METHOD_WITNESS_OFF.
func (f *FunctionCallExpression) genericMethodWitnessArgParamType(i int, funcCtx *class_context.ClassContext) types.JavaType {
	if os.Getenv("JDEC_GENERIC_METHOD_WITNESS_OFF") != "" || funcCtx == nil {
		return nil
	}
	if f.ClassName == "" || i < 0 || i >= len(f.Arguments) {
		return nil
	}
	if f.IsSpecialInvoke && !f.isCurrentClass(funcCtx) {
		return nil // super.m() binds to a supertype declaration -- leave it to other paths.
	}
	// Resolve the callee's generic Signature: same-class from funcCtx's tables (arity- or descriptor-keyed),
	// cross-class from the sibling resolver's (name, arity)-keyed method signatures (guava EndpointPair
	// `<N> ordered(N, N)` called `EndpointPair.ordered(var1, capturedObj)`).
	sig := ""
	if f.ClassName == funcCtx.ClassName {
		if sig = funcCtx.MethodSignature(f.FunctionName, len(f.Arguments)); sig == "" {
			sig = funcCtx.MethodSignatureByDesc(f.FunctionName, f.Descriptor)
		}
	} else if funcCtx.SiblingClassSig != nil {
		if _, methodSigs, ok := funcCtx.SiblingClassSig(strings.ReplaceAll(f.ClassName, ".", "/")); ok && methodSigs != nil {
			sig = methodSigs[class_context.MethodSigKey(f.FunctionName, len(f.Arguments))]
		}
	}
	if sig == "" {
		return nil
	}
	methodTypeParams := types.ClassFormalTypeParamNames(sig) // reuses the leading `<...>` name parser
	_, params, _ := types.ParseMethodSignatureFull(sig, funcCtx)
	if len(methodTypeParams) == 0 || i >= len(params) || params[i] == nil {
		return nil
	}
	// Formal i must be a bare type variable declared by the METHOD (not the class -- a class-scope var is
	// already handled by sameClassMethodParamType).
	pjc, ok := params[i].RawType().(*types.JavaClass)
	if !ok || pjc == nil {
		return nil
	}
	tvName := pjc.Name
	isMethodTypeParam := false
	for _, tp := range methodTypeParams {
		if tp == tvName {
			isMethodTypeParam = true
			break
		}
	}
	if !isMethodTypeParam {
		return nil
	}
	// Find a WITNESS formal j (j != i) that pins tvName from its actual argument, in either shape:
	//   (a) `SomeClass<...tvName...>` -> binding = arg j's type argument at tvName's position;
	//   (b) a BARE `tvName` -> binding = arg j's whole static type (guava `ordered(N nodeU, N nodeV)`:
	//       nodeU=var1 is typed N, so N binds to the caller's N and nodeV's Object arg gets `(N)`).
	// The binding is only accepted when it is itself a type variable IN SCOPE at the call site
	// (funcCtx.IsTypeParam), so the emitted cast is denotable; a concrete-class witness is skipped
	// (casting to it could be wrong -- the real inferred type may be a supertype/LUB).
	for j := 0; j < len(params) && j < len(f.Arguments); j++ {
		if j == i || params[j] == nil {
			continue
		}
		var bound types.JavaType
		if wjc, ok := params[j].RawType().(*types.JavaClass); ok && wjc != nil && wjc.Name == tvName && !params[j].IsArray() {
			// A bare type-variable witness (`N nodeU`): the argument's whole static type binds tvName.
			bound = UnpackSoltValue(f.Arguments[j]).Type()
		} else if wp, ok := types.AsParameterizedType(params[j]); ok {
			pos := -1
			for k, ta := range wp.TypeArgs {
				if ta == nil {
					continue
				}
				// A wildcard type arg (`? extends X` / `? super X` / `?`) is not a bare type
				// variable and its embedded JavaType is nil, so RawType() would panic. Skip it:
				// this witness shape only pins tvName when the type arg IS the bare variable.
				if _, isWild := ta.(*types.JavaWildcardType); isWild {
					continue
				}
				if jc, ok := ta.RawType().(*types.JavaClass); ok && jc != nil && jc.Name == tvName {
					pos = k
					break
				}
			}
			if pos < 0 {
				continue
			}
			at := UnpackSoltValue(f.Arguments[j]).Type()
			if at == nil {
				continue
			}
			ap, ok := types.AsParameterizedType(at)
			if !ok || ap.RawClassName != wp.RawClassName || pos >= len(ap.TypeArgs) || ap.TypeArgs[pos] == nil {
				continue
			}
			bound = ap.TypeArgs[pos]
		}
		if bound == nil {
			continue
		}
		bjc, ok := bound.RawType().(*types.JavaClass)
		if !ok || bjc == nil || !funcCtx.IsTypeParam(bjc.Name) {
			continue
		}
		return bound
	}
	return nil
}

// typeArgVar extracts the referenced TYPE VARIABLE from a type argument, unwrapping a `? super X` /
// `? extends X` wildcard down to its bound: it returns the bound's JavaType, the variable name, and ok.
// A bare type variable argument returns itself. Anything else (concrete class, unbounded `?`, nested
// parameterization) returns ok=false. The bare *JavaWildcardType is asserted BEFORE RawType because a
// wildcard embeds a nil JavaType whose RawType panics (see isWildcardType).
func typeArgVar(ta types.JavaType) (types.JavaType, string, bool) {
	if ta == nil {
		return nil, "", false
	}
	if w, ok := ta.(*types.JavaWildcardType); ok {
		if w.Bound == nil {
			return nil, "", false
		}
		if jc, ok := w.Bound.RawType().(*types.JavaClass); ok && jc != nil {
			return w.Bound, jc.Name, true
		}
		return nil, "", false
	}
	raw := ta.RawType()
	if raw == nil {
		return nil, "", false
	}
	if w, ok := raw.(*types.JavaWildcardType); ok {
		if w.Bound == nil {
			return nil, "", false
		}
		if jc, ok := w.Bound.RawType().(*types.JavaClass); ok && jc != nil {
			return w.Bound, jc.Name, true
		}
		return nil, "", false
	}
	if jc, ok := raw.(*types.JavaClass); ok && jc != nil {
		return ta, jc.Name, true
	}
	return nil, "", false
}

// varargsTypeVarArrayArgParamType recovers the instantiated type of the trailing argument of a call to a
// GENERIC method whose last formal is a VARARGS array of a METHOD-scope type variable (`E... == E[]`),
// when that slot receives a single reference array of a DIFFERENT element type (e.g. an `Object[]`). The
// method type variable E is INFERRED from a "witness" formal `SomeClass<...E...>` -- including a
// `? super E` / `? extends E` wildcard bound -- whose actual argument pins E to a caller-scope type
// variable (guava ImmutableSortedSet.construct `(Comparator<? super E>, int, E...)` called with an
// `Object[]`: without the `(E[])` cast javac over-constrains E to both Object (from the array) and the
// caller's E (from the comparator) -> "method construct cannot be applied to given types"). The recovered
// `Ecaller[]` is returned so ArgumentStrings re-emits the erased `(E[])` cast. Only fires when the
// binding is itself an IN-SCOPE type variable (denotable). Neither genericMethodWitnessArgParamType (bare
// type-var formal only, skips arrays) nor the other resolvers cover a varargs type-var array formal.
// Kill-switch JDEC_VARARGS_TYPEVAR_ARRAY_OFF.
func (f *FunctionCallExpression) varargsTypeVarArrayArgParamType(i int, funcCtx *class_context.ClassContext) types.JavaType {
	if os.Getenv("JDEC_VARARGS_TYPEVAR_ARRAY_OFF") != "" || funcCtx == nil {
		return nil
	}
	if f.ClassName == "" || i < 0 || i >= len(f.Arguments) || i != len(f.Arguments)-1 {
		return nil // varargs is always the trailing formal.
	}
	if f.IsSpecialInvoke && !f.isCurrentClass(funcCtx) {
		return nil
	}
	// The argument must itself be a reference array (the un-cast Object[] javac rejects).
	at := UnpackSoltValue(f.Arguments[i]).Type()
	if at == nil || !at.IsArray() {
		return nil
	}
	sig := ""
	if f.ClassName == funcCtx.ClassName {
		if sig = funcCtx.MethodSignature(f.FunctionName, len(f.Arguments)); sig == "" {
			sig = funcCtx.MethodSignatureByDesc(f.FunctionName, f.Descriptor)
		}
	} else if funcCtx.SiblingClassSig != nil {
		if _, methodSigs, ok := funcCtx.SiblingClassSig(strings.ReplaceAll(f.ClassName, ".", "/")); ok && methodSigs != nil {
			sig = methodSigs[class_context.MethodSigKey(f.FunctionName, len(f.Arguments))]
		}
	}
	if sig == "" {
		return nil
	}
	methodTypeParams := types.ClassFormalTypeParamNames(sig)
	_, params, _ := types.ParseMethodSignatureFull(sig, funcCtx)
	if len(methodTypeParams) == 0 || i >= len(params) || params[i] == nil || !params[i].IsArray() {
		return nil
	}
	arr, ok := params[i].RawType().(*types.JavaArrayType)
	if !ok || arr == nil || arr.JavaType == nil {
		return nil
	}
	elemJc, ok := arr.JavaType.RawType().(*types.JavaClass)
	if !ok || elemJc == nil {
		return nil
	}
	tvName := elemJc.Name
	isMethodTypeParam := false
	for _, tp := range methodTypeParams {
		if tp == tvName {
			isMethodTypeParam = true
			break
		}
	}
	if !isMethodTypeParam {
		return nil
	}
	// Infer tvName from a witness formal j (parameterized, its type arg is tvName -- bare or wildcard
	// bounded) whose actual argument pins tvName to an in-scope type variable.
	for j := 0; j < len(params) && j < len(f.Arguments); j++ {
		if j == i || params[j] == nil {
			continue
		}
		wp, ok := types.AsParameterizedType(params[j])
		if !ok {
			continue
		}
		pos := -1
		for k, ta := range wp.TypeArgs {
			if _, nm, ok := typeArgVar(ta); ok && nm == tvName {
				pos = k
				break
			}
		}
		if pos < 0 {
			continue
		}
		aj := UnpackSoltValue(f.Arguments[j]).Type()
		if _, okp := types.AsParameterizedType(aj); !okp {
			// A raw field read (guava ImmutableSortedSet$Builder `this.comparator` renders as raw
			// Comparator): recover its instantiated generic type `Comparator<? super E>` so the witness
			// pins E to the builder's class-scope E.
			if rec := RecoverThisFieldInstantiatedType(funcCtx, f.Arguments[j]); rec != nil {
				aj = rec
			}
		}
		if aj == nil {
			continue
		}
		ap, ok := types.AsParameterizedType(aj)
		if !ok || ap.RawClassName != wp.RawClassName || pos >= len(ap.TypeArgs) {
			continue
		}
		boundType, boundName, ok := typeArgVar(ap.TypeArgs[pos])
		if !ok || boundType == nil || !funcCtx.IsTypeParam(boundName) {
			continue
		}
		return types.NewJavaArrayType(boundType) // Ecaller[]
	}
	return nil
}

// typeVarElemArrayArgCast reports whether the synthesized `(E[])(arg)` cast should be emitted for an
// argument whose recovered formal type is a reference ARRAY of an IN-SCOPE type variable (`E[]`) while the
// ARGUMENT is itself a reference array of a different element type (e.g. Object[]). The (ok1 && ok2)
// class-vs-class branch never fires here because JavaArrayType.RawType() is not a *JavaClass. Companion to
// varargsTypeVarArrayArgParamType. Kill-switch JDEC_VARARGS_TYPEVAR_ARRAY_OFF.
func (f *FunctionCallExpression) typeVarElemArrayArgCast(argType types.JavaType, resolvedGeneric bool, arg JavaValue, funcCtx *class_context.ClassContext) bool {
	if os.Getenv("JDEC_VARARGS_TYPEVAR_ARRAY_OFF") != "" {
		return false
	}
	if !resolvedGeneric || argType == nil || arg == nil || funcCtx == nil || !argType.IsArray() {
		return false
	}
	arr, ok := argType.RawType().(*types.JavaArrayType)
	if !ok || arr == nil || arr.JavaType == nil {
		return false
	}
	elem, ok := arr.JavaType.RawType().(*types.JavaClass)
	if !ok || elem == nil || !funcCtx.IsTypeParam(elem.Name) {
		return false
	}
	at := arg.Type()
	return at != nil && at.IsArray()
}

// isCurrentClass reports whether the call's target class is the class currently being rendered.
// Used to tell a private same-class invokespecial (`this.m()`) from a super call (`super.m()`).
func (f *FunctionCallExpression) isCurrentClass(funcCtx *class_context.ClassContext) bool {
	return funcCtx != nil && f.ClassName == funcCtx.ClassName
}

// interfaceDefaultSuperQualifier returns the denotable qualifier `Iface` to render an interface-default
// super call as `Iface.super.m()`, or "" when the call is an ordinary superclass `super.m()`. A
// `super.m()` invokespecial (receiver `this`, target class != current class, not <init>) targets EITHER
// the direct superclass OR a directly-implemented interface's default method; javac spells the latter
// `Iface.super.m()`, and a bare `super.m()` there resolves against the superclass (which does not
// declare the method) -> "cannot find symbol". The target is confirmed to be a DIRECT interface of the
// current class (never the superclass) via SiblingSuperTypes, whose first entry is the super_class and
// the rest are the direct interfaces. Returns "" (ordinary super) when the resolver is unavailable, the
// current class is not found, or the target is the superclass / not among the direct interfaces, so the
// legacy `super.m()` rendering is preserved for every non-interface-default case. Kill-switch:
// JDEC_IFACE_DEFAULT_SUPER_OFF=1.
func (f *FunctionCallExpression) interfaceDefaultSuperQualifier(funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_IFACE_DEFAULT_SUPER_OFF") != "" || funcCtx == nil || funcCtx.SiblingSuperTypes == nil {
		return ""
	}
	// An ordinary superclass super-call: the target class IS the superclass. Leave as bare `super.`.
	if f.ClassName == funcCtx.SupperClassName {
		return ""
	}
	supers, ok := funcCtx.SiblingSuperTypes(strings.ReplaceAll(funcCtx.ClassName, ".", "/"))
	if !ok || len(supers) < 2 {
		return ""
	}
	target := strings.ReplaceAll(f.ClassName, ".", "/")
	// supers[0] is the super_class; supers[1:] are the direct interfaces. Only a direct interface target
	// is an interface-default super call.
	for _, iface := range supers[1:] {
		if iface == target {
			return funcCtx.ShortTypeName(f.ClassName)
		}
	}
	return ""
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

// arrayParamRefArgCast reports whether the argument needs a synthesized cast to an ARRAY parameter type
// because the parameter is an array (`byte[]`) while the argument's static type is a NON-array reference
// class (typically `java.lang.Object`, from a null-initialized local). The class-vs-class arg-cast
// branch never reaches this case: an array parameter's RawType() is *JavaArrayType, not *JavaClass, so
// ok1 is false. A non-array reference is not assignable to an array parameter in Java source ("Object
// cannot be converted to byte[]"; spring ASM Attribute.computeAttributesSize/putAttributes pass an
// `Object cattrs = null` into a `byte[]` overload). The value already occupies the array slot in bytecode
// (a null or a checkcast), so the `(byte[])` cast is behaviour-preserving. Tightly gated: array parameter
// + non-array reference argument whose erasure is Object (a concrete-class argument to an array parameter
// would be a genuine type error we must not paper over). Kill-switch: JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF=1.
func arrayParamRefArgCast(argType types.JavaType, arg JavaValue) bool {
	if os.Getenv("JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF") != "" {
		return false
	}
	if argType == nil || arg == nil || !argType.IsArray() {
		return false
	}
	at := arg.Type()
	if at == nil || at.IsArray() {
		return false
	}
	ajc, ok := at.RawType().(*types.JavaClass)
	if !ok || ajc == nil {
		return false // primitive / parameterized / other: not this null-Object shape.
	}
	return ajc.Name == "java.lang.Object"
}

// resolvedParameterizedArgCast reports whether the i-th argument needs a synthesized `(X<...>)` cast
// because a generic resolver recovered the formal as a PARAMETERIZED type (e.g.
// NavigableMap<Cut<C>,Range<C>>.tailMap's key formal resolves to `Cut<C>`) whose RAW class differs from
// the argument's erased static type. The normal (ok1 && ok2) class-vs-class arg-cast branch can never
// reach this case: a parameterized type's RawType() is not a *JavaClass (ok1 is false). The descriptor
// erases the formal to its bound, so a differently-erased argument (an Object read from a raw entry)
// flows in without the source's `(Cut<C>)` cast and javac -- re-resolving against the field's real
// generic type -- rejects it ("Object cannot be converted to Cut<C>"). Only fires for a RECOVERED
// (resolvedGeneric) parameterized formal whose raw class both differs from the argument's raw class and
// is not itself Object, and never when the argument is a bare in-scope type variable (casting `C` to a
// concrete `Cut<C>` would be a spurious over-cast, cf. suppressTypeVarArgCast). guava
// TreeRangeSet$RangesByUpperBound / $SubRangeSetRangesByLowerBound `rangesByLowerBound.tailMap/headMap`.
// Kill-switch JDEC_PARAM_ARG_CAST_OFF.
func resolvedParameterizedArgCast(funcCtx *class_context.ClassContext, argType types.JavaType, resolvedGeneric bool, arg JavaValue) bool {
	if os.Getenv("JDEC_PARAM_ARG_CAST_OFF") != "" {
		return false
	}
	if !resolvedGeneric || argType == nil || arg == nil || funcCtx == nil {
		return false
	}
	pt, ok := types.AsParameterizedType(argType)
	if !ok || pt.RawClassName == "" || pt.RawClassName == "java.lang.Object" {
		return false
	}
	at := arg.Type()
	if at == nil {
		return false
	}
	ajc, ok := at.RawType().(*types.JavaClass)
	if !ok || ajc == nil {
		return false // array / primitive / parameterized argument: not this case.
	}
	if ajc.Name == pt.RawClassName {
		return false // same erasure -- no cast needed (already a Cut / raw Cut).
	}
	if funcCtx.IsTypeParam(ajc.Name) {
		return false // bare in-scope type variable argument: casting to a concrete parameterization is a spurious over-cast.
	}
	return true
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
	// Prefer the DESCRIPTOR-keyed Signature (exact overload) over the arity-keyed one, which is
	// dropped when overloads collide on arity (e.g. SortedLists.binarySearch's two 5-arg overloads) --
	// without it this method could not tell that the erased formal is a type variable. Kill-switch
	// JDEC_SIBLING_DESC_SIG_OFF restores the arity-only lookup for A/B isolation.
	sig := ""
	if os.Getenv("JDEC_SIBLING_DESC_SIG_OFF") == "" {
		sig = methodSigs[class_context.MethodDescKey(f.FunctionName, f.Descriptor)]
	}
	if sig == "" {
		sig = methodSigs[class_context.MethodSigKey(f.FunctionName, len(f.Arguments))]
	}
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
//
// The second case is java.util.EnumSet.of(E first, ...): every overload's formals are the method-scope
// `E extends Enum<E>` erased to `java.lang.Enum` in the descriptor, so the arg-cast logic upcasts the
// concrete enum constant to raw `(Enum)` -- collapsing javac's inference of E and breaking overload
// resolution ("no suitable method found for of(Enum,TaskOption[])"; spring ConcurrentReferenceHashMap$Task
// `EnumSet.of(options[0], options)`). Same reasoning: the argument already flowed into E in bytecode, so
// dropping the cast is behaviour-preserving and lets javac infer E. Keyed on the exact callee class
// (java.util.EnumSet) + method (of) + the Enum-erased formal. Kill-switch: JDEC_ENUMSET_OF_NOCAST_OFF=1.
func jdkCalleeParamIsErasedTypeVar(className, method string, paramIndex, argc int, paramType types.JavaType) bool {
	if paramType == nil {
		return false
	}
	raw, ok := paramType.RawType().(*types.JavaClass)
	if !ok || raw == nil || raw.Name != "java.lang.Enum" {
		return false
	}
	if method == "compareTo" && argc == 1 && paramIndex == 0 &&
		os.Getenv("JDEC_ENUM_COMPARETO_NOCAST_OFF") == "" {
		return true
	}
	if method == "of" && (className == "java/util/EnumSet" || className == "java.util.EnumSet") &&
		os.Getenv("JDEC_ENUMSET_OF_NOCAST_OFF") == "" {
		return true
	}
	return false
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

// lambdaArgFunctionalCast returns the functional-interface cast that a lambda / method-reference
// argument needs when the call's RECEIVER is a generic type used RAW. Calling any method through a
// raw-typed reference erases the ENTIRE method signature (JLS 4.8), even parts that never mention the
// class type variable, so a functional-interface parameter `Consumer<FieldReader>` collapses to raw
// `Consumer` (SAM `accept(Object)`). An explicitly-typed lambda `(FieldReader l0) -> ...` then fails
// ("incompatible parameter types in lambda expression"), and a body dereferencing the parameter fails
// even with an implicit parameter (it would be typed Object). The original source guards this with an
// explicit `(Consumer<FieldReader>) e -> {...}` cast; the decompiler drops it because the descriptor
// parameter type is already erased. Re-emit it, casting to the lambda VALUE's own instantiated
// functional-interface type (recovered from the invokedynamic instantiatedMethodType by
// inferLambdaTypeFromInstantiated, e.g. Consumer<FieldReader>). Canonical case: fastjson2
// JSONSchema.of `((ObjectReaderAdapter) reader).apply((Consumer<FieldReader>) e -> ...)`.
//
// This is the method-call companion of genericCtorDiamond (which fixes the same raw-erasure defect for
// `new Generic(lambda)` via the diamond). Gated to (a) a lambda / method-ref argument, (b) a receiver
// whose static type is a jar-internal GENERIC class used raw (SiblingClassSig confirms formal type
// params AND the receiver carries no type arguments -- a parameterized receiver keeps the SAM param
// intact and needs no cast), and (c) a lambda value whose upgraded type is a denotable parameterized
// functional interface. Kill-switch: JDEC_LAMBDA_RAWRECV_CAST_OFF=1.
func (f *FunctionCallExpression) lambdaArgFunctionalCast(i int, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_LAMBDA_RAWRECV_CAST_OFF") != "" || f.IsStatic || f.Object == nil ||
		funcCtx == nil || funcCtx.SiblingClassSig == nil {
		return ""
	}
	// Constructor calls (`new Generic<>(lambda)`) are handled by genericCtorDiamond, which restores the
	// diamond so javac infers the class type arguments from the constructor arguments. Casting the
	// lambda argument here would OVER-constrain that inference (e.g. `new FieldReaderListFuncImpl<>(
	// (Supplier<List>) ArrayList::new, ...)` -> "cannot infer type arguments"), so leave `<init>` alone.
	if f.FunctionName == "<init>" {
		return ""
	}
	// (a) argument is a lambda / method reference (both carry Flag "lambda").
	cv, ok := UnpackSoltValue(f.Arguments[i]).(*CustomValue)
	if !ok || cv.Flag != "lambda" {
		return ""
	}
	// A method reference binds NATIVELY to the raw SAM (no explicit parameter types to mismatch), so
	// the parameterized-FI cast is unnecessary for it and, for SAMs with nested wildcards, can defeat
	// javac poly inference at the call site (same reasoning as lambdaArgRawJDKReceiverCast). Only an
	// explicitly-typed lambda body needs the cast. Skip method references.
	if cv.IsMethodRef {
		return ""
	}
	lamType := cv.Type()
	if lamType == nil {
		return ""
	}
	if _, ok := types.AsParameterizedType(lamType); !ok {
		return ""
	}
	// (b) receiver is a jar-internal generic class used RAW. A parameterized receiver keeps the
	// method's functional-interface parameter intact, so no cast is needed there.
	recv := f.Object.Type()
	if recv == nil {
		return ""
	}
	if _, ok := types.AsParameterizedType(recv); ok {
		return ""
	}
	jc, ok := recv.RawType().(*types.JavaClass)
	if !ok || jc == nil {
		return ""
	}
	classSig, _, ok := funcCtx.SiblingClassSig(strings.ReplaceAll(jc.Name, ".", "/"))
	if !ok || len(types.ClassFormalTypeParamNames(classSig)) == 0 {
		return ""
	}
	return lamType.String(funcCtx)
}

// jdkGenericLambdaReceivers is the small set of JDK generic types whose fluent methods take a
// functional-interface parameter, so calling them through a RAW receiver erases that parameter's SAM.
// A raw java.util.stream.Stream (produced by `.stream()` on a raw Collection) or a raw java.util.Optional
// makes map(Function)/filter(Predicate)/forEach(Consumer)/... collapse to raw SAMs (apply(Object) etc.),
// which an explicitly-typed lambda can no longer bind to. Kept intentionally tiny and unambiguous.
var jdkGenericLambdaReceivers = map[string]bool{
	"java.util.stream.Stream": true,
	"java.util.Optional":      true,
}

// jdkGenericCtorDiamondClasses is the small set of JDK generic collection classes whose RAW `new X(...)`
// in call-receiver position gets the diamond restored by newRecvJDKGenericDiamond. Kept tiny and
// unambiguous (concrete, non-anonymous, diamond-inferable under --release 8).
var jdkGenericCtorDiamondClasses = map[string]bool{
	"java.util.HashMap":       true,
	"java.util.LinkedHashMap": true,
	"java.util.TreeMap":       true,
	"java.util.ArrayList":     true,
	"java.util.LinkedList":    true,
	"java.util.HashSet":       true,
	"java.util.LinkedHashSet": true,
	"java.util.TreeSet":       true,
}

// newRecvJDKGenericDiamond renders a RAW JDK-generic `new X(args)` receiver with the diamond restored
// (`new X<>(args)`) when THIS call passes a lambda / method reference. Calling a lambda-taking method
// (forEach/removeIf/compute...) through a raw-typed receiver erases the functional-interface parameter's
// SAM to Object (JLS 4.8), so the lambda's parameters degrade to Object and a body dereferencing them
// fails ("Object cannot be converted to String"; spring SimpleAliasRegistry
// `new HashMap(this.aliasMap).forEach((l0, l1) -> {... var1.resolveStringValue(l0) ...})`). The source
// necessarily had a parameterized `new HashMap<>(...)` (or explicit args) for the lambda body to
// compile; the diamond lets javac re-infer the type arguments from the constructor argument (a typed
// map -> HashMap<String,String>), rebinding the lambda parameters. Verified against javac: the diamond
// also compiles with a RAW/empty constructor argument (inference falls back to Object), so this cannot
// introduce a new error. Tightly gated: (a) receiver is a non-array `new` of a whitelisted concrete JDK
// generic collection rendered RAW, (b) at least one argument of THIS call is a lambda/method reference.
// Returns the rendered receiver string, or "" to keep the legacy raw rendering. Kill-switch:
// JDEC_NEW_RECV_DIAMOND_OFF=1.
func (f *FunctionCallExpression) newRecvJDKGenericDiamond(ne *NewExpression, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_NEW_RECV_DIAMOND_OFF") != "" || ne == nil || ne.IsArray() {
		return ""
	}
	hasLambdaArg := false
	for _, a := range f.Arguments {
		if cv, ok := UnpackSoltValue(a).(*CustomValue); ok && cv.Flag == "lambda" {
			hasLambdaArg = true
			break
		}
	}
	if !hasLambdaArg {
		return ""
	}
	jc, ok := ne.JavaType.RawType().(*types.JavaClass)
	if !ok || jc == nil || !jdkGenericCtorDiamondClasses[jc.Name] {
		return ""
	}
	s := ne.String(funcCtx)
	// Only a RAW rendering (no explicit type arguments) gets the diamond; find the argument-list
	// paren and inject `<>` before it.
	idx := strings.IndexByte(s, '(')
	if idx <= 0 || strings.Contains(s[:idx], "<") {
		return ""
	}
	return s[:idx] + "<>" + s[idx:]
}

// lambdaArgRawJDKReceiverCast is the JDK-receiver companion of lambdaArgFunctionalCast. It re-adds the
// functional-interface cast a lambda / method-reference argument needs when the call's RECEIVER is a
// KNOWN JDK GENERIC type used RAW (java.util.stream.Stream / java.util.Optional). Such a receiver arises
// when its source was raw -- e.g. `rawList.stream().filter(...).map((FieldWriter l0) -> ...)` where
// rawList is a raw java.util.List (a cross-class getFieldWriters() whose List<FieldWriter> return the
// descriptor erased). Calling through the raw Stream erases Stream.map's `Function<? super T,? extends R>`
// to raw Function (SAM apply(Object)), so the explicit `(FieldWriter l0)` fails ("incompatible parameter
// types in lambda expression"). The original source's Stream WAS parameterized, so the invokedynamic
// recorded the lambda's instantiated functional type (Function<FieldWriter, Object>) -- re-emit that cast,
// which binds the explicit parameter and stays assignable to the raw Function parameter (unchecked,
// legal). Canonical: fastjson2 JSONPathSegment$CycleNameSegment$MapRecursive. This is a pure re-add of a
// cast the source had; it cannot introduce a type the descriptor did not already permit. Kill-switch:
// JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF=1.
func (f *FunctionCallExpression) lambdaArgRawJDKReceiverCast(i int, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF") != "" || f.IsStatic || f.Object == nil {
		return ""
	}
	// (a) argument is a lambda / method reference (both carry Flag "lambda").
	cv, ok := UnpackSoltValue(f.Arguments[i]).(*CustomValue)
	if !ok || cv.Flag != "lambda" {
		return ""
	}
	// A method reference binds NATURALLY to the raw SAM (it has no explicit parameter types), so
	// the parameterized-FI cast is unnecessary for it AND, for SAMs with nested wildcards (Stream.
	// flatMap's `Function<? super T, ? extends Stream<? extends R>>`), the cast pins a concrete
	// parameterization that defeats javac poly inference ("method flatMap cannot be applied").
	// Only an explicitly-typed lambda body needs the cast to bind. Skip method references.
	// (fastjson2 ObjectReaderCreator.toFieldReaderArray `flatMap(Collection::stream)`.)
	if cv.IsMethodRef {
		return ""
	}
	// (b) the lambda value carries a denotable parameterized functional-interface type.
	lamType := cv.Type()
	if lamType == nil {
		return ""
	}
	if _, ok := types.AsParameterizedType(lamType); !ok {
		return ""
	}
	// (c) receiver is a KNOWN JDK generic type used RAW. A parameterized receiver keeps the method's
	// functional-interface parameter intact, so no cast is needed (and must not be added) there.
	recv := f.Object.Type()
	if recv == nil {
		return ""
	}
	if _, ok := types.AsParameterizedType(recv); ok {
		return ""
	}
	jc, ok := recv.RawType().(*types.JavaClass)
	if !ok || jc == nil || !jdkGenericLambdaReceivers[jc.Name] {
		return ""
	}
	return lamType.String(funcCtx)
}

// doPrivilegedFunctionalCast returns the functional-interface cast a lambda / method-reference argument
// needs when passed to java.security.AccessController.doPrivileged. That JDK method is OVERLOADED on two
// SAM parameter types -- PrivilegedAction<T> and PrivilegedExceptionAction<T> -- and a bare lambda /
// method reference (a poly expression) is applicable to BOTH, so javac rejects the bare form as
// "reference to doPrivileged is ambiguous". The bytecode's invokedynamic result type records EXACTLY
// which functional interface the source targeted (recovered onto the argument's CustomValue.Type()), so
// re-emit the source's `(PrivilegedAction) ...` cast to select that same overload. The doPrivileged
// result is always outer-cast at the call site (e.g. `(ProtectionDomain)(AccessController.doPrivileged(
// ...))`), so even a raw functional-interface cast is faithful. Canonical: fastjson2 DynamicClassLoader
// / JSONFactory static initializers. Kill-switch: JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF=1.
func (f *FunctionCallExpression) doPrivilegedFunctionalCast(i int, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF") != "" {
		return ""
	}
	if f.FunctionName != "doPrivileged" || f.ClassName != "java.security.AccessController" {
		return ""
	}
	// Argument is a lambda / method reference (both carry Flag "lambda"); a plain value is unambiguous.
	cv, ok := UnpackSoltValue(f.Arguments[i]).(*CustomValue)
	if !ok || cv.Flag != "lambda" {
		return ""
	}
	// Cast to the exact functional interface recovered from the invokedynamic result type.
	lamType := cv.Type()
	if lamType == nil {
		return ""
	}
	if s := lamType.String(funcCtx); s != "" {
		return s
	}
	return ""
}

// LambdaAssignFunctionalCast returns the parameterized functional-interface cast an explicitly-typed
// lambda or a method-reference right-hand side needs when REASSIGNED into a slot that is declared with
// the RAW form of that functional interface. A raw target's SAM erases to `apply(Object)` (etc.), so an
// explicitly-typed lambda `(Collection l0) -> ...` or a method reference `Collections::unmodifiable*`
// cannot bind ("incompatible parameter types in lambda expression" / "invalid method reference"). This
// arises when the slot was FIRST declared from a RAW-typed source -- a getfield of a raw generic field
// such as `Function builder = this.builder;` -- and only LATER reassigned the lambda; bootstrap_methods
// upgrades the lambda VALUE's type and a slot whose FIRST store is the lambda adopts it, but not this
// field-first shape. The javac-visible source ALWAYS spells these casts out (verified against fastjson2
// 2.0.43 ObjectReaderImplList: `builder = (Function<Collection, Collection>) Collections::unmodifiable*`
// and `builder = (Function<Collection, Collection>) ((Collection list) -> Collections.singleton(...))`).
//
// The cast target is the lambda's OWN recovered instantiated type (from the invokedynamic
// instantiatedMethodType), which is exactly the type the original source cast to, so the explicit
// parameters / method reference bind and the value stays assignable to the raw slot (unchecked, legal).
// inferLambdaTypeFromInstantiated only ever parameterizes standard JDK functional interfaces, so the
// recovered type is denotable. Fires only when at least one recovered type argument is more specific
// than java.lang.Object (a `Function<Object,Object>`-shaped lambda already binds to the raw SAM and must
// be left untouched to avoid pointless casts). Applied by AssignStatement only on REASSIGNMENT, never on
// the first declaration (which adopts the parameterized type directly). Kill-switch:
// JDEC_LAMBDA_ASSIGN_CAST_OFF=1.
func LambdaAssignFunctionalCast(left, right JavaValue, funcCtx *class_context.ClassContext) string {
	if os.Getenv("JDEC_LAMBDA_ASSIGN_CAST_OFF") != "" || right == nil {
		return ""
	}
	cv, ok := UnpackSoltValue(right).(*CustomValue)
	if !ok || cv.Flag != "lambda" {
		return ""
	}
	lamType := cv.Type()
	if lamType == nil {
		return ""
	}
	lamParam, ok := types.AsParameterizedType(lamType)
	if !ok {
		return ""
	}
	// Require at least one type argument more specific than java.lang.Object: an all-Object
	// parameterization erases to the raw SAM and needs no cast.
	hasSpecificArg := false
	for _, ta := range lamParam.TypeArgs {
		if ta == nil {
			continue
		}
		if jc, ok := ta.RawType().(*types.JavaClass); ok && jc != nil && jc.Name == "java.lang.Object" {
			continue
		}
		hasSpecificArg = true
		break
	}
	if !hasSpecificArg {
		return ""
	}
	return lamType.String(funcCtx)
}

// renderArgAt renders the i-th call argument, applying the generic-erasure parameter-type recovery and
// the synthesized argument cast (`(V)`/`(T)`/primitive) that reproduce the original source. Factored
// out of ArgumentStrings so the varargs-spread path can reuse it for the leading fixed arguments.
func (f *FunctionCallExpression) renderArgAt(i int, funcCtx *class_context.ClassContext) string {
	arg := f.Arguments[i]
	// A lambda / method-ref passed to a method on a RAW generic receiver lost the source's
	// `(Consumer<FieldReader>) e -> ...` functional-interface cast (raw receiver erases the SAM). Re-add it.
	if cast := f.lambdaArgFunctionalCast(i, funcCtx); cast != "" {
		return fmt.Sprintf("(%s)(%s)", cast, arg.String(funcCtx))
	}
	// A lambda / method-ref passed to a fluent method on a RAW JDK generic receiver (Stream/Optional)
	// lost the source's functional-interface cast (the raw receiver erased the SAM). Re-add it.
	if cast := f.lambdaArgRawJDKReceiverCast(i, funcCtx); cast != "" {
		return fmt.Sprintf("(%s)(%s)", cast, arg.String(funcCtx))
	}
	// A lambda / method-ref passed to the OVERLOADED java.security.AccessController.doPrivileged is
	// ambiguous (PrivilegedAction vs PrivilegedExceptionAction). Re-add the source's FI cast.
	if cast := f.doPrivilegedFunctionalCast(i, funcCtx); cast != "" {
		return fmt.Sprintf("(%s)(%s)", cast, arg.String(funcCtx))
	}
	// A same-class `this(...)` constructor self-call whose i-th formal is a wildcard parameterization
	// mentioning a class type variable, fed an argument that erases to the same raw type but a different
	// parameterization, lost the source's unchecked cast to generic erasure -- re-add it (gson
	// LinkedTreeMap / LinkedHashTreeMap `this((Comparator<? super K>) NATURAL_ORDER)`).
	if cast := f.ctorWildcardArgCast(i, funcCtx); cast != "" {
		return fmt.Sprintf("(%s)(%s)", cast, arg.String(funcCtx))
	}
	// A same-class `this(...)` constructor self-call whose i-th formal is a BARE class type variable
	// (`T`), fed a value erased to the bound (`new Object()`), lost the source's unchecked `(T)` cast --
	// re-add it (spring PropertySource `this(name, (T) new Object())`).
	if cast := f.thisCtorTypeVarArgCast(i, funcCtx); cast != "" {
		return fmt.Sprintf("(%s)(%s)", cast, arg.String(funcCtx))
	}
	// A `Comparator<? super K>`-typed argument passed to a JDK sort/search static (Arrays.sort /
	// Arrays.binarySearch / Collections.sort / Collections.binarySearch) alongside a companion array/
	// list argument whose element type erased to Object (the source's `(K[])` cast became `(Object[])`)
	// no longer satisfies the method's `Comparator<? super T>` capture -- javac rejects it
	// ("no suitable method for sort(Object[], Comparator<CAP#1>)"; guava ImmutableSortedMap$Builder /
	// ImmutableSortedMultiset$Builder / ImmutableList). A raw `(Comparator)` cast makes the call an
	// unchecked (behaviour-preserving) invocation that resolves cleanly. Re-add it.
	if cast := f.comparatorRawArgCast(i, funcCtx); cast != "" {
		return fmt.Sprintf("(%s)(%s)", cast, arg.String(funcCtx))
	}
	// A `super(...)` argument feeding a bare type-variable parameter of the superclass constructor lost
	// the source's implicit type-variable typing (the value erased to Object/the bound); re-emit the
	// erased `(N)` cast recovered from the super ctor Signature + the subclass's extends clause.
	if cast := f.superCtorTypeVarArgCast(i, funcCtx); cast != "" {
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
	} else if inst := f.genericMethodWitnessArgParamType(i, funcCtx); inst != nil {
		// Same-class generic method whose formal is a METHOD-scope type variable inferred from a witness
		// argument (guava connectionsOf/reachableNodes `(Graph<N>, N)` fed an Object read from a raw
		// iterator): re-emit the `(N)` cast the erasure dropped.
		argType = inst
		resolvedGeneric = true
	} else if inst := f.varargsTypeVarArrayArgParamType(i, funcCtx); inst != nil {
		// Generic method whose trailing formal is a varargs type-var array `E...` fed a bare Object[]
		// (guava ImmutableSortedSet.construct `(Comparator<? super E>, int, E...)`): re-emit `(E[])`.
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
			!(!resolvedGeneric && jdkCalleeParamIsErasedTypeVar(f.ClassName, f.FunctionName, i, len(f.Arguments), argType)) &&
			!classLiteralArgToClassParam(arg, expectClassType) {
			argStr := arg.String(funcCtx)
			argTypeStr := argType.String(funcCtx)
			arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
			}, func() types.JavaType {
				return argType
			})
		}
	} else if resolvedParameterizedArgCast(funcCtx, argType, resolvedGeneric, arg) {
		// A generic resolver recovered the formal as a PARAMETERIZED type (e.g.
		// NavigableMap<Cut<C>,Range<C>>.tailMap's key formal -> `Cut<C>`) whose raw class differs from
		// the argument's erased static type. The (ok1 && ok2) class-vs-class branch never fires because a
		// parameterized type's RawType() is not a *JavaClass. Re-emit the erased `(Cut<C>)` cast (guava
		// TreeRangeSet$RangesByUpperBound `this.rangesByLowerBound.tailMap((Cut<C>) var2.getKey(), true)`).
		argStr := arg.String(funcCtx)
		argTypeStr := argType.String(funcCtx)
		arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
		}, func() types.JavaType {
			return argType
		})
	} else if arrayParamRefArgCast(argType, arg) {
		// The parameter is an ARRAY type (`byte[]`) but the argument's static type is a non-array
		// reference class (`Object`) -- the (ok1 && ok2) class-vs-class branch never fires because an
		// array type's RawType() is *JavaArrayType, not *JavaClass. A non-array reference is not
		// assignable to an array parameter in source, so javac rejects `foo(objVar)` ("Object cannot be
		// converted to byte[]"). This is the shape of a null-initialized local typed Object that is only
		// ever passed to a typed array parameter (spring ASM Attribute.computeAttributesSize/putAttributes
		// `Object cattrs = null; ...computeAttributesSize(..., cattrs, ...)` where the overload takes
		// `byte[]`). The value already flowed into the array parameter in bytecode (a checkcast or a
		// null), so an explicit `(byte[])` cast is behaviour-preserving. Kill-switch:
		// JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF=1.
		argStr := arg.String(funcCtx)
		argTypeStr := argType.String(funcCtx)
		arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
		}, func() types.JavaType {
			return argType
		})
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
	} else if f.typeVarElemArrayArgCast(argType, resolvedGeneric, arg, funcCtx) {
		// The resolver recovered the varargs formal as a denotable type-var array `E[]` but the ARGUMENT
		// is a reference array of a different element type (Object[]); JavaArrayType.RawType() is not a
		// *JavaClass so the class-vs-class branch above never fires. Re-emit the erased `(E[])` cast.
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
		// Self-parenthesize the polymorphic-signature return cast, exactly like OP_CHECKCAST does, so it
		// keeps correct precedence when it becomes the receiver of a member access / method call. Without
		// the outer parens `(T)(x).m()` parses as `(T)((x).m())` -- a cast binds LOOSER than a call -- so
		// `(Boolean)(mh.invoke(...)).booleanValue()` tries to call booleanValue() on the (Object) invoke
		// result ("cannot find symbol: method booleanValue()"; fastjson2 JSONReader:3130
		// `METHOD_HANDLE_HAS_NEGATIVE.invoke(...)` unboxed to boolean). Extra parens are valid Java in
		// every position, so this never harms assignment/argument uses. Kill-switch:
		// JDEC_POLYSIG_CAST_PARENS_OFF restores the single-paren form.
		if os.Getenv("JDEC_POLYSIG_CAST_PARENS_OFF") != "" {
			return fmt.Sprintf("(%s)(%s)", castType, f.renderCall(funcCtx))
		}
		return fmt.Sprintf("((%s)(%s))", castType, f.renderCall(funcCtx))
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
			// A `super.m()` invokespecial whose target is a directly-implemented INTERFACE (not the
			// superclass) is an interface-default super call, which Java spells `Iface.super.m()`. A bare
			// `super.m()` dispatches to the SUPERCLASS, which does not declare the interface's default
			// method -> "cannot find symbol" (spring StandardAnnotationMetadata/StandardMethodMetadata/
			// SimpleAnnotationMetadata `super.getAnnotationTypes()` etc., where getAnnotationTypes is an
			// AnnotationMetadata default and the superclass is StandardClassMetadata).
			if q := f.interfaceDefaultSuperQualifier(funcCtx); q != "" {
				return fmt.Sprintf("%s.super.%s(%s)", q, functionName, strings.Join(paramStrs, ","))
			}
			return fmt.Sprintf("super.%s(%s)", functionName, strings.Join(paramStrs, ","))
		}
	}

	// A `recv.compare(a, b)` call whose receiver is a wildcard `Comparator<?>`/`Comparator<? super X>`
	// binds to `compare(CAP, CAP)` and rejects the Object arguments; render a raw `((Comparator)(recv))`
	// receiver cast so it becomes an unchecked, behaviour-preserving `compare(Object, Object)`.
	if f.comparatorRawReceiverCast(funcCtx) {
		return fmt.Sprintf("((%s)(%s)).%s(%s)", types.NewJavaClass("java.util.Comparator").String(funcCtx), f.Object.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	}
	// A `recv.add(x)`/`recv.offer(x)` on a wildcard `Collection<? super E>` receiver rejects the Object
	// argument; render a raw `((Collection)(recv))` receiver cast so it becomes an unchecked add(Object).
	if rawCls := f.collectionAddWildcardReceiverRawCast(funcCtx); rawCls != "" {
		return fmt.Sprintf("((%s)(%s)).%s(%s)", types.NewJavaClass(rawCls).String(funcCtx), f.Object.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	}
	// A `recv.apply(x)`/`test(x)`/`accept(x)` on a wildcard single-type-param consumer (e.g.
	// `Predicate<? super Entry<K,V>>`) rejects the erased argument; render a raw `((Predicate)(recv))`
	// receiver cast so it becomes an unchecked, behaviour-preserving call.
	if rawCls := f.wildcardConsumerReceiverRawCast(funcCtx); rawCls != "" {
		return fmt.Sprintf("((%s)(%s)).%s(%s)", types.NewJavaClass(rawCls).String(funcCtx), f.Object.String(funcCtx), functionName, strings.Join(paramStrs, ","))
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
	// A RAW `new HashMap(typedMap)` used directly as the receiver of a lambda-taking call erases the
	// method's functional-interface parameter (raw receiver, JLS 4.8), so the lambda's parameters
	// degrade to Object and a body dereferencing them fails ("Object cannot be converted to String",
	// spring SimpleAliasRegistry `new HashMap(this.aliasMap).forEach((l0, l1) -> ...)`). Restore the
	// source's diamond so javac re-infers the type arguments from the constructor argument.
	if ne, ok := obj.(*NewExpression); ok {
		if s := f.newRecvJDKGenericDiamond(ne, funcCtx); s != "" {
			return fmt.Sprintf("%s.%s(%s)", s, functionName, strings.Join(paramStrs, ","))
		}
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

// structurallyBooleanForIntCoerce reports whether v produces a boolean 0/1 BY CONSTRUCTION -- a
// comparison (`a == b`), a boolean-typed connective/unary expression (`a || b`, `a && b`, `!a`, folded
// from a short-circuit materialization), or a call to a method whose descriptor return type is boolean
// (`Z`) -- as opposed to a bare variable/slot reference. The distinction is critical: a bare boolean-
// typed ref may be a slot MISTYPED boolean while actually holding an arbitrary int (LocalCache$Segment
// `this.count = var13`), where wrapping `? 1 : 0` would corrupt any non-0/1 value; but a comparison /
// boolean expression / Z-returning method GENUINELY yields 0/1, so `int x = <it>` was necessarily
// `x = <it> ? 1 : 0` in source (or x should be boolean) and re-inserting `? 1 : 0` is behaviourally
// identical. The stored value at `istore` is minted int during the linear pass; the boolean structure
// (short-circuit `||`, comparison) is reconstructed by later CFG structuring, leaving `int x = <bool>`
// which javac rejects (fastjson2 FieldWriterObject.getObjectWriter: `int typeMatch = a==b || ...` and
// `typeMatch = TypeUtils.typeMatch(..)` returning Z).
func structurallyBooleanForIntCoerce(v JavaValue, funcCtx *class_context.ClassContext) bool {
	u := UnpackSoltValue(v)
	if u == nil {
		return false
	}
	switch t := u.(type) {
	case *JavaCompare:
		return true
	case *JavaExpression, *FunctionCallExpression:
		return isBooleanTyped(u)
	case *TernaryExpression:
		// A short-circuit `||`/`&&` materialization is carried as a `(cond) ? 1 : 0` ternary tree whose
		// int-0/1 leaves are recursively foldable to a boolean connective (boolReduce). When it folds to
		// a boolean value it is boolean by construction (not a bare ref), so `? 1 : 0` is safe. A genuine
		// value ternary (`cond ? a : b` over non-bool values) does NOT fold and is left alone.
		return isBooleanTyped(boolReduce(t, funcCtx))
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
//
// The structural extension (short-circuit `||`/`&&` / comparison / Z-returning method call, gated by
// JDEC_BOOL_TO_INT_COERCE_EXPR_OFF) covers the same defect on values whose boolean-ness is by
// CONSTRUCTION rather than a bare typed ref -- see structurallyBooleanForIntCoerce. Both exclude bare
// refs, so the mistyped-boolean-int-slot miscompilation risk is avoided.
func CoerceIntAssignRHS(leftType types.JavaType, rhs JavaValue, funcCtx *class_context.ClassContext) JavaValue {
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
	eligible := IntrinsicBooleanValue(rhs)
	if !eligible && os.Getenv("JDEC_BOOL_TO_INT_COERCE_EXPR_OFF") == "" {
		eligible = structurallyBooleanForIntCoerce(rhs, funcCtx)
	}
	if !eligible {
		return rhs
	}
	inner := rhs
	return NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return fmt.Sprintf("(%s) ? (1) : (0)", inner.String(funcCtx))
	}, func() types.JavaType {
		return types.NewJavaPrimer(types.JavaInteger)
	})
}

// CoerceBooleanAssignRHS is the INVERSE of CoerceIntAssignRHS: when a boolean-typed target receives an
// int-materialized RHS (the JVM has no boolean storage, so javac emits a boolean value as iconst_0/1 or
// a `cond ? 1 : 0` diamond), Java forbids the implicit int->boolean conversion, so `boolArr[i] = <int>`
// / `boolField = cond ? 1 : 0` fail to recompile ("int cannot be converted to boolean"). This retypes
// the 0/1 leaves to boolean (coerceBooleanArgument: literal 0/1 -> false/true, ternary arms recursively,
// any other int expr -> `(expr) != (0)`) and then folds the resulting boolean diamond into an idiomatic
// connective (boolReduce: `cond ? true : false` -> `cond`). Values already typed boolean (comparisons,
// predicate calls, boolean refs) are returned untouched. Only fires for a boolean leftType and an
// int-typed rhs. Kill-switch shared with the other bool<->int coercion: JDEC_BOOL_TO_INT_COERCE_OFF.
func CoerceBooleanAssignRHS(leftType types.JavaType, rhs JavaValue, funcCtx *class_context.ClassContext) JavaValue {
	if rhs == nil || leftType == nil {
		return rhs
	}
	if os.Getenv("JDEC_BOOL_TO_INT_COERCE_OFF") == "1" {
		return rhs
	}
	prim, ok := leftType.RawType().(*types.JavaPrimer)
	if !ok || prim.Name != types.JavaBoolean {
		return rhs
	}
	// Only an int-typed rhs is a mistyped boolean; a value already boolean needs nothing.
	rt := rhs.Type()
	if rt == nil {
		return rhs
	}
	rprim, ok := rt.RawType().(*types.JavaPrimer)
	if !ok || rprim.Name != types.JavaInteger {
		return rhs
	}
	return boolReduce(coerceBooleanArgument(rhs), funcCtx)
}

func NewFunctionCallExpression(object JavaValue, methodMember *JavaClassMember, funcType *types.JavaFuncType) *FunctionCallExpression {
	return &FunctionCallExpression{
		FuncType:     funcType,
		Object:       object,
		FunctionName: methodMember.Member,
		ClassName:    methodMember.Name,
		Descriptor:   methodMember.Description,
	}
}
