package javaclassparser

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func (c *ClassObjectDumper) hasPackCarrierCtorCollision(method *MemberInfo) bool {
	if c == nil || c.obj == nil || method == nil {
		return true
	}
	origDesc, err := c.obj.getUtf8(method.DescriptorIndex)
	if err != nil || !strings.HasPrefix(origDesc, "(") {
		return true
	}
	targetDesc := "([Ljava/lang/Object;" + origDesc[1:]
	for _, m := range c.obj.Methods {
		if m == nil {
			continue
		}
		mn, _ := c.obj.getUtf8(m.NameIndex)
		md, _ := c.obj.getUtf8(m.DescriptorIndex)
		if mn == "<init>" && md == targetDesc {
			return true
		}
	}
	return false
}

func ctorTypeMentionsTypeParams(t types.JavaType, typeParams []string, depth int) bool {
	if t == nil || len(typeParams) == 0 || depth > 32 {
		return false
	}
	inScope := func(n string) bool {
		for _, tp := range typeParams {
			if tp == n {
				return true
			}
		}
		return false
	}
	if name, ok := types.TypeVariableName(t); ok && inScope(name) {
		return true
	}
	if t.IsArray() {
		return ctorTypeMentionsTypeParams(t.ElementType(), typeParams, depth+1)
	}
	if pt, ok := types.AsParameterizedType(t); ok && pt != nil {
		for _, arg := range pt.TypeArgs {
			if ctorTypeMentionsTypeParams(arg, typeParams, depth+1) {
				return true
			}
		}
		for _, seg := range pt.OwnerSegments {
			for _, arg := range seg.TypeArgs {
				if ctorTypeMentionsTypeParams(arg, typeParams, depth+1) {
					return true
				}
			}
		}
	}
	if wc, ok := t.RawType().(*types.JavaWildcardType); ok && wc != nil && wc.Bound != nil {
		return ctorTypeMentionsTypeParams(wc.Bound, typeParams, depth+1)
	}
	if jc, ok := t.RawType().(*types.JavaClass); ok && jc != nil && inScope(jc.Name) {
		return true
	}
	return false
}

func ctorValuesMentionTypeParams(vals []values.JavaValue, typeParams []string, depth int) bool {
	if len(typeParams) == 0 || depth > 64 {
		return false
	}
	for _, v := range vals {
		if v == nil {
			continue
		}
		if ctorTypeMentionsTypeParams(v.Type(), typeParams, 0) {
			return true
		}
		if cast, ok := values.UnpackSoltValue(v).(*values.CastExpression); ok && cast != nil {
			if ctorTypeMentionsTypeParams(cast.TargetType, typeParams, 0) {
				return true
			}
		}
		if call, ok := values.UnpackSoltValue(v).(*values.FunctionCallExpression); ok && call != nil {
			if ctorValuesMentionTypeParams(call.Arguments, typeParams, depth+1) {
				return true
			}
			if !call.IsStatic && call.Object != nil && ctorValuesMentionTypeParams([]values.JavaValue{call.Object}, typeParams, depth+1) {
				return true
			}
			continue
		}
		if children, known := values.Children(v); known {
			if ctorValuesMentionTypeParams(children, typeParams, depth+1) {
				return true
			}
		}
	}
	return false
}

func ctorStmtsMentionTypeParams(sts []statements.Statement, typeParams []string, depth int) bool {
	if len(typeParams) == 0 || depth > 32 {
		return false
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		switch s := st.(type) {
		case *statements.AssignStatement:
			if ctorValuesMentionTypeParams([]values.JavaValue{s.LeftValue, s.JavaValue}, typeParams, 0) {
				return true
			}
			if s.ArrayMember != nil && ctorValuesMentionTypeParams([]values.JavaValue{s.ArrayMember}, typeParams, 0) {
				return true
			}
		case *statements.ExpressionStatement:
			if ctorValuesMentionTypeParams([]values.JavaValue{s.Expression}, typeParams, 0) {
				return true
			}
		case *statements.ReturnStatement:
			if ctorValuesMentionTypeParams([]values.JavaValue{s.JavaValue}, typeParams, 0) {
				return true
			}
		case *statements.IfStatement:
			if ctorValuesMentionTypeParams([]values.JavaValue{s.Condition}, typeParams, 0) ||
				ctorStmtsMentionTypeParams(s.IfBody, typeParams, depth+1) ||
				ctorStmtsMentionTypeParams(s.ElseBody, typeParams, depth+1) {
				return true
			}
		case *statements.DoWhileStatement:
			if ctorValuesMentionTypeParams([]values.JavaValue{s.ConditionValue}, typeParams, 0) ||
				ctorStmtsMentionTypeParams(s.Body, typeParams, depth+1) {
				return true
			}
		case *statements.SwitchStatement:
			if ctorValuesMentionTypeParams([]values.JavaValue{s.Value}, typeParams, 0) {
				return true
			}
			for _, item := range s.Cases {
				if item != nil && ctorStmtsMentionTypeParams(item.Body, typeParams, depth+1) {
					return true
				}
			}
		case *statements.CustomStatement:
			if a, ok := s.SourceAssignmentForward(); ok && a != nil {
				if ctorStmtsMentionTypeParams([]statements.Statement{a}, typeParams, depth+1) {
					return true
				}
			}
			if v, ok := s.SourceThrowOperand(); ok && v != nil {
				if ctorValuesMentionTypeParams([]values.JavaValue{v}, typeParams, 0) {
					return true
				}
			}
		}
	}
	return false
}

