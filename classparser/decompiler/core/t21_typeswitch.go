package core

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func init() {
	SetFamilyAdapter(FamilyTypeSwitch, typeSwitchAdapter)
	SetFamilyAdapter(FamilyEnumSwitch, enumSwitchAdapter)
}

type TypeSwitchLabel struct {
	IsNull   bool
	TypeName string
}

func typeSwitchAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	labels, err := ParseTypeSwitchLabels(req.StaticArgs)
	if err != nil {
		return invalidDispatch(req, FamilyTypeSwitch, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	if ft, err := types.ParseMethodDescriptor(req.CallSiteDescriptor); err == nil && ft != nil && ft.FunctionType() != nil {
		params := ft.FunctionType().ParamTypes
		if len(params) != 2 {
			return invalidDispatch(req, FamilyTypeSwitch, DiagBootstrapArgMismatch,
				fmt.Sprintf("typeSwitch callsite must be (Object,int)I, got %d params", len(params)), resultType)
		}
	}
	selector, restart := typeSwitchOperands(req)
	return okDispatch(req, FamilyTypeSwitch, renderTypeSwitchIndex(selector, restart, labels, resultType), values.EffectCall|values.EffectReadMemory)
}

func enumSwitchAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	return unsupportedDispatch(req, FamilyEnumSwitch, DiagBootstrapUnknown, "enumSwitch reconstruction is not claimed lossless in T21", resultType)
}

func ParseTypeSwitchLabels(staticArgs []values.JavaValue) ([]TypeSwitchLabel, error) {
	out := make([]TypeSwitchLabel, 0, len(staticArgs))
	for i, arg := range staticArgs {
		if arg == nil || arg == values.JavaNull {
			out = append(out, TypeSwitchLabel{IsNull: true})
			continue
		}
		name := classNameFromValue(arg)
		if name == "" {
			if lit, ok := LiteralStringData(arg); ok && lit == "" {
				out = append(out, TypeSwitchLabel{IsNull: true})
				continue
			}
			return nil, fmt.Errorf("typeSwitch label %d is not a Class or null", i)
		}
		out = append(out, TypeSwitchLabel{TypeName: name})
	}
	return out, nil
}

func typeSwitchOperands(req CallSiteRequest) (selector, restart values.JavaValue) {
	n := len(req.DynamicArgs)
	if n == 0 {
		return nil, nil
	}
	if n == 1 {
		return req.DynamicArgs[0], nil
	}
	return req.DynamicArgs[n-1], req.DynamicArgs[0]
}

func renderTypeSwitchIndex(selector, restart values.JavaValue, labels []TypeSwitchLabel, resultType types.JavaType) values.JavaValue {
	typ := resultType
	return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		sel := "null"
		if selector != nil {
			sel = selector.String(funcCtx)
		}
		rst := "0"
		if restart != nil {
			rst = restart.String(funcCtx)
		}
		parts := []string{fmt.Sprintf("((%s) == null ? -1", sel)}
		for i, lab := range labels {
			if lab.IsNull {
				parts = append(parts, fmt.Sprintf("((%s) <= %d && (%s) == null) ? %d", rst, i, sel, i))
				continue
			}
			tn := lab.TypeName
			if funcCtx != nil {
				tn = funcCtx.ShortTypeName(strings.ReplaceAll(tn, "/", "."))
			}
			parts = append(parts, fmt.Sprintf("((%s) <= %d && (%s) instanceof %s) ? %d", rst, i, sel, tn, i))
		}
		parts = append(parts, "-1")
		return strings.Join(parts, " : ") + ")"
	}, func() types.JavaType {
		if typ == nil {
			return types.NewJavaPrimer(types.JavaInteger)
		}
		return typ
	})
}

// PatternSwitchRewrite is a reconstructed method body (without the outer braces).
type PatternSwitchRewrite struct {
	Body     string
	Complete bool
	Reason   string
}

type patternCase struct {
	isNull   bool
	isDef    bool
	typeName string
	bind     string
	guard    string
	body     string
}

