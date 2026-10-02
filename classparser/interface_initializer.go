package javaclassparser

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type interfaceFieldWrite struct {
	name, descriptor string
	pc               int
	field            *values.JavaClassMember
}
type interfaceInitializerSegment struct {
	write  interfaceFieldWrite
	prefix []statements.Statement
	value  values.JavaValue
}

func normalizedInitializerOwner(s string) string { return strings.ReplaceAll(s, ".", "/") }

func (c *ClassObjectDumper) interfaceWrites(code *CodeAttribute) ([]interfaceFieldWrite, error) {
	return c.decodeInterfaceWrites(code, true)
}

func (c *ClassObjectDumper) decodeInterfaceWrites(code *CodeAttribute, requirePartition bool) ([]interfaceFieldWrite, error) {
	if code == nil {
		return nil, fmt.Errorf("interface initializer has no code")
	}
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	if err := decoder.ParseOpcode(); err != nil {
		return nil, err
	}
	var writes []interfaceFieldWrite
	for _, op := range decoder.Opcodes() {
		if op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_PUTSTATIC {
			continue
		}
		if len(op.Data) != 2 {
			return nil, fmt.Errorf("malformed interface putstatic")
		}
		field, ok := GetValueFromCP(c.ConstantPool, int(core.Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
		if !ok || field == nil {
			return nil, fmt.Errorf("unresolved interface putstatic")
		}
		if normalizedInitializerOwner(field.Name) != normalizedInitializerOwner(c.obj.GetClassName()) {
			if requirePartition {
				return nil, fmt.Errorf("initializer writes a foreign static field")
			}
			continue
		}
		if requirePartition {
			// Moving the store to a field declaration would move it out of
			// these handlers. A helper cannot claim their original coverage.
			for _, entry := range code.ExceptionTable {
				if entry == nil || entry.StartPc >= entry.EndPc {
					return nil, fmt.Errorf("invalid initializer exception coverage")
				}
				if op.CurrentOffset >= entry.StartPc && op.CurrentOffset < entry.EndPc {
					return nil, fmt.Errorf("initializer field store is protected by a handler")
				}
			}
		}
		writes = append(writes, interfaceFieldWrite{field.Member, field.Description, int(op.CurrentOffset), field})
	}
	return writes, nil
}

// Partition only at original own PUTSTATICs in declaration order. Each prefix
// stays intact in exactly one helper; definitions cannot leak across helpers.
// This preserves calls, allocation order, shared backing arrays, first-failure
// behavior and circular class initialization without replaying an initializer.
func planInterfaceInitializers(body []statements.Statement, owner string, writes []interfaceFieldWrite, fieldOrder []string) ([]interfaceInitializerSegment, error) {
	if len(writes) > 128 {
		return nil, fmt.Errorf("interface initializer exceeds field budget")
	}
	if len(writes) != len(fieldOrder) {
		return nil, fmt.Errorf("interface field/store count differs")
	}
	for i, w := range writes {
		if w.name != fieldOrder[i] || w.pc < 0 {
			return nil, fmt.Errorf("interface store order differs from declarations")
		}
		for j := 0; j < i; j++ {
			if writes[j].name == w.name || writes[j].pc >= w.pc {
				return nil, fmt.Errorf("interface field is written repeatedly or out of order")
			}
		}
	}
	var plans []interfaceInitializerSegment
	var prefix []statements.Statement
	for i, st := range body {
		if middle, ok := st.(*statements.MiddleStatement); ok {
			if middle == nil || middle.Data != nil || (middle.Flag != "start" && middle.Flag != "end") {
				return nil, fmt.Errorf("unresolved initializer marker")
			}
			continue
		}
		if ret, ok := st.(*statements.ReturnStatement); ok {
			if ret == nil || ret.JavaValue != nil || i != len(body)-1 {
				return nil, fmt.Errorf("nonterminal initializer return")
			}
			continue
		}
		assign, ok := st.(*statements.AssignStatement)
		var field *values.JavaClassMember
		if ok && assign != nil {
			field, _ = values.UnpackSoltValue(assign.LeftValue).(*values.JavaClassMember)
		}
		if field == nil {
			prefix = append(prefix, st)
			continue
		}
		if len(plans) >= len(writes) {
			return nil, fmt.Errorf("unexpected initializer field write")
		}
		w := writes[len(plans)]
		if assign.ArrayMember != nil || assign.JavaValue == nil || !assign.HasOriginPC || assign.OriginPC != w.pc || field.Member != w.name || field.Description != w.descriptor || normalizedInitializerOwner(field.Name) != normalizedInitializerOwner(owner) {
			return nil, fmt.Errorf("initializer destination lacks exact putstatic witness")
		}
		if err := closedInterfacePrefix(prefix, assign.JavaValue, owner); err != nil {
			return nil, err
		}
		plans = append(plans, interfaceInitializerSegment{w, append([]statements.Statement{}, prefix...), assign.JavaValue})
		prefix = nil
	}
	if len(prefix) != 0 || len(plans) != len(writes) {
		return nil, fmt.Errorf("initializer effects remain after the last field write")
	}
	return plans, nil
}

type initializerLocalKey struct {
	id  *utils.VariableId
	uid string
}

func initializerKey(ref *values.JavaRef) initializerLocalKey {
	return initializerLocalKey{ref.Id, ref.VarUid}
}
func closedInterfacePrefix(body []statements.Statement, result values.JavaValue, owner string) error {
	definitions := map[initializerLocalKey]bool{}
	reads := map[initializerLocalKey]bool{}
	remaining := 2048
	var value func(values.JavaValue) error
	active := map[values.JavaValue]bool{}
	value = func(v values.JavaValue) error {
		remaining--
		if remaining < 0 {
			return fmt.Errorf("initializer value budget exceeded")
		}
		if v == nil {
			return nil
		}
		if reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil() {
			return fmt.Errorf("nil initializer operand")
		}
		if active[v] {
			return fmt.Errorf("cyclic initializer dependency")
		}
		active[v] = true
		defer delete(active, v)
		if ref, ok := v.(*values.JavaRef); ok {
			if ref.IsThis || ref.Id == nil || ref.CustomValue != nil || ref.StackVar != nil {
				return fmt.Errorf("opaque initializer local")
			}
			reads[initializerKey(ref)] = true
			return nil
		}
		if assignment, ok := v.(*values.AssignmentExpression); ok {
			ref, ok := values.UnpackSoltValue(assignment.Target).(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil {
				return fmt.Errorf("nonlocal embedded initializer write")
			}
			definitions[initializerKey(ref)] = true
			return value(assignment.Value)
		}
		children, known := values.Children(v)
		if !known {
			return fmt.Errorf("opaque initializer value")
		}
		for _, child := range children {
			if err := value(child); err != nil {
				return err
			}
		}
		return nil
	}
	loopDepth := 0
	var loopLabels []string
	var block func([]statements.Statement) error
	block = func(list []statements.Statement) error {
		for _, st := range list {
			remaining--
			if remaining < 0 || st == nil {
				return fmt.Errorf("initializer statement budget exceeded")
			}
			switch x := st.(type) {
			case *statements.AssignStatement:
				if x.ArrayMember != nil {
					if err := value(x.ArrayMember); err != nil {
						return err
					}
				} else if ref, ok := values.UnpackSoltValue(x.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
					definitions[initializerKey(ref)] = true
				} else {
					return fmt.Errorf("nested field write cannot be partitioned")
				}
				if err := value(x.JavaValue); err != nil {
					return err
				}
			case *statements.ExpressionStatement:
				if err := value(x.Expression); err != nil {
					return err
				}
			case *values.JavaExpression:
				if err := value(x); err != nil {
					return err
				}
			case *statements.IfStatement:
				if err := value(x.Condition); err != nil {
					return err
				}
				if err := block(x.IfBody); err != nil {
					return err
				}
				if err := block(x.ElseBody); err != nil {
					return err
				}
			case *statements.DoWhileStatement:
				loopDepth++
				if x.Label != "" {
					loopLabels = append(loopLabels, x.Label)
				}
				if err := value(x.ConditionValue); err != nil {
					return err
				}
				if err := block(x.Body); err != nil {
					return err
				}
				loopDepth--
				if x.Label != "" {
					loopLabels = loopLabels[:len(loopLabels)-1]
				}
			case *statements.TryCatchStatement:
				if err := block(x.TryBody); err != nil {
					return err
				}
				for i, arm := range x.CatchBodies {
					if i >= len(x.Exception) || x.Exception[i] == nil {
						return fmt.Errorf("unbound initializer catch")
					}
					definitions[initializerKey(x.Exception[i])] = true
					if err := block(arm); err != nil {
						return err
					}
				}
			case *statements.CustomStatement:
				if x.ThrownValue != nil {
					if err := value(x.ThrownValue); err != nil {
						return err
					}
				} else if loopDepth == 0 || !((x.Name == "continue" || x.Name == "break") || ((x.LoopTransferKind == "continue" || x.LoopTransferKind == "break") && slices.Contains(loopLabels, x.LoopTargetLabel))) {
					return fmt.Errorf("opaque initializer control")
				}
			case *statements.MiddleStatement:
				if x.Data != nil || (x.Flag != "start" && x.Flag != "end") {
					return fmt.Errorf("opaque initializer marker")
				}
			default:
				return fmt.Errorf("unsupported initializer statement %T", st)
			}
		}
		return nil
	}
	if err := block(body); err != nil {
		return err
	}
	if err := value(result); err != nil {
		return err
	}
	for key := range reads {
		if !definitions[key] {
			return fmt.Errorf("initializer reads a local defined in another segment")
		}
	}
	return nil
}

func (c *ClassObjectDumper) interfaceInitializationOrder(writes []interfaceFieldWrite) ([]string, error) {
	written := map[string]bool{}
	for _, w := range writes {
		written[w.name] = true
	}
	var order []string
	for _, info := range c.obj.Fields {
		name, err := c.obj.getUtf8(info.NameIndex)
		if err != nil {
			return nil, err
		}
		if !written[name] {
			continue
		}
		descriptor, err := c.obj.getUtf8(info.DescriptorIndex)
		if err != nil {
			return nil, err
		}
		if info.AccessFlags&0x0018 != 0x0018 {
			return nil, fmt.Errorf("interface initializer destination is not static final")
		}
		for _, w := range writes {
			if w.name == name && descriptor != w.descriptor {
				return nil, fmt.Errorf("interface field descriptor mismatch")
			}
		}
		for _, attribute := range info.Attributes {
			if _, ok := attribute.(*ConstantValueAttribute); ok {
				return nil, fmt.Errorf("interface clinit overwrites a ConstantValue field")
			}
		}
		order = append(order, name)
	}
	return order, nil
}
func (c *ClassObjectDumper) initializerHelperNames(count int) []string {
	reserved := map[string]bool{}
	for _, m := range c.obj.Methods {
		name, _ := c.obj.getUtf8(m.NameIndex)
		reserved[class_context.SafeIdentifier(name)] = true
	}
	for _, f := range c.obj.Fields {
		name, _ := c.obj.getUtf8(f.NameIndex)
		reserved[class_context.SafeIdentifier(name)] = true
	}
	for _, constant := range c.ConstantPool {
		if class, ok := constant.(*ConstantClassInfo); ok && class != nil {
			raw, _ := c.obj.getUtf8(class.NameIndex)
			prefix := c.obj.GetClassName() + "$"
			if strings.HasPrefix(raw, prefix) {
				nested := strings.TrimPrefix(raw, prefix)
				if !strings.Contains(nested, "$") {
					reserved[class_context.SafeIdentifier(nested)] = true
				}
			}
		}
	}
	var out []string
	for i := 0; len(out) < count; i++ {
		name := fmt.Sprintf("jdec$init$%d", i)
		if !reserved[name] {
			out = append(out, name)
			reserved[name] = true
		}
	}
	return out
}
func (c *ClassObjectDumper) renderInterfaceInitializers(body []statements.Statement, code *CodeAttribute, ctx *class_context.ClassContext, render func([]statements.Statement) string) ([]*dumpedMethods, error) {
	writes, err := c.interfaceWrites(code)
	if err != nil {
		return nil, err
	}
	order, err := c.interfaceInitializationOrder(writes)
	if err != nil {
		return nil, err
	}
	plans, err := planInterfaceInitializers(body, c.obj.GetClassName(), writes, order)
	if err != nil {
		return nil, err
	}
	names := c.initializerHelperNames(len(plans))
	helpers := make([]*dumpedMethods, 0, len(plans))
	initializers := map[string]string{}
	oldName, oldDescriptor, oldType := ctx.FunctionName, ctx.CurrentMethodDesc, ctx.FunctionType
	defer func() { ctx.FunctionName, ctx.CurrentMethodDesc, ctx.FunctionType = oldName, oldDescriptor, oldType }()
	for i, plan := range plans {
		fieldType, fieldErr := c.interfaceFieldType(plan.write.name, plan.write.descriptor)
		if fieldErr != nil {
			return nil, fieldErr
		}
		if fieldType == nil {
			return nil, fmt.Errorf("interface initializer has unresolved field type")
		}
		ctx.FunctionName = names[i]
		ctx.CurrentMethodDesc = "()" + plan.write.descriptor
		ctx.FunctionType = &types.JavaFuncType{ReturnType: fieldType}
		value := values.ErasedFactoryAssignmentView(plan.value, fieldType, ctx)
		helperBody := render(append(append([]statements.Statement{}, plan.prefix...), statements.NewReturnStatement(value)))
		helperCode := fmt.Sprintf("static %s %s() {\n%s\n%s}", fieldType.String(ctx), names[i], helperBody, c.GetTabString())
		if slices.Contains(c.obj.AccessFlagsVerbose, "annotation") {
			helperCode = fmt.Sprintf("class %s { public static %s value(){\n%s\n%s} }", names[i], fieldType.String(ctx), helperBody, c.GetTabString())
			initializers[plan.write.name] = names[i] + ".value()"
		} else {
			initializers[plan.write.name] = names[i] + "()"
		}
		if err := c.holdOutput(int64(len(helperCode))); err != nil {
			return nil, err
		}
		helpers = append(helpers, &dumpedMethods{methodName: names[i], descriptor: ctx.CurrentMethodDesc, code: helperCode, bodyCode: helperBody})
	}
	for name, value := range initializers {
		c.fieldDefaultValue[name] = value
	}
	return helpers, nil
}
func (c *ClassObjectDumper) interfaceInitializerStubs(reason error) []*dumpedMethods {
	c.appendDiagnostic(DecompileDiagnostic{Code: "unproven_interface_initializer", Method: "<clinit>()V", Message: reason.Error()})
	if c.report != nil {
		c.report.StubMethods = append(c.report.StubMethods, "<clinit>()V")
	}
	// Do not use name-only constructor store totals: a foreign field with
	// the same name is not evidence that this interface field was written.
	ownWrites := map[string]string{}
	unresolved := false
	for _, method := range c.obj.Methods {
		name, _ := c.obj.getUtf8(method.NameIndex)
		if name != "<clinit>" {
			continue
		}
		for _, attribute := range method.Attributes {
			if code, ok := attribute.(*CodeAttribute); ok {
				writes, err := c.decodeInterfaceWrites(code, false)
				if err != nil {
					unresolved = true
				}
				for _, w := range writes {
					ownWrites[w.name] = w.descriptor
				}
			}
		}
	}
	var written []interfaceFieldWrite
	for _, f := range c.obj.Fields {
		name, _ := c.obj.getUtf8(f.NameIndex)
		descriptor, _ := c.obj.getUtf8(f.DescriptorIndex)
		if ownWrites[name] != descriptor {
			if !unresolved || f.AccessFlags&0x0018 != 0x0018 {
				continue
			}
			constant := false
			for _, attr := range f.Attributes {
				_, isConstant := attr.(*ConstantValueAttribute)
				constant = constant || isConstant
			}
			if constant {
				continue
			}
		}
		written = append(written, interfaceFieldWrite{name: name, descriptor: descriptor})
	}
	names := c.initializerHelperNames(len(written))
	var helpers []*dumpedMethods
	for i, w := range written {
		typ, err := types.ParseDescriptor(w.descriptor)
		if err != nil {
			continue
		}
		body := "throw new UnsupportedOperationException(\"yak-decompiler: unproven interface initializer\");"
		code := fmt.Sprintf("static %s %s(){%s}", typ.String(c.FuncCtx), names[i], body)
		helpers = append(helpers, &dumpedMethods{methodName: names[i], descriptor: "()" + w.descriptor, code: code, bodyCode: body})
		if slices.Contains(c.obj.AccessFlagsVerbose, "annotation") {
			helpers[len(helpers)-1].code = fmt.Sprintf("class %s { public static %s value(){%s} }", names[i], typ.String(c.FuncCtx), body)
			c.fieldDefaultValue[w.name] = names[i] + ".value()"
		} else {
			c.fieldDefaultValue[w.name] = names[i] + "()"
		}
	}
	return helpers
}

func (c *ClassObjectDumper) interfaceFieldType(name, descriptor string) (types.JavaType, error) {
	typ, err := types.ParseDescriptor(descriptor)
	if err != nil {
		return nil, err
	}
	for _, field := range c.obj.Fields {
		raw, _ := c.obj.getUtf8(field.NameIndex)
		if raw != name {
			continue
		}
		for _, attr := range field.Attributes {
			if signature, ok := attr.(*SignatureAttribute); ok {
				sig, err := c.obj.getUtf8(signature.SignatureIndex)
				if err != nil {
					return nil, err
				}
				parsed := types.ParseSignature(sig)
				if parsed == nil {
					return nil, fmt.Errorf("unresolved interface field Signature")
				}
				typ = parsed
			}
		}
		return typ, nil
	}
	return nil, fmt.Errorf("missing interface field metadata")
}
