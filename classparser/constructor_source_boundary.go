package javaclassparser

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// constructorSourceBoundary is a source-only plan. It never changes an original
// constructor, descriptor, declaration of throws, or bytecode value graph.
// Prefix effects are either kept behind a proved inert Object initialization or
// evaluated in the first delegation argument, before any effect of the callee.
type constructorSourceBoundary struct {
	pc          int
	delegate    *values.FunctionCallExpression
	prefix      []statements.Statement
	params      []values.JavaValue
	carrier     string
	bridgeNames map[int]string
	helpers     []*dumpedMethods
}

func constructorBoundaryValue(v values.JavaValue, allowed map[*values.JavaRef]bool, active map[values.JavaValue]bool, calls map[int]*values.FunctionCallExpression, remaining *int) bool {
	if v == nil || reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil() {
		return false
	}
	*remaining--
	if *remaining < 0 || active[v] {
		return false
	}
	active[v] = true
	defer delete(active, v)
	if ref, ok := v.(*values.JavaRef); ok {
		return ref != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil && allowed[ref]
	}
	if call, ok := v.(*values.FunctionCallExpression); ok {
		if call == nil || !call.HasOriginPC || call.OriginPC < 0 || call.FunctionName == "<init>" || call.Descriptor == "" || call.Kind == values.InvokeDynamic || call.Kind == values.InvokeSpecial {
			return false
		}
		if !call.IsStatic {
			ref, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
			if !ok || ref == nil || !allowed[ref] || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
				return false
			}
		}
		if prior := calls[call.OriginPC]; prior != nil {
			return false
		}
		calls[call.OriginPC] = call
	}
	children, known := values.Children(v)
	if call, ok := v.(*values.FunctionCallExpression); ok && call.IsStatic {
		children = call.Arguments
	}
	if !known {
		return false
	}
	for _, child := range children {
		if !constructorBoundaryValue(child, allowed, active, calls, remaining) {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) planConstructorSourceBoundary(code *CodeAttribute, body []statements.Statement, params []values.JavaValue, method *MemberInfo) (*constructorSourceBoundary, error) {
	name, _ := c.obj.getUtf8(method.NameIndex)
	if name != "<init>" {
		return nil, nil
	}
	p := &constructorSourceBoundary{pc: -1, bridgeNames: map[int]string{}}
	for _, param := range params {
		if ref, ok := param.(*values.JavaRef); ok && ref.IsThis {
			continue
		}
		p.params = append(p.params, param)
	}
	for i, st := range body {
		expr, ok := st.(*statements.ExpressionStatement)
		if !ok || expr == nil {
			continue
		}
		call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
		if !ok || call == nil || call.FunctionName != "<init>" || call.Kind != values.InvokeSpecial || !call.HasOriginPC {
			continue
		}
		ref, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
		if !ok || ref == nil || !ref.IsThis {
			continue
		}
		if p.delegate != nil {
			return nil, fmt.Errorf("ambiguous constructor initialization")
		}
		p.pc, p.delegate = call.OriginPC, call
		for _, prefix := range body[:i] {
			if middle, ok := prefix.(*statements.MiddleStatement); ok && middle != nil && middle.Data == nil {
				continue
			}
			p.prefix = append(p.prefix, prefix)
		}
	}
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	if err := decoder.ParseOpcode(); err != nil {
		return nil, err
	}
	// The simulator omits Object(), including one preceded by parameter checks.
	// Recover only its exact ALOAD_0/invokespecial ()V pair. No arbitrary no-arg
	// superclass is inferred to be inert or initialized here.
	if p.delegate == nil && c.obj.GetSupperClassName() == "java/lang/Object" {
		ops := decoder.Opcodes()
		for i, op := range ops {
			if i == 0 || op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_INVOKESPECIAL || len(op.Data) < 2 {
				continue
			}
			previous := ops[i-1]
			if previous == nil || previous.Instr == nil || previous.Instr.OpCode != core.OP_ALOAD_0 {
				continue
			}
			member, ok := GetValueFromCP(c.ConstantPool, int(core.Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
			if !ok || member == nil || strings.ReplaceAll(member.Name, ".", "/") != "java/lang/Object" || member.Member != "<init>" || member.Description != "()V" {
				continue
			}
			if p.pc >= 0 {
				return nil, fmt.Errorf("ambiguous implicit constructor initialization")
			}
			p.pc = int(op.CurrentOffset)
		}
		if p.pc >= 0 {
			exceptions, known := exactInvocationExceptions(c.FuncCtx.InvocationMetadata, "java/lang/Object", "<init>", "()V")
			if !known || len(exceptions) != 0 {
				return nil, fmt.Errorf("implicit constructor requires complete inert Object declaration")
			}
			// Retain every prefix statement. Object initialization cannot invoke user
			// code or initialize another class; the JVM already allocated this receiver.
			return p, nil
		}
	}
	if p.pc < 0 {
		// No-arg super calls may be omitted by the simulator even when the
		// superclass observes this via virtual dispatch. An earlier capture
		// write cannot silently move behind that implicit Java source call.
		if p.delegate == nil && c.obj.GetSupperClassName() != "java/lang/Object" {
			ops := constructorMotionOps(decoder)
			for i, op := range ops {
				member := constructorMotionMember(c.obj, op, core.OP_INVOKESPECIAL)
				if member != nil && member.Name == c.obj.GetSupperClassName() && member.Member == "<init>" && member.Description == "()V" && i > 0 && core.GetRetrieveIdx(ops[i-1]) == 0 && constructorMotionLoad(ops[i-1], "Ljava/lang/Object;") && i > 1 {
					// Recover only an exact leading sequence of capture triples.
					// The common proof below binds each source assignment to its
					// original parameter slot, field and pre-delegation position.
					count := (i - 1) / 3
					if (i-1)%3 == 0 && count > 0 {
						for _, st := range body {
							if middle, ok := st.(*statements.MiddleStatement); ok && middle != nil && middle.Data == nil {
								continue
							}
							if len(p.prefix) == count {
								break
							}
							p.prefix = append(p.prefix, st)
						}
						p.pc = int(op.CurrentOffset)
						p.delegate = &values.FunctionCallExpression{ClassName: member.Name, FunctionName: "<init>", Descriptor: "()V", Kind: values.InvokeSpecial, IsSpecialInvoke: true, Object: &values.JavaRef{IsThis: true}, OriginPC: p.pc, HasOriginPC: true}
						if len(p.prefix) == count && c.constructorCapturesCommute(p, code, method, decoder) {
							return p, nil
						}
					}
					return nil, fmt.Errorf("implicit constructor prefix lacks receiver-observation proof")
				}
			}
		}
		return p, nil
	}
	if strings.ReplaceAll(p.delegate.ClassName, ".", "/") != c.obj.GetClassName() && strings.ReplaceAll(p.delegate.ClassName, ".", "/") != c.obj.GetSupperClassName() {
		return nil, fmt.Errorf("foreign constructor initialization")
	}
	if !constructorOriginalInvoke(decoder, p.delegate, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) }) {
		return nil, fmt.Errorf("constructor delegation origin mismatch")
	}
	if len(p.prefix) == 0 {
		// A checked producer in a delegation argument needs its own exact source
		// bridge. Its receiver and arguments remain outside the helper's catch.
		if err := c.planConstructorArgumentBridges(p, code, method); err != nil {
			return nil, err
		}
		return p, nil
	}
	if c.constructorCapturesCommute(p, code, method, decoder) {
		return p, nil
	}
	// A prefix cannot cross an original exception domain. The bounded carrier
	// admits straight-line expression effects and original parameter uses only;
	// local definitions surviving the delegation require a different proof.
	if len(code.ExceptionTable) != 0 || len(p.delegate.Arguments) == 0 || c.isGenuineEnum() {
		return nil, fmt.Errorf("unsupported protected or zero-argument constructor prefix")
	}
	allowed := map[*values.JavaRef]bool{}
	for _, param := range p.params {
		ref, ok := param.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || ref.IsThis {
			return nil, fmt.Errorf("unproved constructor parameter identity")
		}
		allowed[ref] = true
	}
	calls := map[int]*values.FunctionCallExpression{}
	remaining := 1024
	last := -1
	for _, st := range p.prefix {
		expr, ok := st.(*statements.ExpressionStatement)
		if !ok || expr == nil {
			return nil, fmt.Errorf("constructor prefix has escaping local or control flow")
		}
		call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
		if !ok || call == nil || !call.HasOriginPC || call.OriginPC <= last || call.OriginPC >= p.pc || !constructorBoundaryValue(expr.Expression, allowed, map[values.JavaValue]bool{}, calls, &remaining) {
			return nil, fmt.Errorf("unproved constructor prefix effect ordering")
		}
		last = call.OriginPC
	}
	for _, arg := range p.delegate.Arguments {
		if !constructorBoundaryValue(arg, allowed, map[values.JavaValue]bool{}, calls, &remaining) {
			return nil, fmt.Errorf("unproved constructor argument dependencies")
		}
	}
	for pc, call := range calls {
		if pc >= p.pc || !constructorOriginalInvoke(decoder, call, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) }) {
			return nil, fmt.Errorf("constructor prefix origin mismatch")
		}
	}
	if err := c.planConstructorArgumentBridges(p, code, method); err != nil {
		return nil, err
	}
	desc, _ := c.obj.getUtf8(method.DescriptorIndex)
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil || len(mt.FunctionType().ParamTypes) != len(p.params) {
		return nil, fmt.Errorf("invalid constructor parameter descriptor")
	}
	target, err := types.ParseMethodDescriptor(p.delegate.Descriptor)
	if err != nil || len(target.FunctionType().ParamTypes) != len(p.delegate.Arguments) {
		return nil, fmt.Errorf("invalid constructor delegation descriptor")
	}
	// Constructing a source helper must not turn a class-scoped formal into an
	// illegal static use or silently change source binding under erasure.
	if len(c.FuncCtx.TypeParams) > 0 {
		return nil, fmt.Errorf("generic constructor prefix requires a formal binding proof")
	}
	helperName, err := c.constructorBoundaryHelperName("prefix", p.pc)
	if err != nil {
		return nil, err
	}
	p.carrier = helperName
	declarations := []string{}
	for i, param := range p.params {
		declarations = append(declarations, mt.FunctionType().ParamTypes[i].String(c.FuncCtx)+" "+param.String(c.FuncCtx))
	}
	var prefix strings.Builder
	for _, st := range p.prefix {
		prefix.WriteString(st.String(c.FuncCtx))
		prefix.WriteString(";\n")
	}
	ret := target.FunctionType().ParamTypes[0].String(c.FuncCtx)
	// A poly first argument must keep its declaration's instantiated target.
	// Erasing Consumer<String> to Consumer here changes the generated SAM bridge
	// and overload binding even though the constructor descriptor is unchanged.
	if owner, known := c.FuncCtx.InvocationMetadata(p.delegate.ClassName); known {
		for _, declaration := range owner.Methods {
			if declaration.Name != "<init>" || declaration.Desc != p.delegate.Descriptor || declaration.Signature == "" {
				continue
			}
			formals, params, result := types.ParseMethodSignatureFull(declaration.Signature, c.FuncCtx)
			if formals != "" || result == nil || len(params) != len(target.FunctionType().ParamTypes) || !constructorStaticSourceType(params[0], c.FuncCtx) {
				return nil, fmt.Errorf("constructor carrier requires closed generic formal binding")
			}
			ret = params[0].String(c.FuncCtx)
		}
	}
	helperBody := prefix.String() + "return (" + ret + ")(" + p.delegate.ArgumentStrings(c.FuncCtx)[0] + ");"
	helperBody = c.wrapCheckedEscapeBody(helperBody)
	p.helpers = append(p.helpers, &dumpedMethods{methodName: helperName, code: "private static " + ret + " " + helperName + "(" + strings.Join(declarations, ",") + ") {" + helperBody + "}", bodyCode: helperBody, checkedEscape: true})
	return p, nil
}

