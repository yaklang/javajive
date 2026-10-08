package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	u "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
	"unicode"
)

type nativeAnonymousExpressionInitializer struct {
	method    *MemberInfo
	code      *CodeAttribute
	ops       []*core.OpCode
	start     int
	stores    map[int]string
	signature string
	byPC      map[int]*core.OpCode
	branches  map[int]*core.OpCode // exact reference producer PC -> null branch
	armValues map[int][2]int       // original fallthrough / target result producer
	events    []int                // structured original event stream
}

// Keep the entire original post-SUPER computation at its original position.
// Original frames check stack/local categories; source ASTs must
// subsequently retain every store, call, allocation and capture read once in
// bytecode order. A direct parameter read cannot stand for a mutable capture.
func nativeAnonymousExpressionInitializerProof(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, start int, child *nativeAnonymousClass, work *workbudget.Budget) *nativeAnonymousExpressionInitializer {
	if obj == nil || code == nil || child == nil || start < 0 || start >= len(ops) || len(ops) > 512 || len(code.ExceptionTable) != 0 {
		return nil
	}
	if ops[len(ops)-1] == nil || ops[len(ops)-1].Instr == nil || ops[len(ops)-1].Instr.OpCode != core.OP_RETURN {
		return nil
	}
	if work != nil && work.CheckAlloc(int64(len(ops)-start)*64) != nil {
		return nil
	}
	plan := &nativeAnonymousExpressionInitializer{code: code, ops: ops, start: start, stores: map[int]string{}, byPC: map[int]*core.OpCode{}}
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if n == "<init>" && d == child.descriptor {
			if plan.method != nil {
				return nil
			}
			plan.method = m
		}
	}
	if plan.method == nil {
		return nil
	}
	seenSignature := false
	for _, attribute := range plan.method.Attributes {
		if signature, ok := attribute.(*SignatureAttribute); ok {
			if signature == nil || seenSignature {
				return nil
			}
			seenSignature = true
			var valid bool
			plan.signature, valid = sourceBridgeUTF8(obj, signature.SignatureIndex)
			if !valid || !nativeProofWork(work, int64(len(plan.signature))) {
				return nil
			}
			formals, _, valid := types.SignatureTypeVariableReferences(plan.signature)
			_, _, result := types.ParseMethodSignatureFull(plan.signature, nil)
			// Anonymous source has no constructor declaration in which to bind
			// constructor-owned formals. Lexical enclosing formals remain valid.
			if !valid || len(formals) != 0 || result == nil || result.String(&class_context.ClassContext{}) != "void" {
				return nil
			}
		}
	}
	fields := map[string]*MemberInfo{}
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, ok := sourceBridgeUTF8(obj, f.NameIndex)
		if !ok || fields[n] != nil || f.AccessFlags&8 == 0 && fieldHasConstantValue(f) {
			return nil
		}
		fields[n] = f
	}
	used := map[string]bool{}
	for i := start; i < len(ops); i++ {
		op := ops[i]
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return nil
		}
		pc := int(op.CurrentOffset)
		if plan.byPC[pc] != nil {
			return nil
		}
		plan.byPC[pc] = op
		kind := op.Instr.OpCode
		if kind == core.OP_RETURN {
			if i != len(ops)-1 || len(plan.stores) == 0 {
				return nil
			}
			continue
		}
		// This capability admits expression packets and separately certified
		// diamonds, not arbitrary control flow or local declarations. The
		// original synthetic captures are read through GETFIELD.
		if slot := core.GetRetrieveIdx(op); slot >= 0 && (slot != 0 || !constructorMotionLoad(op, "Ljava/lang/Object;")) {
			return nil
		}
		switch kind {
		case core.OP_ISTORE, core.OP_ISTORE_0, core.OP_ISTORE_1, core.OP_ISTORE_2, core.OP_ISTORE_3, core.OP_LSTORE, core.OP_LSTORE_0, core.OP_LSTORE_1, core.OP_LSTORE_2, core.OP_LSTORE_3, core.OP_FSTORE, core.OP_FSTORE_0, core.OP_FSTORE_1, core.OP_FSTORE_2, core.OP_FSTORE_3, core.OP_DSTORE, core.OP_DSTORE_0, core.OP_DSTORE_1, core.OP_DSTORE_2, core.OP_DSTORE_3, core.OP_ASTORE, core.OP_ASTORE_0, core.OP_ASTORE_1, core.OP_ASTORE_2, core.OP_ASTORE_3, core.OP_IINC, core.OP_WIDE, core.OP_GOTO_W, core.OP_JSR, core.OP_JSR_W, core.OP_RET, core.OP_TABLESWITCH, core.OP_LOOKUPSWITCH, core.OP_ATHROW, core.OP_MONITORENTER, core.OP_MONITOREXIT, core.OP_PUTSTATIC, core.OP_INVOKEDYNAMIC, core.OP_IDIV, core.OP_LDIV, core.OP_IREM, core.OP_LREM, core.OP_IASTORE, core.OP_LASTORE, core.OP_FASTORE, core.OP_DASTORE, core.OP_AASTORE, core.OP_BASTORE, core.OP_CASTORE, core.OP_SASTORE:
			return nil
		}
		if kind >= core.OP_IFEQ && kind <= core.OP_IF_ACMPNE || kind >= core.OP_IRETURN && kind <= core.OP_ARETURN {
			return nil
		}
		if kind == core.OP_LDC || kind == core.OP_LDC_W || kind == core.OP_LDC2_W {
			descriptor, valid := constructorMotionLiteral(obj, op)
			if !valid {
				return nil
			}
			literal := nativeAnonymousInitializerLiteral(obj, op)
			// A general expression packet must retain the same bit-exact literal
			// proof as the simpler initializer path, including NaN payloads.
			if literal == nil || nativeAnonymousInitializerTypedLiteral(literal, descriptor, descriptor) == nil {
				return nil
			}
		}
		if kind == core.OP_NEWARRAY && (len(op.Data) != 1 || types.GetPrimerArrayType(int(op.Data[0])) == nil) {
			return nil
		}
		if kind == core.OP_GETFIELD {
			f := constructorMotionMember(obj, op, kind)
			if f == nil || class_context.SafeIdentifier(f.Member) != f.Member {
				return nil
			}
			if f.Name != obj.GetClassName() {
				// Original foreign declaration and exact source receiver type are
				// closed separately, before any source packet is admitted.
				continue
			}
			field := fields[f.Member]
			if field == nil || field.AccessFlags&8 != 0 || class_context.SafeIdentifier(f.Member) != f.Member {
				return nil
			}
			descriptor, known := sourceBridgeUTF8(obj, field.DescriptorIndex)
			if !known || descriptor != f.Description {
				return nil
			}
			if _, captured := child.fields[f.Member]; !captured && field.AccessFlags&0x1000 != 0 {
				return nil
			}
		}
		if kind != core.OP_PUTFIELD {
			continue
		}
		f := constructorMotionMember(obj, op, kind)
		if f == nil || f.Name != obj.GetClassName() {
			return nil
		}
		if _, captured := child.fields[f.Member]; captured {
			return nil
		}
		field := fields[f.Member]
		if field == nil || field.AccessFlags&(8|0x1000) != 0 || used[f.Member] && field.AccessFlags&0x10 != 0 || class_context.SafeIdentifier(f.Member) != f.Member {
			return nil
		}
		d, ok := sourceBridgeUTF8(obj, field.DescriptorIndex)
		if !ok || d != f.Description {
			return nil
		}
		// Stores are original events identified by PC, not one initializer per
		// field. A mutable/volatile field may be written repeatedly; the source
		// closure must retain every write and intermediate read in exact order.
		// Repeated final stores are valid in JVM constructors but cannot be
		// expressed by Java definite-assignment rules, so keep them refused.
		used[f.Member] = true
		plan.stores[pc] = f.Member
	}
	var closed bool
	plan.events, plan.branches, plan.armValues, closed = nativeAnonymousInitializerControlEvents(obj, ops, start, work)
	if !closed {
		return nil
	}
	d := NewClassObjectDumper(obj)
	d.Work = work
	if _, valid := d.nativeMemberAllocationInvocations(plan.method, code); !valid {
		return nil
	}
	return plan
}

