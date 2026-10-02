package javaclassparser

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func originalMethodExceptions(object *ClassObject, method *MemberInfo) ([]string, bool) {
	if object == nil || method == nil {
		return nil, false
	}
	var out []string
	for _, attribute := range method.Attributes {
		if attr, ok := attribute.(*ExceptionsAttribute); ok {
			for _, index := range attr.ExceptionIndexTable {
				constant, err := object.getConstantInfo(index)
				if err != nil {
					return nil, false
				}
				class, ok := constant.(*ConstantClassInfo)
				if !ok || class == nil {
					return nil, false
				}
				name, err := object.getUtf8(class.NameIndex)
				if err != nil || name == "" {
					return nil, false
				}
				out = append(out, strings.ReplaceAll(name, ".", "/"))
			}
		}
	}
	return out, true
}

func exactInvocationExceptions(provider callbinding.Provider, owner, name, descriptor string) ([]string, bool) {
	if provider == nil {
		return nil, false
	}
	queue := []string{strings.ReplaceAll(owner, ".", "/")}
	seen := map[string]bool{}
	for len(queue) > 0 && len(seen) < 128 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		class, ok := provider(current)
		if !ok || !class.MembersComplete {
			return nil, false
		}
		var found []callbinding.Method
		for _, method := range class.Methods {
			if method.Name == name && method.Desc == descriptor {
				found = append(found, method)
			}
		}
		if len(found) > 1 {
			return nil, false
		}
		if len(found) == 1 {
			return slices.Clone(found[0].Exceptions), found[0].ExceptionsKnown
		}
		if name == "<init>" || !class.ParentsComplete {
			return nil, false
		}
		if len(queue)+len(class.Parents) > 128 {
			return nil, false
		}
		queue = append(queue, class.Parents...)
	}
	return nil, false
}

func originalExceptionCovered(exception, declared string, provider callbinding.Provider) bool {
	if exception == declared || declared == "java/lang/Throwable" {
		return true
	}
	if provider == nil {
		return false
	}
	queue := []string{exception}
	seen := map[string]bool{}
	for len(queue) > 0 && len(seen) < 128 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		class, known := provider(current)
		parents := class.Parents
		if !known || strings.ReplaceAll(class.Name, ".", "/") != current || !class.ParentsComplete {
			platformParents, platformKnown := types.KnownPlatformSuperTypes(strings.ReplaceAll(current, "/", "."))
			if !platformKnown {
				return false
			}
			parents = make([]string, len(platformParents))
			for i, parent := range platformParents {
				parents[i] = strings.ReplaceAll(parent, ".", "/")
			}
		}
		if len(queue)+len(parents) > 128 {
			return false
		}
		for _, parent := range parents {
			if parent == declared {
				return true
			}
			queue = append(queue, parent)
		}
	}
	return false
}

func originalExceptionUnchecked(exception string, provider callbinding.Provider) bool {
	for _, anchor := range []string{"java/lang/RuntimeException", "java/lang/Error"} {
		if originalExceptionCovered(exception, anchor, provider) {
			return true
		}
	}
	return false
}