func collectStmtVarTypeByID(sts []statements.Statement, targetIDs map[*coreutils.VariableId]bool, outRef map[*coreutils.VariableId]*values.JavaRef, outType map[*coreutils.VariableId]types.JavaType, conflict map[*coreutils.VariableId]bool, ctx *class_context.ClassContext, depth int) {
	if depth > 64 {
		return
	}
	recordRef := func(ref *values.JavaRef) {
		if ref == nil || ref.Id == nil || !targetIDs[ref.Id] {
			return
		}
		if outRef[ref.Id] == nil {
			outRef[ref.Id] = ref
		}
		rt := ref.Type()
		if rt == nil || ctorDefaultInitializer(rt) == "" {
			conflict[ref.Id] = true
			return
		}
		if prev := outType[ref.Id]; prev != nil {
			if prev.String(ctx) != rt.String(ctx) {
				conflict[ref.Id] = true
			}
			return
		}
		outType[ref.Id] = rt
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		switch s := st.(type) {
		case *statements.AssignStatement:
			if s.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); ok && ref != nil {
					recordRef(ref)
				}
			}
		case *statements.CustomStatement:
			if as, ok := s.SourceAssignmentForward(); ok && as != nil {
				collectStmtVarTypeByID([]statements.Statement{as}, targetIDs, outRef, outType, conflict, ctx, depth+1)
			}
		case *statements.IfStatement:
			collectStmtVarTypeByID(s.IfBody, targetIDs, outRef, outType, conflict, ctx, depth+1)
			collectStmtVarTypeByID(s.ElseBody, targetIDs, outRef, outType, conflict, ctx, depth+1)
		case *statements.DoWhileStatement:
			collectStmtVarTypeByID(s.Body, targetIDs, outRef, outType, conflict, ctx, depth+1)
		case *statements.SwitchStatement:
			for _, item := range s.Cases {
				if item != nil {
					collectStmtVarTypeByID(item.Body, targetIDs, outRef, outType, conflict, ctx, depth+1)
				}
			}
		}
	}
}

func unpackFromObjectArray(arrName string, index int, typ types.JavaType, ctx *class_context.ClassContext) string {
	elem := fmt.Sprintf("%s[%d]", arrName, index)
	if typ == nil {
		return elem
	}
	if !typ.IsArray() {
		if prim, ok := typ.RawType().(*types.JavaPrimer); ok && prim != nil {
			switch prim.Name {
			case types.JavaBoolean:
				return fmt.Sprintf("((Boolean)(%s)).booleanValue()", elem)
			case types.JavaByte:
				return fmt.Sprintf("((Byte)(%s)).byteValue()", elem)
			case types.JavaShort:
				return fmt.Sprintf("((Short)(%s)).shortValue()", elem)
			case types.JavaChar:
				return fmt.Sprintf("((Character)(%s)).charValue()", elem)
			case types.JavaInteger:
				return fmt.Sprintf("((Integer)(%s)).intValue()", elem)
			case types.JavaLong:
				return fmt.Sprintf("((Long)(%s)).longValue()", elem)
			case types.JavaFloat:
				return fmt.Sprintf("((Float)(%s)).floatValue()", elem)
			case types.JavaDouble:
				return fmt.Sprintf("((Double)(%s)).doubleValue()", elem)
			}
		}
	}
	ts := typ.String(ctx)
	if ts == "" || ts == "Object" || ts == "java.lang.Object" {
		return elem
	}
	return fmt.Sprintf("((%s)(%s))", ts, elem)
}

func packArgElementExpr(arg values.JavaValue, formal types.JavaType, rawStr string, ctx *class_context.ClassContext) string {
	if formal == nil {
		return rawStr
	}
	if !formal.IsArray() {
		if prim, ok := formal.RawType().(*types.JavaPrimer); ok && prim != nil {
			if rawStr == "null" {
				if init := ctorDefaultInitializer(formal); init != "" {
					return init
				}
			}
			return fmt.Sprintf("(%s)(%s)", prim.Name, rawStr)
		}
	}
	ts := formal.String(ctx)
	if ts == "" {
		return rawStr
	}
	return fmt.Sprintf("(%s)(%s)", ts, rawStr)
}

func (c *ClassObjectDumper) collectPackedEscapedVars(p *constructorSourceBoundary, prefix []statements.Statement, extraReads []values.JavaValue, postSuper []statements.Statement, paramIndexByID map[*coreutils.VariableId]int, paramTypes []types.JavaType) ([]packedEscapedVar, map[*coreutils.VariableId]bool, bool) {
	prefixWrittenIDs := map[*coreutils.VariableId]bool{}
	var prefixUsesThis bool
	collectStmtVars(prefix, c.FuncCtx, nil, prefixWrittenIDs, nil, &prefixUsesThis, 0)
	if prefixUsesThis {
		return nil, nil, false
	}
	for _, v := range extraReads {
		var uThis bool
		collectValueVarReads(v, c.FuncCtx, nil, nil, &uThis, 0)
		if uThis {
			return nil, nil, false
		}
	}

	helperReads := map[*coreutils.VariableId]bool{}
	collectStmtVarReads(prefix, c.FuncCtx, helperReads, 0)
	for _, v := range extraReads {
		collectValueVarReads(v, c.FuncCtx, helperReads, nil, nil, 0)
	}
	for id := range helperReads {
		if _, isParam := paramIndexByID[id]; !isParam && !prefixWrittenIDs[id] {
			return nil, nil, false
		}
	}

	postReads := map[*coreutils.VariableId]bool{}
	collectStmtVarReads(postSuper, c.FuncCtx, postReads, 0)

	escapedIDs := map[*coreutils.VariableId]bool{}
	for id := range prefixWrittenIDs {
		if postReads[id] {
			escapedIDs[id] = true
		}
	}
	if len(escapedIDs) == 0 {
		return nil, nil, true
	}

	refByID := map[*coreutils.VariableId]*values.JavaRef{}
	typByID := map[*coreutils.VariableId]types.JavaType{}
	conflict := map[*coreutils.VariableId]bool{}
	collectStmtVarTypeByID(prefix, escapedIDs, refByID, typByID, conflict, c.FuncCtx, 0)

	var list []packedEscapedVar
	for id := range escapedIDs {
		if conflict[id] {
			return nil, nil, false
		}
		idx, isParam := paramIndexByID[id]
		var ref *values.JavaRef
		var typ types.JavaType
		if isParam {
			ref, _ = p.params[idx].(*values.JavaRef)
			if idx < len(paramTypes) {
				typ = paramTypes[idx]
			}
		} else {
			ref = refByID[id]
			typ = typByID[id]
		}
		if ref == nil || typ == nil || ctorDefaultInitializer(typ) == "" {
			return nil, nil, false
		}
		list = append(list, packedEscapedVar{
			id:         id,
			name:       ref.String(c.FuncCtx),
			typ:        typ,
			typStr:     typ.String(c.FuncCtx),
			isParam:    isParam,
			paramIndex: idx,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].name != list[j].name {
			return list[i].name < list[j].name
		}
		return list[i].paramIndex < list[j].paramIndex
	})
	return list, escapedIDs, true
}