// TryReconstructPatternSwitch recognizes the javac 21 typeSwitch restart loop and
// emits a Java 21 pattern switch. Complex/record-deconstruction shapes return ok=false.
func TryReconstructPatternSwitch(d *Decompiler) (PatternSwitchRewrite, bool) {
	out := PatternSwitchRewrite{}
	if d == nil {
		return out, false
	}
	if len(d.opCodes) == 0 {
		if err := d.ParseOpcode(); err != nil {
			out.Reason = err.Error()
			return out, false
		}
	}
	if patternHasMatchException(d) {
		out.Reason = "record/deconstruction pattern (MatchException)"
		return out, false
	}
	byOff := map[int]*OpCode{}
	var ops []*OpCode
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || op.Instr.OpCode == OP_START {
			continue
		}
		ops = append(ops, op)
		byOff[int(op.CurrentOffset)] = op
	}
	indyIdx := -1
	var labels []TypeSwitchLabel
	for i, op := range ops {
		if op.Instr.OpCode != OP_INVOKEDYNAMIC {
			continue
		}
		labs, isType := typeSwitchLabelsFromOp(d, op)
		if !isType {
			continue
		}
		if indyIdx >= 0 {
			out.Reason = "multiple typeSwitch sites"
			return out, false
		}
		indyIdx = i
		labels = labs
	}
	if indyIdx < 0 {
		out.Reason = "no typeSwitch"
		return out, false
	}
	if indyIdx+1 >= len(ops) {
		return out, false
	}
	sw := ops[indyIdx+1]
	if sw.Instr.OpCode != OP_TABLESWITCH && sw.Instr.OpCode != OP_LOOKUPSWITCH {
		out.Reason = "typeSwitch not followed by switch"
		return out, false
	}
	if indyIdx < 2 {
		return out, false
	}
	restartSlot := GetRetrieveIdx(ops[indyIdx-1])
	selectorSlot := GetRetrieveIdx(ops[indyIdx-2])
	if restartSlot < 0 || selectorSlot < 0 {
		out.Reason = "cannot recover selector/restart slots"
		return out, false
	}
	loopPC := int(ops[indyIdx-2].CurrentOffset)
	hasNullCase := false
	if sw.SwitchJmpCase != nil {
		if _, ok := sw.SwitchJmpCase.Get(-1); ok {
			hasNullCase = true
		}
	}
	mergePC := inferMergePC(sw, byOff)
	if mergePC < 0 {
		out.Reason = "cannot find switch merge"
		return out, false
	}

	selectorExpr, requireNonNull := recoverSelectorExpr(d, ops, selectorSlot, loopPC)
	if selectorExpr == "" {
		selectorExpr = localName(d, selectorSlot)
	}
	if requireNonNull && hasNullCase {
		// unusual; keep both
	}

	var cases []patternCase
	bindSeq := map[string]int{}
	if hasNullCase {
		target, _ := sw.SwitchJmpCase.Get(-1)
		body, ok := evalExprToGoto(d, byOff, int(target), mergePC, selectorSlot, nil)
		if !ok {
			out.Reason = "cannot recover null case body"
			return out, false
		}
		cases = append(cases, patternCase{isNull: true, body: body})
	}
	for i, lab := range labels {
		if lab.IsNull {
			out.Reason = "null type label combined with -1 encoding is unsupported"
			return out, false
		}
		target, ok := sw.SwitchJmpCase.Get(i)
		if !ok {
			out.Reason = fmt.Sprintf("missing switch case %d", i)
			return out, false
		}
		pc := patternCase{typeName: shortType(d, lab.TypeName)}
		pc.bind = nextBindName(pc.typeName, bindSeq)
		body, guard, decon, ok := recoverTypeCase(d, byOff, int(target), mergePC, loopPC, restartSlot, selectorSlot, pc.bind, i)
		if decon {
			out.Reason = "record deconstruction / nested pattern"
			return out, false
		}
		if !ok {
			out.Reason = fmt.Sprintf("cannot recover type case %d", i)
			return out, false
		}
		pc.body = body
		pc.guard = guard
		cases = append(cases, pc)
	}
	defOff := int(sw.SwitchDefaultOffset)
	if defBody, ok := evalExprToGoto(d, byOff, defOff, mergePC, selectorSlot, nil); ok {
		cases = append(cases, patternCase{isDef: true, body: defBody})
	} else {
		out.Reason = "cannot recover default body"
		return out, false
	}

	if !hasNullCase && !requireNonNull {
		// Null without case null still NPEs from javac's requireNonNull; if the
		// bytecode omitted it, keep original null semantics by not inventing case null.
	}

	var b strings.Builder
	b.WriteString("return switch (")
	b.WriteString(selectorExpr)
	b.WriteString(") {\n")
	for _, c := range cases {
		b.WriteString("\t\t")
		switch {
		case c.isNull:
			b.WriteString("case null -> ")
			b.WriteString(c.body)
			b.WriteString(";\n")
		case c.isDef:
			b.WriteString("default -> ")
			b.WriteString(c.body)
			b.WriteString(";\n")
		default:
			b.WriteString("case ")
			b.WriteString(c.typeName)
			b.WriteString(" ")
			b.WriteString(c.bind)
			if c.guard != "" {
				b.WriteString(" when ")
				b.WriteString(c.guard)
			}
			b.WriteString(" -> ")
			b.WriteString(c.body)
			b.WriteString(";\n")
		}
	}
	b.WriteString("\t};")
	out.Body = b.String()
	out.Complete = true
	return out, true
}

