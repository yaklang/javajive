package core

import (
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func init() {
	SetFamilyAdapter(FamilyConcat, t18ConcatAdapter)
}

// t18RecipePart is one token of a StringConcatFactory recipe.
// kind 0 = ordinary literal chunk (raw UTF-16/Go-string data), 1 = TAG_ARG, 2 = TAG_CONST.
type t18RecipePart struct {
	kind byte
	lit  string
}

func t18ConcatAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	if err := validateConcatRequest(req); err != nil {
		return invalidDispatch(req, FamilyConcat, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	// Production normally passes dedicated operand temps. Constructors are the
	// exception: Java forbids statements before this()/super(), so safe operands
	// must remain in the argument expression. A String or primitive conversion is
	// inert, and Java + evaluates those operands left-to-right. Object conversion
	// may call toString(), so keep rejecting any unsnapshotted effectful concat
	// unless every dynamic operand has an inert conversion.
	if len(req.DynamicArgs) > 1 {
		hasEffects := false
		for _, arg := range req.DynamicArgs {
			if !values.IsPure(arg) {
				hasEffects = true
				break
			}
		}
		if hasEffects {
			for _, arg := range req.DynamicArgs {
				if !t18ConversionIsInert(arg) {
					return unsupportedDispatch(req, FamilyConcat, DiagBootstrapUnknown, "concat operands require materialized evaluation snapshots", resultType)
				}
			}
		}
	}
	var (
		val values.JavaValue
		err error
	)
	switch req.Identity.Name {
	case "makeConcat":
		val, err = t18BuildMakeConcat(req, resultType)
	case "makeConcatWithConstants":
		val, err = t18BuildMakeConcatWithConstants(req, resultType)
	default:
		return unsupportedDispatch(req, FamilyConcat, DiagBootstrapUnknown, "unknown concat member "+req.Identity.Name, resultType)
	}
	if err != nil {
		return invalidDispatch(req, FamilyConcat, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	return okDispatch(req, FamilyConcat, val, values.EffectCall)
}

func t18BuildMakeConcat(req CallSiteRequest, resultType types.JavaType) (values.JavaValue, error) {
	dyn := t18SnapshotArgs(req.DynamicArgs)
	eval := t18EvalOrder(dyn)
	return t18ConcatValue(eval, nil, resultType), nil
}

func t18BuildMakeConcatWithConstants(req CallSiteRequest, resultType types.JavaType) (values.JavaValue, error) {
	units, ok := LiteralStringUnits(req.StaticArgs[0])
	if !ok {
		return nil, fmt.Errorf("makeConcatWithConstants recipe is not a string constant (tag error)")
	}
	parts := t18TokenizeRecipeUnits(units)
	dyn := t18SnapshotArgs(req.DynamicArgs)
	eval := t18EvalOrder(dyn)
	consts := t18SnapshotArgs(req.StaticArgs[1:])

	argI, constI := 0, 0
	operands := make([]values.JavaValue, 0, len(parts))
	lits := make([]string, 0, len(parts))
	kinds := make([]byte, 0, len(parts))
	for _, p := range parts {
		switch p.kind {
		case 0:
			operands = append(operands, nil)
			lits = append(lits, p.lit)
			kinds = append(kinds, 0)
		case 1:
			if argI >= len(eval) {
				return nil, fmt.Errorf("recipe TAG_ARG leftover/missing: consumed %d of %d dynamic args", argI, len(eval))
			}
			operands = append(operands, eval[argI])
			lits = append(lits, "")
			kinds = append(kinds, 1)
			argI++
		case 2:
			if constI >= len(consts) {
				return nil, fmt.Errorf("recipe TAG_CONST leftover/missing: consumed %d of %d static constants", constI, len(consts))
			}
			operands = append(operands, consts[constI])
			lits = append(lits, "")
			kinds = append(kinds, 2)
			constI++
		}
	}
	if argI != len(eval) {
		return nil, fmt.Errorf("recipe TAG_ARG leftover/missing: consumed %d of %d dynamic args (trailing args not dropped)", argI, len(eval))
	}
	if constI != len(consts) {
		return nil, fmt.Errorf("recipe TAG_CONST leftover/missing: consumed %d of %d static constants", constI, len(consts))
	}
	tracked := append([]values.JavaValue{}, eval...)
	tracked = append(tracked, consts...)
	return t18ConcatValueFromParts(kinds, lits, operands, tracked, resultType), nil
}

// t18TokenizeRecipe walks the RAW recipe string (JavaLiteral.Data), not printer output.
// U+0001/U+0002 are tags only as recipe units; data copies of those chars live in TAG_CONST
// payloads / dynamic string args and are never re-scanned as tags.
func t18TokenizeRecipe(recipe string) []t18RecipePart {
	return t18TokenizeRecipeUnits(utf16.Encode([]rune(recipe)))
}

func t18TokenizeRecipeUnits(units []uint16) []t18RecipePart {
	var parts []t18RecipePart
	var buf []uint16
	flush := func() {
		if len(buf) == 0 {
			return
		}
		parts = append(parts, t18RecipePart{kind: 0, lit: string(utf16.Decode(buf))})
		buf = buf[:0]
	}
	for _, u := range units {
		switch u {
		case 1:
			flush()
			parts = append(parts, t18RecipePart{kind: 1})
		case 2:
			flush()
			parts = append(parts, t18RecipePart{kind: 2})
		default:
			buf = append(buf, u)
		}
	}
	flush()
	return parts
}

func t18SnapshotArgs(args []values.JavaValue) []values.JavaValue {
	out := make([]values.JavaValue, len(args))
	for i, a := range args {
		out[i] = t18Snapshot(a)
	}
	return out
}

func t18Snapshot(v values.JavaValue) values.JavaValue {
	return values.UnpackSoltValue(v)
}

// t18EvalOrder restores left-to-right evaluation from stack-pop order (last param first).
func t18EvalOrder(pop []values.JavaValue) []values.JavaValue {
	out := make([]values.JavaValue, len(pop))
	for i := range pop {
		out[i] = pop[len(pop)-1-i]
	}
	return out
}

func t18ConcatValue(eval []values.JavaValue, extra []values.JavaValue, resultType types.JavaType) values.JavaValue {
	kinds := make([]byte, len(eval))
	lits := make([]string, len(eval))
	ops := make([]values.JavaValue, len(eval))
	for i, v := range eval {
		kinds[i] = 1
		ops[i] = v
	}
	tracked := append([]values.JavaValue{}, eval...)
	tracked = append(tracked, extra...)
	return t18ConcatValueFromParts(kinds, lits, ops, tracked, resultType)
}

func t18ConcatValueFromParts(kinds []byte, lits []string, ops []values.JavaValue, tracked []values.JavaValue, resultType types.JavaType) values.JavaValue {
	typ := resultType
	cv := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return t18RenderParts(kinds, lits, ops, funcCtx)
	}, func() types.JavaType {
		if typ == nil {
			return types.NewJavaPrimer(types.JavaString)
		}
		return typ
	}, func(oldId *utils.VariableId, newId *utils.VariableId) {
		for _, a := range tracked {
			if a != nil {
				a.ReplaceVar(oldId, newId)
			}
		}
		for _, a := range ops {
			if a != nil {
				a.ReplaceVar(oldId, newId)
			}
		}
	})
	cv.Flag = "concat"
	cv.CapturesKnown = true
	cv.Captures = tracked
	return cv
}

func t18RenderParts(kinds []byte, lits []string, ops []values.JavaValue, funcCtx *class_context.ClassContext) string {
	if len(kinds) == 0 {
		return `""`
	}
	rendered := make([]string, 0, len(kinds)+1)
	var firstVal values.JavaValue
	firstSet := false
	for i, k := range kinds {
		var piece string
		var val values.JavaValue
		switch k {
		case 0:
			piece = values.JavaUnitsToStringLiteral(utf16.Encode([]rune(lits[i])))
			val = values.NewJavaLiteral(lits[i], types.NewJavaPrimer(types.JavaString))
		case 1, 2:
			val = ops[i]
			if k == 2 {
				piece = t18RenderConst(val, funcCtx)
			} else {
				piece = t18RenderOperand(val, funcCtx)
			}
		}
		if funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.Err() != nil {
			return ""
		}
		if piece == "" {
			continue
		}
		if !firstSet {
			firstVal = val
			firstSet = true
		}
		rendered = append(rendered, piece)
	}
	if len(rendered) == 0 {
		return `""`
	}
	seed := t18NeedsStringSeed(firstVal, rendered[0])
	if funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.RenderGuarded() {
		w := workbudget.NewWriter(funcCtx.Work)
		w.SetBase(funcCtx.OutputHeld)
		if seed {
			if err := w.WriteString(`"" + `); err != nil {
				return ""
			}
		}
		for i, piece := range rendered {
			if i > 0 {
				if err := w.WriteString(" + "); err != nil {
					return ""
				}
			}
			if err := w.WriteString(piece); err != nil {
				return ""
			}
		}
		return w.String()
	}
	out := rendered[0]
	for i := 1; i < len(rendered); i++ {
		out = out + " + " + rendered[i]
	}
	if seed {
		out = `"" + ` + out
	}
	return out
}

func t18RenderConst(v values.JavaValue, funcCtx *class_context.ClassContext) string {
	v = values.UnpackSoltValue(v)
	if units, ok := LiteralStringUnits(v); ok {
		return values.JavaUnitsToStringLiteral(units)
	}
	return t18RenderOperand(v, funcCtx)
}

// t18RenderOperand prints a concat operand. Null literals become `(Object)null` so javac
// cannot bind String.valueOf(char[]). Locals render by variable id (not StackVar), so a
// later slot reuse / null fold cannot turn `o` into a bare `null`.
func t18RenderOperand(v values.JavaValue, funcCtx *class_context.ClassContext) string {
	v = values.UnpackSoltValue(v)
	if fc, ok := v.(*values.FunctionCallExpression); ok && t18IsStringValueOfObject(fc) {
		return t18RenderOperand(fc.Arguments[0], funcCtx)
	}
	if t18IsBareNull(v) {
		return "(Object)null"
	}
	if ref, ok := v.(*values.JavaRef); ok {
		return t18RenderRef(ref, funcCtx)
	}
	s := ""
	if v != nil {
		s = v.String(funcCtx)
	} else {
		s = "(Object)null"
	}
	if t18ConcatArgNeedsParens(v) {
		return "(" + s + ")"
	}
	return s
}

func t18RenderRef(ref *values.JavaRef, funcCtx *class_context.ClassContext) string {
	if ref == nil {
		return "(Object)null"
	}
	if ref.IsThis {
		return "this"
	}
	if ref.CustomValue != nil {
		return ref.CustomValue.String(funcCtx)
	}
	if ref.Id != nil {
		return ref.Id.String()
	}
	return ref.String(funcCtx)
}

func t18IsBareNull(v values.JavaValue) bool {
	v = values.UnpackSoltValue(v)
	if v == nil {
		return true
	}
	if _, ok := v.(*values.JavaRef); ok {
		return false
	}
	if v == values.JavaNull {
		return true
	}
	return values.IsNullLiteral(v)
}

func t18IsStringValueOfObject(fc *values.FunctionCallExpression) bool {
	if fc == nil || fc.FunctionName != "valueOf" || !fc.IsStatic || len(fc.Arguments) != 1 {
		return false
	}
	cls := NormalizeBootstrapOwner(fc.ClassName)
	if cls != "java.lang.String" {
		return false
	}
	if fc.Descriptor == "([C)Ljava/lang/String;" {
		return false
	}
	if fc.Descriptor == "" && t18IsCharArray(fc.Arguments[0]) {
		return false
	}
	return true
}

func t18IsCharArray(v values.JavaValue) bool {
	if v == nil || v.Type() == nil {
		return false
	}
	t := v.Type()
	if !t.IsArray() || t.ArrayDim() != 1 {
		return false
	}
	elem := t.ElementType()
	if elem == nil {
		return false
	}
	p, ok := elem.RawType().(*types.JavaPrimer)
	return ok && p != nil && p.Name == types.JavaChar
}

func t18IsStringTyped(v values.JavaValue) bool {
	if v == nil || v.Type() == nil {
		return false
	}
	s := v.Type().String(&class_context.ClassContext{})
	return s == "String" || s == "java.lang.String"
}

func t18ConversionIsInert(v values.JavaValue) bool {
	v = values.UnpackSoltValue(v)
	if t18IsBareNull(v) || t18IsStringTyped(v) {
		return true
	}
	if v == nil || v.Type() == nil || v.Type().IsArray() {
		return false
	}
	p, ok := v.Type().RawType().(*types.JavaPrimer)
	return ok && p != nil && p.Name != types.JavaString && p.Name != types.JavaVoid
}

func t18NeedsStringSeed(first values.JavaValue, firstRendered string) bool {
	trim := strings.TrimSpace(firstRendered)
	if strings.HasPrefix(trim, `"`) || strings.HasPrefix(trim, `""`) {
		return false
	}
	if t18IsStringTyped(first) {
		return false
	}
	return true
}

// t18ConcatArgNeedsParens reimplements the bootstrap_methods.go hint: bitwise/shift/ternary
// (and any other binary expression) must be wrapped so surrounding `+` cannot steal operands.
func t18ConcatArgNeedsParens(v values.JavaValue) bool {
	switch e := values.UnpackSoltValue(v).(type) {
	case *values.JavaExpression:
		if e == nil {
			return false
		}
		if len(e.Values) == 2 {
			return true
		}
		switch e.Op {
		case values.SHL, values.SHR, values.USHR, values.AND, values.OR, values.XOR,
			values.ADD, values.SUB, values.MUL, values.DIV, values.REM,
			values.LOGICAL_AND, values.LOGICAL_OR:
			return true
		}
		return false
	case *values.TernaryExpression:
		return true
	default:
		return false
	}
}