// planRetainedPackCarrier (Subcase D) handles retained constructors whose prefix
// feeds multiple delegate arguments or writes locals that escape into postSuper.
func (c *ClassObjectDumper) planRetainedPackCarrier(p *constructorSourceBoundary, method *MemberInfo, postSuper []statements.Statement) bool {
	if c == nil || p == nil || p.delegate == nil || method == nil {
		return false
	}
	if !c.options.RetainUnprovedSource || c.getenv("JDEC_CTOR_RETAIN_PREFIX_OFF") != "" {
		return false
	}
	if c.isGenuineEnum() || c.hasPackCarrierCtorCollision(method) {
		return false
	}
	desc, _ := c.obj.getUtf8(method.DescriptorIndex)
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil || len(mt.FunctionType().ParamTypes) != len(p.params) {
		return false
	}
	target, err := types.ParseMethodDescriptor(p.delegate.Descriptor)
	if err != nil || len(target.FunctionType().ParamTypes) != len(p.delegate.Arguments) {
		return false
	}

	paramIndexByID := map[*coreutils.VariableId]int{}
	declared := map[string]bool{}
	for i, param := range p.params {
		ref, ok := param.(*values.JavaRef)
		if !ok || ref == nil || ref.IsThis || ref.Id == nil {
			return false
		}
		paramIndexByID[ref.Id] = i
		declared[ref.String(c.FuncCtx)] = true
	}

	escaped, escapedSet, ok := c.collectPackedEscapedVars(p, p.prefix, p.delegate.Arguments, postSuper, paramIndexByID, mt.FunctionType().ParamTypes)
	if !ok || len(escaped) != 0 {
		return false
	}
	if hasOutOfOrderPrefixDelegateBarriers(p.prefix, p.delegate.Arguments, c.FuncCtx) {
		return false
	}
	if ctorStmtsMentionTypeParams(p.prefix, c.FuncCtx.TypeParams, 0) ||
		ctorValuesMentionTypeParams(p.delegate.Arguments, c.FuncCtx.TypeParams, 0) {
		return false
	}
	for _, pt := range mt.FunctionType().ParamTypes {
		if ctorTypeMentionsTypeParams(pt, c.FuncCtx.TypeParams, 0) {
			return false
		}
	}
	for _, pt := range target.FunctionType().ParamTypes {
		if ctorTypeMentionsTypeParams(pt, c.FuncCtx.TypeParams, 0) {
			return false
		}
	}
	for _, ev := range escaped {
		if ctorTypeMentionsTypeParams(ev.typ, c.FuncCtx.TypeParams, 0) {
			return false
		}
	}

	// Dry-run hoisting and statement rendering now so any unsupported statement
	// in p.prefix fails closed before committing to packCarrier.
	dryDeclared := cloneDeclaredMap(declared)
	for _, arg := range p.delegate.Arguments {
		if _, okH := collectRetainedPrefixHoistedDecls(p.prefix, arg, c.FuncCtx, dryDeclared); !okH {
			return false
		}
	}
	if _, okR := renderRetainedPrefixStatements(p.prefix, c.FuncCtx, dryDeclared); !okR {
		return false
	}

	helperName, err := c.constructorBoundaryHelperName("pack", p.pc)
	if err != nil {
		return false
	}
	packParam := "jdec$a"
	for declared[packParam] {
		packParam += "$"
	}

	// Fold redundant bare declarations in postSuper for locals restored right after super().
	for _, st := range postSuper {
		as, _ := unwrapAssignStmtSimple(st)
		if as != nil && as.IsDeclare && as.JavaValue == nil && as.ArrayMember == nil {
			if ref, ok := values.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && escapedSet[ref.Id] {
				if p.foldedStatements == nil {
					p.foldedStatements = map[statements.Statement]bool{}
				}
				p.foldedStatements[st] = true
			}
		}
	}

	p.packCarrier = true
	p.retainCarrier = true
	p.packHelperName = helperName
	p.packParamName = packParam
	p.packMethod = method
	p.packMethodType = mt.FunctionType()
	p.packTargetParamTypes = target.FunctionType().ParamTypes
	p.packEscaped = escaped
	p.packEscapedByID = escapedSet
	return true
}

func unwrapAssignStmtSimple(st statements.Statement) (*statements.AssignStatement, bool) {
	if as, ok := st.(*statements.AssignStatement); ok && as != nil {
		return as, false
	}
	if cs, ok := st.(*statements.CustomStatement); ok && cs != nil {
		if as, ok := cs.SourceAssignmentForward(); ok && as != nil {
			return as, true
		}
	}
	return nil, false
}

func prefixStmtHasBarrier(sts []statements.Statement, depth int) bool {
	if depth > 16 {
		return false
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		if as, _ := unwrapAssignStmtSimple(st); as != nil {
			if as.JavaValue != nil {
				if eff, _ := values.InspectValue(as.JavaValue); eff&values.BarrierEffects != 0 {
					return true
				}
			}
			continue
		}
		switch s := st.(type) {
		case *statements.ExpressionStatement:
			if s.Expression != nil {
				if eff, _ := values.InspectValue(s.Expression); eff&values.BarrierEffects != 0 {
					return true
				}
			}
		case *statements.IfStatement:
			if prefixStmtHasBarrier(s.IfBody, depth+1) || prefixStmtHasBarrier(s.ElseBody, depth+1) {
				return true
			}
		case *statements.DoWhileStatement:
			if prefixStmtHasBarrier(s.Body, depth+1) {
				return true
			}
		case *statements.WhileStatement:
			if prefixStmtHasBarrier(s.Body, depth+1) {
				return true
			}
		}
	}
	return false
}

func hasOutOfOrderPrefixDelegateBarriers(prefix []statements.Statement, args []values.JavaValue, ctx *class_context.ClassContext) bool {
	if !prefixStmtHasBarrier(prefix, 0) {
		return false
	}
	written := map[*coreutils.VariableId]bool{}
	collectStmtVars(prefix, ctx, nil, written, nil, nil, 0)
	earliestBarrierArg := -1
	for i, arg := range args {
		if eff, _ := values.InspectValue(arg); eff&(values.EffectCall|values.EffectWriteMemory|values.EffectMonitor|values.EffectOpaque) != 0 {
			earliestBarrierArg = i
			break
		}
	}
	if earliestBarrierArg < 0 {
		return false
	}
	for k := earliestBarrierArg + 1; k < len(args); k++ {
		reads := map[*coreutils.VariableId]bool{}
		collectValueVarReads(args[k], ctx, reads, nil, nil, 0)
		for id := range reads {
			if written[id] {
				return true
			}
		}
	}
	return false
}