func constructorOriginalInvoke(decoder *core.Decompiler, call *values.FunctionCallExpression, get func(int) values.JavaValue) bool {
	if get == nil || decoder == nil || call == nil || !call.HasOriginPC || call.OriginPC < 0 || call.OriginPC > 65535 {
		return false
	}
	op := decoder.OpcodeByPC(uint16(call.OriginPC))
	if op == nil || op.Instr == nil || len(op.Data) < 2 {
		return false
	}
	want := map[values.InvokeKind]int{values.InvokeStatic: core.OP_INVOKESTATIC, values.InvokeVirtual: core.OP_INVOKEVIRTUAL, values.InvokeInterface: core.OP_INVOKEINTERFACE, values.InvokeSpecial: core.OP_INVOKESPECIAL}[call.Kind]
	if want == 0 || op.Instr.OpCode != want || call.IsStatic != (call.Kind == values.InvokeStatic) {
		return false
	}
	member, ok := get(int(core.Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
	return ok && member != nil && strings.ReplaceAll(member.Name, ".", "/") == strings.ReplaceAll(call.ClassName, ".", "/") && member.Member == call.FunctionName && member.Description == call.Descriptor
}

// Static carriers cannot mention class/method type variables or inaccessible
// declaration types. The exact original Signature supplies concrete arguments.
func constructorStaticSourceType(typ types.JavaType, ctx *class_context.ClassContext) bool {
	remaining := 128
	var inspect func(types.JavaType) bool
	inspect = func(t types.JavaType) bool {
		remaining--
		if remaining < 0 || t == nil || ctx == nil || ctx.InvocationMetadata == nil {
			return false
		}
		declaration := func(name string) bool {
			name = strings.ReplaceAll(name, ".", "/")
			decl, known := ctx.InvocationMetadata(name)
			return known && decl.Name == name && decl.Public
		}
		switch raw := t.RawType().(type) {
		case *types.JavaPrimer:
			return true
		case *types.JavaArrayType:
			return inspect(t.ElementType())
		case *types.JavaClass:
			return declaration(raw.Name)
		case *types.JavaParameterizedType:
			if !declaration(raw.RawClassName) {
				return false
			}
			for _, arg := range raw.TypeArgs {
				if !inspect(arg) {
					return false
				}
			}
			return true
		default:
			return false
		}
	}
	return inspect(typ)
}

func (c *ClassObjectDumper) constructorBoundaryHelperName(role string, pc int) (string, error) {
	inherited, complete := c.checkedEscapeInheritedNames()
	if !complete {
		return "", fmt.Errorf("constructor helper requires complete inherited declarations")
	}
	name := fmt.Sprintf("jdec$ctor$%x$%s$%d", []byte(c.obj.GetClassName()), role, pc)
	for _, method := range c.obj.Methods {
		n, _ := c.obj.getUtf8(method.NameIndex)
		inherited[class_context.SafeIdentifier(n)] = true
	}
	for _, helper := range c.constructorBoundaryHelpers {
		inherited[helper.methodName] = true
	}
	for inherited[name] {
		name += "$"
	}
	if len(name) > 65535 {
		return "", fmt.Errorf("constructor helper name exceeds classfile limit")
	}
	return name, nil
}

func (c *ClassObjectDumper) planConstructorArgumentBridges(p *constructorSourceBoundary, code *CodeAttribute, method *MemberInfo) error {
	if p.delegate == nil {
		return nil
	}
	declaredBefore, knownBefore := originalMethodExceptions(c.obj, method)
	if !knownBefore {
		return fmt.Errorf("unknown constructor Exceptions metadata")
	}
	rawDecoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	if err := rawDecoder.ParseOpcode(); err != nil {
		return err
	}
	requiresBridge := false
	for _, op := range rawDecoder.Opcodes() {
		if op == nil || op.Instr == nil || int(op.CurrentOffset) >= p.pc {
			continue
		}
		switch op.Instr.OpCode {
		case core.OP_INVOKEVIRTUAL, core.OP_INVOKEINTERFACE, core.OP_INVOKESTATIC:
		default:
			continue
		}
		if len(op.Data) < 2 {
			return fmt.Errorf("truncated constructor prefix invoke")
		}
		member, ok := GetValueFromCP(c.ConstantPool, int(core.Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
		if !ok || member == nil {
			return fmt.Errorf("missing constructor prefix member")
		}
		exceptions, complete := exactInvocationExceptions(c.FuncCtx.InvocationMetadata, member.Name, member.Member, member.Description)
		if !complete {
			continue
		}
		for _, exception := range exceptions {
			covered := originalExceptionUnchecked(exception, c.FuncCtx.InvocationMetadata)
			for _, allow := range declaredBefore {
				covered = covered || originalExceptionCovered(exception, allow, c.FuncCtx.InvocationMetadata)
			}
			requiresBridge = requiresBridge || !covered
		}
	}
	if !requiresBridge {
		return nil
	}
	allowed := map[*values.JavaRef]bool{}
	for _, param := range p.params {
		if ref, ok := param.(*values.JavaRef); ok && ref != nil {
			allowed[ref] = true
		}
	}
	calls := map[int]*values.FunctionCallExpression{}
	remaining := 1024
	for _, arg := range p.delegate.Arguments {
		if !constructorBoundaryValue(arg, allowed, map[values.JavaValue]bool{}, calls, &remaining) {
			return fmt.Errorf("unproved constructor argument effect graph")
		}
	}
	for _, st := range p.prefix {
		if expr, ok := st.(*statements.ExpressionStatement); ok && expr != nil {
			if !constructorBoundaryValue(expr.Expression, allowed, map[values.JavaValue]bool{}, calls, &remaining) {
				return fmt.Errorf("unproved constructor prefix dependency graph")
			}
		}
	}
	declared, known := originalMethodExceptions(c.obj, method)
	if !known {
		return fmt.Errorf("unknown constructor Exceptions metadata")
	}
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	if err := decoder.ParseOpcode(); err != nil {
		return err
	}
	pcs := make([]int, 0, len(calls))
	for pc := range calls {
		pcs = append(pcs, pc)
	}
	sort.Ints(pcs)
	for _, pc := range pcs {
		call := calls[pc]
		if pc >= p.pc {
			return fmt.Errorf("constructor expression lacks pre-delegation invoke origin")
		}
		op := decoder.OpcodeByPC(uint16(pc))
		if op == nil || op.Instr == nil || len(op.Data) < 2 {
			return fmt.Errorf("missing original constructor argument invoke")
		}
		want := map[values.InvokeKind]int{values.InvokeStatic: core.OP_INVOKESTATIC, values.InvokeVirtual: core.OP_INVOKEVIRTUAL, values.InvokeInterface: core.OP_INVOKEINTERFACE}[call.Kind]
		member, ok := GetValueFromCP(c.ConstantPool, int(core.Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
		if want == 0 || op.Instr.OpCode != want || !ok || member == nil || strings.ReplaceAll(member.Name, ".", "/") != strings.ReplaceAll(call.ClassName, ".", "/") || member.Member != call.FunctionName || member.Description != call.Descriptor {
			return fmt.Errorf("constructor argument invoke identity mismatch")
		}
		exceptions, complete := exactInvocationExceptions(c.FuncCtx.InvocationMetadata, call.ClassName, call.FunctionName, call.Descriptor)
		if !complete {
			return fmt.Errorf("constructor argument lacks complete invocation declaration")
		}
		needed := false
		for _, exception := range exceptions {
			covered := originalExceptionUnchecked(exception, c.FuncCtx.InvocationMetadata)
			for _, allow := range declared {
				covered = covered || originalExceptionCovered(exception, allow, c.FuncCtx.InvocationMetadata)
			}
			needed = needed || !covered
		}
		if !needed {
			continue
		}
		if len(code.ExceptionTable) != 0 {
			return fmt.Errorf("protected constructor argument requires its original handler domain")
		}
		table, complete := c.FuncCtx.InvocationMetadata(call.ClassName)
		if !complete || !table.MembersComplete || !table.Public {
			return fmt.Errorf("constructor argument owner is not source-accessible")
		}
		matches := 0
		for _, target := range table.Methods {
			if target.Name == call.FunctionName && target.Desc == call.Descriptor {
				matches++
				if target.Generic || (!target.Public && strings.ReplaceAll(call.ClassName, ".", "/") != c.obj.GetClassName()) || target.Static != call.IsStatic {
					return fmt.Errorf("constructor argument source binding is unproved")
				}
			}
		}
		if matches != 1 {
			return fmt.Errorf("constructor argument declaration is ambiguous")
		}
		typ, err := types.ParseMethodDescriptor(call.Descriptor)
		if err != nil || len(typ.FunctionType().ParamTypes) != len(call.Arguments) {
			return fmt.Errorf("invalid checked producer descriptor")
		}
		helper, err := c.constructorBoundaryHelperName("invoke", pc)
		if err != nil {
			return err
		}
		declarations, args := []string{}, []string{}
		receiver := types.NewJavaClass(strings.ReplaceAll(call.ClassName, "/", ".")).String(c.FuncCtx)
		if !call.IsStatic {
			declarations = append(declarations, receiver+" receiver")
			receiver = "receiver"
		}
		for i, param := range typ.FunctionType().ParamTypes {
			n := fmt.Sprintf("argument%d", i)
			declarations = append(declarations, param.String(c.FuncCtx)+" "+n)
			args = append(args, n)
		}
		invocation := receiver + "." + class_context.SafeIdentifier(call.FunctionName) + "(" + strings.Join(args, ",") + ")"
		ret := typ.FunctionType().ReturnType.String(c.FuncCtx)
		body := invocation + ";"
		if ret != "void" {
			body = "return " + invocation + ";"
		}
		body = c.wrapCheckedEscapeBody(body)
		p.bridgeNames[pc] = helper
		p.helpers = append(p.helpers, &dumpedMethods{methodName: helper, code: "private static " + ret + " " + helper + "(" + strings.Join(declarations, ",") + ") {" + body + "}", bodyCode: body, checkedEscape: true})
	}
	if len(p.bridgeNames) > 0 {
		c.FuncCtx.ConstructorInvokeBridge = func(owner, name, desc string, kind uint8, pc int) (string, bool) {
			call := calls[pc]
			if call == nil || call.ClassName != owner || call.FunctionName != name || call.Descriptor != desc || uint8(call.Kind) != kind {
				return "", false
			}
			n, ok := p.bridgeNames[pc]
			return n, ok
		}
	}
	return nil
}

func (p *constructorSourceBoundary) renderDelegation(ctx *class_context.ClassContext) string {
	if p == nil || p.delegate == nil {
		return ""
	}
	if p.carrier == "" {
		return p.delegate.String(ctx)
	}
	args := p.delegate.ArgumentStrings(ctx)
	inputs := make([]string, len(p.params))
	for i, param := range p.params {
		inputs[i] = param.String(ctx)
	}
	args[0] = p.carrier + "(" + strings.Join(inputs, ",") + ")"
	target := "super"
	if strings.ReplaceAll(p.delegate.ClassName, ".", "/") == strings.ReplaceAll(ctx.ClassName, ".", "/") {
		target = "this"
	}
	return target + "(" + strings.Join(args, ",") + ")"
}