func patternHasMatchException(d *Decompiler) bool {
	for _, e := range d.ExceptionTable {
		if e == nil {
			continue
		}
		// Handler that constructs MatchException is recorded as catch_type of Throwable
		// plus later new MatchException; scan opcodes for MatchException class.
	}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			continue
		}
		if op.Instr.OpCode == OP_NEW || op.Instr.OpCode == OP_CHECKCAST || op.Instr.OpCode == OP_INSTANCEOF {
			if len(op.Data) < 2 || d.constantPoolGetter == nil {
				continue
			}
			v := d.constantPoolGetter(int(Convert2bytesToInt(op.Data)))
			if cv, ok := v.(*values.JavaClassValue); ok && cv != nil && cv.JavaType != nil {
				n := strings.ReplaceAll(cv.JavaType.String(&class_context.ClassContext{}), "/", ".")
				if strings.Contains(n, "MatchException") {
					return true
				}
			}
		}
		if op.Instr.OpCode == OP_INVOKESPECIAL && d.constantPoolGetter != nil && len(op.Data) >= 2 {
			v := d.constantPoolGetter(int(Convert2bytesToInt(op.Data)))
			if m, ok := v.(*values.JavaClassMember); ok && m != nil {
				if strings.Contains(strings.ReplaceAll(m.Name, "/", "."), "MatchException") {
					return true
				}
			}
		}
	}
	return false
}

func typeSwitchLabelsFromOp(d *Decompiler, op *OpCode) ([]TypeSwitchLabel, bool) {
	if d.ConstantPoolInvokeDynamicInfo == nil || len(op.Data) < 2 {
		return nil, false
	}
	index, name, _ := d.ConstantPoolInvokeDynamicInfo(int(Convert2bytesToInt(op.Data)))
	if name != "typeSwitch" {
		return nil, false
	}
	if int(index) < 0 || int(index) >= len(d.BootstrapMethods) {
		return nil, false
	}
	bsm := d.BootstrapMethods[index]
	if bsm == nil {
		return nil, false
	}
	if m, ok := bsm.Ref.(*values.JavaClassMember); ok && m != nil {
		id, err := IdentityFromMember(m)
		if err != nil || !id.Equal(IdentityTypeSwitch) {
			return nil, false
		}
	}
	labs, err := ParseTypeSwitchLabels(bsm.Arguments)
	if err != nil {
		return nil, false
	}
	return labs, true
}

func inferMergePC(sw *OpCode, byOff map[int]*OpCode) int {
	if sw.SwitchJmpCase == nil {
		return -1
	}
	counts := map[int]int{}
	sw.SwitchJmpCase.ForEach(func(k int, off int32) bool {
		if op := byOff[int(off)]; op != nil {
			if t, ok := trailingGoto(op, byOff, 24); ok {
				counts[t]++
			}
		}
		return true
	})
	if op := byOff[int(sw.SwitchDefaultOffset)]; op != nil {
		if t, ok := trailingGoto(op, byOff, 24); ok {
			counts[t]++
		}
	}
	best, bestN := -1, 0
	for pc, n := range counts {
		if n > bestN {
			best, bestN = pc, n
		}
	}
	return best
}

func trailingGoto(start *OpCode, byOff map[int]*OpCode, maxHops int) (int, bool) {
	op := start
	for i := 0; i < maxHops && op != nil; i++ {
		if op.Instr.OpCode == OP_GOTO || op.Instr.OpCode == OP_GOTO_W {
			return ifBranchPC(op), true
		}
		if isReturnOp(op.Instr.OpCode) {
			return int(op.CurrentOffset), true
		}
		next := nextOp(op, byOff)
		if next == nil {
			break
		}
		op = next
	}
	return 0, false
}