type packBranchArmPost struct {
	armID     int
	stmts     []statements.Statement
	hasReturn bool
}

func splitPackBranchArmPost(sts []statements.Statement, isCtorThisCall func(statements.Statement) (*values.FunctionCallExpression, bool)) ([]statements.Statement, bool, bool) {
	var post []statements.Statement
	hasReturn := false
	var checkPostStmts func([]statements.Statement, int) bool
	checkPostStmts = func(list []statements.Statement, depth int) bool {
		if depth > 16 {
			return false
		}
		for _, st := range list {
			if st == nil {
				continue
			}
			if _, isInit := isCtorThisCall(st); isInit {
				return false
			}
			switch s := st.(type) {
			case *statements.MiddleStatement, *statements.AssignStatement, *statements.ExpressionStatement:
				continue
			case *statements.IfStatement:
				if !checkPostStmts(s.IfBody, depth+1) || !checkPostStmts(s.ElseBody, depth+1) {
					return false
				}
			case *statements.CustomStatement:
				if _, ok := s.SourceAssignmentForward(); ok {
					continue
				}
				if _, thrown := s.SourceThrowOperand(); thrown {
					continue
				}
				return false
			default:
				return false
			}
		}
		return true
	}

	for _, rem := range sts {
		if rem == nil {
			continue
		}
		if _, isMid := rem.(*statements.MiddleStatement); isMid {
			continue
		}
		if cs, ok := rem.(*statements.CustomStatement); ok {
			if k, _, okT := cs.SourceTransferProtocol(); okT && k == "break" {
				continue
			}
		}
		if ret, isRet := rem.(*statements.ReturnStatement); isRet {
			if ret.JavaValue != nil || hasReturn {
				return nil, false, false
			}
			hasReturn = true
			continue
		}
		if hasReturn {
			return nil, false, false
		}
		if !checkPostStmts([]statements.Statement{rem}, 0) {
			return nil, false, false
		}
		post = append(post, rem)
	}
	return post, hasReturn, true
}

func renderPackArmPostStmts(sts []statements.Statement, hasReturn, emitReturn bool, ctx *class_context.ClassContext, decls map[string]bool) (string, bool) {
	var b strings.Builder
	for _, st := range sts {
		if st == nil {
			continue
		}
		if cs, ok := st.(*statements.CustomStatement); ok {
			if _, thrown := cs.SourceThrowOperand(); thrown {
				b.WriteString(st.String(ctx) + ";\n")
				continue
			}
		}
		one, okOne := renderRetainedPrefixStatements([]statements.Statement{st}, ctx, decls)
		if !okOne {
			return "", false
		}
		b.WriteString(one)
	}
	if hasReturn && emitReturn {
		b.WriteString("return;\n")
	}
	return b.String(), true
}