func (c *ClassObjectDumper) nativeAnonymousExpressionInitializerSource(child *nativeAnonymousClass, bindings map[string]string) (string, bool) {
	plan := child.expressionInitializer
	if plan == nil || c.FuncCtx == nil {
		return "", false
	}
	names, known := nativeAnonymousInitializerFieldNames(child.object, c.nativeAnnotationDeclarationResolver(), c.Work)
	if !known {
		return "", false
	}
	for _, text := range bindings {
		if names[text] {
			return "", false
		}
	}
	mt, err := types.ParseMethodDescriptor(child.descriptor)
	if err != nil {
		return "", false
	}
	originalCtx, originalMethod, originalType := c.FuncCtx, c.CurrentMethod, c.MethodType
	copy := *originalCtx
	copy.LocalNames = map[*u.VariableId]string{}
	for id, name := range originalCtx.LocalNames {
		copy.LocalNames[id] = name
	}
	c.FuncCtx = &copy
	c.CurrentMethod = plan.method
	defer func() { c.FuncCtx = originalCtx; c.CurrentMethod = originalMethod; c.MethodType = originalType }()
	c.MethodType = mt.FunctionType()
	copy.FunctionName = "<init>"
	copy.CurrentMethodDesc = child.descriptor
	// DumpClass last rendered an ordinary body method. Its method-owned
	// formals cannot bind initializer expressions in the constructor scope.
	copy.CurrentMethodSig = plan.signature
	copy.FunctionType = c.MethodType
	copy.IsStatic = false
	copy.QualifiedStaticFields = true
	copy.RetainImplicitConstructorCalls = true
	c.wireNativeAnonymousSource()
	_, body, err := ParseBytesCode(c, plan.code, u.NewRootVariableId())
	if err != nil {
		return "", false
	}
	post := false
	stores := map[int]bool{}
	materialized := map[string]*values.JavaRef{}
	localNames, closedNames := nativeAnonymousInitializerReservedLocalNames(child.object, &copy, bindings, names, c.Work)
	if !closedNames {
		return "", false
	}
	localNumber := 0
	events := []int{}
	var source strings.Builder
	source.WriteString("{\n")
	for _, s := range body {
		if middle, ok := s.(*statements.MiddleStatement); ok && (middle.Flag == "start" || middle.Flag == "end") {
			continue
		}
		if !post {
			if expression, ok := s.(*statements.ExpressionStatement); ok {
				call, ok := values.UnpackSoltValue(expression.Expression).(*values.FunctionCallExpression)
				if !ok || !call.HasOriginPC || call.OriginPC != child.superPC || call.FunctionName != "<init>" {
					return "", false
				}
				post = true
				continue
			}
			assign, ok := s.(*statements.AssignStatement)
			if !ok || !assign.HasOriginPC {
				return "", false
			}
			matched := false
			for _, pc := range child.capturePCs {
				matched = matched || pc == assign.OriginPC
			}
			if !matched {
				return "", false
			}
			continue
		}
		if ret, ok := s.(*statements.ReturnStatement); ok {
			if ret.JavaValue != nil || !ret.HasOriginPC || ret.OriginPC != int(plan.ops[len(plan.ops)-1].CurrentOffset) {
				return "", false
			}
			continue
		}
		assign, ok := s.(*statements.AssignStatement)
		if !ok || assign.IsDeclare || assign.ArrayMember != nil || !assign.HasOriginPC || stores[assign.OriginPC] {
			return "", false
		}
		if ref, local := assign.LeftValue.(*values.JavaRef); local {
			if !assign.IsFirst || ref.VarUid == "" || materialized[ref.VarUid] != nil || !nativeAnonymousInitializerStackMaterialization(plan, assign, ref, c.Work) {
				return "", false
			}
			if !nativeAnonymousInitializerExpressionEventsWithMaterialized(child, plan, assign.JavaValue, &events, c.nativeAnnotationDeclarationResolver(), c.Work, materialized, c.FuncCtx) {
				return "", false
			}
			materialized[ref.VarUid] = ref
			for {
				localNumber++
				name := "$jdec$stack" + strconv.Itoa(localNumber)
				if !localNames[name] {
					copy.LocalNames[ref.Id] = name
					localNames[name] = true
					break
				}
			}
			source.WriteString(assign.String(c.FuncCtx) + ";\n")
			continue
		}
		field, ok := assign.LeftValue.(*values.RefMember)
		if !ok || field.Member != plan.stores[assign.OriginPC] {
			return "", false
		}
		ref, ok := values.UnpackSoltValue(field.Object).(*values.JavaRef)
		if !ok || !ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
			return "", false
		}
		if !nativeAnonymousInitializerExpressionEventsWithMaterialized(child, plan, assign.JavaValue, &events, c.nativeAnnotationDeclarationResolver(), c.Work, materialized, c.FuncCtx) {
			return "", false
		}
		events = append(events, assign.OriginPC)
		stores[assign.OriginPC] = true
		source.WriteString(assign.String(c.FuncCtx) + ";\n")
	}
	if !post {
		return "", false
	}
	// Embedded stores produce values as well as writes. Count every original
	// store event, including those nested in another field's RHS.
	for _, pc := range events {
		if _, isStore := plan.stores[pc]; isStore {
			stores[pc] = true
		}
	}
	if len(stores) != len(plan.stores) {
		return "", false
	}
	expected := plan.events
	if len(expected) != len(events) {
		return "", false
	}
	for i, pc := range expected {
		if pc != events[i] {
			return "", false
		}
	}
	source.WriteString("}\n")
	return source.String(), !c.nativeCaptureFailed
}