func recoverSelectorExpr(d *Decompiler, ops []*OpCode, selectorSlot, loopPC int) (string, bool) {
	requireNonNull := false
	var lastStore *OpCode
	for _, op := range ops {
		if int(op.CurrentOffset) >= loopPC {
			break
		}
		if isRequireNonNull(d, op) {
			requireNonNull = true
		}
		if GetStoreIdx(op) == selectorSlot {
			lastStore = op
		}
	}
	if lastStore == nil {
		return "", requireNonNull
	}
	expr, ok := evalStackAtStore(d, ops, lastStore, selectorSlot)
	if !ok {
		return "", requireNonNull
	}
	return expr, requireNonNull
}

func isRequireNonNull(d *Decompiler, op *OpCode) bool {
	if op.Instr.OpCode != OP_INVOKESTATIC || d.constantPoolGetter == nil || len(op.Data) < 2 {
		return false
	}
	v := d.constantPoolGetter(int(Convert2bytesToInt(op.Data)))
	m, ok := v.(*values.JavaClassMember)
	return ok && m != nil && strings.ReplaceAll(m.Name, "/", ".") == "java.util.Objects" && m.Member == "requireNonNull"
}

func evalStackAtStore(d *Decompiler, ops []*OpCode, store *OpCode, selectorSlot int) (string, bool) {
	// Walk a short prefix and simulate a stack of expression strings.
	stack := []string{}
	for _, op := range ops {
		if op == store {
			if len(stack) == 0 {
				return "", false
			}
			return stack[len(stack)-1], true
		}
		if int(op.CurrentOffset) >= int(store.CurrentOffset) {
			break
		}
		if !simExprOp(d, op, &stack, nil) {
			stack = nil
		}
	}
	return "", false
}

func recoverTypeCase(d *Decompiler, byOff map[int]*OpCode, start, mergePC, loopPC, restartSlot, selectorSlot int, bind string, caseIndex int) (body, guard string, decon, ok bool) {
	op := byOff[start]
	if op == nil {
		return "", "", false, false
	}
	// Expected: aload selector; checkcast T; astore bind
	if GetRetrieveIdx(op) != selectorSlot {
		return "", "", false, false
	}
	op = nextOp(op, byOff)
	if op == nil || op.Instr.OpCode != OP_CHECKCAST {
		return "", "", false, false
	}
	op = nextOp(op, byOff)
	bindSlot := GetStoreIdx(op)
	if bindSlot < 0 {
		return "", "", false, false
	}
	env := map[int]string{bindSlot: bind, selectorSlot: localName(d, selectorSlot)}
	cur := nextOp(op, byOff)
	if cur == nil {
		return "", "", false, false
	}

	// Detect record deconstruction: two or more invokevirtual on the binding stored to fresh locals.
	if isDeconstructionPrefix(cur, byOff, bindSlot, mergePC, loopPC) {
		return "", "", true, false
	}

	// Guard: compute boolean/int then if* to success, fail path stores restart and goto loop.
	if g, success, hasGuard := recoverGuard(d, cur, byOff, mergePC, loopPC, restartSlot, env, caseIndex); hasGuard {
		b, ok := evalExprToGoto(d, byOff, success, mergePC, selectorSlot, env)
		if !ok {
			return "", "", false, false
		}
		return b, g, false, true
	}
	b, ok := evalExprToGoto(d, byOff, int(cur.CurrentOffset), mergePC, selectorSlot, env)
	return b, "", false, ok
}

func isDeconstructionPrefix(start *OpCode, byOff map[int]*OpCode, bindSlot, mergePC, loopPC int) bool {
	invokes := 0
	op := start
	for i := 0; i < 16 && op != nil; i++ {
		pc := int(op.CurrentOffset)
		if pc == mergePC || pc == loopPC {
			break
		}
		if op.Instr.OpCode == OP_GOTO || op.Instr.OpCode == OP_GOTO_W || isReturnOp(op.Instr.OpCode) {
			break
		}
		if op.Instr.OpCode == OP_INVOKEVIRTUAL || op.Instr.OpCode == OP_INVOKEINTERFACE {
			invokes++
		}
		op = nextOp(op, byOff)
	}
	return invokes >= 2
}