// A typed handler suppresses its caught checked exception only if every typed
// path has no rethrow or throws a proven unchecked operand at its original
// decoded ATHROW. Raw catch-all cleanup never proves
// suppression: its synthetic ATHROW propagates the original exception.
func typedAbsorbingHandlers(body []statements.Statement, uncheckedThrows map[int]bool) map[int]bool {
	result := map[int]bool{}
	remaining := 1024
	var absorbs func([]statements.Statement) bool
	absorbs = func(list []statements.Statement) bool {
		for _, statement := range list {
			remaining--
			if remaining < 0 || statement == nil {
				return false
			}
			switch x := statement.(type) {
			case *statements.CustomStatement:
				if x == nil || x.ThrownValue == nil || !x.HasOriginPC || !uncheckedThrows[x.OriginPC] {
					return false
				}
			case *statements.IfStatement:
				if x == nil || !absorbs(x.IfBody) || !absorbs(x.ElseBody) {
					return false
				}
			case *statements.TryCatchStatement:
				if x == nil || !absorbs(x.TryBody) {
					return false
				}
				for _, arm := range x.CatchBodies {
					if !absorbs(arm) {
						return false
					}
				}
			case *statements.AssignStatement, *statements.ExpressionStatement, *statements.ReturnStatement, *values.JavaExpression:
			case *statements.MiddleStatement:
				if x == nil || x.Data != nil || (x.Flag != "start" && x.Flag != "end") {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	var visit func([]statements.Statement)
	visit = func(list []statements.Statement) {
		for _, statement := range list {
			remaining--
			if remaining < 0 {
				return
			}
			switch x := statement.(type) {
			case *statements.TryCatchStatement:
				if x == nil {
					continue
				}
				for i, handler := range x.Handlers {
					if !handler.CatchAll && handler.EntryPC >= 0 && i < len(x.CatchBodies) && absorbs(x.CatchBodies[i]) {
						result[handler.EntryPC] = true
					}
				}
				visit(x.TryBody)
				for _, arm := range x.CatchBodies {
					visit(arm)
				}
			case *statements.IfStatement:
				if x != nil {
					visit(x.IfBody)
					visit(x.ElseBody)
				}
			case *statements.WhileStatement:
				if x != nil {
					visit(x.Body)
				}
			case *statements.DoWhileStatement:
				if x != nil {
					visit(x.Body)
				}
			case *statements.ForStatement:
				if x != nil {
					visit(x.SubStatements)
				}
			case *statements.SynchronizedStatement:
				if x != nil {
					visit(x.Body)
				}
			case *statements.SwitchStatement:
				if x != nil {
					for _, arm := range x.Cases {
						if arm != nil {
							visit(arm.Body)
						}
					}
				}
			}
		}
	}
	visit(body)
	return result
}

// Positive evidence comes from the actual callee declaration and decoded
// invoke PC. An absent declaration is unknown, never a guessed checked throw.
// Unknown exception ancestry is conservatively treated as potentially checked;
// the resulting bridge propagates the same Throwable without wrapping it.
func (c *ClassObjectDumper) methodNeedsCheckedEscape(code *CodeAttribute, body []statements.Statement, method *MemberInfo, plans ...*constructorSourceBoundary) (bool, error) {
	if code == nil || c.FuncCtx == nil {
		return false, nil
	}
	declared, known := originalMethodExceptions(c.obj, method)
	if !known {
		return false, fmt.Errorf("unknown caller Exceptions metadata")
	}
	provider := c.FuncCtx.InvocationMetadata
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	if err := decoder.ParseOpcode(); err != nil {
		return false, err
	}
	delegatePC := constructorDelegationPC(body)
	var sourcePlan *constructorSourceBoundary
	if len(plans) > 0 {
		sourcePlan = plans[0]
		if sourcePlan != nil {
			delegatePC = sourcePlan.pc
		}
	}
	// The IR may omit an implicit no-argument superclass delegation. Recover
	// its exact raw prefix only when complete declaration metadata proves all
	// checked exceptions are already declared by this caller. The implicit
	// Java super() still runs first, preserving its effects and failures.
	ops := decoder.Opcodes()
	if len(ops) > 0 && ops[0] != nil && ops[0].Instr != nil && ops[0].Instr.OpCode == core.OP_START {
		ops = ops[1:]
	}
	if delegatePC < 0 && len(ops) >= 2 && ops[0] != nil && ops[0].Instr != nil && ops[0].Instr.OpCode == core.OP_ALOAD_0 && ops[0].CurrentOffset == 0 && ops[1] != nil && ops[1].Instr != nil && ops[1].Instr.OpCode == core.OP_INVOKESPECIAL && len(ops[1].Data) >= 2 {
		member, ok := GetValueFromCP(c.ConstantPool, int(core.Convert2bytesToInt(ops[1].Data[:2]))).(*values.JavaClassMember)
		if ok && member != nil && strings.ReplaceAll(member.Name, ".", "/") == c.obj.GetSupperClassName() && member.Member == "<init>" && member.Description == "()V" {
			exceptions, complete := exactInvocationExceptions(provider, member.Name, member.Member, member.Description)
			// Object() is a language-defined no-throws constructor; every
			// other superclass requires its original declaration witness.
			if strings.ReplaceAll(member.Name, ".", "/") == "java/lang/Object" {
				exceptions, complete = nil, true
			}
			for _, exception := range exceptions {
				if originalExceptionUnchecked(exception, provider) {
					continue
				}
				covered := false
				for _, allowed := range declared {
					covered = covered || originalExceptionCovered(exception, allowed, provider)
				}
				if !covered {
					complete = false
					break
				}
			}
			if complete {
				delegatePC = int(ops[1].CurrentOffset)
			}
		}
	}
	thrownTypes := checkedEscapeThrownTypes(body, provider)
	uncheckedThrows := map[int]bool{}
	for _, op := range decoder.Opcodes() {
		if op != nil && op.Instr != nil && op.Instr.OpCode == core.OP_ATHROW {
			pc := int(op.CurrentOffset)
			if name := thrownTypes[pc]; name != "" && originalExceptionUnchecked(name, provider) {
				uncheckedThrows[pc] = true
			}
		}
	}
	absorbing := typedAbsorbingHandlers(body, uncheckedThrows)
	for _, op := range decoder.Opcodes() {
		if op == nil || op.Instr == nil {
			continue
		}
		var exceptions []string
		if op.Instr.OpCode == core.OP_ATHROW {
			// An invocation is not required for a checked object to escape.
			// Pair the typed operand with the actual decoded ATHROW PC.
			if exception := thrownTypes[int(op.CurrentOffset)]; exception != "" {
				exceptions = []string{exception}
			} else {
				continue
			}
		} else {
			switch op.Instr.OpCode {
			case core.OP_INVOKEVIRTUAL, core.OP_INVOKESTATIC, core.OP_INVOKESPECIAL, core.OP_INVOKEINTERFACE:
			default:
				continue
			}
			if len(op.Data) < 2 {
				return false, fmt.Errorf("truncated invoke witness")
			}
			member, ok := GetValueFromCP(c.ConstantPool, int(core.Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
			if !ok || member == nil {
				return false, fmt.Errorf("missing invoke member")
			}
			var known bool
			exceptions, known = exactInvocationExceptions(provider, member.Name, member.Member, member.Description)
			if !known {
				continue
			}
		}
		for _, exception := range exceptions {
			if originalExceptionUnchecked(exception, provider) {
				continue
			}
			covered := false
			for _, allowed := range declared {
				covered = covered || originalExceptionCovered(exception, allowed, provider)
			}
			if covered {
				continue
			}
			for _, entry := range code.ExceptionTable {
				if entry == nil || entry.CatchType == 0 || op.CurrentOffset < entry.StartPc || op.CurrentOffset >= entry.EndPc || !absorbing[int(entry.HandlerPc)] {
					continue
				}
				catch, ok := GetValueFromCP(c.ConstantPool, int(entry.CatchType)).(*values.JavaClassValue)
				if !ok || catch == nil {
					continue
				}
				if name, ok := valuesClassName(catch); ok && originalExceptionCovered(exception, name, provider) {
					covered = true
					break
				}
			}
			if !covered {
				name, _ := c.obj.getUtf8(method.NameIndex)
				if name == "<init>" {
					if sourcePlan != nil && sourcePlan.bridgeNames[int(op.CurrentOffset)] != "" {
						continue
					}
					if delegatePC < 0 || int(op.CurrentOffset) <= delegatePC {
						// Java requires this/super (including its argument effects)
						// before any try. A body bridge cannot cover that prefix.
						return false, fmt.Errorf("checked constructor delegation requires original declared throws")
					}
				}
				if _, complete := c.checkedEscapeInheritedNames(); !complete {
					return false, fmt.Errorf("checked escape helper requires complete inherited member names")
				}
				return true, nil
			}
		}
	}
	return false, nil
}

func checkedEscapeThrowableType(name string, provider callbinding.Provider) bool {
	// Use the existing finite canonical platform hierarchy for standard
	// Throwable types, then require original complete declaration ancestry
	// for user classes. A spelling ending in Exception supplies no evidence.
	if types.IsThrowableRooted(strings.ReplaceAll(name, "/", ".")) {
		return true
	}
	queue := []string{name}
	seen := map[string]bool{}
	for len(queue) > 0 && len(seen) < 128 {
		current := queue[0]
		queue = queue[1:]
		if types.IsThrowableRooted(strings.ReplaceAll(current, "/", ".")) {
			return true
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		if provider == nil {
			return false
		}
		table, known := provider(current)
		if !known || table.Name != current || !table.ParentsComplete || len(queue)+len(table.Parents) > 128 {
			return false
		}
		queue = append(queue, table.Parents...)
	}
	return false
}

// Retain only immutable handler/control evidence: typed thrown operands tied
// to original ATHROW locations. Missing origins, erased/unknown type variables,
// contradictory copies and opaque statements cannot establish an escape.
func checkedEscapeThrownTypes(body []statements.Statement, provider callbinding.Provider) map[int]string {
	result, ambiguous := map[int]string{}, map[int]bool{}
	queue := append([]statements.Statement{}, body...)
	seen := map[statements.Statement]bool{}
	for len(queue) > 0 && len(seen) < 8192 {
		st := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if st == nil || seen[st] {
			continue
		}
		seen[st] = true
		if thrown, ok := st.(*statements.CustomStatement); ok && thrown != nil && thrown.HasOriginPC && thrown.OriginPC >= 0 && thrown.ThrownValue != nil && thrown.ThrownValue.Type() != nil && !values.IsNullLiteral(values.UnpackSoltValue(thrown.ThrownValue)) {
			if raw, ok := types.RawClassFQN(thrown.ThrownValue.Type()); ok && raw != "" && provider != nil {
				name := strings.ReplaceAll(raw, ".", "/")
				if checkedEscapeThrowableType(name, provider) {
					pc := thrown.OriginPC
					if previous := result[pc]; previous != "" && previous != name {
						ambiguous[pc] = true
					} else {
						result[pc] = name
					}
				}
			}
		}
		switch st := st.(type) {
		case *statements.IfStatement:
			if st == nil {
				continue
			}
			queue = append(queue, st.IfBody...)
			queue = append(queue, st.ElseBody...)
		case *statements.TryCatchStatement:
			if st == nil {
				continue
			}
			queue = append(queue, st.TryBody...)
			for _, handler := range st.CatchBodies {
				queue = append(queue, handler...)
			}
		case *statements.WhileStatement:
			if st == nil {
				continue
			}
			queue = append(queue, st.Body...)
		case *statements.DoWhileStatement:
			if st == nil {
				continue
			}
			queue = append(queue, st.Body...)
		case *statements.ForStatement:
			if st == nil {
				continue
			}
			queue = append(queue, st.SubStatements...)
		case *statements.SynchronizedStatement:
			if st == nil {
				continue
			}
			queue = append(queue, st.Body...)
		case *statements.SwitchStatement:
			if st == nil {
				continue
			}
			for _, arm := range st.Cases {
				if arm != nil {
					queue = append(queue, arm.Body...)
				}
			}
		}
		if len(queue)+len(seen) > 8192 {
			return map[int]string{}
		}
	}
	if len(queue) > 0 {
		return map[int]string{}
	}
	for pc := range ambiguous {
		delete(result, pc)
	}
	return result
}

func valuesClassName(value *values.JavaClassValue) (string, bool) {
	if value == nil {
		return "", false
	}
	name, ok := types.RawClassFQN(value.JavaType)
	return strings.ReplaceAll(name, ".", "/"), ok
}

// Generated static methods participate in inherited hiding and erasure clashes.
// Reserve every source name from the complete parent graph. Missing tables or
// cycles cannot establish that a synthetic identifier is free.
func (c *ClassObjectDumper) checkedEscapeInheritedNames() (map[string]bool, bool) {
	reserved := map[string]bool{}
	// Annotation bridges live in a new nested class. They do not add a static
	// method to the annotation's inherited method namespace; the outer binary
	// member name is reserved separately against every original nested class.
	if slices.Contains(c.obj.AccessFlagsVerbose, "annotation") {
		return reserved, true
	}
	parents := append([]string{}, c.obj.GetInterfacesName()...)
	if superclass := c.obj.GetSupperClassName(); superclass != "" {
		parents = append(parents, superclass)
	}
	active, seen := map[string]bool{}, map[string]bool{}
	var visit func(string, int) bool
	visit = func(owner string, depth int) bool {
		owner = strings.ReplaceAll(owner, ".", "/")
		if owner == "java/lang/Object" {
			return true
		} // fixed language-defined member namespace
		if depth > 64 || len(seen) >= 128 || active[owner] || owner == c.obj.GetClassName() {
			return false
		}
		if seen[owner] {
			return true
		}
		if c.FuncCtx == nil || c.FuncCtx.InvocationMetadata == nil {
			return false
		}
		table, known := c.FuncCtx.InvocationMetadata(owner)
		if !known || strings.ReplaceAll(table.Name, ".", "/") != owner || !table.MembersComplete || !table.ParentsComplete {
			return false
		}
		active[owner] = true
		for _, member := range table.Methods {
			reserved[class_context.SafeIdentifier(member.Name)] = true
		}
		for _, parent := range table.Parents {
			if !visit(parent, depth+1) {
				return false
			}
		}
		delete(active, owner)
		seen[owner] = true
		return true
	}
	for _, parent := range parents {
		if !visit(parent, 0) {
			return reserved, false
		}
	}
	return reserved, true
}

func (c *ClassObjectDumper) checkedEscapeHelperName() string {
	reserved, _ := c.checkedEscapeInheritedNames()
	// Annotation lowering names a nested helper type; reserve every original
	// source identifier as well so a preserved parameter cannot shadow it.
	for _, constant := range c.ConstantPool {
		if utf8, ok := constant.(*ConstantUtf8Info); ok && utf8 != nil {
			reserved[class_context.SafeIdentifier(utf8.Value)] = true
		}
	}
	if c.FuncCtx != nil {
		for _, name := range c.FuncCtx.Arguments {
			reserved[class_context.SafeIdentifier(name)] = true
		}
		for _, name := range c.FuncCtx.LocalNames {
			reserved[class_context.SafeIdentifier(name)] = true
		}
	}
	for _, collection := range [][]*MemberInfo{c.obj.Methods, c.obj.Fields} {
		for _, member := range collection {
			name, _ := c.obj.getUtf8(member.NameIndex)
			reserved[class_context.SafeIdentifier(name)] = true
		}
	}
	for _, constant := range c.ConstantPool {
		if class, ok := constant.(*ConstantClassInfo); ok && class != nil {
			raw, _ := c.obj.getUtf8(class.NameIndex)
			prefix := c.obj.GetClassName() + "$"
			if strings.HasPrefix(raw, prefix) {
				reserved[class_context.SafeIdentifier(strings.TrimPrefix(raw, prefix))] = true
			}
		}
	}
	for i := 0; ; i++ {
		name := fmt.Sprintf("jdec$rethrow$%d", i)
		if !reserved[name] {
			return name
		}
	}
}

func (c *ClassObjectDumper) checkedEscapeCatchName() string {
	reserved := map[string]bool{}
	// Every original parameter/debug local name is a UTF8 constant even when
	// its attribute is opaque to this reader. Reserve the source spellings as
	// well as current scoped IR names, so future debug-name preservation cannot
	// introduce a collision with this generated catch declaration.
	for _, constant := range c.ConstantPool {
		if utf8, ok := constant.(*ConstantUtf8Info); ok && utf8 != nil {
			reserved[class_context.SafeIdentifier(utf8.Value)] = true
		}
	}
	if c.FuncCtx != nil {
		for _, name := range c.FuncCtx.LocalNames {
			reserved[class_context.SafeIdentifier(name)] = true
		}
		for _, name := range c.FuncCtx.Arguments {
			reserved[class_context.SafeIdentifier(name)] = true
		}
	}
	for i := 0; ; i++ {
		name := fmt.Sprintf("jdec$escape$%d", i)
		if !reserved[name] {
			return name
		}
	}
}

func (c *ClassObjectDumper) wrapCheckedEscapeBody(body string) string {
	name := c.checkedEscapeHelperName()
	caught := c.checkedEscapeCatchName()
	// The method name is unique throughout the complete family. An
	// unqualified invocation therefore binds this exact generated static
	// method without putting an owner type in the value namespace. The
	// throws-only E bound infers RuntimeException on Java 8+.
	call := name
	if slices.Contains(c.obj.AccessFlagsVerbose, "annotation") {
		call = name + ".rethrow"
	}
	return "try {\n" + body + "\n} catch (java.lang.Throwable " + caught + ") {\nthrow " + call + "(" + caught + ");\n}\n"
}

func constructorDelegationPC(body []statements.Statement) int {
	for _, statement := range body {
		if _, ok := statement.(*statements.MiddleStatement); ok {
			continue
		}
		if expr, ok := statement.(*statements.ExpressionStatement); ok && expr != nil {
			if call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression); ok && call != nil && call.HasOriginPC && call.Kind == values.InvokeSpecial && call.FunctionName == "<init>" {
				if receiver, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef); ok && receiver != nil && receiver.IsThis {
					return call.OriginPC
				}
			}
		}
		return -1
	}
	return -1
}

func (c *ClassObjectDumper) checkedEscapeHelper() *dumpedMethods {
	name := c.checkedEscapeHelperName()
	body := "throw (E) failure;"
	access := "private static"
	if c.isInterfaceLike() {
		access = "static"
	}
	code := fmt.Sprintf("%s <E extends java.lang.Throwable> java.lang.RuntimeException %s(java.lang.Throwable failure) throws E {%s}", access, name, body)
	if slices.Contains(c.obj.AccessFlagsVerbose, "annotation") {
		code = fmt.Sprintf("class %s {public static <E extends java.lang.Throwable> java.lang.RuntimeException rethrow(java.lang.Throwable failure) throws E {%s}}", name, body)
	}
	return &dumpedMethods{methodName: name, code: code, bodyCode: body}
}