func nativeAnonymousInitializerExpressionEvents(child *nativeAnonymousClass, plan *nativeAnonymousExpressionInitializer, value values.JavaValue, events *[]int, resolve func(string) (*ClassObject, bool), work *workbudget.Budget, contexts ...*class_context.ClassContext) bool {
	return nativeAnonymousInitializerExpressionEventsWithMaterialized(child, plan, value, events, resolve, work, nil, contexts...)
}
func nativeAnonymousInitializerExpressionEventsWithMaterialized(child *nativeAnonymousClass, plan *nativeAnonymousExpressionInitializer, value values.JavaValue, events *[]int, resolve func(string) (*ClassObject, bool), work *workbudget.Budget, materialized map[string]*values.JavaRef, contexts ...*class_context.ClassContext) bool {
	var ctx *class_context.ClassContext
	if len(contexts) == 1 {
		ctx = contexts[0]
	}
	path := map[values.JavaValue]bool{}
	steps := 0
	byPC := plan.byPC
	callEvent := func(call *values.FunctionCallExpression) bool {
		if call == nil || !call.HasOriginPC {
			return false
		}
		op := byPC[call.OriginPC]
		if op == nil {
			return false
		}
		member := constructorMotionMember(child.object, op, op.Instr.OpCode)
		if member == nil || strings.ReplaceAll(call.ClassName, ".", "/") != member.Name || call.FunctionName != member.Member || call.Descriptor != member.Description {
			return false
		}
		kind, known := map[int]values.InvokeKind{core.OP_INVOKEVIRTUAL: values.InvokeVirtual, core.OP_INVOKESTATIC: values.InvokeStatic, core.OP_INVOKESPECIAL: values.InvokeSpecial, core.OP_INVOKEINTERFACE: values.InvokeInterface}[op.Instr.OpCode]
		if !known || call.Kind != kind {
			return false
		}
		*events = append(*events, call.OriginPC)
		return true
	}
	var visit func(values.JavaValue) bool
	visit = func(v values.JavaValue) bool {
		if sourceProofNil(v) {
			return true
		}
		if path[v] || steps >= 4096 || !nativeProofWork(work, 1) {
			return false
		}
		steps++
		path[v] = true
		defer delete(path, v)
		switch x := v.(type) {
		case *values.TernaryExpression:
			branch, fallthroughTrue := nativeAnonymousInitializerSourceBranch(plan, x.Condition)
			if branch == nil || !visit(x.Condition) {
				return false
			}
			pc := int(branch.CurrentOffset)
			yes, no := x.TrueValue, x.FalseValue
			if !fallthroughTrue {
				yes, no = no, yes
			}
			arms, known := plan.armValues[pc]
			if !known || !nativeAnonymousInitializerArmValue(yes, arms[0]) || !nativeAnonymousInitializerArmValue(no, arms[1]) {
				return false
			}
			*events = append(*events, nativeAnonymousInitializerBranchToken(pc, 0))
			if !visit(yes) {
				return false
			}
			*events = append(*events, nativeAnonymousInitializerBranchToken(pc, 1))
			if !visit(no) {
				return false
			}
			*events = append(*events, nativeAnonymousInitializerBranchToken(pc, 2))
			return true
		case *values.AssignmentExpression:
			if !nativeAnonymousInitializerOriginalStore(child, plan, x, work) {
				return false
			}
			field := x.Target.(*values.RefMember)
			// A write target is not a GETFIELD read. Evaluate its receiver,
			// then the checked RHS, then perform the original store exactly once.
			if !visit(field.Object) || !visit(x.Value) {
				return false
			}
			*events = append(*events, x.OriginPC)
			return true
		case *values.JavaRef:
			if original := materialized[x.VarUid]; nativeAnonymousInitializerMaterializedRead(x, original) {
				if ctx != nil && x.Id != nil {
					if ctx.LocalNames == nil {
						ctx.LocalNames = map[*u.VariableId]string{}
					}
					ctx.LocalNames[x.Id] = original.String(ctx)
				}
				return true
			}
			if !x.IsThis || x.CustomValue != nil || x.StackVar != nil {
				return false
			}
		case *values.NewExpression:
			if !x.HasOriginPC {
				return false
			}
			op := byPC[x.OriginPC]
			if op == nil {
				return false
			}
			if op.Instr.OpCode == core.OP_NEW {
				if x.ConstructorCall == nil {
					return false
				}
				owner, ok := sourceBridgeClassName(child.object, core.Convert2bytesToInt(op.Data))
				if !ok || strings.ReplaceAll(x.ConstructorCall.ClassName, ".", "/") != owner {
					return false
				}
				// Object allocation precedes evaluation of its constructor arguments.
				*events = append(*events, x.OriginPC)
			} else if !nativeAnonymousInitializerArrayAllocation(child.object, op, x) {
				return false
			}
		}

		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, next := range children {
			if !visit(next) {
				return false
			}
		}
		switch x := v.(type) {
		case *values.JavaClassMember:
			if !x.HasOriginPC {
				return false
			}
			op := byPC[x.OriginPC]
			if op == nil {
				return false
			}
			original := constructorMotionMember(child.object, op, core.OP_GETSTATIC)
			if original == nil || original.Name != x.Name || original.Member != x.Member || original.Description != x.Description || !nativeAnonymousInitializerFieldDeclaration(original, true, resolve, work) {
				return false
			}
			*events = append(*events, x.OriginPC)
		case *values.RefMember:
			if !x.HasOriginPC {
				return false
			}
			op := byPC[x.OriginPC]
			if op == nil {
				return false
			}
			f := constructorMotionMember(child.object, op, core.OP_GETFIELD)
			if f == nil || f.Member != x.Member {
				return false
			}
			if f.Name == child.object.GetClassName() {
				r, ok := values.UnpackSoltValue(x.Object).(*values.JavaRef)
				if !ok || !r.IsThis {
					return false
				}
			} else if !nativeAnonymousInitializerForeignField(f, x.Object, resolve, work) {
				return false
			}
			*events = append(*events, x.OriginPC)
		case *values.FunctionCallExpression:
			if !callEvent(x) {
				return false
			}
		case *values.NewExpression:
			if byPC[x.OriginPC].Instr.OpCode == core.OP_NEW {
				if !callEvent(x.ConstructorCall) {
					return false
				}
			} else {
				// Array dimensions are evaluated before allocation or its failure.
				*events = append(*events, x.OriginPC)
			}
		case *values.CastExpression:
			if x.OriginalCheckCast {
				if !nativeAnonymousInitializerOriginalCast(child.object, byPC[x.OriginPC], x, ctx, work) {
					return false
				}
				*events = append(*events, x.OriginPC)
			}
		case *values.JavaArrayMember:
			if !x.HasOriginPC || !nativeAnonymousInitializerArrayRead(byPC[x.OriginPC], x) {
				return false
			}
			*events = append(*events, x.OriginPC)
		case *values.ArrayLengthExpression:
			if !x.HasOriginPC || byPC[x.OriginPC] == nil || byPC[x.OriginPC].Instr.OpCode != core.OP_ARRAYLENGTH {
				return false
			}
			*events = append(*events, x.OriginPC)
		}
		return true
	}
	return visit(value)
}