func recoverGuard(d *Decompiler, start *OpCode, byOff map[int]*OpCode, mergePC, loopPC, restartSlot int, env map[int]string, caseIndex int) (guard string, successPC int, ok bool) {
	op := start
	stack := []string{}
	for i := 0; i < 24 && op != nil; i++ {
		pc := int(op.CurrentOffset)
		if pc == mergePC || pc == loopPC {
			return "", 0, false
		}
		if isIfOp(op.Instr.OpCode) {
			fail := nextOp(op, byOff)
			if fail == nil {
				return "", 0, false
			}
			if !isRestartFail(fail, byOff, loopPC, restartSlot, caseIndex+1) {
				return "", 0, false
			}
			g, ok := guardFromIf(op, stack)
			if !ok {
				return "", 0, false
			}
			return g, ifBranchPC(op), true
		}
		if !simExprOp(d, op, &stack, env) {
			return "", 0, false
		}
		op = nextOp(op, byOff)
	}
	return "", 0, false
}

func isRestartFail(op *OpCode, byOff map[int]*OpCode, loopPC, restartSlot, nextIndex int) bool {
	// iconst/bipush next; istore restart; goto loop
	val, ok := iconstValue(op)
	if !ok || val != nextIndex {
		return false
	}
	st := nextOp(op, byOff)
	if st == nil || GetStoreIdx(st) != restartSlot {
		return false
	}
	g := nextOp(st, byOff)
	if g == nil {
		return false
	}
	return (g.Instr.OpCode == OP_GOTO || g.Instr.OpCode == OP_GOTO_W) && ifBranchPC(g) == loopPC
}

func guardFromIf(op *OpCode, stack []string) (string, bool) {
	switch op.Instr.OpCode {
	case OP_IFNE:
		if len(stack) < 1 {
			return "", false
		}
		return stack[len(stack)-1], true
	case OP_IFEQ:
		if len(stack) < 1 {
			return "", false
		}
		return "!(" + stack[len(stack)-1] + ")", true
	case OP_IF_ICMPGT:
		if len(stack) < 2 {
			return "", false
		}
		return "(" + stack[len(stack)-2] + ") > (" + stack[len(stack)-1] + ")", true
	case OP_IF_ICMPGE:
		if len(stack) < 2 {
			return "", false
		}
		return "(" + stack[len(stack)-2] + ") >= (" + stack[len(stack)-1] + ")", true
	case OP_IF_ICMPLT:
		if len(stack) < 2 {
			return "", false
		}
		return "(" + stack[len(stack)-2] + ") < (" + stack[len(stack)-1] + ")", true
	case OP_IF_ICMPLE:
		if len(stack) < 2 {
			return "", false
		}
		return "(" + stack[len(stack)-2] + ") <= (" + stack[len(stack)-1] + ")", true
	case OP_IF_ICMPEQ:
		if len(stack) < 2 {
			return "", false
		}
		return "(" + stack[len(stack)-2] + ") == (" + stack[len(stack)-1] + ")", true
	case OP_IF_ICMPNE:
		if len(stack) < 2 {
			return "", false
		}
		return "(" + stack[len(stack)-2] + ") != (" + stack[len(stack)-1] + ")", true
	default:
		return "", false
	}
}

func evalExprToGoto(d *Decompiler, byOff map[int]*OpCode, start, mergePC, selectorSlot int, env map[int]string) (string, bool) {
	op := byOff[start]
	if op == nil {
		return "", false
	}
	stack := []string{}
	for i := 0; i < 32 && op != nil; i++ {
		pc := int(op.CurrentOffset)
		if pc == mergePC {
			if len(stack) == 0 {
				return "", false
			}
			return stack[len(stack)-1], true
		}
		if op.Instr.OpCode == OP_GOTO || op.Instr.OpCode == OP_GOTO_W {
			if ifBranchPC(op) == mergePC {
				if len(stack) == 0 {
					return "", false
				}
				return stack[len(stack)-1], true
			}
			op = byOff[ifBranchPC(op)]
			continue
		}
		if isReturnOp(op.Instr.OpCode) {
			if len(stack) == 0 {
				return "", false
			}
			return stack[len(stack)-1], true
		}
		if !simExprOp(d, op, &stack, env) {
			return "", false
		}
		op = nextOp(op, byOff)
	}
	return "", false
}