// planSwitchIfConstructorPackCarrier handles retained constructors whose `super(...)` /
// `this(...)` call appears inside a switch/if decision (e.g. R8 horizontally-merged ctors)
// targeting one uniform `(ClassName, Descriptor)` signature across every arm.
func (c *ClassObjectDumper) planSwitchIfConstructorPackCarrier(p *constructorSourceBoundary, body []statements.Statement, code *CodeAttribute, method *MemberInfo) bool {
	if c == nil || p == nil || method == nil || !c.options.RetainUnprovedSource || c.getenv("JDEC_CTOR_RETAIN_PREFIX_OFF") != "" || c.isGenuineEnum() || c.hasPackCarrierCtorCollision(method) {
		return false
	}
	isCtorThisCall := func(st statements.Statement) (*values.FunctionCallExpression, bool) {
		expr, ok := st.(*statements.ExpressionStatement)
		if !ok || expr == nil {
			return nil, false
		}
		call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
		if !ok || call == nil || call.FunctionName != "<init>" || call.Kind != values.InvokeSpecial || !call.HasOriginPC {
			return nil, false
		}
		ref, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
		isThisRecv := ok && ref != nil && (ref.IsThis || (c.getenv("JDEC_THIS_ALIAS_SUPER_OFF") == "" && ref.IsThisAlias))
		return call, isThisRecv
	}

	var calls []*values.FunctionCallExpression
	var armPosts []packBranchArmPost
	armIDByCall := map[*values.FunctionCallExpression]int{}
	aliasAssigns := map[statements.Statement]*coreutils.VariableId{}
	isThisAliasAssign := func(st statements.Statement) (*coreutils.VariableId, bool) {
		as, _ := unwrapAssignStmtSimple(st)
		if as == nil || as.ArrayMember != nil || as.LeftValue == nil || as.JavaValue == nil {
			return nil, false
		}
		lref, okL := values.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
		rref, okR := values.UnpackSoltValue(as.JavaValue).(*values.JavaRef)
		if !okL || !okR || lref == nil || rref == nil || lref.IsThis || lref.Id == nil || !rref.IsThis {
			return nil, false
		}
		return lref.Id, true
	}
	var splitBranch func(sts []statements.Statement, depth int) (renderTree []statements.Statement, pureTree []statements.Statement, ok bool)
	splitBranch = func(sts []statements.Statement, depth int) ([]statements.Statement, []statements.Statement, bool) {
		if depth > 16 {
			return nil, nil, false
		}
		outRender := make([]statements.Statement, 0, len(sts))
		outPure := make([]statements.Statement, 0, len(sts))
		for idx, st := range sts {
			if st == nil {
				continue
			}
			if aliasID, okA := isThisAliasAssign(st); okA {
				aliasAssigns[st] = aliasID
			}
			if call, ok := isCtorThisCall(st); ok {
				postArm, hasRet, okPost := splitPackBranchArmPost(sts[idx+1:], isCtorThisCall)
				if !okPost {
					return nil, nil, false
				}
				armID := len(armPosts)
				calls = append(calls, call)
				armIDByCall[call] = armID
				armPosts = append(armPosts, packBranchArmPost{armID: armID, stmts: postArm, hasReturn: hasRet})
				outRender = append(outRender, st)
				break
			}
			switch s := st.(type) {
			case *statements.IfStatement:
				ifRender, ifPure, okI := splitBranch(s.IfBody, depth+1)
				elseRender, elsePure, okE := splitBranch(s.ElseBody, depth+1)
				if !okI || !okE {
					return nil, nil, false
				}
				cpR := *s
				cpR.IfBody = ifRender
				cpR.ElseBody = elseRender
				outRender = append(outRender, &cpR)
				cpP := *s
				cpP.IfBody = ifPure
				cpP.ElseBody = elsePure
				outPure = append(outPure, &cpP)
			case *statements.SwitchStatement:
				cpR := *s
				cpP := *s
				cpR.Cases = make([]*statements.CaseItem, 0, len(s.Cases))
				cpP.Cases = make([]*statements.CaseItem, 0, len(s.Cases))
				for _, item := range s.Cases {
					if item == nil {
						cpR.Cases = append(cpR.Cases, nil)
						cpP.Cases = append(cpP.Cases, nil)
						continue
					}
					cRender, cPure, okC := splitBranch(item.Body, depth+1)
					if !okC {
						return nil, nil, false
					}
					itemR := *item
					itemR.Body = cRender
					cpR.Cases = append(cpR.Cases, &itemR)
					itemP := *item
					itemP.Body = cPure
					cpP.Cases = append(cpP.Cases, &itemP)
				}
				outRender = append(outRender, &cpR)
				outPure = append(outPure, &cpP)
			case *statements.WhileStatement, *statements.DoWhileStatement, *statements.ForStatement, *statements.TryCatchStatement, *statements.SynchronizedStatement:
				return nil, nil, false
			default:
				outRender = append(outRender, st)
				outPure = append(outPure, st)
			}
		}
		return outRender, outPure, true
	}

	branchIdx := -1
	var preOnly []statements.Statement
	var purePre []statements.Statement
	for i, st := range body {
		before := len(calls)
		prePart, purePart, okS := splitBranch([]statements.Statement{st}, 0)
		if !okS {
			return false
		}
		if len(calls) > before {
			if branchIdx >= 0 {
				return false
			}
			branchIdx = i
		}
		preOnly = append(preOnly, prePart...)
		purePre = append(purePre, purePart...)
	}
	if branchIdx < 0 || len(calls) < 2 {
		return false
	}
	prefix := append([]statements.Statement(nil), body[:branchIdx+1]...)
	preOnly = preOnly[:branchIdx+1]
	purePre = purePre[:branchIdx+1]

	var postSuper []statements.Statement
	if branchIdx+1 < len(body) {
		postSuper = body[branchIdx+1:]
	}
	var allPost []statements.Statement
	allPost = append(allPost, postSuper...)
	for _, ap := range armPosts {
		allPost = append(allPost, ap.stmts...)
	}
	var allArgVals []values.JavaValue
	for _, call := range calls {
		allArgVals = append(allArgVals, call.Arguments...)
	}

	if len(aliasAssigns) > 0 {
		// Drop dead `<alias> = this;` receiver copies when `<alias>` is used solely as
		// the receiver of an arm's `super(...)` / `this(...)` call and nowhere else.
		var stripDeadAliasStmts func(sts []statements.Statement, skip map[statements.Statement]bool, depth int) []statements.Statement
		stripDeadAliasStmts = func(sts []statements.Statement, skip map[statements.Statement]bool, depth int) []statements.Statement {
			if depth > 16 {
				return sts
			}
			out := make([]statements.Statement, 0, len(sts))
			for _, st := range sts {
				if st == nil || skip[st] {
					continue
				}
				switch s := st.(type) {
				case *statements.IfStatement:
					cp := *s
					cp.IfBody = stripDeadAliasStmts(s.IfBody, skip, depth+1)
					cp.ElseBody = stripDeadAliasStmts(s.ElseBody, skip, depth+1)
					out = append(out, &cp)
				case *statements.SwitchStatement:
					cp := *s
					cp.Cases = make([]*statements.CaseItem, 0, len(s.Cases))
					for _, item := range s.Cases {
						if item == nil {
							cp.Cases = append(cp.Cases, nil)
							continue
						}
						ic := *item
						ic.Body = stripDeadAliasStmts(item.Body, skip, depth+1)
						cp.Cases = append(cp.Cases, &ic)
					}
					out = append(out, &cp)
				default:
					out = append(out, st)
				}
			}
			return out
		}
		aliasSkip := make(map[statements.Statement]bool, len(aliasAssigns))
		for st := range aliasAssigns {
			aliasSkip[st] = true
		}
		pureNoAlias := stripDeadAliasStmts(purePre, aliasSkip, 0)
		aliasReads := map[*coreutils.VariableId]bool{}
		collectStmtVarReads(pureNoAlias, c.FuncCtx, aliasReads, 0)
		collectStmtVarReads(allPost, c.FuncCtx, aliasReads, 0)
		for _, v := range allArgVals {
			collectValueVarReads(v, c.FuncCtx, aliasReads, nil, nil, 0)
		}
		deadAliases := map[statements.Statement]bool{}
		for st, id := range aliasAssigns {
			if !aliasReads[id] {
				deadAliases[st] = true
			}
		}
		if len(deadAliases) > 0 {
			preOnly = stripDeadAliasStmts(preOnly, deadAliases, 0)
			purePre = stripDeadAliasStmts(purePre, deadAliases, 0)
		}
	}

	first := calls[0]
	targetOwner := strings.ReplaceAll(first.ClassName, ".", "/")
	if targetOwner != c.obj.GetClassName() && targetOwner != c.obj.GetSupperClassName() {
		return false
	}
	for _, call := range calls[1:] {
		if strings.ReplaceAll(call.ClassName, ".", "/") != targetOwner || call.Descriptor != first.Descriptor {
			return false
		}
	}

	target, err := types.ParseMethodDescriptor(first.Descriptor)
	if err != nil || len(target.FunctionType().ParamTypes) != len(first.Arguments) {
		return false
	}
	desc, _ := c.obj.getUtf8(method.DescriptorIndex)
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil || len(mt.FunctionType().ParamTypes) != len(p.params) {
		return false
	}

	paramIndexByID := map[*coreutils.VariableId]int{}
	declared := map[string]bool{}
	for i, param := range p.params {
		ref, ok := param.(*values.JavaRef)
		if !ok || ref == nil || ref.IsThis || ref.Id == nil {
			return false
		}
		paramIndexByID[ref.Id] = i
		declared[ref.String(c.FuncCtx)] = true
	}

	escaped, escapedSet, ok := c.collectPackedEscapedVars(p, purePre, allArgVals, allPost, paramIndexByID, mt.FunctionType().ParamTypes)
	if !ok {
		return false
	}
	if ctorStmtsMentionTypeParams(purePre, c.FuncCtx.TypeParams, 0) ||
		ctorValuesMentionTypeParams(allArgVals, c.FuncCtx.TypeParams, 0) {
		return false
	}
	for _, pt := range mt.FunctionType().ParamTypes {
		if ctorTypeMentionsTypeParams(pt, c.FuncCtx.TypeParams, 0) {
			return false
		}
	}
	for _, pt := range target.FunctionType().ParamTypes {
		if ctorTypeMentionsTypeParams(pt, c.FuncCtx.TypeParams, 0) {
			return false
		}
	}
	for _, ev := range escaped {
		if ctorTypeMentionsTypeParams(ev.typ, c.FuncCtx.TypeParams, 0) {
			return false
		}
	}

	emitArmReturn := len(postSuper) > 0
	var renderedArms []string
	for _, ap := range armPosts {
		s, okA := renderPackArmPostStmts(ap.stmts, ap.hasReturn, emitArmReturn, c.FuncCtx, cloneDeclaredMap(declared))
		if !okA {
			return false
		}
		renderedArms = append(renderedArms, s)
	}
	armSwitchNeeded := false
	hasAnyArmPost := false
	for idx, s := range renderedArms {
		if s != "" {
			hasAnyArmPost = true
		}
		if idx > 0 && s != renderedArms[0] {
			armSwitchNeeded = true
		}
	}

	// Mark hoisted escaped declarations as declared in dry-run helper so branch-local
	// escaped variables are assigned cleanly rather than redeclared per case.
	dryDecls := cloneDeclaredMap(declared)
	for _, ev := range escaped {
		if !ev.isParam && ev.name != "" {
			dryDecls[ev.name] = true
		}
	}

	// Transform branch arms: every arm path must terminate at a matching super/this call
	// (or throw) without falling out after the switch/if without returning Object[].
	var renderSwitchIfPrefix func(sts []statements.Statement, ctx *class_context.ClassContext, decls map[string]bool) (string, bool, bool)
	renderSwitchIfPrefix = func(sts []statements.Statement, ctx *class_context.ClassContext, decls map[string]bool) (string, bool, bool) {
		var b strings.Builder
		terminated := false
		for _, st := range sts {
			if st == nil {
				continue
			}
			if call, isInit := isCtorThisCall(st); isInit {
				argStrs := call.ArgumentStrings(ctx)
				if len(argStrs) != len(target.FunctionType().ParamTypes) {
					return "", false, false
				}
				elems := make([]string, 0, len(argStrs)+len(escaped)+1)
				for ai, as := range argStrs {
					elems = append(elems, packArgElementExpr(call.Arguments[ai], target.FunctionType().ParamTypes[ai], as, ctx))
				}
				for _, ev := range escaped {
					if ref, _ := p.params[ev.paramIndex].(*values.JavaRef); ev.isParam && ref != nil {
						elems = append(elems, ref.String(ctx))
					} else {
						elems = append(elems, ev.name)
					}
				}
				if armSwitchNeeded {
					elems = append(elems, fmt.Sprintf("Integer.valueOf(%d)", armIDByCall[call]))
				}
				b.WriteString("return new Object[]{ " + strings.Join(elems, ", ") + " };\n")
				terminated = true
				break
			}
			switch s := st.(type) {
			case *statements.IfStatement:
				if len(s.ElseBody) == 0 {
					return "", false, false
				}
				cond := values.SimplifyConditionValue(s.Condition).String(ctx)
				thenStr, thenTerm, okT := renderSwitchIfPrefix(s.IfBody, ctx, cloneDeclaredMap(decls))
				elseStr, elseTerm, okE := renderSwitchIfPrefix(s.ElseBody, ctx, cloneDeclaredMap(decls))
				if !okT || !okE || !thenTerm || !elseTerm {
					return "", false, false
				}
				b.WriteString("if (" + cond + ") {\n" + thenStr + "} else {\n" + elseStr + "}\n")
				terminated = true
			case *statements.SwitchStatement:
				selector, names, projected := s.SourceSelection(ctx)
				if !projected {
					selector = s.Value.String(ctx)
				}
				hasDefault := false
				var cb strings.Builder
				lastTerminated := false
				for ci := 0; ci < len(s.Cases); ci++ {
					item := s.Cases[ci]
					if item == nil {
						continue
					}
					if item.IsDefault {
						hasDefault = true
					}
					label := fmt.Sprintf("case %d:\n", item.IntValue)
					if item.IsDefault {
						label = "default:\n"
					} else if projected {
						label = "case " + names[item.IntValue] + ":\n"
					}
					if len(item.Body) == 0 {
						if ci == len(s.Cases)-1 {
							return "", false, false
						}
						cb.WriteString(label)
						lastTerminated = false
						continue
					}
					bodyStr, bodyTerm, okC := renderSwitchIfPrefix(item.Body, ctx, cloneDeclaredMap(decls))
					if !okC || !bodyTerm {
						return "", false, false
					}
					cb.WriteString(label + "{\n" + bodyStr + "}\n")
					lastTerminated = true
				}
				if !hasDefault || !lastTerminated {
					return "", false, false
				}
				b.WriteString("switch (" + selector + ") {\n" + cb.String() + "}\n")
				terminated = true
			default:
				if cs, ok := st.(*statements.CustomStatement); ok {
					if _, thrown := cs.SourceThrowOperand(); thrown {
						b.WriteString(st.String(ctx) + ";\n")
						terminated = true
						break
					}
				}
				one, okOne := renderRetainedPrefixStatements([]statements.Statement{st}, ctx, decls)
				if !okOne {
					return "", false, false
				}
				b.WriteString(one)
			}
			if terminated {
				break
			}
		}
		return b.String(), terminated, true
	}

	// Dry-run check on initial context.
	_, term, okR := renderSwitchIfPrefix(preOnly, c.FuncCtx, dryDecls)
	if !okR || !term {
		return false
	}

	helperName, err := c.constructorBoundaryHelperName("pack", first.OriginPC)
	if err != nil {
		return false
	}
	packParam := "jdec$a"
	for declared[packParam] {
		packParam += "$"
	}

	for _, st := range postSuper {
		as, _ := unwrapAssignStmtSimple(st)
		if as != nil && as.IsDeclare && as.JavaValue == nil && as.ArrayMember == nil {
			if ref, ok := values.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && escapedSet[ref.Id] {
				if p.foldedStatements == nil {
					p.foldedStatements = map[statements.Statement]bool{}
				}
				p.foldedStatements[st] = true
			}
		}
	}

	synthDelegate := &values.FunctionCallExpression{
		ClassName:       first.ClassName,
		FunctionName:    "<init>",
		Descriptor:      first.Descriptor,
		Kind:            values.InvokeSpecial,
		IsSpecialInvoke: true,
		Object:          &values.JavaRef{IsThis: true},
		Arguments:       slicesCloneValues(first.Arguments),
		OriginPC:        first.OriginPC,
		HasOriginPC:     true,
	}

	p.pc = first.OriginPC
	p.delegate = synthDelegate
	p.prefix = prefix
	p.packBranchPrefix = preOnly
	p.packCarrier = true
	p.retainCarrier = true
	p.packBranchDelegate = true
	p.packHelperName = helperName
	p.packParamName = packParam
	p.packMethod = method
	p.packMethodType = mt.FunctionType()
	p.packTargetParamTypes = target.FunctionType().ParamTypes
	p.packEscaped = escaped
	p.packEscapedByID = escapedSet
	if hasAnyArmPost {
		p.packArmPosts = armPosts
		p.packArmSwitchNeeded = armSwitchNeeded
		p.packArmEmitReturn = emitArmReturn
	}
	_ = code
	return true
}