// Source field selection uses the receiver's static type. Keep it equal to the
// original symbolic owner and require its actual declaration, so hidden fields,
// constant-variable inlining and guessed generic names cannot change the read.
func nativeAnonymousInitializerForeignField(field *values.JavaClassMember, receiver values.JavaValue, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if field == nil || sourceProofNil(receiver) || receiver.Type() == nil || receiver.Type().IsArray() || resolve == nil || !nativeProofWork(work, 1) {
		return false
	}
	owner, known := receiver.Type().RawType().(*types.JavaClass)
	if !known || strings.ReplaceAll(owner.Name, ".", "/") != field.Name {
		return false
	}
	return nativeAnonymousInitializerFieldDeclaration(field, false, resolve, work)
}

// A source field read must select the actual symbolic declaring class. Refuse
// constant variables, which javac can inline and thereby skip initialization,
// and require each original read PC to remain in the expression event stream.
func nativeAnonymousInitializerFieldDeclaration(field *values.JavaClassMember, static bool, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if field == nil || resolve == nil || class_context.SafeIdentifier(field.Member) != field.Member || !nativeProofWork(work, 1) {
		return false
	}
	object, known := resolve(field.Name)
	if !known || object == nil || object.GetClassName() != field.Name {
		return false
	}
	var declaration *MemberInfo
	for _, candidate := range object.Fields {
		if candidate == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, valid := sourceBridgeUTF8(object, candidate.NameIndex)
		if !valid {
			return false
		}
		if name != field.Member {
			continue
		}
		if declaration != nil {
			return false
		}
		declaration = candidate
	}
	if declaration == nil || (declaration.AccessFlags&8 != 0) != static || declaration.AccessFlags&(2|0x1000) != 0 || fieldHasConstantValue(declaration) {
		return false
	}
	descriptor, valid := sourceBridgeUTF8(object, declaration.DescriptorIndex)
	return valid && descriptor == field.Description
}