func simExprOp(d *Decompiler, op *OpCode, stack *[]string, env map[int]string) bool {
	code := op.Instr.OpCode
	push := func(s string) { *stack = append(*stack, s) }
	pop := func() string {
		if len(*stack) == 0 {
			return "/*empty*/"
		}
		s := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		return s
	}
	switch code {
	case OP_NOP, OP_POP:
		if code == OP_POP && len(*stack) > 0 {
			pop()
		}
		return true
	case OP_DUP:
		if len(*stack) == 0 {
			return false
		}
		push((*stack)[len(*stack)-1])
		return true
	case OP_ACONST_NULL:
		push("null")
		return true
	case OP_ICONST_M1, OP_ICONST_0, OP_ICONST_1, OP_ICONST_2, OP_ICONST_3, OP_ICONST_4, OP_ICONST_5:
		push(fmt.Sprintf("%d", int(code-OP_ICONST_0)))
		return true
	case OP_BIPUSH:
		if len(op.Data) < 1 {
			return false
		}
		push(fmt.Sprintf("%d", int(int8(op.Data[0]))))
		return true
	case OP_SIPUSH:
		if len(op.Data) < 2 {
			return false
		}
		push(fmt.Sprintf("%d", int(int16(binary.BigEndian.Uint16(op.Data)))))
		return true
	case OP_LDC, OP_LDC_W, OP_LDC2_W:
		if d.constantPoolGetter == nil || len(op.Data) < 1 {
			return false
		}
		idx := int(op.Data[0])
		if code != OP_LDC {
			idx = int(Convert2bytesToInt(op.Data))
		}
		push(renderConst(d, idx))
		return true
	case OP_ILOAD, OP_LLOAD, OP_FLOAD, OP_DLOAD, OP_ALOAD,
		OP_ILOAD_0, OP_ILOAD_1, OP_ILOAD_2, OP_ILOAD_3,
		OP_LLOAD_0, OP_LLOAD_1, OP_LLOAD_2, OP_LLOAD_3,
		OP_FLOAD_0, OP_FLOAD_1, OP_FLOAD_2, OP_FLOAD_3,
		OP_DLOAD_0, OP_DLOAD_1, OP_DLOAD_2, OP_DLOAD_3,
		OP_ALOAD_0, OP_ALOAD_1, OP_ALOAD_2, OP_ALOAD_3:
		slot := GetRetrieveIdx(op)
		if env != nil {
			if n, ok := env[slot]; ok {
				push(n)
				return true
			}
		}
		push(localName(d, slot))
		return true
	case OP_ISTORE, OP_LSTORE, OP_FSTORE, OP_DSTORE, OP_ASTORE,
		OP_ISTORE_0, OP_ISTORE_1, OP_ISTORE_2, OP_ISTORE_3,
		OP_LSTORE_0, OP_LSTORE_1, OP_LSTORE_2, OP_LSTORE_3,
		OP_FSTORE_0, OP_FSTORE_1, OP_FSTORE_2, OP_FSTORE_3,
		OP_DSTORE_0, OP_DSTORE_1, OP_DSTORE_2, OP_DSTORE_3,
		OP_ASTORE_0, OP_ASTORE_1, OP_ASTORE_2, OP_ASTORE_3:
		if len(*stack) == 0 {
			return false
		}
		v := pop()
		if env != nil {
			env[GetStoreIdx(op)] = v
		}
		return true
	case OP_CHECKCAST:
		return true
	case OP_GETFIELD:
		obj := pop()
		push(obj + "." + memberName(d, op))
		return true
	case OP_GETSTATIC:
		push(memberOwner(d, op) + "." + memberName(d, op))
		return true
	case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE, OP_INVOKESPECIAL, OP_INVOKESTATIC:
		return simInvoke(d, op, stack, code == OP_INVOKESTATIC)
	case OP_INVOKEDYNAMIC:
		return simIndyConcat(d, op, stack)
	case OP_IADD, OP_LADD, OP_FADD, OP_DADD:
		b, a := pop(), pop()
		push("(" + a + " + " + b + ")")
		return true
	case OP_ISUB, OP_LSUB, OP_FSUB, OP_DSUB:
		b, a := pop(), pop()
		push("(" + a + " - " + b + ")")
		return true
	case OP_IMUL, OP_LMUL, OP_FMUL, OP_DMUL:
		b, a := pop(), pop()
		push("(" + a + " * " + b + ")")
		return true
	case OP_ARRAYLENGTH:
		push(pop() + ".length")
		return true
	case OP_I2L, OP_I2F, OP_I2D, OP_L2I, OP_L2F, OP_L2D, OP_F2I, OP_F2L, OP_F2D, OP_D2I, OP_D2L, OP_D2F, OP_I2B, OP_I2C, OP_I2S:
		return true
	default:
		return false
	}
}

