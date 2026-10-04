package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	u "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

type nativeAnonymousExpressionInitializer struct {
	method    *MemberInfo
	code      *CodeAttribute
	ops       []*core.OpCode
	start     int
	stores    map[int]string
	signature string
	byPC      map[int]*core.OpCode
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
		// This capability admits expression packets, not local declarations or
		// control flow. The original synthetic captures are read through GETFIELD.
		if slot := core.GetRetrieveIdx(op); slot >= 0 && (slot != 0 || !constructorMotionLoad(op, "Ljava/lang/Object;")) {
			return nil
		}
		switch kind {
		case core.OP_ISTORE, core.OP_ISTORE_0, core.OP_ISTORE_1, core.OP_ISTORE_2, core.OP_ISTORE_3, core.OP_LSTORE, core.OP_LSTORE_0, core.OP_LSTORE_1, core.OP_LSTORE_2, core.OP_LSTORE_3, core.OP_FSTORE, core.OP_FSTORE_0, core.OP_FSTORE_1, core.OP_FSTORE_2, core.OP_FSTORE_3, core.OP_DSTORE, core.OP_DSTORE_0, core.OP_DSTORE_1, core.OP_DSTORE_2, core.OP_DSTORE_3, core.OP_ASTORE, core.OP_ASTORE_0, core.OP_ASTORE_1, core.OP_ASTORE_2, core.OP_ASTORE_3, core.OP_IINC, core.OP_WIDE, core.OP_GOTO, core.OP_GOTO_W, core.OP_JSR, core.OP_JSR_W, core.OP_RET, core.OP_TABLESWITCH, core.OP_LOOKUPSWITCH, core.OP_ATHROW, core.OP_MONITORENTER, core.OP_MONITOREXIT, core.OP_PUTSTATIC, core.OP_GETSTATIC, core.OP_INVOKEDYNAMIC, core.OP_IDIV, core.OP_LDIV, core.OP_IREM, core.OP_LREM, core.OP_CHECKCAST, core.OP_IALOAD, core.OP_LALOAD, core.OP_FALOAD, core.OP_DALOAD, core.OP_AALOAD, core.OP_BALOAD, core.OP_CALOAD, core.OP_SALOAD, core.OP_IASTORE, core.OP_LASTORE, core.OP_FASTORE, core.OP_DASTORE, core.OP_AASTORE, core.OP_BASTORE, core.OP_CASTORE, core.OP_SASTORE, core.OP_MULTIANEWARRAY:
			return nil
		}
		if kind >= core.OP_IFEQ && kind <= core.OP_IF_ACMPNE || kind == core.OP_IFNULL || kind == core.OP_IFNONNULL || kind >= core.OP_IRETURN && kind <= core.OP_ARETURN {
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
		if field == nil || field.AccessFlags&(8|0x1000) != 0 || used[f.Member] || class_context.SafeIdentifier(f.Member) != f.Member {
			return nil
		}
		d, ok := sourceBridgeUTF8(obj, field.DescriptorIndex)
		if !ok || d != f.Description {
			return nil
		}
		used[f.Member] = true
		plan.stores[pc] = f.Member
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
	c.wireNativeAnonymousSource()
	_, body, err := ParseBytesCode(c, plan.code, u.NewRootVariableId())
	if err != nil {
		return "", false
	}
	post := false
	stores := map[int]bool{}
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
		field, ok := assign.LeftValue.(*values.RefMember)
		if !ok || field.Member != plan.stores[assign.OriginPC] {
			return "", false
		}
		ref, ok := values.UnpackSoltValue(field.Object).(*values.JavaRef)
		if !ok || !ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
			return "", false
		}
		if !nativeAnonymousInitializerExpressionEvents(child, plan, assign.JavaValue, &events, c.nativeAnnotationDeclarationResolver(), c.Work) {
			return "", false
		}
		events = append(events, assign.OriginPC)
		stores[assign.OriginPC] = true
		source.WriteString(assign.String(c.FuncCtx) + ";\n")
	}
	if !post || len(stores) != len(plan.stores) {
		return "", false
	}
	expected := []int{}
	for _, op := range plan.ops[plan.start:] {
		k := op.Instr.OpCode
		if k == core.OP_PUTFIELD || k == core.OP_GETFIELD || k == core.OP_NEW || k == core.OP_NEWARRAY || k == core.OP_ANEWARRAY || k == core.OP_ARRAYLENGTH || k >= core.OP_INVOKEVIRTUAL && k <= core.OP_INVOKEINTERFACE {
			expected = append(expected, int(op.CurrentOffset))
		}
	}
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

func nativeAnonymousInitializerExpressionEvents(child *nativeAnonymousClass, plan *nativeAnonymousExpressionInitializer, value values.JavaValue, events *[]int, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
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
		case *values.JavaRef:
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
	if declaration == nil || declaration.AccessFlags&(8|2|0x1000) != 0 || fieldHasConstantValue(declaration) {
		return false
	}
	descriptor, valid := sourceBridgeUTF8(object, declaration.DescriptorIndex)
	return valid && descriptor == field.Description
}

func nativeAnonymousInitializerArrayAllocation(object *ClassObject, op *core.OpCode, value *values.NewExpression) bool {
	if value.ConstructorCall != nil || value.ArgumentsGetter != nil || len(value.Initializer) != 0 || len(value.Length) != 1 {
		return false
	}
	var element types.JavaType
	switch op.Instr.OpCode {
	case core.OP_NEWARRAY:
		if len(op.Data) != 1 {
			return false
		}
		element = types.GetPrimerArrayType(int(op.Data[0]))
	case core.OP_ANEWARRAY:
		if len(op.Data) != 2 {
			return false
		}
		name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
		if !known {
			return false
		}
		if strings.HasPrefix(name, "[") {
			var err error
			element, err = types.ParseDescriptor(name)
			if err != nil {
				return false
			}
		} else {
			element = types.NewJavaClass(name)
		}
	default:
		return false
	}
	if element == nil {
		return false
	}
	expected := types.NewJavaArrayType(element)
	return nativeAnonymousInitializerSameArrayType(value.Type(), expected)
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