func nativeAnonymousInitializerArrayAllocation(object *ClassObject, op *core.OpCode, value *values.NewExpression) bool {
	if value.ConstructorCall != nil || value.ArgumentsGetter != nil || len(value.Initializer) != 0 {
		return false
	}
	var expected types.JavaType
	dimensions := 1
	switch op.Instr.OpCode {
	case core.OP_NEWARRAY:
		if len(op.Data) != 1 {
			return false
		}
		element := types.GetPrimerArrayType(int(op.Data[0]))
		if element == nil {
			return false
		}
		expected = types.NewJavaArrayType(element)
	case core.OP_ANEWARRAY:
		if len(op.Data) != 2 {
			return false
		}
		name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
		if !known {
			return false
		}
		var element types.JavaType
		if strings.HasPrefix(name, "[") {
			var err error
			element, err = types.ParseDescriptor(name)
			if err != nil {
				return false
			}
		} else {
			element = types.NewJavaClass(name)
		}
		expected = types.NewJavaArrayType(element)
	case core.OP_MULTIANEWARRAY:
		if len(op.Data) != 3 {
			return false
		}
		name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data[:2]))
		if !known || !strings.HasPrefix(name, "[") {
			return false
		}
		var err error
		expected, err = types.ParseDescriptor(name)
		if err != nil {
			return false
		}
		dimensions = int(op.Data[2])
		if dimensions < 1 || dimensions > expected.ArrayDim() {
			return false
		}
	default:
		return false
	}
	// Every dimension is evaluated before the single allocation instruction.
	// Zero outer dimensions still evaluate and check negative inner dimensions.
	return len(value.Length) == dimensions && nativeAnonymousInitializerSameArrayType(value.Type(), expected)
}