func simInvoke(d *Decompiler, op *OpCode, stack *[]string, isStatic bool) bool {
	if d.constantPoolGetter == nil || len(op.Data) < 2 {
		return false
	}
	v := d.constantPoolGetter(int(Convert2bytesToInt(op.Data)))
	m, ok := v.(*values.JavaClassMember)
	if !ok || m == nil {
		return false
	}
	nParams := countDescParams(m.Description)
	args := make([]string, nParams)
	for i := nParams - 1; i >= 0; i-- {
		if len(*stack) == 0 {
			return false
		}
		args[i] = (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
	}
	recv := ""
	if !isStatic {
		if len(*stack) == 0 {
			return false
		}
		recv = (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
	}
	call := m.Member + "(" + strings.Join(args, ", ") + ")"
	if isStatic {
		owner := m.Name
		if d.FunctionContext != nil {
			cur := strings.ReplaceAll(d.FunctionContext.ClassName, "/", ".")
			own := strings.ReplaceAll(owner, "/", ".")
			if own == cur {
				*stack = append(*stack, call)
				return true
			}
			owner = d.FunctionContext.ShortTypeName(own)
		}
		*stack = append(*stack, owner+"."+call)
		return true
	}
	*stack = append(*stack, recv+"."+call)
	return true
}

func simIndyConcat(d *Decompiler, op *OpCode, stack *[]string) bool {
	if d.ConstantPoolInvokeDynamicInfo == nil || len(op.Data) < 2 {
		return false
	}
	index, name, desc := d.ConstantPoolInvokeDynamicInfo(int(Convert2bytesToInt(op.Data)))
	if name != "makeConcatWithConstants" && name != "makeConcat" {
		return false
	}
	nParams := countDescParams(desc)
	args := make([]string, nParams)
	for i := nParams - 1; i >= 0; i-- {
		if len(*stack) == 0 {
			return false
		}
		args[i] = (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
	}
	if name == "makeConcat" {
		*stack = append(*stack, strings.Join(args, " + "))
		return true
	}
	if int(index) < 0 || int(index) >= len(d.BootstrapMethods) || d.BootstrapMethods[index] == nil {
		return false
	}
	recipe, ok := LiteralStringData(d.BootstrapMethods[index].Arguments[0])
	if !ok {
		return false
	}
	*stack = append(*stack, spliceConcatRecipe(recipe, args))
	return true
}

func spliceConcatRecipe(recipe string, args []string) string {
	var b strings.Builder
	ai := 0
	first := true
	emit := func(s string) {
		if s == "" {
			return
		}
		if !first {
			b.WriteString(" + ")
		}
		first = false
		b.WriteString(s)
	}
	lit := ""
	flushLit := func() {
		if lit == "" {
			return
		}
		emit(fmt.Sprintf("%q", lit))
		lit = ""
	}
	for _, r := range recipe {
		if r == '\u0001' {
			flushLit()
			if ai < len(args) {
				emit(args[ai])
				ai++
			}
			continue
		}
		if r == '\u0002' {
			flushLit()
			continue
		}
		lit += string(r)
	}
	flushLit()
	if b.Len() == 0 {
		return `""`
	}
	return b.String()
}

func renderConst(d *Decompiler, idx int) string {
	if d.ConstantPoolLiteralGetter != nil {
		v := d.ConstantPoolLiteralGetter(idx)
		if v != nil {
			if lit, ok := v.(*values.JavaLiteral); ok && lit != nil {
				if s, ok := lit.Data.(string); ok {
					return fmt.Sprintf("%q", s)
				}
				return fmt.Sprint(lit.Data)
			}
			if v == values.JavaNull {
				return "null"
			}
		}
	}
	if d.constantPoolGetter == nil {
		return "null"
	}
	v := d.constantPoolGetter(idx)
	if lit, ok := v.(*values.JavaLiteral); ok && lit != nil {
		if s, ok := lit.Data.(string); ok {
			return fmt.Sprintf("%q", s)
		}
		return fmt.Sprint(lit.Data)
	}
	if cv, ok := v.(*values.JavaClassValue); ok && cv != nil {
		return cv.JavaType.String(&class_context.ClassContext{}) + ".class"
	}
	if s, ok := LiteralStringData(v); ok {
		return fmt.Sprintf("%q", s)
	}
	return "null"
}

func localName(d *Decompiler, slot int) string {
	if d != nil && d.FunctionContext != nil && !d.FunctionContext.IsStatic && slot == 0 {
		return "this"
	}
	return fmt.Sprintf("var%d", slot)
}

func shortType(d *Decompiler, name string) string {
	name = strings.ReplaceAll(name, "/", ".")
	if d != nil && d.FunctionContext != nil {
		return d.FunctionContext.ShortTypeName(name)
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

func nextBindName(typeName string, seq map[string]int) string {
	base := "v"
	switch typeName {
	case "String":
		base = "s"
	case "Integer":
		base = "i"
	case "Long":
		base = "l"
	case "Double":
		base = "d"
	case "Float":
		base = "f"
	case "Boolean":
		base = "b"
	default:
		if typeName != "" {
			r := []rune(typeName)
			base = strings.ToLower(string(r[0]))
		}
	}
	n := seq[base]
	seq[base] = n + 1
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s%d", base, n)
}

func nextOp(op *OpCode, byOff map[int]*OpCode) *OpCode {
	if op == nil {
		return nil
	}
	best := -1
	var found *OpCode
	cur := int(op.CurrentOffset)
	for pc, n := range byOff {
		if pc > cur && (best < 0 || pc < best) {
			best = pc
			found = n
		}
	}
	return found
}

// Opcodes returns the decoded instruction stream, parsing on demand.
func (d *Decompiler) Opcodes() []*OpCode {
	if d == nil {
		return nil
	}
	if len(d.opCodes) == 0 {
		_ = d.ParseOpcode()
	}
	return d.opCodes
}

func ifBranchPC(op *OpCode) int {
	if op == nil || len(op.Data) < 2 {
		return -1
	}
	if op.Instr.OpCode == OP_GOTO_W && len(op.Data) >= 4 {
		off := int32(binary.BigEndian.Uint32(op.Data))
		return int(op.CurrentOffset) + int(off)
	}
	off := int16(binary.BigEndian.Uint16(op.Data))
	return int(op.CurrentOffset) + int(off)
}

func isIfOp(code int) bool {
	switch code {
	case OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE,
		OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE,
		OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IFNULL, OP_IFNONNULL:
		return true
	}
	return false
}

func isReturnOp(code int) bool {
	switch code {
	case OP_IRETURN, OP_LRETURN, OP_FRETURN, OP_DRETURN, OP_ARETURN, OP_RETURN:
		return true
	}
	return false
}

func iconstValue(op *OpCode) (int, bool) {
	if op == nil {
		return 0, false
	}
	switch op.Instr.OpCode {
	case OP_ICONST_M1:
		return -1, true
	case OP_ICONST_0, OP_ICONST_1, OP_ICONST_2, OP_ICONST_3, OP_ICONST_4, OP_ICONST_5:
		return int(op.Instr.OpCode - OP_ICONST_0), true
	case OP_BIPUSH:
		if len(op.Data) < 1 {
			return 0, false
		}
		return int(int8(op.Data[0])), true
	case OP_SIPUSH:
		if len(op.Data) < 2 {
			return 0, false
		}
		return int(int16(binary.BigEndian.Uint16(op.Data))), true
	}
	return 0, false
}

func countDescParams(desc string) int {
	n, err := ParamCountOfDescriptor(desc)
	if err != nil {
		return 0
	}
	return n
}

func memberName(d *Decompiler, op *OpCode) string {
	if d.constantPoolGetter == nil || len(op.Data) < 2 {
		return "m"
	}
	v := d.constantPoolGetter(int(Convert2bytesToInt(op.Data)))
	if m, ok := v.(*values.JavaClassMember); ok && m != nil {
		return m.Member
	}
	return "m"
}

func memberOwner(d *Decompiler, op *OpCode) string {
	if d.constantPoolGetter == nil || len(op.Data) < 2 {
		return "Owner"
	}
	v := d.constantPoolGetter(int(Convert2bytesToInt(op.Data)))
	if m, ok := v.(*values.JavaClassMember); ok && m != nil {
		if d.FunctionContext != nil {
			return d.FunctionContext.ShortTypeName(strings.ReplaceAll(m.Name, "/", "."))
		}
		return m.Name
	}
	return "Owner"
}