func slicesCloneValues(src []values.JavaValue) []values.JavaValue {
	if len(src) == 0 {
		return nil
	}
	dst := make([]values.JavaValue, len(src))
	copy(dst, src)
	return dst
}

// buildPackHelper finalizes the private static Object[] pack helper using the
// method's final parameter/local names in funcCtx and appends it to p.helpers.
func (p *constructorSourceBoundary) buildPackHelper(c *ClassObjectDumper, funcCtx *class_context.ClassContext, escapedIDMap map[*coreutils.VariableId]bool) (string, bool) {
	if p == nil || !p.packCarrier || p.packMethodType == nil || funcCtx == nil {
		return "", false
	}
	declared := map[string]bool{}
	decls := make([]string, 0, len(p.params))
	for i, param := range p.params {
		ref, ok := param.(*values.JavaRef)
		if !ok || ref == nil {
			return "", false
		}
		name := ref.String(funcCtx)
		declared[name] = true
		decls = append(decls, p.packMethodType.ParamTypes[i].String(funcCtx)+" "+name)
	}

	// Render final escaped initialization lines to go right after super(...) in the carrier ctor.
	var escInit strings.Builder
	var hoistPackEscaped strings.Builder
	carrierDeclared := cloneDeclaredMap(declared)
	for k, ev := range p.packEscaped {
		idx := len(p.packTargetParamTypes) + k
		unpacked := unpackFromObjectArray(p.packParamName, idx, ev.typ, funcCtx)
		finalName := ev.name
		if ev.isParam {
			if ref, ok := p.params[ev.paramIndex].(*values.JavaRef); ok && ref != nil {
				finalName = ref.String(funcCtx)
			}
			escInit.WriteString(c.GetTabString() + finalName + " = " + unpacked + ";\n")
		} else {
			if n := funcCtx.LocalNames[ev.id]; n != "" {
				finalName = n
			} else if ev.id != nil && ev.id.String() != "" {
				finalName = ev.id.String()
			}
			escInit.WriteString(c.GetTabString() + ev.typ.String(funcCtx) + " " + finalName + " = " + unpacked + ";\n")
			carrierDeclared[finalName] = true
			if p.packBranchDelegate {
				declared[finalName] = true
				hoistPackEscaped.WriteString(ev.typ.String(funcCtx) + " " + finalName + " = " + ctorDefaultInitializer(ev.typ) + ";\n")
			}
		}
	}
	if len(p.packArmPosts) > 0 {
		if !p.packArmSwitchNeeded {
			rendered, okA := renderPackArmPostStmts(p.packArmPosts[0].stmts, p.packArmPosts[0].hasReturn, p.packArmEmitReturn, funcCtx, cloneDeclaredMap(carrierDeclared))
			if !okA {
				return "", false
			}
			for _, line := range strings.Split(strings.TrimSpace(rendered), "\n") {
				if line != "" {
					escInit.WriteString(c.GetTabString() + line + "\n")
				}
			}
		} else {
			armIdx := len(p.packTargetParamTypes) + len(p.packEscaped)
			escInit.WriteString(c.GetTabString() + fmt.Sprintf("switch (((Integer)(%s[%d])).intValue()) {\n", p.packParamName, armIdx))
			for _, ap := range p.packArmPosts {
				rendered, okA := renderPackArmPostStmts(ap.stmts, ap.hasReturn, p.packArmEmitReturn, funcCtx, cloneDeclaredMap(carrierDeclared))
				if !okA {
					return "", false
				}
				if strings.TrimSpace(rendered) == "" {
					continue
				}
				escInit.WriteString(c.GetTabString() + fmt.Sprintf("case %d: {\n", ap.armID))
				for _, line := range strings.Split(strings.TrimSpace(rendered), "\n") {
					if line != "" {
						escInit.WriteString(c.GetTabString() + "\t" + line + "\n")
					}
				}
				if !ap.hasReturn || !p.packArmEmitReturn {
					escInit.WriteString(c.GetTabString() + "\tbreak;\n")
				}
				escInit.WriteString(c.GetTabString() + "}\n")
			}
			escInit.WriteString(c.GetTabString() + "}\n")
		}
	}
	_ = escapedIDMap

	var bodyStr string
	if p.packBranchDelegate {
		isCtorThisCall := func(st statements.Statement) (*values.FunctionCallExpression, bool) {
			expr, ok := st.(*statements.ExpressionStatement)
			if !ok || expr == nil {
				return nil, false
			}
			call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
			if !ok || call == nil || call.FunctionName != "<init>" || call.Kind != values.InvokeSpecial {
				return nil, false
			}
			ref, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
			isThisRecv := ok && ref != nil && (ref.IsThis || (c.getenv("JDEC_THIS_ALIAS_SUPER_OFF") == "" && ref.IsThisAlias))
			return call, isThisRecv
		}
		armSeq := 0
		var renderSwitchIfFinal func(sts []statements.Statement, declMap map[string]bool) (string, bool)
		renderSwitchIfFinal = func(sts []statements.Statement, declMap map[string]bool) (string, bool) {
			var b strings.Builder
			for _, st := range sts {
				if st == nil {
					continue
				}
				if call, isInit := isCtorThisCall(st); isInit {
					argStrs := call.ArgumentStrings(funcCtx)
					if len(argStrs) != len(p.packTargetParamTypes) {
						return "", false
					}
					elems := make([]string, 0, len(argStrs)+len(p.packEscaped)+1)
					for ai, as := range argStrs {
						elems = append(elems, packArgElementExpr(call.Arguments[ai], p.packTargetParamTypes[ai], as, funcCtx))
					}
					for _, ev := range p.packEscaped {
						nm := ev.name
						if ev.isParam {
							if ref, ok := p.params[ev.paramIndex].(*values.JavaRef); ok && ref != nil {
								nm = ref.String(funcCtx)
							}
						} else if n := funcCtx.LocalNames[ev.id]; n != "" {
							nm = n
						} else if ev.id != nil && ev.id.String() != "" {
							nm = ev.id.String()
						}
						elems = append(elems, nm)
					}
					if p.packArmSwitchNeeded {
						elems = append(elems, fmt.Sprintf("Integer.valueOf(%d)", armSeq))
					}
					armSeq++
					b.WriteString("return new Object[]{ " + strings.Join(elems, ", ") + " };\n")
					break
				}
				switch s := st.(type) {
				case *statements.IfStatement:
					cond := values.SimplifyConditionValue(s.Condition).String(funcCtx)
					tStr, okT := renderSwitchIfFinal(s.IfBody, cloneDeclaredMap(declMap))
					eStr, okE := renderSwitchIfFinal(s.ElseBody, cloneDeclaredMap(declMap))
					if !okT || !okE {
						return "", false
					}
					b.WriteString("if (" + cond + ") {\n" + tStr + "} else {\n" + eStr + "}\n")
				case *statements.SwitchStatement:
					selector, names, projected := s.SourceSelection(funcCtx)
					if !projected {
						selector = s.Value.String(funcCtx)
					}
					var cb strings.Builder
					for _, item := range s.Cases {
						if item == nil {
							continue
						}
						label := fmt.Sprintf("case %d:\n", item.IntValue)
						if item.IsDefault {
							label = "default:\n"
						} else if projected {
							label = "case " + names[item.IntValue] + ":\n"
						}
						if len(item.Body) == 0 {
							cb.WriteString(label)
							continue
						}
						cStr, okC := renderSwitchIfFinal(item.Body, cloneDeclaredMap(declMap))
						if !okC {
							return "", false
						}
						cb.WriteString(label + "{\n" + cStr + "}\n")
					}
					b.WriteString("switch (" + selector + ") {\n" + cb.String() + "}\n")
				default:
					if cs, ok := st.(*statements.CustomStatement); ok {
						if _, thrown := cs.SourceThrowOperand(); thrown {
							b.WriteString(st.String(funcCtx) + ";\n")
							break
						}
					}
					one, okOne := renderRetainedPrefixStatements([]statements.Statement{st}, funcCtx, declMap)
					if !okOne {
						return "", false
					}
					b.WriteString(one)
				}
			}
			return b.String(), true
		}
		var okF bool
		branchInput := p.packBranchPrefix
		if len(branchInput) == 0 {
			branchInput = p.prefix
		}
		bodyStr, okF = renderSwitchIfFinal(branchInput, declared)
		if !okF {
			return "", false
		}
		bodyStr = hoistPackEscaped.String() + bodyStr
	} else {
		hoistSeen := map[string]bool{}
		var hoists strings.Builder
		for _, arg := range p.delegate.Arguments {
			hd, okH := collectRetainedPrefixHoistedDecls(p.prefix, arg, funcCtx, declared)
			if !okH {
				return "", false
			}
			for _, line := range strings.Split(strings.TrimSpace(hd), "\n") {
				if line != "" && !hoistSeen[line] {
					hoistSeen[line] = true
					hoists.WriteString(line + "\n")
				}
			}
		}
		pCode, okP := renderRetainedPrefixStatements(p.prefix, funcCtx, declared)
		if !okP {
			return "", false
		}
		argStrs := p.delegate.ArgumentStrings(funcCtx)
		if len(argStrs) != len(p.packTargetParamTypes) {
			return "", false
		}
		elems := make([]string, 0, len(argStrs)+len(p.packEscaped))
		for ai, as := range argStrs {
			elems = append(elems, packArgElementExpr(p.delegate.Arguments[ai], p.packTargetParamTypes[ai], as, funcCtx))
		}
		for _, ev := range p.packEscaped {
			nm := ev.name
			if ev.isParam {
				if ref, ok := p.params[ev.paramIndex].(*values.JavaRef); ok && ref != nil {
					nm = ref.String(funcCtx)
				}
			} else if n := funcCtx.LocalNames[ev.id]; n != "" {
				nm = n
			} else if ev.id != nil && ev.id.String() != "" {
				nm = ev.id.String()
			}
			elems = append(elems, nm)
		}
		bodyStr = hoists.String() + pCode + "return new Object[]{ " + strings.Join(elems, ", ") + " };"
	}

	wrappedBody := c.wrapCheckedEscapeBody(bodyStr)
	p.helpers = append(p.helpers, &dumpedMethods{
		methodName:    p.packHelperName,
		code:          "private static Object[] " + p.packHelperName + "(" + strings.Join(decls, ",") + ") {" + wrappedBody + "}",
		bodyCode:      wrappedBody,
		checkedEscape: true,
	})
	return escInit.String(), true
}

func (p *constructorSourceBoundary) packThisDelegationArgs(funcCtx *class_context.ClassContext) (packCall string, carrierFormalDecls string) {
	names := make([]string, 0, len(p.params))
	decls := make([]string, 0, len(p.params))
	for i, param := range p.params {
		nm := param.String(funcCtx)
		names = append(names, nm)
		if p.packMethodType != nil && i < len(p.packMethodType.ParamTypes) {
			decls = append(decls, p.packMethodType.ParamTypes[i].String(funcCtx)+" "+nm)
		}
	}
	packExpr := p.packHelperName + "(" + strings.Join(names, ",") + ")"
	if len(names) == 0 {
		return packExpr, "Object[] " + p.packParamName
	}
	return packExpr + ", " + strings.Join(names, ", "), "Object[] " + p.packParamName + ", " + strings.Join(decls, ", ")
}