func nativeAnonymousInitializerSameArrayType(actual, expected types.JavaType) bool {
	if actual == nil || expected == nil || !actual.IsArray() || !expected.IsArray() {
		return false
	}
	a, ak := actual.RawType().(*types.JavaArrayType)
	b, bk := expected.RawType().(*types.JavaArrayType)
	if !ak || !bk || a == nil || b == nil || a.Dimension < 1 || a.Dimension > 255 || a.Dimension != b.Dimension || a.JavaType == nil || b.JavaType == nil {
		return false
	}
	switch t := a.JavaType.RawType().(type) {
	case *types.JavaPrimer:
		r, ok := b.JavaType.RawType().(*types.JavaPrimer)
		return ok && t != nil && r != nil && t.Name == r.Name
	case *types.JavaClass:
		r, ok := b.JavaType.RawType().(*types.JavaClass)
		return ok && t != nil && r != nil && strings.ReplaceAll(t.Name, ".", "/") == strings.ReplaceAll(r.Name, ".", "/")
	default:
		return false
	}
}

// Source indexing evaluates the array and index once, then performs the same
// null/bounds check and typed JVM load. Keep this operation's original PC after
// both operands; a store, an invented access or a mismatched component refuses.
func nativeAnonymousInitializerArrayRead(op *core.OpCode, value *values.JavaArrayMember) bool {
	if op == nil || op.Instr == nil || len(op.Data) != 0 || value == nil || sourceProofNil(value.Object) || sourceProofNil(value.Index) || value.Object.Type() == nil || value.Index.Type() == nil {
		return false
	}
	array := value.Object.Type()
	if !array.IsArray() || array.ArrayDim() < 1 || array.ArrayDim() > 255 || array.ElementType() == nil {
		return false
	}
	index, known := value.Index.Type().RawType().(*types.JavaPrimer)
	if !known || index.Name != types.JavaInteger && index.Name != types.JavaByte && index.Name != types.JavaShort && index.Name != types.JavaChar {
		return false
	}
	element := array.ElementType()
	if op.Instr.OpCode == core.OP_AALOAD {
		_, reference := element.RawType().(*types.JavaClass)
		return element.IsArray() || reference
	}
	primitive, known := element.RawType().(*types.JavaPrimer)
	if !known {
		return false
	}
	expected, known := map[int]string{core.OP_IALOAD: types.JavaInteger, core.OP_LALOAD: types.JavaLong, core.OP_FALOAD: types.JavaFloat, core.OP_DALOAD: types.JavaDouble, core.OP_CALOAD: types.JavaChar, core.OP_SALOAD: types.JavaShort}[op.Instr.OpCode]
	if op.Instr.OpCode == core.OP_BALOAD {
		return primitive.Name == types.JavaByte || primitive.Name == types.JavaBoolean
	}
	return known && primitive.Name == expected
}

// The source cast must retain the actual CHECKCAST target and exception point.
// Generic formals can shadow a class spelling even when its binary target was
// correct, so close the rendered type head over the actual lexical signatures.
func nativeAnonymousInitializerOriginalCast(object *ClassObject, op *core.OpCode, value *values.CastExpression, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if object == nil || op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_CHECKCAST || len(op.Data) != 2 || value == nil || ctx == nil || !nativeProofWork(work, 1) {
		return false
	}
	pc, descriptor, known := value.OriginalCheckCastWitness(ctx)
	if !known || pc != int(op.CurrentOffset) {
		return false
	}
	name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
	if !known {
		return false
	}
	expected := name
	if !strings.HasPrefix(name, "[") {
		expected = "L" + name + ";"
	}
	if expected != descriptor {
		return false
	}
	source := value.TargetType.String(ctx)
	head := source
	if end := strings.IndexAny(head, ".<[ "); end >= 0 {
		head = head[:end]
	}
	if head == "" {
		return false
	}
	signatures := append([]string{ctx.ClassSig, ctx.CurrentMethodSig}, ctx.LexicalTypeParamSignatures...)
	for _, signature := range signatures {
		if signature == "" {
			continue
		}
		if !nativeProofWork(work, int64(len(signature))) {
			return false
		}
		formals, _, valid := types.SignatureTypeVariableReferences(signature)
		if !valid {
			return false
		}
		for _, formal := range formals {
			if formal == head {
				return false
			}
		}
	}
	return true
}

func nativeAnonymousInitializerOriginalStore(child *nativeAnonymousClass, plan *nativeAnonymousExpressionInitializer, value *values.AssignmentExpression, work *workbudget.Budget) bool {
	if child == nil || child.object == nil || plan == nil || value == nil || !nativeProofWork(work, 1) {
		return false
	}
	pc, owner, name, descriptor, known := value.OriginalFieldStoreWitness()
	if !known || plan.stores[pc] != name || owner != child.object.GetClassName() {
		return false
	}
	original := constructorMotionMember(child.object, plan.byPC[pc], core.OP_PUTFIELD)
	if original == nil || original.Name != owner || original.Member != name || original.Description != descriptor {
		return false
	}
	field, ok := value.Target.(*values.RefMember)
	if !ok || field == nil || field.Member != name || field.Type() == nil {
		return false
	}
	receiver, ok := values.UnpackSoltValue(field.Object).(*values.JavaRef)
	if !ok || receiver == nil || !receiver.IsThis || receiver.CustomValue != nil || receiver.StackVar != nil {
		return false
	}
	expected, err := types.ParseDescriptor(descriptor)
	if err != nil || expected == nil {
		return false
	}
	if expected.IsArray() {
		return nativeAnonymousInitializerSameArrayType(field.Type(), expected)
	}
	switch typ := expected.RawType().(type) {
	case *types.JavaPrimer:
		actual, ok := field.Type().RawType().(*types.JavaPrimer)
		return ok && typ != nil && actual != nil && typ.Name == actual.Name
	case *types.JavaClass:
		actual, ok := field.Type().RawType().(*types.JavaClass)
		return ok && typ != nil && actual != nil && strings.ReplaceAll(typ.Name, ".", "/") == strings.ReplaceAll(actual.Name, ".", "/")
	}
	return false
}

func nativeAnonymousInitializerStackMaterialization(plan *nativeAnonymousExpressionInitializer, assign *statements.AssignStatement, ref *values.JavaRef, work *workbudget.Budget) bool {
	if plan == nil || assign == nil || ref == nil || !assign.HasOriginPC || !assign.IsFirst || assign.IsDeclare || assign.ArrayMember != nil || assign.LeftValue != ref || !nativeProofWork(work, 1) {
		return false
	}
	pc, kind, known := ref.OriginalStackMaterializationWitness(assign.JavaValue)
	if !known || pc != assign.OriginPC {
		return false
	}
	op := plan.byPC[pc]
	if op == nil || op.Instr == nil || op.IsCustom || len(op.Data) != 0 || kind != op.Instr.OpCode {
		return false
	}
	switch kind {
	case core.OP_DUP, core.OP_DUP_X1, core.OP_DUP_X2, core.OP_DUP2, core.OP_DUP2_X1, core.OP_DUP2_X2:
		return true
	}
	return false
}

func nativeAnonymousInitializerMaterializedRead(value, original *values.JavaRef) bool {
	if value == nil || original == nil || value.VarUid == "" || value.VarUid != original.VarUid || value.Id == nil || original.Id == nil || value.Id != original.Id {
		return false
	}
	// A shallow reference clone retains the declaration's source type view.
	// A separately rebound view could select another overload at a later use.
	if value.Type().GetJavaTypeRef() != original.Type().GetJavaTypeRef() || (value.WebDeclType == nil) != (original.WebDeclType == nil) {
		return false
	}
	if value.WebDeclType != nil && value.WebDeclType.GetJavaTypeRef() != original.WebDeclType.GetJavaTypeRef() {
		return false
	}
	pc, kind, known := value.OriginalStackMaterializationWitness(original.Val)
	originalPC, originalKind, originalKnown := original.OriginalStackMaterializationWitness(original.Val)
	return known && originalKnown && pc == originalPC && kind == originalKind
}

// Generated locals share the lexical scope of projected capture expressions.
// Reserve capture/member/type/formal symbols before choosing a fresh name.
func nativeAnonymousInitializerReservedLocalNames(object *ClassObject, ctx *class_context.ClassContext, bindings map[string]string, fields map[string]bool, work *workbudget.Budget) (map[string]bool, bool) {
	if object == nil {
		return nil, false
	}
	names := map[string]bool{}
	valid := true
	reserve := func(text string) {
		if !valid || !nativeProofWork(work, int64(len(text)+1)) {
			valid = false
			return
		}
		names[text] = true
		names[class_context.SafeIdentifier(text)] = true
		for _, part := range strings.FieldsFunc(text, func(r rune) bool {
			// '$' is part of a Java identifier, including the generated names.
			// Capture bindings can be qualified or parenthesized expressions.
			return r != '$' && r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			names[part] = true
			names[class_context.SafeIdentifier(part)] = true
		}
	}
	for name := range fields {
		reserve(name)
	}
	for _, text := range bindings {
		reserve(text)
	}
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if c, ok := constant.(*ConstantClassInfo); ok {
			if c == nil {
				return nil, false
			}
			if name, known := sourceBridgeUTF8(object, c.NameIndex); known {
				reserve(name)
			} else {
				return nil, false
			}
		}
	}
	if ctx != nil {
		for _, signature := range append([]string{ctx.ClassSig, ctx.CurrentMethodSig}, ctx.LexicalTypeParamSignatures...) {
			if !nativeProofWork(work, int64(len(signature)+1)) {
				return nil, false
			}
			formals, _, valid := types.SignatureTypeVariableReferences(signature)
			if signature != "" && !valid {
				return nil, false
			}
			if valid {
				for _, name := range formals {
					reserve(name)
				}
			}
		}
		for _, name := range ctx.LocalNames {
			reserve(name)
		}
	}
	return names, valid
}
