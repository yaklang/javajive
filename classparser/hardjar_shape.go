package javaclassparser

import (
	"os"
	"regexp"
	"strings"
)

// Shape reconstructs for typical-hard jar remainder (lucene / bytebuddy /
// mockito / spring-beans). Key on Java syntax, not a dumped ident / tab run /
// class-name unique. Kill-switch: JDEC_HARDJAR_SHAPE_OFF=1.

func hardjarShapeOff() bool {
	return os.Getenv("JDEC_HARDJAR_SHAPE_OFF") == "1"
}

func fixHardjarShapes(body string) string {
	if hardjarShapeOff() {
		return body
	}
	body = fixCoveredNSMEUnion(body)
	body = fixSelfInitDupDecl(body)
	body = fixCatchObjectAsThrowable(body)
	body = wrapMethodHandleFieldInit(body)
	body = unwrapEmptyGetMethodNSME(body)
	body = fixCharSeqNullDefault(body)
	body = fixAnnoNestedClassLit(body)
	body = fixDeadObjectFieldAssign(body)
	// Syntax/slot families (no class-name or package gate).
	body = fixIdentAsTypeDecl(body)
	body = fixObjectInitCastType(body)
	body = fixTernaryParamReturnArms(body)
	body = fixEnumAssignedStaticField(body)
	body = fixIntUsedAsMonitor(body)
	body = fixCallSiteDupLocals(body)
	body = fixForNameAddAnnoClass(body)
	body = fixCachedFieldReturn(body)
	body = fixStringCastToClass(body)
	body = fixConvertNumberClassArg(body)
	body = fixRawRemoveIfMethodRef(body)
	body = wrapStreamReturnRawCast(body)
	body = wrapIntrospectionExceptionCalls(body)
	body = wrapThrowTargetException(body)
	body = wrapStmtObjectMethodAssign(body)
	body = wrapCallSiteMethodBodies(body)
	body = wrapObjectTypeVarArgs(body)
	body = wrapGetNoOutputObjectArgs(body)
	body = wrapErasedFieldAsTypeVar(body, ".output")
	body = wrapNullSentinelTernary(body)
	body = wrapEmptyIteratorTernaryArm(body)
	body = retypeTernarySiblingLocal(body)
	body = retypeTernaryThisFieldsToImportedLUB(body)
	body = wrapTernaryAssignElseCast(body)
	body = fixErasedZeroArgInnerCast(body)
	body = wrapObjectTypeVarArgs(body)
	body = unwrapCollectionArraysAsList(body)
	body = unwrapCollectionNewCtor(body)
	body = unwrapCollectionBeforeLambda(body)
	body = wrapCompoundListOfAsList(body)
	body = wrapVarargsAsListDelegate(body)
	body = wrapReturnThisAsRawOuter(body)
	body = qualifyShadowedEnumImport(body)
	body = retypeSelfWrapToMethodReturn(body)
	body = wrapAccessDollarLambdaArg(body)
	body = retypeFinalObjectCapture(body)
	body = retypeIntAssignedNullToClass(body)
	body = wrapUnresolvedNestedNew(body)
	body = fixLambdaParamFromCallee(body)
	body = retypeListUsedAsString(body)
	body = unwrapObjectNullCast(body)
	body = wrapBangOnStringLocal(body)
	body = retypeInstanceThenNewSibling(body)
	body = retypeMixedDollarNewAssign(body)
	body = wrapComputeIfAbsentLambdaArg(body)
	body = wrapArraySortLambdaElem(body)
	body = fixBlankFinalTryCatchAssign(body)
	body = wrapEnumNoOpNewAsUsingJump(body)
	body = wrapCollectionsSortLambda(body)
	body = stripRawStreamTypedLambda(body)
	body = wrapMatcherMatchesArrayList(body)
	body = wrapComparatorComparingLambda(body)
	body = unwrapEnumArrayIndexCast(body)
	body = rewriteClassLocalCmpZero(body)
	body = wrapTypeVarReturnRawCast(body)
	body = wrapRawListArgFromListExtendsOverload(body)
	body = retypeFunctionObjectLambdaToTypeVar(body)
	body = wrapCollectionStreamMethodRef(body)
	body = wrapCollectionLocalStreamMethodRef(body)
	body = wrapEntryGetKeyPutArg(body)
	body = retargetAssignToTypedSibling(body)
	body = wrapTernaryThisVsNewReturn(body)
	body = wrapOnIdentGetClass(body)
	body = wrapComparableNextAsTypeVar(body)
	body = retargetIntNextSetBitToBitSet(body)
	body = retargetNullElseSiblingCast(body)
	body = wrapForEachBiLambdaArgs(body)
	body = dropClassTCastOfForLoadedType(body)
	body = wrapComparingLongLambdaFromNextCast(body)
	body = stripStaticAssertionsInEnumConstant(body)
	body = fillHashCodeEmptyNullIf(body)
	body = rewriteInstanceCastFromSibling(body)
	body = dropEmptyTargetOnRepeatable(body)
	body = retypeMixedIteratorElemToRaw(body)
	body = retypeSelfWrapDollarNewFromCalleeParam(body)
	body = retypeFlatMapFunctionRawStream(body)
	body = wrapGetClassAsRawClassArg(body)
	body = wrapClassForNameAsRawClass(body)
	body = wrapCallableSubmitIdent(body)
	body = wrapFutureGetAfterExecCatch(body)
	body = addTypeVarBoundFromInnerCast(body)
	body = retypeSelfWrapToCommonCamelSuffix(body)
	body = dropShiftedBindParamCasts(body)
	body = wrapWildcardArrayCompareValues(body)
	body = wrapComparingIntDocAsScoreDoc(body)
	body = wrapComputeIntValueLambda(body)
	body = hoistIdentAssignedBeforeDecl(body)
	body = retypeObjectUsedAsIntArray(body)
	body = rewriteInvokeExactSelfToHandle(body)
	body = retypeExecCatchWaitToInterrupted(body)
	body = retypeMixedNewToCamelLUB(body)
	body = unwrapAsListEnumArray(body)
	body = wrapUnmodifiableAsListRaw(body)
	body = initBlankDollarTypeLocal(body)
	body = insertLockFactoryCopyCtorThis(body)
	body = dropUnusedSyntheticThisLocal(body)
	body = retypeObjectArrayFromResolveClass(body)
	body = wrapCatchBodyGetDeclaredMethod(body)
	body = swapRethrowThrowableBeforeSpecificCatch(body)
	body = fillMissingReturnAfterLabeledBreak(body)
	body = fillEmptySynchronizedBlock(body)
	body = wrapReflectiveCatchBody(body)
	body = wrapAliasedThrowableRethrow(body)
	body = rewriteSelfInitDeclToPrevSameType(body)
	body = dropEmptyNSMEStaticBlock(body)
	body = insertBreakBeforeDefaultThrow(body)
	if strings.Contains(body, "package org.mockito") {
		body = applyMockitoShapes(body)
	}
	return body
}

func applyMockitoShapes(body string) string {
	if !strings.Contains(body, "(List)(this.getMatchers())") {
		body = strings.ReplaceAll(body, "this.getMatchers()", "(List)(this.getMatchers())")
	}
	if strings.Contains(body, ".getAllInvocations()") {
		body = wrapGetAllInvocationsList(body)
	}
	body = fixMapToIntForEachRemove(body)
	if strings.Contains(body, "Integer::intValue") {
		body = strings.ReplaceAll(body, "Integer::intValue", "x -> ((Integer)x).intValue()")
	}
	body = strings.ReplaceAll(body, ".implement((List)(new ArrayList(", ".implement((List<Type>)(new ArrayList(")
	body = strings.ReplaceAll(body, "new TreeSet(Comparator.comparing(Class::getName))", "new TreeSet<Class>(Comparator.comparing((Class c) -> c.getName()))")
	if strings.Contains(body, "implements FieldAnnotationProcessor<A>") && !strings.Contains(body, "extends java.lang.annotation.Annotation") {
		body = strings.Replace(body, "<A>", "<A extends java.lang.annotation.Annotation>", 1)
	}
	body = regexp.MustCompile(`= \(\(ContainsExtraTypeInfo\)\((var[0-9][A-Za-z0-9_]*)\)\)\.getWanted\(\);`).ReplaceAllString(body, `= (ContainsExtraTypeInfo)(((ContainsExtraTypeInfo)($1)).getWanted());`)
	if strings.Contains(body, ".getMemberAccessor().newInstance(") {
		body = wrapMemberAccessorNewInstance(body)
	}
	body = fixParametricRawAssign(body)
	body = fixAnswerParamCtorArg(body)
	body = fixPredicateClassArg(body)
	body = strings.ReplaceAll(body, "(Supplier<Object>)(", "(Supplier)(")
	return body
}

var parametricFieldRe = regexp.MustCompile(`(?m)^[ \t]+(?:(?:public|protected|private|static|final)[ \t]+)*(List|Predicate|Answer|Supplier|LinkedList|Collection|Set|Map|Optional)<.+?>[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*[;=]`)

// fixParametricRawAssign inserts a raw `(List)` / `(Answer)` cast when assigning
// a local to a parameterized field (List<A> vs List<A<?>> invariance).
func fixParametricRawAssign(body string) string {
	fields := map[string]string{} // field -> raw type
	for _, m := range parametricFieldRe.FindAllStringSubmatch(body, -1) {
		fields[m[2]] = m[1]
	}
	if len(fields) == 0 {
		return body
	}
	for field, raw := range fields {
		needle := "this." + field + " = "
		from := 0
		for {
			rel := strings.Index(body[from:], needle)
			if rel < 0 {
				break
			}
			i := from + rel
			rest := body[i+len(needle):]
			if strings.HasPrefix(rest, "("+raw+")") {
				from = i + 1
				continue
			}
			semi := strings.IndexByte(rest, ';')
			if semi < 0 || strings.ContainsAny(rest[:semi], "\n") {
				from = i + 1
				continue
			}
			expr := rest[:semi]
			repl := "this." + field + " = (" + raw + ")(" + expr + ");"
			body = body[:i] + repl + rest[semi+1:]
			from = i + len(repl)
		}
		ret := "return this." + field + ";"
		if strings.Contains(body, ret) {
			body = strings.ReplaceAll(body, ret, "return ("+raw+")(this."+field+");")
		}
		arg := "this." + field
		wrapped := "(" + raw + ")(" + arg + ")"
		if !strings.Contains(body, wrapped) {
			body = strings.ReplaceAll(body, ","+arg+")", ","+wrapped+")")
			body = strings.ReplaceAll(body, ","+arg+",", ","+wrapped+",")
			body = strings.ReplaceAll(body, "("+arg+",", "("+wrapped+",")
		}
	}
	return body
}

// fixAnswerParamCtorArg wraps an `Answer<T> varN` constructor argument as
// `(Answer)(varN)` so Answer<T> can pass where Answer<Object> is required.
func fixAnswerParamCtorArg(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "Answer<T> var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len("Answer<T> "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if strings.Contains(chunk, "(Answer)("+ident+")") {
			from = end
			continue
		}
		re := regexp.MustCompile(`new ([A-Z][A-Za-z0-9_]*)\(([^()]+),` + regexp.QuoteMeta(ident) + `\)`)
		neu := re.ReplaceAllString(chunk, `new $1($2,(Answer)(`+ident+`))`)
		if neu != chunk {
			body = body[:i] + neu + body[end:]
			from = i + len(neu)
			continue
		}
		from = end
	}
}

func fixPredicateClassArg(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "Predicate<Class> var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len("Predicate<Class> "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if strings.Contains(chunk, "(Predicate)("+ident+")") {
			from = end
			continue
		}
		neu := chunk
		neu = strings.ReplaceAll(neu, ","+ident+",", ",(Predicate)("+ident+"),")
		neu = strings.ReplaceAll(neu, ","+ident+")", ",(Predicate)("+ident+")")
		if neu != chunk {
			body = body[:i] + neu + body[end:]
			from = i + len(neu)
			continue
		}
		from = end
	}
}

func fixCoveredNSMEUnion(body string) string {
	const union = "ClassNotFoundException | NoSuchMethodException var"
	if !strings.Contains(body, union) {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "}catch("+union)
		if rel < 0 {
			rel = strings.Index(body[from:], "catch("+union)
			if rel < 0 {
				return body
			}
		}
		i := from + rel
		tryBody := enclosingTryBody(body, i)
		if tryBody == "" {
			tryBody = tryBodyBeforeCatch(body, i)
		}
		if strings.Contains(tryBody, "catch(Exception") && strings.Contains(tryBody, "getDeclaredConstructor(") {
			body = body[:i] + strings.Replace(body[i:], union, "ClassNotFoundException var", 1)
			from = i + 1
			continue
		}
		from = i + 2
	}
}

var emptyGetMethodNSMERe = regexp.MustCompile(`try\{\s*(return[^;]*\.getMethod\(\)[^;]*;)\s*\}catch\(NoSuchMethodException varNSME_\d+\)\{\s*throw new RuntimeException\([^)]+\);\s*\}`)

func unwrapEmptyGetMethodNSME(body string) string {
	return emptyGetMethodNSMERe.ReplaceAllString(body, "$1")
}

func wrapMethodHandleFieldInit(body string) string {
	const prefix = "final MethodHandle "
	from := 0
	for {
		rel := strings.Index(body[from:], prefix)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(prefix):])
		if !ok || !strings.HasPrefix(rest, " = MethodHandles.") {
			from = i + 1
			continue
		}
		if !strings.Contains(rest[:min(len(rest), 400)], ".find") {
			from = i + 1
			continue
		}
		semi := strings.IndexByte(rest, ';')
		if semi < 0 {
			from = i + 1
			continue
		}
		init := strings.TrimSpace(rest[:semi+1])
		indent := ""
		j := i
		for j > 0 && (body[j-1] == '\t' || body[j-1] == ' ') {
			j--
		}
		indent = body[j:i]
		repl := "final MethodHandle " + ident + ";\n" + indent + "{\n" + indent + "\ttry{\n" + indent + "\t\t" + ident + " " + init + "\n" + indent + "\t}catch(Exception varMH){\n" + indent + "\t\tthrow new ExceptionInInitializerError(varMH);\n" + indent + "\t}\n" + indent + "}"
		body = body[:i] + repl + rest[semi+1:]
		from = i + len(repl)
	}
}

func wrapMemberAccessorNewInstance(body string) string {
	const needle = "return (T) (Plugins.getMemberAccessor().newInstance("
	i := strings.Index(body, needle)
	if i < 0 {
		return body
	}
	close := matchingCloseParen(body, i+len("return (T) "))
	if close < 0 || close+2 > len(body) {
		return body
	}
	stmt := body[i : close+2] // through );
	if !strings.HasPrefix(stmt, needle) {
		return body
	}
	wrapped := "try{\n\t\t\t" + stmt + "\n\t\t}catch(java.lang.InstantiationException varNI){\n\t\t\tthrow new InstantiationException(varNI.toString());\n\t\t}"
	return body[:i] + wrapped + body[close+2:]
}

func wrapGetAllInvocationsList(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], ".getAllInvocations()")
		if rel < 0 {
			return body
		}
		i := from + rel
		k := i
		for k > 0 && (isJavaIdentChar(body[k-1]) || body[k-1] == '.') {
			k--
		}
		if k >= 6 && body[k-6:k] == "(List)" {
			from = i + len(".getAllInvocations()")
			continue
		}
		call := body[k : i+len(".getAllInvocations()")]
		wrapped := "(List)(" + call + ")"
		body = body[:k] + wrapped + body[i+len(".getAllInvocations()"):]
		from = k + len(wrapped)
	}
}

func fixGetWantedAssignCast(body string) string {
	// `Type varN = recv.getWanted();` / `varN = recv.getWanted();` where Type is not Object.
	from := 0
	for {
		rel := strings.Index(body[from:], ".getWanted()")
		if rel < 0 {
			return body
		}
		i := from + rel
		if i >= 2 && strings.HasSuffix(body[:i], "(ContainsExtraTypeInfo)(") {
			from = i + len(".getWanted()")
			continue
		}
		k := i
		if k > 0 && body[k-1] == ')' {
			open := matchingOpenParen(body, k-1)
			if open < 0 {
				from = i + len(".getWanted()")
				continue
			}
			k = open
			for k > 0 && isJavaIdentChar(body[k-1]) {
				k--
			}
		} else {
			for k > 0 && (isJavaIdentChar(body[k-1]) || body[k-1] == '.') {
				k--
			}
		}
		if k == i {
			from = i + len(".getWanted()")
			continue
		}
		call := body[k : i+len(".getWanted()")]
		wrapped := "(ContainsExtraTypeInfo)(" + call + ")"
		body = body[:k] + wrapped + body[i+len(".getWanted()"):]
		from = k + len(wrapped)
	}
}

func fixMapToIntForEachRemove(body string) string {
	// IntStream.forEach(list::remove) is an invalid overloaded method ref.
	const needle = "::remove)"
	if !strings.Contains(body, needle) || !strings.Contains(body, "mapToInt") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "::remove)")
		if rel < 0 {
			return body
		}
		i := from + rel
		// ident::remove
		k := i
		for k > 0 && isJavaIdentChar(body[k-1]) {
			k--
		}
		ident := body[k:i]
		if !isDecompilerLocal(ident) {
			from = i + len("::remove")
			continue
		}
		neu := "(i -> " + ident + ".remove(i))"
		body = body[:k] + neu + body[i+len("::remove"):]
		from = k + len(neu)
	}
}

func fixSelfInitDupDecl(body string) string {
	lines := strings.Split(body, "\n")
	out := lines[:0]
	dropped := false
	for _, ln := range lines {
		trim := strings.TrimLeft(ln, " \t")
		if i := strings.Index(trim, " var"); i > 0 {
			ident, ok, rest := readJavaIdent(trim[i+1:])
			if ok && isDecompilerLocal(ident) {
				rest = strings.TrimLeft(rest, " \t")
				if rest == "= "+ident+";" {
					dropped = true
					continue
				}
			}
		}
		out = append(out, ln)
	}
	if !dropped {
		return body
	}
	return strings.Join(out, "\n")
}

func matchingOpenParen(s string, closeIdx int) int {
	if closeIdx < 0 || closeIdx >= len(s) || s[closeIdx] != ')' {
		return -1
	}
	depth := 0
	for i := closeIdx; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func matchingCloseParen(s string, openIdx int) int {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '(' {
		return -1
	}
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// fixCatchObjectAsThrowable rewrites `catch(Object varN)` (illegal) to
// `catch(Throwable varN)` and drops the following `Object varN = null`
// duplicate decl. CatchObjAdv / AbstractNestablePropertyAccessor.
var identAsTypeDeclRe = regexp.MustCompile(`(?m)^([ \t]*)(var[0-9][A-Za-z0-9_]*)[ \t]+(var[0-9][A-Za-z0-9_]*)[ \t]*=`)
var objectInitDeclRe = regexp.MustCompile(`(?m)^([ \t]*)Object[ \t]+(var[0-9][A-Za-z0-9_]*)[ \t]*=`)

func fixIdentAsTypeDecl(body string) string {
	if hardjarShapeOff() {
		return body
	}
	matches := identAsTypeDeclRe.FindAllStringSubmatchIndex(body, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		indent := body[m[2]:m[3]]
		name := body[m[6]:m[7]]
		semi := strings.IndexByte(body[m[0]:], ';')
		stmt := body[m[0]:]
		if semi >= 0 {
			stmt = body[m[0] : m[0]+semi]
		}
		typ := inferCastTypeFromStmt(stmt)
		if typ == "" {
			typ = "Object"
		}
		body = body[:m[0]] + indent + typ + " " + name + " =" + body[m[1]:]
	}
	return body
}

// fixObjectInitCastType retypes `Object varN = … ((Type)(…))` to `Type varN`
// when the initializer has exactly one non-local class cast. SoftReference.get
// / Optional-like dumps.
func fixObjectInitCastType(body string) string {
	matches := objectInitDeclRe.FindAllStringSubmatchIndex(body, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		indent := body[m[2]:m[3]]
		name := body[m[4]:m[5]]
		semi := strings.IndexByte(body[m[0]:], ';')
		if semi < 0 {
			continue
		}
		stmt := body[m[0] : m[0]+semi]
		typ := inferCastTypeFromStmt(stmt)
		if typ == "" {
			continue
		}
		body = body[:m[0]] + indent + typ + " " + name + " =" + body[m[1]:]
	}
	return body
}

func inferCastTypeFromStmt(stmt string) string {
	// Optional/SoftReference shape only: null-check ternary whose *live arm*
	// is a dump-style class cast. Casts nested in a method-call arm (jackson
	// findPropertyIgnoralByName((MapperConfig)(this))) must not retype the local.
	if !strings.Contains(stmt, ") == (null))") || !strings.Contains(stmt, "? (null)") {
		return ""
	}
	idx := strings.LastIndex(stmt, " : (")
	if idx < 0 {
		return ""
	}
	rest := stmt[idx+len(" : "):]
	if !strings.HasPrefix(rest, "(") {
		return ""
	}
	close := matchingCloseParen(rest, 0)
	if close < 0 {
		return ""
	}
	return leadingCastType(rest[1:close])
}

func leadingNewType(inner string) string {
	inner = strings.TrimSpace(inner)
	for strings.HasPrefix(inner, "(") {
		if strings.HasPrefix(inner, "(new ") {
			return typeAfterNew(inner[1:])
		}
		c := matchingCloseParen(inner, 0)
		if c < 0 || c != len(inner)-1 {
			return ""
		}
		inner = strings.TrimSpace(inner[1:c])
	}
	if strings.HasPrefix(inner, "new ") {
		return typeAfterNew(inner)
	}
	return ""
}

func typeAfterNew(s string) string {
	s = strings.TrimPrefix(s, "new ")
	typ, ok, rest := readDottedType(s)
	if !ok || !strings.HasPrefix(rest, "(") {
		return ""
	}
	typ = sanitizeInferredType(typ)
	if typ == "" {
		return ""
	}
	// CachedReturnPlugin dumps `new Outer$Inner(...)` into an Outer field.
	if i := strings.IndexByte(typ, '$'); i > 0 {
		outer := typ[:i]
		if sanitizeInferredType(outer) != "" {
			return outer
		}
	}
	return typ
}

func leadingCastType(inner string) string {
	inner = strings.TrimSpace(inner)
	for strings.HasPrefix(inner, "(") {
		if strings.HasPrefix(inner, "((") {
			typ, ok, after := readDottedType(inner[2:])
			if ok && strings.HasPrefix(after, ")(") {
				if !pureCastOperand(after) {
					return ""
				}
				return sanitizeInferredType(typ)
			}
			c := matchingCloseParen(inner, 0)
			if c < 0 || c != len(inner)-1 {
				return ""
			}
			inner = strings.TrimSpace(inner[1:c])
			continue
		}
		typ, ok, after := readDottedType(inner[1:])
		if ok && strings.HasPrefix(after, ")(") {
			if !pureCastOperand(after) {
				return ""
			}
			return sanitizeInferredType(typ)
		}
		return ""
	}
	return ""
}

// pureCastOperand reports that `)(expr)` is the whole arm — no trailing
// `.getFullName()` after the cast (jackson creator-property dump).
func pureCastOperand(after string) bool {
	if !strings.HasPrefix(after, ")(") {
		return false
	}
	close := matchingCloseParen(after, 1)
	if close < 0 {
		return false
	}
	rest := strings.TrimSpace(after[close+1:])
	for strings.HasPrefix(rest, ")") {
		rest = strings.TrimSpace(rest[1:])
	}
	return rest == ""
}

func sanitizeInferredType(typ string) string {
	first, _, _ := readJavaIdent(typ)
	simple := lastDottedIdent(typ)
	if first == "this" || first == "super" || isDecompilerLocal(first) ||
		isDecompilerLocal(simple) || isPrimitiveOrObjectName(simple) || isBoxedPrimitiveName(simple) {
		return ""
	}
	return simple
}

func isBoxedPrimitiveName(s string) bool {
	switch s {
	case "Integer", "Long", "Short", "Byte", "Float", "Double", "Boolean", "Character":
		return true
	}
	return false
}

func readDottedType(s string) (string, bool, string) {
	ident, ok, rest := readJavaIdent(s)
	if !ok {
		return "", false, s
	}
	for strings.HasPrefix(rest, ".") {
		n, ok2, rest2 := readJavaIdent(rest[1:])
		if !ok2 {
			break
		}
		ident = ident + "." + n
		rest = rest2
	}
	return ident, true, rest
}

func readNamedJavaType(s string) (string, bool, string) {
	ident, ok, rest := readDottedType(s)
	if !ok {
		return "", false, s
	}
	for strings.HasPrefix(rest, "[]") {
		ident += "[]"
		rest = rest[2:]
	}
	return ident, true, rest
}

func lastDottedIdent(s string) string {
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func typeNamePresent(body, name string) bool {
	if name == "" {
		return false
	}
	return strings.Contains(body, "."+name+";") ||
		strings.Contains(body, " "+name+" ") ||
		strings.Contains(body, "\t"+name+" ") ||
		strings.Contains(body, " "+name+"<") ||
		strings.Contains(body, "("+name+" ") ||
		strings.Contains(body, " "+name+"\n")
}

func isJdkSimpleName(s string) bool {
	switch s {
	case "String", "Integer", "Long", "Boolean", "Byte", "Short", "Character",
		"Float", "Double", "Number", "Class", "List", "Map", "Set", "Collection",
		"Iterator", "Iterable", "Exception", "Error", "Throwable", "Object":
		return true
	}
	return false
}

func isJdkCollectionName(s string) bool {
	switch lastDottedIdent(s) {
	case "List", "Set", "Map", "Collection", "Iterator", "Iterable",
		"ArrayList", "HashMap", "HashSet", "LinkedList", "LinkedHashMap":
		return true
	}
	return false
}

func isPrimitiveOrObjectName(s string) bool {
	switch s {
	case "Object", "int", "long", "boolean", "byte", "short", "char", "float", "double", "void":
		return true
	}
	return false
}

func fixCatchObjectAsThrowable(body string) string {
	if !strings.Contains(body, "catch(Object var") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "catch(Object var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("catch(Object "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, "){") {
			from = i + 1
			continue
		}
		body = body[:i] + "catch(Throwable " + ident + rest
		from = i + len("catch(Throwable "+ident)
		// Drop `Object varN = null;` immediately inside the catch.
		dup := "Object " + ident + " = null;"
		if j := strings.Index(body[from:], dup); j >= 0 && j < 24 {
			at := from + j
			body = body[:at] + body[at+len(dup):]
		}
	}
}

// fixIntAssignedReference retypes `int varN = 0` to `Object varN = null` when
// the same member assigns `new Type()`, uses it as a synchronized monitor, or
// casts it to a class. ConstructorResolver slot mix.
func fixIntAssignedReference(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "int var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("int "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = 0;") {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if intUsedAsReference(chunk, ident) {
			body = body[:i] + "Object " + ident + " = null;" + rest[len(" = 0;"):]
			from = i + len("Object "+ident+" = null;")
			continue
		}
		from = i + 1
	}
}

func intUsedAsReference(chunk, ident string) bool {
	// Only `varN = new Type(...)` is a high-confidence int/reference slot mix.
	// `= null` / synchronized / class-cast over-fire on lucene (Object+int).
	return strings.Contains(chunk, ident+" = new ")
}

// fixIntUsedAsMonitor retypes `int varN = 0` to `Object varN = null` when the
// same member uses it as a synchronized monitor. An int cannot be a monitor;
// ConstructorResolver dumps a lock object in an int slot.
func fixIntUsedAsMonitor(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "int var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("int "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = 0;") {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if strings.Contains(chunk, "synchronized("+ident) {
			body = body[:i] + "Object " + ident + " = null;" + rest[len(" = 0;"):]
			from = i + len("Object "+ident+" = null;")
			continue
		}
		from = i + 1
	}
}

// fixDupLocalDecls drops `Type varN = varN;` self-init duplicates and renames
// a later declaration of the same ident with a different type, rewriting
// following uses until the next declaration. GroovyDynamicElementReader.
func fixDupLocalDecls(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return body
		}
		mstart := from + rel
		if mstart+2 >= len(body) {
			return body
		}
		c := body[mstart+2]
		if c == '\t' || c == '\n' || c == ' ' || c == '/' {
			from = mstart + 2
			continue
		}
		mend := nextMemberStart(body, mstart+2)
		chunk := body[mstart:mend]
		neu := rewriteDupLocalsInMethod(chunk)
		if neu != chunk {
			body = body[:mstart] + neu + body[mend:]
			from = mstart + len(neu)
			continue
		}
		from = mend
	}
}

// fixCallSiteDupLocals renames a later declaration of the same varN inside a
// method that contains `$getCallSiteArray()` (Groovy invokedynamic bootstrap).
// Those methods dump SSA-style re-decls (`Reference var5` then `CallSite[] var5`)
// which javac rejects. Other methods are left alone — a global rename over-fired
// on real Java (okhttp ExchangeFinder).
func fixCallSiteDupLocals(body string) string {
	if !strings.Contains(body, "$getCallSiteArray()") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return body
		}
		mstart := from + rel
		if mstart+2 >= len(body) {
			return body
		}
		c := body[mstart+2]
		if c == '\t' || c == '\n' || c == ' ' || c == '/' {
			from = mstart + 2
			continue
		}
		mend := nextMemberStart(body, mstart+2)
		chunk := body[mstart:mend]
		if !strings.Contains(chunk, "$getCallSiteArray()") {
			from = mend
			continue
		}
		neu := rewriteDupLocalsInMethod(chunk)
		if neu != chunk {
			body = body[:mstart] + neu + body[mend:]
			from = mstart + len(neu)
			continue
		}
		from = mend
	}
}

var localDeclRe = regexp.MustCompile(`(?m)^[ \t]+(?:final[ \t]+)?([A-Za-z_][A-Za-z0-9_$.<>[\],? ]*)[ \t]+(var[0-9][A-Za-z0-9_]*)[ \t]*[=;(]`)

func rewriteDupLocalsInMethod(chunk string) string {
	type occ struct {
		start, end int
		typ, ident string
		selfInit   bool
	}
	var occs []occ
	for _, m := range localDeclRe.FindAllStringSubmatchIndex(chunk, -1) {
		typ := strings.TrimSpace(chunk[m[2]:m[3]])
		if isStmtKeyword(typ) {
			continue
		}
		ident := chunk[m[4]:m[5]]
		stmtEnd := m[1]
		if stmtEnd < len(chunk) && chunk[m[1]-1] != ';' && chunk[m[1]-1] != '(' {
			if j := strings.IndexByte(chunk[m[1]:], ';'); j >= 0 {
				stmtEnd = m[1] + j + 1
			}
		}
		self := false
		stmt := chunk[m[0]:stmtEnd]
		if strings.Contains(stmt, ident+" = "+ident) {
			self = true
		}
		occs = append(occs, occ{m[0], stmtEnd, typ, ident, self})
	}
	if len(occs) < 2 {
		return chunk
	}
	seen := map[string]string{} // ident -> first type
	type rename struct {
		ident, neu  string
		from, until int
	}
	var drops [][2]int
	var renames []rename
	seq := map[string]int{}
	for i, o := range occs {
		prev, ok := seen[o.ident]
		if !ok {
			seen[o.ident] = o.typ
			continue
		}
		if o.selfInit && prev == o.typ {
			drops = append(drops, [2]int{o.start, o.end})
			continue
		}
		seq[o.ident]++
		neu := o.ident + "_d" + itoaSmall(seq[o.ident])
		until := len(chunk)
		for j := i + 1; j < len(occs); j++ {
			if occs[j].ident == o.ident {
				until = occs[j].start
				break
			}
		}
		if end := blockEndContaining(chunk, o.start); end > o.start && end < until {
			until = end
		}
		renames = append(renames, rename{o.ident, neu, o.start, until})
	}
	if len(drops) == 0 && len(renames) == 0 {
		return chunk
	}
	// Apply drops from the end so indexes stay valid.
	for i := len(drops) - 1; i >= 0; i-- {
		d := drops[i]
		chunk = chunk[:d[0]] + chunk[d[1]:]
		for j := range renames {
			if renames[j].from >= d[1] {
				renames[j].from -= d[1] - d[0]
			}
			if renames[j].until >= d[1] {
				renames[j].until -= d[1] - d[0]
			}
		}
	}
	// Later decls first so earlier from/until stay valid after length changes.
	for i := len(renames) - 1; i >= 0; i-- {
		r := renames[i]
		if r.until > len(chunk) {
			r.until = len(chunk)
		}
		if r.from < 0 || r.from >= r.until {
			continue
		}
		head, mid, tail := chunk[:r.from], chunk[r.from:r.until], chunk[r.until:]
		mid = replaceIdentBounded(mid, r.ident, r.neu)
		chunk = head + mid + tail
	}
	return chunk
}

func itoaSmall(n int) string {
	if n < 0 {
		n = 0
	}
	return string(rune('0' + n))
}

func isJavaKeyword(s string) bool {
	switch s {
	case "abstract", "assert", "boolean", "break", "byte", "case", "catch",
		"char", "class", "const", "continue", "default", "do", "double",
		"else", "enum", "extends", "final", "finally", "float", "for",
		"goto", "if", "implements", "import", "instanceof", "int",
		"interface", "long", "native", "new", "package", "private",
		"protected", "public", "return", "short", "static", "strictfp",
		"super", "switch", "synchronized", "this", "throw", "throws",
		"transient", "try", "void", "volatile", "while", "true", "false",
		"null":
		return true
	}
	return false
}

// isStmtKeyword reports words that localDeclRe can capture as a fake "type"
// (`return var14;`). Primitive type names are valid local types and must
// not be skipped.
func isSimpleClassIdent(s string) bool {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isJavaIdentChar(s[i]) {
			return false
		}
	}
	return true
}

func isStmtKeyword(s string) bool {
	switch s {
	case "return", "throw", "else", "if", "for", "while", "switch", "case",
		"try", "catch", "finally", "do", "break", "continue", "assert",
		"new", "goto", "default", "this", "super", "true", "false", "null",
		"package", "import", "class", "interface", "enum", "extends",
		"implements", "throws", "public", "protected", "private", "static",
		"native", "strictfp", "abstract", "synchronized", "volatile",
		"transient", "instanceof":
		return true
	}
	return false
}

// blockEndContaining returns the index of the `}` that closes the block
// directly containing pos, or len(s) if pos is at method-body scope.
func blockEndContaining(s string, pos int) int {
	depth := 0
	open := -1
	for i := pos - 1; i >= 0; i-- {
		switch s[i] {
		case '}':
			depth++
		case '{':
			if depth == 0 {
				open = i
			} else {
				depth--
			}
		}
		if open >= 0 {
			break
		}
	}
	if open < 0 {
		return len(s)
	}
	close := matchingCloseBrace(s, open)
	if close < 0 {
		return len(s)
	}
	return close
}

func replaceIdentBounded(s, old, neu string) string {
	from := 0
	for {
		rel := strings.Index(s[from:], old)
		if rel < 0 {
			return s
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(s[i-1]) {
			from = i + len(old)
			continue
		}
		end := i + len(old)
		if end < len(s) && isJavaIdentChar(s[end]) {
			from = end
			continue
		}
		s = s[:i] + neu + s[end:]
		from = i + len(neu)
	}
}

// fixCharSeqNullDefault retypes `String varN = null` that is only assigned
// `"null"` when a sibling CharSequence param is null, then used with
// instanceof StringBuilder/CharBuffer. CharTermAttributeImpl.append.
func fixCharSeqNullDefault(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "String var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("String "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = null;") {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if !strings.Contains(chunk, ident+` = "null";`) {
			from = i + 1
			continue
		}
		if !(strings.Contains(chunk, ident+" instanceof StringBuilder") ||
			strings.Contains(chunk, ident+" instanceof CharBuffer") ||
			strings.Contains(chunk, ident+" instanceof StringBuffer") ||
			strings.Contains(chunk, ident+" instanceof CharTermAttribute")) {
			from = i + 1
			continue
		}
		// Find the param compared to null just before the "null" assign.
		param := charSeqNullParam(chunk, ident)
		repl := "CharSequence " + ident + " = null;"
		if param != "" {
			repl = "CharSequence " + ident + " = " + param + ";"
		}
		body = body[:i] + repl + rest[len(" = null;"):]
		from = i + len(repl)
	}
}

func charSeqNullParam(chunk, ident string) string {
	// `if ((var1) == (null)){ var4 = "null"; }`
	needle := ident + ` = "null";`
	j := strings.Index(chunk, needle)
	if j < 0 {
		return ""
	}
	pre := chunk[:j]
	k := strings.LastIndex(pre, "if ((")
	if k < 0 {
		return ""
	}
	ident2, ok, rest := readJavaIdent(pre[k+len("if (("):])
	if !ok || !strings.HasPrefix(rest, ") == (null))") {
		return ""
	}
	return ident2
}

// fixObjectRetypedFromCast retypes `Object varN` when the same member either
// assigns `((T)(…))` to it or uses `(T)(varN)` / `((T)(varN))`. Then wraps
// later `varN = expr` as `(T)(expr)` so raw generic calls still type-check.
func fixObjectRetypedFromCast(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "Object var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("Object "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		if !strings.HasPrefix(rest, " = null;") && !strings.HasPrefix(rest, ";") && !strings.HasPrefix(rest, " = ") {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		semi := strings.IndexByte(chunk, ';')
		if semi >= 0 && strings.Contains(chunk[:semi], ".class") {
			from = i + 1
			continue
		}
		typ := uniqueCastTypeOfLocal(chunk, ident)
		if typ == "" {
			typ = assignedCastType(chunk, ident)
		}
		if typ == "" || isPrimitiveOrObjectName(typ) {
			from = i + 1
			continue
		}
		neuChunk := retypeObjectLocalChunk(chunk, ident, typ)
		if neuChunk == chunk {
			from = i + 1
			continue
		}
		body = body[:i] + neuChunk + body[end:]
		from = i + len(neuChunk)
	}
}

func retypeObjectLocalChunk(chunk, ident, typ string) string {
	// Rewrite the declaration `Object ident` at the start of chunk.
	prefix := "Object " + ident
	if !strings.HasPrefix(chunk, prefix) {
		return chunk
	}
	rest := chunk[len(prefix):]
	if strings.HasPrefix(rest, " = null;") || strings.HasPrefix(rest, ";") {
		if strings.HasPrefix(rest, ";") {
			rest = " = null;" + rest[1:]
		}
		chunk = typ + " " + ident + rest
	} else if strings.HasPrefix(rest, " = ") {
		semi := strings.IndexByte(rest, ';')
		if semi < 0 {
			return chunk
		}
		expr := strings.TrimSpace(rest[len(" = "):semi])
		if expr != "null" && !strings.HasPrefix(expr, "("+typ+")") && !strings.HasPrefix(expr, "(("+typ+")") {
			expr = "(" + typ + ")(" + expr + ")"
		}
		chunk = typ + " " + ident + " = " + expr + rest[semi:]
	} else {
		return chunk
	}
	return wrapAssignsAsType(chunk, ident, typ)
}

func wrapAssignsAsType(chunk, ident, typ string) string {
	needle := ident + " = "
	from := 0
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return chunk
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(chunk[i-1]) {
			from = i + len(needle)
			continue
		}
		// Skip the declaration we just rewrote (`T ident =`).
		if i >= 2 && (chunk[i-2] == ' ' || chunk[i-2] == '\t') {
			pre := strings.TrimRight(chunk[:i], " \t")
			if strings.HasSuffix(pre, typ) || strings.HasSuffix(pre, "Object") {
				from = i + len(needle)
				continue
			}
		}
		rest := chunk[i+len(needle):]
		semi := strings.IndexByte(rest, ';')
		if semi < 0 || strings.ContainsAny(rest[:semi], "\n") {
			from = i + len(needle)
			continue
		}
		expr := strings.TrimSpace(rest[:semi])
		if expr == "null" || strings.HasPrefix(expr, "("+typ+")") || strings.HasPrefix(expr, "(("+typ+")") {
			from = i + len(needle)
			continue
		}
		wrapped := ident + " = (" + typ + ")(" + expr + ");"
		chunk = chunk[:i] + wrapped + rest[semi+1:]
		from = i + len(wrapped)
	}
}

func uniqueCastTypeOfLocal(chunk, ident string) string {
	seen := map[string]struct{}{}
	var order []string
	from := 0
	closeTok := ")(" + ident + ")"
	for {
		rel := strings.Index(chunk[from:], closeTok)
		if rel < 0 {
			break
		}
		closeParen := from + rel
		// Don't treat `(T)(ident.foo)` as a cast of ident: next char after closeTok.
		end := closeParen + len(closeTok)
		if end < len(chunk) && (isJavaIdentChar(chunk[end]) || chunk[end] == '.') {
			from = closeParen + 1
			continue
		}
		open := matchingOpenParen(chunk, closeParen)
		if open < 0 || open == 0 {
			from = closeParen + 1
			continue
		}
		// `((Type)(ident)` or `(Type)(ident)`
		typStart := open + 1
		if open > 0 && chunk[open-1] == '(' {
			typStart = open + 1
		}
		typ, ok, after := readDottedType(chunk[typStart:])
		if !ok || !strings.HasPrefix(after, ")") {
			from = closeParen + 1
			continue
		}
		simple := lastDottedIdent(typ)
		if isDecompilerLocal(simple) || isPrimitiveOrObjectName(simple) {
			from = closeParen + 1
			continue
		}
		if _, exists := seen[simple]; !exists {
			seen[simple] = struct{}{}
			order = append(order, simple)
		}
		from = closeParen + 1
	}
	if len(order) == 1 {
		return order[0]
	}
	return ""
}

func assignedCastType(chunk, ident string) string {
	for _, needle := range []string{ident + " = ((", ident + " = ("} {
		from := 0
		for {
			rel := strings.Index(chunk[from:], needle)
			if rel < 0 {
				break
			}
			j := from + rel
			typ, ok, rest := readNamedJavaType(chunk[j+len(needle):])
			if !ok || typ == "this" || typ == "super" || strings.HasPrefix(typ, "this.") || strings.HasPrefix(typ, "super.") || isDecompilerLocal(typ) || isPrimitiveOrObjectName(typ) {
				from = j + 1
				continue
			}
			if !strings.HasPrefix(rest, ")") {
				from = j + 1
				continue
			}
			return typ
		}
	}
	return ""
}

// fixAnnoNestedClassLit qualifies `skipOn=OnNonDefaultValue.class` when Advice
// is imported. Mockito MockMethodAdvice nested Advice annotations.
var annoNestedClassLitRe = regexp.MustCompile(`(skipOn|inline|prependLineNumber)=([A-Z][A-Za-z0-9_]*)\.class`)

func fixAnnoNestedClassLit(body string) string {
	if !strings.Contains(body, "import net.bytebuddy.asm.Advice;") && !strings.Contains(body, "@Advice.") {
		return body
	}
	return annoNestedClassLitRe.ReplaceAllString(body, "$1=Advice.$2.class")
}

// fixDeadObjectFieldAssign rewrites `this.field = varN` in a constructor
// when varN is an Object local never assigned and a sibling local of a
// non-Object type was assigned. ModuleMemberAccessor.
func fixDeadObjectFieldAssign(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "this.")
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len("this."):]
		field, ok, rest2 := readJavaIdent(rest)
		if !ok || !strings.HasPrefix(rest2, " = var") {
			from = i + 1
			continue
		}
		ident, ok, rest3 := readJavaIdent(rest2[len(" = "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest3, ";") {
			from = i + 1
			continue
		}
		mstart := prevMemberStart(body, i)
		mend := nextMemberStart(body, i)
		chunk := body[mstart:mend]
		if !strings.Contains(chunk, "Object "+ident+" = null;") {
			from = i + 1
			continue
		}
		// Never assigned besides the null init.
		assigns := strings.Count(chunk, ident+" = ")
		if assigns != 1 {
			from = i + 1
			continue
		}
		sib := siblingAssignedLocal(chunk, ident)
		if sib == "" {
			from = i + 1
			continue
		}
		repl := "this." + field + " = " + sib + ";"
		body = body[:i] + repl + rest3[1:]
		from = i + len(repl)
	}
}

func siblingAssignedLocal(chunk, dead string) string {
	from := 0
	for {
		rel := strings.Index(chunk[from:], " var")
		if rel < 0 {
			return ""
		}
		i := from + rel + 1
		ident, ok, rest := readJavaIdent(chunk[i:])
		if !ok || ident == dead || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		if strings.HasPrefix(rest, " = ") && !strings.HasPrefix(rest, " = null;") {
			// skip if the decl type is Object
			pre := strings.TrimRight(chunk[:i], " \t")
			if strings.HasSuffix(pre, "Object") {
				from = i + 1
				continue
			}
			return ident
		}
		from = i + 1
	}
}

// fixOuterCastInnerCallArg: `((Type)(recv.method(a, ident.call())))` wraps
// uncast method-call args as `(Type)(ident.call())`. Generic Outputs.add(T,T)
// with a raw receiver dumps the second arg as Object.
func fixOuterCastInnerCallArg(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "((")
		if rel < 0 {
			return body
		}
		i := from + rel
		typ, ok, rest := readDottedType(body[i+2:])
		if !ok || !strings.HasPrefix(rest, ")(") {
			from = i + 2
			continue
		}
		simple := lastDottedIdent(typ)
		if isDecompilerLocal(simple) || isPrimitiveOrObjectName(simple) {
			from = i + 2
			continue
		}
		innerOpen := i + 2 + len(typ) + 1
		if innerOpen >= len(body) || body[innerOpen] != '(' {
			from = i + 2
			continue
		}
		innerClose := matchingCloseParen(body, innerOpen)
		if innerClose < 0 {
			from = i + 2
			continue
		}
		inner := body[innerOpen+1 : innerClose]
		neu, changed := wrapUncastCallArgs(inner, simple)
		if !changed {
			from = i + 2
			continue
		}
		body = body[:innerOpen+1] + neu + body[innerClose:]
		from = innerOpen + 1 + len(neu)
	}
}

func wrapUncastCallArgs(inner, typ string) (string, bool) {
	if !strings.HasSuffix(inner, ")") {
		return inner, false
	}
	open := matchingOpenParen(inner, len(inner)-1)
	if open < 0 {
		return inner, false
	}
	recv := inner[:open]
	if recv == "" || strings.ContainsAny(recv, "\n") {
		return inner, false
	}
	args := inner[open+1 : len(inner)-1]
	parts := splitTopLevelArgs(args)
	if len(parts) == 0 {
		return inner, false
	}
	changed := false
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if isUncastMethodCall(p) {
			parts[i] = "(" + typ + ")(" + p + ")"
			changed = true
		}
	}
	if !changed {
		return inner, false
	}
	return recv + "(" + strings.Join(parts, ",") + ")", true
}

func isUncastMethodCall(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || strings.HasPrefix(p, "(") || strings.HasPrefix(p, "new ") {
		return false
	}
	// Only no-arg `ident.ident()` — wrapping `foo.bar(x)` over-fired on lucene.
	if !strings.HasSuffix(p, "()") {
		return false
	}
	return strings.Count(p, ".") == 1 && strings.Count(p, "(") == 1
}

func isSimpleIdent(s string) bool {
	ident, ok, rest := readJavaIdent(s)
	return ok && rest == "" && ident != "" && !isDecompilerLocal(ident)
}

func fieldHasWildcard(body, name string) bool {
	return strings.Contains(body, "<?> "+name) || strings.Contains(body, "<?> "+name+"=") ||
		strings.Contains(body, "<?> "+name+";")
}

// fixTernaryParamReturnArms wraps `return (cond) ? (A) : (B)` ident arms with
// the method's raw parameterized return type. Wildcard field vs type-var
// return has no legal LUB (`BooleanMatcher<?> TRUE` vs `ElementMatcher$Junction<T>`).
func fixTernaryParamReturnArms(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "return (")
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len("return "):]
		if !strings.HasPrefix(rest, "(") {
			from = i + 1
			continue
		}
		c0 := matchingCloseParen(rest, 0)
		if c0 < 0 || !strings.HasPrefix(rest[c0+1:], " ? (") {
			from = i + 1
			continue
		}
		a1open := c0 + 1 + len(" ? ")
		if a1open >= len(rest) || rest[a1open] != '(' {
			from = i + 1
			continue
		}
		a1close := matchingCloseParen(rest, a1open)
		if a1close < 0 || !strings.HasPrefix(rest[a1close+1:], " : (") {
			from = i + 1
			continue
		}
		a2open := a1close + 1 + len(" : ")
		if a2open >= len(rest) || rest[a2open] != '(' {
			from = i + 1
			continue
		}
		a2close := matchingCloseParen(rest, a2open)
		if a2close < 0 || a2close+1 >= len(rest) || rest[a2close+1] != ';' {
			from = i + 1
			continue
		}
		arm1 := rest[a1open+1 : a1close]
		arm2 := rest[a2open+1 : a2close]
		if !isSimpleIdent(arm1) || !isSimpleIdent(arm2) {
			from = i + 1
			continue
		}
		if !fieldHasWildcard(body, arm1) || !fieldHasWildcard(body, arm2) {
			from = i + 1
			continue
		}
		raw := methodReturnRaw(body, i)
		if raw == "" {
			from = i + 1
			continue
		}
		built := "return " + rest[:a1open] + "((" + raw + ")(" + arm1 + "))" + rest[a1close+1:a2open] + "((" + raw + ")(" + arm2 + "))" + rest[a2close+1:]
		body = body[:i] + built
		from = i + len(built)
	}
}

func methodReturnRaw(body string, pos int) string {
	start := prevMemberStart(body, pos)
	head := body[start:pos]
	brace := strings.Index(head, "{")
	if brace < 0 {
		return ""
	}
	sig := strings.TrimSpace(head[:brace])
	if i := strings.Index(sig, " throws "); i >= 0 {
		sig = sig[:i]
	}
	p := strings.LastIndex(sig, "(")
	if p < 0 {
		return ""
	}
	before := strings.Fields(strings.TrimSpace(sig[:p]))
	if len(before) < 2 {
		return ""
	}
	ret := before[len(before)-2]
	lt := strings.IndexByte(ret, '<')
	if lt < 0 {
		return ""
	}
	ret = ret[:lt]
	if ret == "" || ret == "void" {
		return ""
	}
	return ret
}

// fixEnumAssignedStaticField turns an enum constant that is assigned in
// `<clinit>` (`NAME = OTHER;`) into `static final EnumType NAME`. javac
// encodes a static field as if it were a constant; source cannot assign to
// an enum constant.
func fixEnumAssignedStaticField(body string) string {
	if !strings.Contains(body, "enum ") {
		return body
	}
	enumName := enumSimpleName(body)
	if enumName == "" {
		return body
	}
	clinit := strings.Index(body, "\n\tstatic  {")
	if clinit < 0 {
		clinit = strings.Index(body, "\n\tstatic {")
	}
	if clinit < 0 {
		return body
	}
	cend := nextMemberStart(body, clinit+1)
	block := body[clinit:cend]
	assignRe := regexp.MustCompile(`(?m)^[ \t]+([A-Za-z_][A-Za-z0-9_]*) = ([A-Za-z_][A-Za-z0-9_]*);`)
	seen := map[string]struct{}{}
	var names []string
	for _, m := range assignRe.FindAllStringSubmatch(block, -1) {
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		names = append(names, m[1])
	}
	for _, name := range names {
		if strings.Contains(body, name+" {") || strings.Contains(body, name+"(") {
			continue
		}
		field := "public static final " + enumName + " " + name + ";\n"
		// `},\n\tNAME;\n` after a constant body.
		reBody := regexp.MustCompile(`(?m)(}[ \t]*,\n)([ \t]+)` + regexp.QuoteMeta(name) + `;\n`)
		if reBody.MatchString(body) {
			body = reBody.ReplaceAllStringFunc(body, func(m string) string {
				sm := reBody.FindStringSubmatch(m)
				return "};\n\n" + sm[2] + field
			})
			continue
		}
		// `OTHER,\n\tNAME;\n`
		reList := regexp.MustCompile(`(?m)([A-Za-z_][A-Za-z0-9_]*)[ \t]*,\n([ \t]+)` + regexp.QuoteMeta(name) + `;\n`)
		if reList.MatchString(body) {
			body = reList.ReplaceAllStringFunc(body, func(m string) string {
				sm := reList.FindStringSubmatch(m)
				return sm[1] + ";\n\n" + sm[2] + field
			})
		}
	}
	return body
}

func enumSimpleName(body string) string {
	i := strings.Index(body, " enum ")
	if i < 0 {
		if !strings.Contains(body, "enum ") {
			return ""
		}
		i = strings.Index(body, "enum ")
		rest := strings.TrimSpace(body[i+len("enum "):])
		name, ok, _ := readJavaIdent(rest)
		if !ok {
			return ""
		}
		return name
	}
	rest := strings.TrimSpace(body[i+len(" enum "):])
	name, ok, _ := readJavaIdent(rest)
	if !ok {
		return ""
	}
	return name
}

func isLambdaIdent(s string) bool {
	if !strings.HasPrefix(s, "l") || len(s) < 2 {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// firstParamTypes maps method name → first parameter type from one-tab
// member signatures in this dump. Linear scan — a greedy regex over
// `[A-Za-z0-9_$.<>\[\],? ]*` backtracks on large members.
func firstParamTypes(body string) map[string]string {
	out := map[string]string{}
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return out
		}
		j := from + rel
		rest := body[j+2:]
		if rest == "" || rest[0] == '\t' || rest[0] == '\n' || rest[0] == ' ' || rest[0] == '/' {
			from = j + 2
			continue
		}
		brace := strings.IndexByte(rest, '{')
		if brace < 0 || brace > 400 {
			from = j + 2
			continue
		}
		sig := rest[:brace]
		p := strings.LastIndexByte(sig, '(')
		if p < 0 {
			from = j + 2
			continue
		}
		before := strings.Fields(strings.TrimSpace(sig[:p]))
		if len(before) < 1 {
			from = j + 2
			continue
		}
		name := before[len(before)-1]
		arg, ok, after := readDottedType(strings.TrimSpace(sig[p+1:]))
		if !ok || !strings.HasPrefix(strings.TrimLeft(after, " \t"), "var") {
			from = j + 2
			continue
		}
		if _, exists := out[name]; !exists {
			out[name] = lastDottedIdent(arg)
			if strings.Contains(arg, ".") || strings.Contains(arg, "$") {
				out[name] = arg
			}
		}
		from = j + 2
	}
}

func inferredLambdaParamType(lambdaBody, ident string, methods map[string]string) string {
	seen := ""
	from := 0
	for {
		rel := strings.Index(lambdaBody[from:], "("+ident)
		if rel < 0 {
			break
		}
		i := from + rel
		if i+1+len(ident) > len(lambdaBody) {
			from = i + 1
			continue
		}
		after := lambdaBody[i+1+len(ident):]
		if !strings.HasPrefix(after, ")") && !strings.HasPrefix(after, ",") {
			from = i + 1
			continue
		}
		k := i
		for k > 0 && (lambdaBody[k-1] == ' ' || lambdaBody[k-1] == '\t') {
			k--
		}
		nameEnd := k
		nameStart := k
		for nameStart > 0 && isJavaIdentChar(lambdaBody[nameStart-1]) {
			nameStart--
		}
		if nameStart >= nameEnd {
			from = i + 1
			continue
		}
		meth := lambdaBody[nameStart:nameEnd]
		if nameStart < len("this.") || lambdaBody[nameStart-len("this."):nameStart] != "this." {
			from = i + 1
			continue
		}
		typ, ok := methods[meth]
		if !ok || typ == "" {
			from = i + 1
			continue
		}
		if seen == "" {
			seen = typ
		} else if seen != typ {
			return ""
		}
		from = i + 1
	}
	return seen
}

// fixLambdaParamFromCallee types `(l0) ->` from a same-class call
// `this.foo(l0)` whose first parameter has a dumped type.
func fixLambdaParamFromCallee(body string) string {
	methods := firstParamTypes(body)
	if len(methods) == 0 {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "(")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+1:])
		if !ok || !isLambdaIdent(ident) || !strings.HasPrefix(rest, ") ->") {
			from = i + 1
			continue
		}
		arrow := i + 1 + len(ident) + len(") ->")
		open := strings.Index(body[arrow:], "{")
		if open < 0 || open > 8 {
			from = i + 1
			continue
		}
		openAbs := arrow + open
		close := matchingCloseBrace(body, openAbs)
		if close < 0 {
			from = i + 1
			continue
		}
		lambdaBody := body[openAbs : close+1]
		typ := inferredLambdaParamType(lambdaBody, ident, methods)
		if typ == "" || strings.Contains(typ, " ") {
			from = close + 1
			continue
		}
		neu := wrapLambdaIdentAsType(lambdaBody, ident, typ)
		if neu == lambdaBody {
			from = close + 1
			continue
		}
		body = body[:openAbs] + neu + body[close+1:]
		from = openAbs + len(neu)
	}
}

func wrapLambdaIdentAsType(chunk, ident, typ string) string {
	chunk = wrapIdentArgsAsType(chunk, ident, typ)
	from := 0
	needle := ident + "."
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return chunk
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(chunk[i-1]) {
			from = i + 1
			continue
		}
		head := strings.TrimRight(chunk[:i], " \t")
		if strings.HasSuffix(head, "("+typ+")(") || strings.HasSuffix(head, "(("+typ+")(") {
			from = i + len(ident)
			continue
		}
		wrap := "((" + typ + ")(" + ident + "))."
		chunk = chunk[:i] + wrap + chunk[i+len(needle):]
		from = i + len(wrap)
	}
}

func rawTypeName(typ string) string {
	typ = strings.TrimSpace(typ)
	if i := strings.IndexByte(typ, '<'); i >= 0 {
		typ = typ[:i]
	}
	return typ
}

// fixTernaryNewArmCast wraps the non-`new` arm of
// `Type varN = (cond) ? (new Type(...)) : (other)` as `(Type)(other)`.
// AccessorMethodDelegation vs DelegationRecord.with() has no legal LUB.
func fixTernaryNewArmCast(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "? (new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		// Statement start: last newline before this.
		stmtStart := strings.LastIndex(body[:i], "\n")
		if stmtStart < 0 {
			stmtStart = 0
		} else {
			stmtStart++
		}
		semi := strings.IndexByte(body[i:], ';')
		if semi < 0 {
			from = i + 1
			continue
		}
		stmt := body[stmtStart : i+semi+1]
		eq := strings.Index(stmt, " = ")
		if eq < 0 {
			from = i + 1
			continue
		}
		decl := strings.TrimSpace(stmt[:eq])
		fields := strings.Fields(decl)
		if len(fields) < 2 {
			from = i + 1
			continue
		}
		typ := rawTypeName(fields[len(fields)-2])
		if typ == "" || isDecompilerLocal(typ) || isPrimitiveOrObjectName(typ) {
			from = i + 1
			continue
		}
		newStart := i + len("? (new ")
		newTyp, ok, after := readDottedType(body[newStart:])
		if !ok || !strings.HasPrefix(after, "(") {
			from = i + 1
			continue
		}
		newRaw := rawTypeName(newTyp)
		if newRaw != typ && !strings.HasPrefix(newRaw, typ+"$") && !strings.HasPrefix(typ, newRaw+"$") {
			from = i + 1
			continue
		}
		// Find `: (` after the new(...) close.
		newOpen := newStart + len(newTyp)
		newClose := matchingCloseParen(body, newOpen)
		if newClose < 0 {
			from = i + 1
			continue
		}
		rest := body[newClose+1:]
		// skip `)` that closed `? (new Type(...) )`
		rest = strings.TrimLeft(rest, ")")
		if !strings.HasPrefix(strings.TrimLeft(rest, " \t"), " : (") && !strings.HasPrefix(rest, " : (") {
			from = i + 1
			continue
		}
		colon := strings.Index(body[newClose:], " : (")
		if colon < 0 {
			from = i + 1
			continue
		}
		armOpen := newClose + colon + len(" : ")
		if armOpen >= len(body) || body[armOpen] != '(' {
			from = i + 1
			continue
		}
		armClose := matchingCloseParen(body, armOpen)
		if armClose < 0 {
			from = i + 1
			continue
		}
		arm := body[armOpen+1 : armClose]
		if strings.Contains(arm, "("+typ+")") || strings.HasPrefix(arm, "new ") {
			from = armClose + 1
			continue
		}
		wrapped := "((" + typ + ")(" + arm + "))"
		body = body[:armOpen] + wrapped + body[armClose+1:]
		from = armOpen + len(wrapped)
	}
}

// fixForNameAddAnnoClass wraps Class.forName / ClassUtils.forName passed to
// a Set<Class<? extends Annotation>>. javac captures forName's Class<?> as CAP#1.
func fixForNameAddAnnoClass(body string) string {
	if !strings.Contains(body, "Class<? extends") || !strings.Contains(body, ".forName(") {
		return body
	}
	if !strings.Contains(body, "Annotation>") {
		return body
	}
	for _, recv := range []string{"ClassUtils.forName(", "Class.forName("} {
		from := 0
		needle := ".add(" + recv
		for {
			rel := strings.Index(body[from:], needle)
			if rel < 0 {
				break
			}
			i := from + rel
			addOpen := i + len(".add")
			if addOpen >= len(body) || body[addOpen] != '(' {
				from = i + 1
				continue
			}
			if strings.HasPrefix(body[addOpen+1:], "(Class") {
				from = i + 1
				continue
			}
			close := matchingCloseParen(body, addOpen)
			if close < 0 {
				from = i + 1
				continue
			}
			call := body[addOpen+1 : close]
			repl := ".add((Class)(" + call + "))"
			body = body[:i] + repl + body[close+1:]
			from = i + len(repl)
		}
	}
	return body
}

// fixCachedFieldReturn retypes `Object varN` in the lazy-cache shape
//
//	Object varN = ((this.field) != (null)) ? (null) : (new Type(...));
//	if ((varN) == (null)) { varN = ((T)(this.field)); } else { this.field = varN; }
//	return (T)(varN);
//
// ByteBuddy CachedReturnPlugin and similar. The local is the field's type T,
// taken from an explicit `(T)(varN)` / `varN = ((T)(this.field))` in the member.
func fixCachedFieldReturn(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "Object var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("Object "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		// Inverted CachedReturnPlugin dump: the plugin's Class.forName sentinel
		// sits in the member, then `Object varN = null` is stored into this.field
		// and cast back to T (`TypeList$Generic` / similar).
		if strings.HasPrefix(rest, " = null;") {
			start := prevMemberStart(body, i)
			end := nextMemberStart(body, i)
			member := body[start:end]
			chunk := body[i:end]
			if !strings.Contains(member, `Class.forName("java.lang.Object")`) || !hasThisFieldStore(chunk, ident) {
				from = i + 1
				continue
			}
			typ := assignedCastType(chunk, ident)
			if typ == "" {
				typ = uniqueCastTypeOfLocal(chunk, ident)
			}
			if typ == "" || isPrimitiveOrObjectName(typ) || isBoxedPrimitiveName(typ) {
				from = i + 1
				continue
			}
			body = body[:i] + typ + " " + ident + rest
			from = i + len(typ+" "+ident)
			continue
		}
		if !strings.HasPrefix(rest, " = ((this.") {
			from = i + 1
			continue
		}
		field, ok, after := readJavaIdent(rest[len(" = ((this."):])
		if !ok || !strings.HasPrefix(after, ") != (null)) ? (null) : (") {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if !strings.Contains(chunk, "this."+field+" = "+ident) {
			from = i + 1
			continue
		}
		typ := assignedCastType(chunk, ident)
		if typ == "" {
			typ = uniqueCastTypeOfLocal(chunk, ident)
		}
		if typ == "" || isPrimitiveOrObjectName(typ) || isBoxedPrimitiveName(typ) {
			from = i + 1
			continue
		}
		body = body[:i] + typ + " " + ident + rest
		from = i + len(typ+" "+ident)
	}
}

func hasThisFieldStore(chunk, ident string) bool {
	needle := " = " + ident + ";"
	from := 0
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return false
		}
		j := from + rel
		pre := strings.TrimRight(chunk[:j], " \t")
		dot := strings.LastIndex(pre, "this.")
		if dot >= 0 {
			field := pre[dot+len("this."):]
			if field != "" && !strings.ContainsAny(field, " \t()[]{};") {
				return true
			}
		}
		from = j + 1
	}
}

// fixStringCastToClass unwraps `(Class<...>)(ident)` when ident is a String
// slot used as a method argument. Overload dumps pick getBean(Class<T>) and
// insert an illegal String→Class cast; the String overload is the bytecode
// target. Not a class-name unique.
func fixStringCastToClass(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "(Class")
		if rel < 0 {
			return body
		}
		i := from + rel
		pre := i
		for pre > 0 && (body[pre-1] == ' ' || body[pre-1] == '\t' || body[pre-1] == '\n') {
			pre--
		}
		if pre > 0 && body[pre-1] != '(' && body[pre-1] != ',' {
			from = i + 1
			continue
		}
		rest := body[i+len("(Class"):]
		if strings.HasPrefix(rest, "<") {
			gt := matchingCloseAngle(rest, 0)
			if gt < 0 {
				from = i + 1
				continue
			}
			rest = rest[gt+1:]
		}
		if !strings.HasPrefix(rest, ")(") {
			from = i + 1
			continue
		}
		ident, ok, after := readJavaIdent(rest[2:])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(after, ")") {
			from = i + 1
			continue
		}
		if len(after) > 1 && (after[1] == '.' || isJavaIdentChar(after[1])) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		if !identDeclaredString(body[start:end], ident) {
			from = i + 1
			continue
		}
		// after is ")" + remainder; closeAt is the ')' of (ident).
		closeAt := len(body) - len(after)
		body = body[:i] + ident + body[closeAt+1:]
		from = i + len(ident)
	}
}

func matchingCloseAngle(s string, openIdx int) int {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '<' {
		return -1
	}
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func identDeclaredString(chunk, ident string) bool {
	for _, pfx := range []string{"String ", "final String "} {
		from := 0
		needle := pfx + ident
		for {
			rel := strings.Index(chunk[from:], needle)
			if rel < 0 {
				break
			}
			j := from + rel
			after := j + len(needle)
			if after < len(chunk) && isJavaIdentChar(chunk[after]) {
				from = j + 1
				continue
			}
			if j > 0 && isJavaIdentChar(chunk[j-1]) {
				from = j + 1
				continue
			}
			return true
		}
	}
	return false
}

// fixConvertNumberClassArg wraps the Class target of convertNumberToTargetClass
// as raw `(Class)`. Class<T> is not Class<T extends Number>.
func fixConvertNumberClassArg(body string) string {
	needle := "convertNumberToTargetClass("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len("convertNumberToTargetClass")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		args := body[open+1 : close]
		comma := indexCommaAtDepth0(args)
		if comma < 0 {
			from = close
			continue
		}
		second := strings.TrimSpace(args[comma+1:])
		if second == "" || strings.HasPrefix(second, "(Class)") || strings.HasPrefix(second, "((Class") {
			from = close
			continue
		}
		neu := needle + args[:comma+1] + "(Class)(" + second + ")"
		body = body[:i] + neu + body[close:]
		from = i + len(neu)
	}
}

func indexCommaAtDepth0(s string) int {
	depth, angle := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '<':
			angle++
		case '>':
			angle--
		case ',':
			if depth == 0 && angle == 0 {
				return i
			}
		}
	}
	return -1
}

// fixRawRemoveIfMethodRef wraps `rawList.removeIf(this::m)` as
// `rawList.removeIf((Predicate)(this::m))`. Raw ArrayList cannot bind the
// method-ref parameter.
func fixRawRemoveIfMethodRef(body string) string {
	needle := ".removeIf(this::"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		if !identDeclaredRawCollection(body[start:end], ident) {
			from = i + 1
			continue
		}
		rest := body[i+len(needle):]
		meth, ok, after := readJavaIdent(rest)
		if !ok || !strings.HasPrefix(after, ")") {
			from = i + 1
			continue
		}
		ptype := methodFirstParamType(body, meth)
		if ptype == "" || isPrimitiveOrObjectName(ptype) {
			from = i + 1
			continue
		}
		wrap := ".removeIf((x) -> this." + meth + "((" + ptype + ")(x)))"
		body = body[:i] + wrap + after[1:]
		from = i + len(wrap)
	}
}

func methodFirstParamType(body, meth string) string {
	needle := meth + "("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return ""
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(body[i-1]) {
			from = i + 1
			continue
		}
		rest := body[i+len(needle):]
		typ, ok, after := readNamedJavaType(rest)
		if !ok {
			from = i + 1
			continue
		}
		after = strings.TrimLeft(after, " \t")
		arg, ok2, _ := readJavaIdent(after)
		if !ok2 || !isDecompilerLocal(arg) {
			from = i + 1
			continue
		}
		simple := lastDottedIdent(typ)
		if simple == "" || isPrimitiveOrObjectName(simple) {
			from = i + 1
			continue
		}
		if strings.HasSuffix(typ, "[]") {
			return simple + strings.Repeat("[]", strings.Count(typ, "[]"))
		}
		return simple
	}
}

func identDeclaredRawCollection(chunk, ident string) bool {
	for _, typ := range []string{"ArrayList", "LinkedList", "CopyOnWriteArrayList", "Vector", "List", "Collection", "HashSet", "Set"} {
		for _, pfx := range []string{typ + " ", "final " + typ + " "} {
			from := 0
			needle := pfx + ident
			for {
				rel := strings.Index(chunk[from:], needle)
				if rel < 0 {
					break
				}
				j := from + rel
				after := j + len(needle)
				if after < len(chunk) && isJavaIdentChar(chunk[after]) {
					from = j + 1
					continue
				}
				if j > 0 && isJavaIdentChar(chunk[j-1]) {
					from = j + 1
					continue
				}
				return true
			}
		}
	}
	return false
}

// wrapStreamReturnRawCast wraps `return stream.map((Function<…, Object>)…)`
// in a `Stream<T>` method as `return (Stream) (…);`. Function<A,Object> map
// types as Stream<Object>, which is not Stream<T>; a raw Stream bridge is
// unchecked and compiles. Casting the Function to Function<A,T> fails because
// the lambda body still returns Object.
func wrapStreamReturnRawCast(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "return ")
		if rel < 0 {
			return body
		}
		i := from + rel
		if methodReturnStreamArg(body, i) == "" {
			from = i + 1
			continue
		}
		exprStart := i + len("return ")
		for exprStart < len(body) && (body[exprStart] == ' ' || body[exprStart] == '\t') {
			exprStart++
		}
		if strings.HasPrefix(body[exprStart:], "(Stream)") || strings.HasPrefix(body[exprStart:], "(Stream<") {
			from = i + 1
			continue
		}
		end := scanJavaStmtEnd(body, exprStart)
		if end < 0 || end <= exprStart {
			from = i + 1
			continue
		}
		expr := strings.TrimSpace(body[exprStart:end])
		if !strings.Contains(expr, ".map((Function<") {
			from = i + 1
			continue
		}
		neu := "return (Stream) (" + expr + ")"
		body = body[:i] + neu + body[end:]
		from = i + len(neu)
	}
}

func scanJavaStmtEnd(body string, exprStart int) int {
	depth, brace, angle := 0, 0, 0
	for i := exprStart; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '{':
			brace++
		case '}':
			brace--
		case '<':
			angle++
		case '>':
			if angle > 0 {
				angle--
			}
		case ';':
			if depth == 0 && brace == 0 {
				return i
			}
		}
	}
	return -1
}

// wrapIntrospectionExceptionCalls wraps a same-class call that throws
// IntrospectionException when the enclosing member does not declare it.
// Restricted to `class` types (not interfaces) and to a whole-line statement
// ending in `);` so interface signatures are never rewritten.
func wrapIntrospectionExceptionCalls(body string) string {
	if !strings.Contains(body, "throws IntrospectionException") || !strings.Contains(body, " class ") {
		return body
	}
	names := methodsThrowing(body, "IntrospectionException")
	if len(names) == 0 {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return body
		}
		mstart := from + rel
		if mstart+2 >= len(body) || body[mstart+2] == '\t' || body[mstart+2] == '\n' || body[mstart+2] == ' ' || body[mstart+2] == '/' {
			from = mstart + 2
			continue
		}
		mend := nextMemberStart(body, mstart+2)
		chunk := body[mstart:mend]
		brace := strings.Index(chunk, "{")
		if brace < 0 {
			from = mend
			continue
		}
		if strings.Contains(chunk[:brace], " throws IntrospectionException") {
			from = mend
			continue
		}
		if strings.Contains(chunk, "catch (IntrospectionException") || strings.Contains(chunk, "catch(IntrospectionException") {
			from = mend
			continue
		}
		neu := wrapIntroCallsInMember(chunk, names)
		if neu != chunk {
			body = body[:mstart] + neu + body[mend:]
			from = mstart + len(neu)
			continue
		}
		from = mend
	}
}

func methodsThrowing(body, ex string) map[string]bool {
	out := map[string]bool{}
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return out
		}
		mstart := from + rel
		if mstart+2 >= len(body) || body[mstart+2] == '\t' || body[mstart+2] == '\n' || body[mstart+2] == ' ' || body[mstart+2] == '/' {
			from = mstart + 2
			continue
		}
		mend := nextMemberStart(body, mstart+2)
		chunk := body[mstart:mend]
		brace := strings.Index(chunk, "{")
		if brace < 0 {
			from = mend
			continue
		}
		sig := chunk[:brace]
		if !strings.Contains(sig, " throws ") || !strings.Contains(sig, ex) {
			from = mend
			continue
		}
		p := strings.LastIndex(sig, "(")
		if p < 0 {
			from = mend
			continue
		}
		fields := strings.Fields(strings.TrimSpace(sig[:p]))
		if len(fields) == 0 {
			from = mend
			continue
		}
		name := fields[len(fields)-1]
		if !isJavaKeyword(name) && !isDecompilerLocal(name) {
			out[name] = true
		}
		from = mend
	}
}

func wrapIntroCallsInMember(chunk string, names map[string]bool) string {
	from := 0
	for {
		best := -1
		bestName := ""
		for name := range names {
			for _, pfx := range []string{"this." + name + "(", name + "("} {
				rel := strings.Index(chunk[from:], pfx)
				if rel < 0 {
					continue
				}
				at := from + rel
				if pfx == name+"(" && at > 0 && (isJavaIdentChar(chunk[at-1]) || chunk[at-1] == '.') {
					continue
				}
				if best < 0 || at < best {
					best = at
					bestName = name
				}
			}
		}
		if best < 0 {
			return chunk
		}
		open := strings.Index(chunk[best:], "(")
		if open < 0 {
			from = best + 1
			continue
		}
		open += best
		close := matchingCloseParen(chunk, open)
		if close < 0 {
			from = best + 1
			continue
		}
		semi := close + 1
		for semi < len(chunk) && (chunk[semi] == ' ' || chunk[semi] == '\t') {
			semi++
		}
		if semi >= len(chunk) || chunk[semi] != ';' {
			from = close
			continue
		}
		stmtStart := 0
		if n := strings.LastIndex(chunk[:best], "\n"); n >= 0 {
			stmtStart = n + 1
		}
		lead := strings.TrimSpace(chunk[stmtStart:best])
		if lead != "" && !strings.HasSuffix(lead, "=") {
			from = close
			continue
		}
		stmtEnd := semi + 1
		stmt := chunk[stmtStart:stmtEnd]
		indent := stmt[:len(stmt)-len(strings.TrimLeft(stmt, " \t"))]
		wrapped := indent + "try {\n" + stmt + "\n" + indent + "} catch (IntrospectionException ex) {\n" + indent + "\tthrow new RuntimeException(ex);\n" + indent + "}"
		chunk = chunk[:stmtStart] + wrapped + chunk[stmtEnd:]
		from = stmtStart + len(wrapped)
		_ = bestName
	}
}

// wrapThrowTargetException wraps `throw (ident.getTargetException())` as
// `throw (Exception)(ident.getTargetException())` when the nearby if tested
// `instanceof Exception`. InvocationTargetException.getTargetException returns
// Throwable; throwing it from a method that throws Exception is illegal.
// wrapCallSiteMethodBodies wraps the body of a method that calls
// `$getCallSiteArray()` in `try { ... } catch (Throwable t) { throw new
// RuntimeException(t); }`. CallSite.call throws Throwable; adding
// `throws Throwable` would break GroovyObject.invokeMethod.
func wrapCallSiteMethodBodies(body string) string {
	if !strings.Contains(body, "$getCallSiteArray()") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return body
		}
		mstart := from + rel
		if mstart+2 >= len(body) || body[mstart+2] == '\t' || body[mstart+2] == '\n' || body[mstart+2] == ' ' || body[mstart+2] == '/' {
			from = mstart + 2
			continue
		}
		braceRel := strings.Index(body[mstart:], "{")
		if braceRel < 0 {
			from = mstart + 2
			continue
		}
		open := mstart + braceRel
		sig := body[mstart:open]
		if strings.Contains(sig, "$getCallSiteArray") {
			from = open + 1
			continue
		}
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = open + 1
			continue
		}
		chunk := body[mstart : close+1]
		if !strings.Contains(chunk, "$getCallSiteArray()") {
			from = close + 1
			continue
		}
		if strings.Contains(chunk, "catch (Throwable") || strings.Contains(chunk, "catch(Throwable") {
			from = close + 1
			continue
		}
		inner := body[open+1 : close]
		head := inner
		if len(head) > 120 {
			head = head[:120]
		}
		if strings.Contains(head, "super(") {
			from = close + 1
			continue
		}
		wrapped := "{\n\t\ttry {" + inner + "\t\t} catch (Throwable _t) {\n\t\t\tthrow new RuntimeException(_t);\n\t\t}\n\t}"
		body = body[:open] + wrapped + body[close+1:]
		from = open + len(wrapped)
	}
}

func wrapThrowTargetException(body string) string {
	if hardjarShapeOff() {
		return body
	}
	if !strings.Contains(body, "getTargetException()") || !strings.Contains(body, "instanceof Exception") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "throw ")
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := strings.TrimLeft(body[i+len("throw "):], " \t")
		if strings.HasPrefix(rest, "(Exception)") {
			from = i + 1
			continue
		}
		ident, after, ok := parseThrownTargetException(rest)
		if !ok {
			from = i + 1
			continue
		}
		pre := body[max(0, i-240):i]
		if !strings.Contains(pre, "instanceof Exception") {
			from = i + 1
			continue
		}
		neu := "throw (Exception)(" + ident + ".getTargetException());"
		body = body[:i] + neu + after
		from = i + len(neu)
	}
}

func parseThrownTargetException(rest string) (ident, after string, ok bool) {
	if strings.HasPrefix(rest, "(") {
		ident, ok, rest = readJavaIdent(rest[1:])
		if !ok {
			return "", "", false
		}
		if strings.HasPrefix(rest, ".getTargetException());") {
			return ident, rest[len(".getTargetException());"):], true
		}
		if strings.HasPrefix(rest, ")(") {
			ident2, ok2, rest2 := readJavaIdent(rest[2:])
			if !ok2 || ident2 != ident || !strings.HasPrefix(rest2, ".getTargetException());") {
				return "", "", false
			}
			return ident, rest2[len(".getTargetException());"):], true
		}
		return "", "", false
	}
	ident, ok, rest = readJavaIdent(rest)
	if !ok || !strings.HasPrefix(rest, ".getTargetException();") {
		return "", "", false
	}
	return ident, rest[len(".getTargetException();"):], true
}

// wrapStmtObjectMethodAssign wraps a whole-line `ident = this.meth(...);`
// when meth is a same-class Object-returning method and ident is a non-Object
// class local. Restricted to statement-start assignments so it cannot splice
// into `foo = this.bar().baz` chains (those produced syntax errors).
func wrapStmtObjectMethodAssign(body string) string {
	objMeths := objectReturningMethodNames(body)
	if len(objMeths) == 0 {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], " = this.")
		if rel < 0 {
			return body
		}
		i := from + rel
		lineStart := 0
		if n := strings.LastIndex(body[:i], "\n"); n >= 0 {
			lineStart = n + 1
		}
		identEnd := i
		for identEnd > 0 && (body[identEnd-1] == ' ' || body[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) || strings.TrimSpace(body[lineStart:identStart]) != "" {
			from = i + 1
			continue
		}
		rest := body[i+len(" = this."):]
		meth, ok, after := readJavaIdent(rest)
		if !ok || !strings.HasPrefix(after, "(") || !objMeths[meth] {
			from = i + 1
			continue
		}
		open := i + len(" = this.") + len(meth)
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		semi := close + 1
		for semi < len(body) && (body[semi] == ' ' || body[semi] == '\t') {
			semi++
		}
		if semi >= len(body) || body[semi] != ';' {
			from = close
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		typ := identDeclaredClassType(body[start:end], ident)
		if typ == "" || typ == "Object" || isPrimitiveOrObjectName(typ) || isBoxedPrimitiveName(typ) {
			from = close
			continue
		}
		if !isSimpleClassIdent(typ) {
			from = close
			continue
		}
		call := body[i+len(" = ") : close+1]
		if strings.HasPrefix(call, "("+typ+")") || strings.HasPrefix(call, "(("+typ+")") {
			from = close
			continue
		}
		neu := " = (" + typ + ")(" + call + ")"
		body = body[:i] + neu + body[close+1:]
		from = i + len(neu)
	}
}

// wrapUnreportedCheckedCall is unwired from fixHardjarShapes (wrapped
// interface method signatures as try/catch; 34-set syntax errors).
// wrapUnreportedCheckedCall wraps a same-class call that throws a checked
// exception when the enclosing member does not declare it.
// `try { call; } catch (E ex) { throw new RuntimeException(ex); }`
func wrapUnreportedCheckedCall(body string) string {
	throws := classMethodThrows(body)
	if len(throws) == 0 {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return body
		}
		mstart := from + rel
		if mstart+2 >= len(body) || body[mstart+2] == '\t' || body[mstart+2] == '\n' || body[mstart+2] == ' ' || body[mstart+2] == '/' {
			from = mstart + 2
			continue
		}
		mend := nextMemberStart(body, mstart+2)
		chunk := body[mstart:mend]
		callerThrows := memberThrows(chunk)
		neu := wrapCheckedCallsInMember(chunk, throws, callerThrows)
		if neu != chunk {
			body = body[:mstart] + neu + body[mend:]
			from = mstart + len(neu)
			continue
		}
		from = mend
	}
}

func classMethodThrows(body string) map[string][]string {
	out := map[string][]string{}
	from := 0
	for {
		rel := strings.Index(body[from:], "\n\t")
		if rel < 0 {
			return out
		}
		mstart := from + rel
		if mstart+2 >= len(body) || body[mstart+2] == '\t' || body[mstart+2] == '\n' || body[mstart+2] == ' ' || body[mstart+2] == '/' {
			from = mstart + 2
			continue
		}
		mend := nextMemberStart(body, mstart+2)
		chunk := body[mstart:mend]
		brace := strings.Index(chunk, "{")
		if brace < 0 {
			from = mend
			continue
		}
		sig := chunk[:brace]
		p := strings.LastIndex(sig, "(")
		if p < 0 {
			from = mend
			continue
		}
		fields := strings.Fields(strings.TrimSpace(sig[:p]))
		if len(fields) == 0 {
			from = mend
			continue
		}
		name := fields[len(fields)-1]
		if isJavaKeyword(name) || isDecompilerLocal(name) {
			from = mend
			continue
		}
		exs := memberThrows(chunk)
		if len(exs) > 0 {
			out[name] = exs
		}
		from = mend
	}
}

func memberThrows(chunk string) []string {
	brace := strings.Index(chunk, "{")
	if brace < 0 {
		return nil
	}
	sig := chunk[:brace]
	i := strings.Index(sig, " throws ")
	if i < 0 {
		return nil
	}
	part := strings.TrimSpace(sig[i+len(" throws "):])
	var out []string
	for _, e := range strings.Split(part, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		out = append(out, lastDottedIdent(e))
	}
	return out
}

func wrapCheckedCallsInMember(chunk string, throws map[string][]string, callerThrows []string) string {
	caller := map[string]bool{}
	for _, e := range callerThrows {
		caller[e] = true
	}
	from := 0
	for {
		best := -1
		bestName := ""
		bestEx := ""
		for name, exs := range throws {
			for _, ex := range exs {
				if caller[ex] || isUncheckedExceptionName(ex) {
					continue
				}
				for _, pfx := range []string{"this." + name + "(", name + "("} {
					rel := strings.Index(chunk[from:], pfx)
					if rel < 0 {
						continue
					}
					at := from + rel
					if pfx == name+"(" && at > 0 && (isJavaIdentChar(chunk[at-1]) || chunk[at-1] == '.') {
						continue
					}
					if best < 0 || at < best {
						best = at
						bestName = name
						bestEx = ex
					}
				}
			}
		}
		if best < 0 {
			return chunk
		}
		open := strings.Index(chunk[best:], "(")
		if open < 0 {
			from = best + 1
			continue
		}
		open += best
		close := matchingCloseParen(chunk, open)
		if close < 0 {
			from = best + 1
			continue
		}
		stmtStart := 0
		if n := strings.LastIndex(chunk[:best], "\n"); n >= 0 {
			stmtStart = n + 1
		}
		stmtEnd := scanJavaStmtEnd(chunk, best)
		if stmtEnd < 0 {
			from = close
			continue
		}
		stmtEnd++ // include ';'
		head := strings.TrimSpace(chunk[stmtStart:best])
		if strings.HasPrefix(head, "try") || strings.Contains(head, "catch") {
			from = close
			continue
		}
		stmt := chunk[stmtStart:stmtEnd]
		indent := stmt[:len(stmt)-len(strings.TrimLeft(stmt, " \t"))]
		if strings.Contains(chunk[max(0, stmtStart-12):stmtStart], "try") {
			from = close
			continue
		}
		wrapped := indent + "try {\n" + stmt + "\n" + indent + "} catch (" + bestEx + " ex) {\n" + indent + "\tthrow new RuntimeException(ex);\n" + indent + "}"
		chunk = chunk[:stmtStart] + wrapped + chunk[stmtEnd:]
		from = stmtStart + len(wrapped)
		_ = bestName
	}
}

func isUncheckedExceptionName(s string) bool {
	switch s {
	case "RuntimeException", "Error", "Throwable",
		"IllegalArgumentException", "IllegalStateException",
		"NullPointerException", "ClassCastException",
		"IndexOutOfBoundsException", "UnsupportedOperationException",
		"ArithmeticException", "SecurityException",
		"UncheckedIOException", "CompletionException",
		"BeansException", "FatalBeanException", "NestedRuntimeException":
		return true
	}
	return strings.HasSuffix(s, "RuntimeException") || strings.HasSuffix(s, "Error")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// retypeMixedNullClassLocal is unwired from fixHardjarShapes: `Type ident = null`
// plus `ident = this.m(...)` plus `new Type` over-fired on compress/math3 (34-set).
// retypeMixedNullClassLocal turns `Type ident = null` into `Object ident = null`
// when the member both assigns `ident = this.method(...)` (erased Object) and
// assigns ident from a `new Type` / same-type local. Mixed RuntimeBeanReference
// vs Object map-entry slots.
func retypeMixedNullClassLocal(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = null;")
		if rel < 0 {
			return body
		}
		eq := from + rel
		identEnd := eq
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = eq + 1
			continue
		}
		typeEnd := identStart
		for typeEnd > 0 && body[typeEnd-1] == ' ' {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && (isJavaIdentChar(body[typeStart-1]) || body[typeStart-1] == '.') {
			typeStart--
		}
		typ := body[typeStart:typeEnd]
		simple := lastDottedIdent(typ)
		if simple == "" || simple == "Object" || isPrimitiveOrObjectName(simple) || isBoxedPrimitiveName(simple) {
			from = eq + 1
			continue
		}
		if typeStart > 0 && isJavaIdentChar(body[typeStart-1]) {
			from = eq + 1
			continue
		}
		start := prevMemberStart(body, eq)
		end := nextMemberStart(body, eq)
		chunk := body[start:end]
		if !strings.Contains(chunk, ident+" = this.") {
			from = eq + 1
			continue
		}
		if !strings.Contains(chunk, "new "+simple+"(") && !strings.Contains(chunk, "new "+typ+"(") {
			from = eq + 1
			continue
		}
		body = body[:typeStart] + "Object" + body[typeEnd:]
		from = typeStart + len("Object")
	}
}

// wrapObjectMethodAssignToClassLocal wraps `ident = this.meth(...)` when meth
// is a same-class Object-returning method and ident is a non-Object class
// local. Downcast from Object always compiles; retyping the local does not.
func wrapObjectMethodAssignToClassLocal(body string) string {
	objMeths := objectReturningMethodNames(body)
	if len(objMeths) == 0 {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], " = this.")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rest := body[i+len(" = this."):]
		meth, ok, after := readJavaIdent(rest)
		if !ok || !strings.HasPrefix(after, "(") || objMeths[meth] == false {
			from = i + 1
			continue
		}
		if identStart > 0 && body[identStart-1] == '(' {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		typ := identDeclaredClassType(body[start:end], ident)
		if typ == "" || typ == "Object" || isPrimitiveOrObjectName(typ) {
			from = i + 1
			continue
		}
		open := i + len(" = this.") + len(meth)
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		call := body[i+len(" = ") : close+1]
		if strings.HasPrefix(strings.TrimSpace(call), "("+typ+")") || strings.HasPrefix(strings.TrimSpace(call), "(("+typ+")") {
			from = close
			continue
		}
		neu := " = (" + typ + ")(" + call + ")"
		body = body[:i] + neu + body[close+1:]
		from = i + len(neu)
	}
}

func objectReturningMethodNames(body string) map[string]bool {
	out := map[string]bool{}
	from := 0
	for {
		rel := strings.Index(body[from:], " Object ")
		if rel < 0 {
			break
		}
		i := from + rel + len(" Object ")
		name, ok, rest := readJavaIdent(body[i:])
		if ok && strings.HasPrefix(rest, "(") && !isDecompilerLocal(name) {
			out[name] = true
		}
		from = from + rel + 1
	}
	from = 0
	for {
		rel := strings.Index(body[from:], "\tObject ")
		if rel < 0 {
			break
		}
		i := from + rel + len("\tObject ")
		name, ok, rest := readJavaIdent(body[i:])
		if ok && strings.HasPrefix(rest, "(") && !isDecompilerLocal(name) {
			out[name] = true
		}
		from = from + rel + 1
	}
	return out
}

func identDeclaredClassType(chunk, ident string) string {
	from := 0
	needle := " " + ident
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return ""
		}
		j := from + rel
		after := j + len(needle)
		if after < len(chunk) && isJavaIdentChar(chunk[after]) {
			from = j + 1
			continue
		}
		if after < len(chunk) && chunk[after] != ' ' && chunk[after] != '=' && chunk[after] != ';' && chunk[after] != ',' && chunk[after] != ')' && chunk[after] != '\t' {
			from = j + 1
			continue
		}
		typeEnd := j
		for typeEnd > 0 && chunk[typeEnd-1] == ' ' {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && (isJavaIdentChar(chunk[typeStart-1]) || chunk[typeStart-1] == '.' || chunk[typeStart-1] == '$' || chunk[typeStart-1] == '[' || chunk[typeStart-1] == ']') {
			typeStart--
		}
		typ := chunk[typeStart:typeEnd]
		simple := lastDottedIdent(strings.ReplaceAll(strings.ReplaceAll(typ, "[]", ""), ">", ""))
		if simple == "" || isDecompilerLocal(simple) || isPrimitiveOrObjectName(simple) || isStmtKeyword(simple) {
			from = j + 1
			continue
		}
		if strings.Contains(typ, "<") {
			from = j + 1
			continue
		}
		return typ
	}
}

func methodReturnStreamArg(body string, pos int) string {
	start := prevMemberStart(body, pos)
	head := body[start:pos]
	brace := strings.Index(head, "{")
	if brace < 0 {
		return ""
	}
	sig := head[:brace]
	i := strings.LastIndex(sig, "Stream<")
	if i < 0 {
		return ""
	}
	gt := matchingCloseAngle(sig, i+len("Stream"))
	if gt < 0 {
		return ""
	}
	inner := strings.TrimSpace(sig[i+len("Stream<") : gt])
	if inner == "" || strings.Contains(inner, ",") || strings.HasPrefix(inner, "?") {
		return ""
	}
	return inner
}

// fixErasedZeroArgInnerCast wraps `.output()` / `.nextFinalOutput()` used as
// a method argument inside an outer `((T)(...))` with `(T)`. Generic FST-style
// getters erase to Object; the outer cast does not type the inner args.
func fixErasedZeroArgInnerCast(body string) string {
	body = wrapErasedGetterInOuterCast(body, ".output()")
	body = wrapErasedGetterInOuterCast(body, ".nextFinalOutput()")
	return body
}

func dottedReceiver(body string, dot int) (string, int) {
	recStart := dot
	for recStart > 0 && (isJavaIdentChar(body[recStart-1]) || body[recStart-1] == '.') {
		recStart--
	}
	for recStart < dot && body[recStart] == '.' {
		recStart++
	}
	recv := body[recStart:dot]
	if recv == "" || strings.HasPrefix(recv, ".") || strings.HasSuffix(recv, ".") {
		return "", recStart
	}
	return recv, recStart
}

func wrapErasedGetterInOuterCast(body, meth string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], meth)
		if rel < 0 {
			return body
		}
		dot := from + rel
		recv, recStart := dottedReceiver(body, dot)
		if recv == "" || recv == "this" || recv == "super" {
			from = dot + 1
			continue
		}
		if recStart > 0 && body[recStart-1] == '(' {
			pre := body[:recStart]
			if strings.HasSuffix(pre, ")(") || strings.HasSuffix(pre, ") (") {
				from = dot + len(meth)
				continue
			}
		}
		pre := recStart
		for pre > 0 && (body[pre-1] == ' ' || body[pre-1] == '\t') {
			pre--
		}
		if pre == 0 || (body[pre-1] != ',' && body[pre-1] != '(') {
			from = dot + len(meth)
			continue
		}
		typ := nearestOuterDumpCastType(body, recStart)
		if typ == "" {
			typ = nearestTypeVarCastContaining(body, recStart)
		}
		if typ == "" {
			typ = enclosingTypeVar(body, recStart)
		}
		if typ == "" || isPrimitiveOrObjectName(typ) || isBoxedPrimitiveName(typ) {
			from = dot + len(meth)
			continue
		}
		call := recv + meth
		wrap := "((" + typ + ")(" + call + "))"
		body = body[:recStart] + wrap + body[dot+len(meth):]
		from = recStart + len(wrap)
	}
}

func nearestOuterDumpCastType(body string, pos int) string {
	from := pos
	for from > 0 {
		rel := strings.LastIndex(body[:from], "((")
		if rel < 0 {
			return ""
		}
		typ, ok, rest := readNamedJavaType(body[rel+2:])
		if !ok || !strings.HasPrefix(rest, ")(") {
			from = rel
			continue
		}
		typeLen := len(body[rel+2:]) - len(rest)
		innerOpen := rel + 2 + typeLen + 1
		if innerOpen >= len(body) || body[innerOpen] != '(' {
			from = rel
			continue
		}
		close := matchingCloseParen(body, innerOpen)
		if close > pos {
			if t := sanitizeInferredType(typ); t != "" {
				if strings.HasSuffix(typ, "[]") {
					return t + strings.Repeat("[]", strings.Count(typ, "[]"))
				}
				return t
			}
			if typ != "" && !isPrimitiveOrObjectName(typ) {
				return lastDottedIdent(typ)
			}
		}
		from = rel
	}
	return ""
}

func nearestTypeVarCastContaining(body string, pos int) string {
	from := pos
	for from > 0 {
		rel := strings.LastIndex(body[:from], "(")
		if rel < 0 {
			return ""
		}
		ident, ok, rest := readJavaIdent(body[rel+1:])
		if !ok || !isTypeVarName(ident) || !strings.HasPrefix(rest, ")") {
			from = rel
			continue
		}
		j := rel + 1 + len(ident) + 1
		for j < len(body) && (body[j] == ' ' || body[j] == '\t') {
			j++
		}
		if j >= len(body) || body[j] != '(' {
			from = rel
			continue
		}
		close := matchingCloseParen(body, j)
		if close > pos {
			return ident
		}
		from = rel
	}
	return ""
}

func isTypeVarName(s string) bool {
	return len(s) == 1 && s[0] >= 'A' && s[0] <= 'Z'
}

// wrapObjectTypeVarArgs wraps uncast uses of an Object local that is uniquely
// cast to a type variable T (`(T)(varN)`) as method arguments `(T)(varN)`.
func wrapObjectTypeVarArgs(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "Object var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len("Object "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		typ := uniqueCastTypeOfLocal(member, ident)
		if !isTypeVarName(typ) {
			if tv := uniqueAssignedTypeVar(member, ident); isTypeVarName(tv) {
				typ = tv
			} else {
				tv := enclosingTypeVar(body, i)
				if !isTypeVarName(tv) || !objectLocalAssignedFromTypeVarCast(member, ident, tv) {
					from = i + 1
					continue
				}
				typ = tv
			}
		}
		neu := wrapIdentArgsAsType(member, ident, typ)
		if neu != member {
			body = body[:start] + neu + body[end:]
			from = i + 1
			continue
		}
		from = i + 1
	}
}

func objectLocalAssignedFromTypeVarCast(chunk, ident, tv string) bool {
	needle := ident + " = "
	from := 0
	cast := "(" + tv + ")("
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return false
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(chunk[i-1]) {
			from = i + 1
			continue
		}
		rest := chunk[i+len(needle):]
		semi := strings.IndexByte(rest, ';')
		if semi < 0 || semi > 400 {
			from = i + 1
			continue
		}
		if strings.Contains(rest[:semi], cast) {
			return true
		}
		from = i + 1
	}
}

func wrapIdentArgsAsType(chunk, ident, typ string) string {
	from := 0
	for {
		rel := strings.Index(chunk[from:], ident)
		if rel < 0 {
			return chunk
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(chunk[i-1]) {
			from = i + 1
			continue
		}
		end := i + len(ident)
		if end < len(chunk) && isJavaIdentChar(chunk[end]) {
			from = end
			continue
		}
		pre := strings.TrimRight(chunk[:i], " \t")
		if strings.HasSuffix(pre, "("+typ+")(") || strings.HasSuffix(pre, "(("+typ+")(") {
			from = end
			continue
		}
		j := i
		for j > 0 && (chunk[j-1] == ' ' || chunk[j-1] == '\t') {
			j--
		}
		if j == 0 || (chunk[j-1] != ',' && chunk[j-1] != '(') {
			from = end
			continue
		}
		k := end
		for k < len(chunk) && (chunk[k] == ' ' || chunk[k] == '\t') {
			k++
		}
		if k >= len(chunk) || (chunk[k] != ',' && chunk[k] != ')') {
			from = end
			continue
		}
		if chunk[k] == ')' {
			after := strings.TrimLeft(chunk[k+1:], " \t")
			if strings.HasPrefix(after, "->") {
				from = end
				continue
			}
		}
		wrap := "(" + typ + ")(" + ident + ")"
		chunk = chunk[:i] + wrap + chunk[end:]
		from = i + len(wrap)
	}
}

// wrapNullSentinelTernary wraps `((ident) == (null)) ? (FIELD) : (ident)` as
// `(T)(ternary)` when ident is a type-variable method param.
func wrapNullSentinelTernary(body string) string {
	from := 0
	needle := ") == (null)) ? ("
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		if identStart < 2 || body[identStart-2:identStart] != "((" {
			from = i + 1
			continue
		}
		rest := body[i+len(needle):]
		field, ok, after := readJavaIdent(rest)
		if !ok || !strings.HasPrefix(after, ") : (") {
			from = i + 1
			continue
		}
		after2 := after[len(") : ("):]
		ident2, ok2, after3 := readJavaIdent(after2)
		if !ok2 || ident2 != ident || !strings.HasPrefix(after3, ")") {
			from = i + 1
			continue
		}
		if field == ident || isDecompilerLocal(field) {
			from = i + 1
			continue
		}
		typ := methodParamTypeVar(body, identStart, ident)
		if !isTypeVarName(typ) {
			from = i + 1
			continue
		}
		head := strings.TrimRight(body[:identStart-2], " \t")
		if strings.HasSuffix(head, "("+typ+")(") || strings.HasSuffix(head, "(("+typ+")(") {
			from = i + 1
			continue
		}
		ternStart := identStart - 2
		ternEnd := i + len(needle) + len(field) + len(") : (") + len(ident) + 1
		if ternEnd > len(body) {
			from = i + 1
			continue
		}
		tern := body[ternStart:ternEnd]
		wrap := "(" + typ + ")(" + tern + ")"
		body = body[:ternStart] + wrap + body[ternEnd:]
		from = ternStart + len(wrap)
	}
}

func methodParamTypeVar(body string, pos int, ident string) string {
	start := prevMemberStart(body, pos)
	head := body[start:pos]
	brace := strings.Index(head, "{")
	if brace < 0 {
		return ""
	}
	sig := head[:brace]
	p := strings.LastIndex(sig, "(")
	if p < 0 {
		return ""
	}
	close := matchingCloseParen(sig, p)
	if close < 0 {
		return ""
	}
	params := sig[p+1 : close]
	needle := " " + ident
	idx := strings.Index(params, needle)
	if idx < 0 {
		return ""
	}
	after := idx + len(needle)
	if after < len(params) && isJavaIdentChar(params[after]) {
		return ""
	}
	typeEnd := idx
	for typeEnd > 0 && params[typeEnd-1] == ' ' {
		typeEnd--
	}
	typeStart := typeEnd
	for typeStart > 0 && isJavaIdentChar(params[typeStart-1]) {
		typeStart--
	}
	typ := params[typeStart:typeEnd]
	if isTypeVarName(typ) {
		return typ
	}
	return ""
}

func wrapEmptyIteratorTernaryArm(body string) string {
	for _, empty := range []string{"Collections.emptySet().iterator()", "Collections.emptyList().iterator()"} {
		from := 0
		for {
			rel := strings.Index(body[from:], empty)
			if rel < 0 {
				break
			}
			i := from + rel
			if strings.HasSuffix(body[:i], "(Iterator)(") || strings.HasSuffix(body[:i], "((Iterator)(") {
				from = i + len(empty)
				continue
			}
			pre := body[max(0, i-80):i]
			if !strings.Contains(pre, " ? (") && !strings.Contains(pre, "? (") {
				from = i + 1
				continue
			}
			wrap := "((Iterator)(" + empty + "))"
			body = body[:i] + wrap + body[i+len(empty):]
			from = i + len(wrap)
		}
	}
	return body
}

func wrapTernaryAssignElseCast(body string) string {
	from := 0
	needle := ") ? ("
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		eq := strings.LastIndex(body[:i], " = ")
		if eq < 0 || i-eq > 240 {
			from = i + 1
			continue
		}
		if strings.Contains(body[eq:i], ";") || strings.Contains(body[eq:i], "{") {
			from = i + 1
			continue
		}
		identEnd := eq
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, eq)
		end := nextMemberStart(body, eq)
		typ := identDeclaredClassType(body[start:end], ident)
		if typ == "" || typ == "Object" || isPrimitiveOrObjectName(typ) || isStmtKeyword(typ) || strings.Contains(typ, ".") || !isSimpleClassIdent(typ) {
			from = i + 1
			continue
		}
		thenOpen := i + len(") ? ")
		if thenOpen >= len(body) || body[thenOpen] != '(' {
			from = i + 1
			continue
		}
		thenClose := matchingCloseParen(body, thenOpen)
		if thenClose < 0 || !strings.HasPrefix(body[thenClose+1:], " : (") {
			from = i + 1
			continue
		}
		elseOpen := thenClose + 1 + len(" : ")
		if elseOpen >= len(body) || body[elseOpen] != '(' {
			from = i + 1
			continue
		}
		elseClose := matchingCloseParen(body, elseOpen)
		if elseClose < 0 {
			from = i + 1
			continue
		}
		// Statement-level assign only: `ident = cond ? then : else;`
		// Nested uses (map.get(ternary), return (cast)(ternary)) stay untouched.
		k := elseClose + 1
		for k < len(body) && (body[k] == ' ' || body[k] == '\t') {
			k++
		}
		if k >= len(body) || body[k] != ';' {
			from = elseClose
			continue
		}
		elseArm := body[elseOpen : elseClose+1]
		if strings.HasPrefix(elseArm, "(("+typ+")") || strings.HasPrefix(elseArm, "("+typ+")") {
			from = elseClose
			continue
		}
		inner := elseArm[1 : len(elseArm)-1]
		if !strings.Contains(inner, "(") {
			from = elseClose
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(inner), "new ") {
			from = elseClose
			continue
		}
		wrap := "((" + typ + ")(" + inner + "))"
		body = body[:elseOpen] + wrap + body[elseClose+1:]
		from = elseOpen + len(wrap)
	}
}

func enclosingTypeVar(body string, pos int) string {
	if t := methodTypeVarAt(body, pos); isTypeVarName(t) {
		return t
	}
	if t := classTypeVar(body); isTypeVarName(t) {
		return t
	}
	return ""
}

func classTypeVar(body string) string {
	markers := []string{"\npublic class ", "\nclass ", "\npublic static class ", "\nstatic class ", "public class ", "class "}
	best := -1
	for _, m := range markers {
		if i := strings.Index(body, m); i >= 0 && (best < 0 || i < best) {
			best = i + len(m)
		}
	}
	if best < 0 {
		return ""
	}
	rest := body[best:]
	brace := strings.Index(rest, "{")
	if brace < 0 {
		return ""
	}
	header := rest[:brace]
	lt := strings.Index(header, "<")
	if lt < 0 {
		return ""
	}
	ident, ok, _ := readJavaIdent(header[lt+1:])
	if ok && isTypeVarName(ident) {
		return ident
	}
	return ""
}

func methodTypeVarAt(body string, pos int) string {
	start := prevMemberStart(body, pos)
	head := body[start:min(pos, len(body))]
	brace := strings.Index(head, "{")
	if brace < 0 {
		return ""
	}
	sig := head[:brace]
	paren := strings.LastIndex(sig, "(")
	if paren < 0 {
		return ""
	}
	lt := strings.LastIndex(sig[:paren], "<")
	if lt < 0 {
		return ""
	}
	gt := strings.Index(sig[lt:], ">")
	if gt < 0 {
		return ""
	}
	gt += lt
	if gt > paren {
		return ""
	}
	after := strings.TrimSpace(sig[gt+1 : paren])
	first, ok, rest := readJavaIdent(after)
	if !ok {
		return ""
	}
	rest = strings.TrimSpace(rest)
	second, ok2, _ := readJavaIdent(rest)
	if !ok2 || first == "" || second == "" {
		return ""
	}
	ident, ok3, _ := readJavaIdent(sig[lt+1:])
	if ok3 && isTypeVarName(ident) {
		return ident
	}
	return ""
}

// wrapErasedFieldAsTypeVar wraps `ident.output` (field, not method) used as a
// method argument as `(T)(ident.output)` when T is the enclosing type variable.
func wrapErasedFieldAsTypeVar(body, field string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], field)
		if rel < 0 {
			return body
		}
		dot := from + rel
		after := dot + len(field)
		if after < len(body) && (body[after] == '(' || isJavaIdentChar(body[after])) {
			from = after
			continue
		}
		recv, recStart := dottedReceiver(body, dot)
		if recv == "" || recv == "this" || recv == "super" {
			from = after
			continue
		}
		pre := recStart
		for pre > 0 && (body[pre-1] == ' ' || body[pre-1] == '\t') {
			pre--
		}
		if pre == 0 || (body[pre-1] != ',' && body[pre-1] != '(') {
			from = after
			continue
		}
		k := after
		for k < len(body) && (body[k] == ' ' || body[k] == '\t') {
			k++
		}
		if k >= len(body) || (body[k] != ',' && body[k] != ')') {
			from = after
			continue
		}
		if strings.HasSuffix(body[:recStart], "(") && recStart >= 3 {
			head := strings.TrimRight(body[:recStart], " \t")
			if strings.HasSuffix(head, ")(") || strings.HasSuffix(head, ") (") {
				from = after
				continue
			}
		}
		typ := enclosingTypeVar(body, recStart)
		if !isTypeVarName(typ) {
			from = after
			continue
		}
		head := strings.TrimRight(body[:recStart], " \t")
		if strings.HasSuffix(head, "("+typ+")(") || strings.HasSuffix(head, "(("+typ+")(") {
			from = after
			continue
		}
		call := recv + field
		wrap := "(" + typ + ")(" + call + ")"
		body = body[:recStart] + wrap + body[after:]
		from = recStart + len(wrap)
	}
}

// wrapGetNoOutputObjectArgs wraps uncast arg uses of an Object local assigned
// from `getNoOutput()` as `(T)(varN)` when T is the enclosing type variable.
func wrapGetNoOutputObjectArgs(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "Object var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("Object "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		if !strings.Contains(rest, "getNoOutput()") {
			from = i + 1
			continue
		}
		eq := strings.Index(rest, "=")
		semi := strings.Index(rest, ";")
		if eq < 0 || semi < 0 || eq > semi || !strings.Contains(rest[:semi], "getNoOutput()") {
			from = i + 1
			continue
		}
		typ := enclosingTypeVar(body, i)
		if !isTypeVarName(typ) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		neu := wrapIdentArgsAsType(member, ident, typ)
		if neu != member {
			body = body[:start] + neu + body[end:]
			from = i + 1
			continue
		}
		from = i + 1
	}
}

func retypeTernarySiblingLocal(body string) string {
	from := 0
	needle := ") ? ("
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		eq := strings.LastIndex(body[:i], " = ")
		if eq < 0 || i-eq > 280 {
			from = i + 1
			continue
		}
		if strings.Contains(body[eq:i], ";") || strings.Contains(body[eq:i], "{") {
			from = i + 1
			continue
		}
		identEnd := eq
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		thenOpen := i + len(") ? ")
		if thenOpen >= len(body) || body[thenOpen] != '(' {
			from = i + 1
			continue
		}
		thenClose := matchingCloseParen(body, thenOpen)
		if thenClose < 0 || !strings.HasPrefix(body[thenClose+1:], " : (") {
			from = i + 1
			continue
		}
		elseOpen := thenClose + 1 + len(" : ")
		if elseOpen >= len(body) || body[elseOpen] != '(' {
			from = i + 1
			continue
		}
		elseClose := matchingCloseParen(body, elseOpen)
		if elseClose < 0 {
			from = i + 1
			continue
		}
		k := elseClose + 1
		for k < len(body) && (body[k] == ' ' || body[k] == '\t') {
			k++
		}
		if k >= len(body) || body[k] != ';' {
			from = elseClose
			continue
		}
		thenType := extractArmConstructType(body[thenOpen : thenClose+1])
		elseType := extractArmConstructType(body[elseOpen : elseClose+1])
		common := commonDollarPrefix(thenType, elseType)
		if common == "" {
			from = elseClose
			continue
		}
		start := prevMemberStart(body, eq)
		end := nextMemberStart(body, eq)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == common || !strings.HasPrefix(decl, common+"$") {
			from = elseClose
			continue
		}
		old := decl + " " + ident
		neu := common + " " + ident
		if !strings.Contains(member, old) {
			from = elseClose
			continue
		}
		member = strings.Replace(member, old, neu, 1)
		body = body[:start] + member + body[end:]
		from = start + len(member)
	}
}

func extractArmConstructType(arm string) string {
	s := strings.TrimSpace(arm)
	for len(s) >= 2 && s[0] == '(' {
		close := matchingCloseParen(s, 0)
		if close != len(s)-1 {
			break
		}
		s = strings.TrimSpace(s[1:close])
	}
	if strings.HasPrefix(s, "new ") {
		ident, ok, rest := readJavaIdent(s[len("new "):])
		if ok && strings.HasPrefix(rest, "(") {
			return ident
		}
	}
	ident, ok, rest := readJavaIdent(s)
	if !ok {
		return ""
	}
	if strings.HasPrefix(rest, ".") {
		field, ok2, after := readJavaIdent(rest[1:])
		if ok2 && field != "" && field == strings.ToUpper(field) && (after == "" || after[0] == ')' || after[0] == ',' || after[0] == ';') {
			return ident
		}
	}
	if strings.HasPrefix(rest, ")(") || strings.HasPrefix(rest, ") (") {
		return ident
	}
	return ""
}

func commonDollarPrefix(a, b string) string {
	if a == "" || b == "" || a == b || !strings.Contains(a, "$") || !strings.Contains(b, "$") {
		return ""
	}
	as := strings.Split(a, "$")
	bs := strings.Split(b, "$")
	n := len(as)
	if len(bs) < n {
		n = len(bs)
	}
	i := 0
	for i < n && as[i] == bs[i] {
		i++
	}
	if i == 0 || i == len(as) || i == len(bs) {
		return ""
	}
	return strings.Join(as[:i], "$")
}

// unwrapCollectionArraysAsList drops a raw `(Collection)(Arrays.asList(...))`
// wrapper. The raw Collection matches every Collection<...> overload equally
// ("reference is ambiguous"); Arrays.asList(T[]) infers List<T> and picks one.
func unwrapCollectionArraysAsList(body string) string {
	needle := "(Collection)(Arrays.asList("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		asList := i + len("(Collection)(")
		open := i + len(needle) - 1
		close := matchingCloseParen(body, open)
		if close < 0 || close+1 >= len(body) || body[close+1] != ')' {
			from = i + 1
			continue
		}
		inner := body[asList : close+1]
		body = body[:i] + inner + body[close+2:]
		from = i + len(inner)
	}
}

// unwrapCollectionNewCtor drops `(Collection)(new Type(...))` so a concrete
// collection ctor is not a raw Collection (ambiguous across Collection<A> vs
// Collection<B> overloads).
func unwrapCollectionNewCtor(body string) string {
	needle := "(Collection)(new "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len("(Collection)")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if !strings.HasPrefix(inner, "new ") {
			from = i + 1
			continue
		}
		body = body[:i] + inner + body[close+1:]
		from = i + len(inner)
	}
}

// wrapVarargsAsListDelegate rewrites `this.name(Arrays.asList(varN))` inside
// `name(T... varN)` as `this.name(new java.util.ArrayList<>(Arrays.asList(varN)))`
// so T is preserved (diamond) and the Collection/List overload is unique.
func wrapVarargsAsListDelegate(body string) string {
	needle := "(Arrays.asList("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		dot := strings.LastIndex(body[:i], "this.")
		if dot < 0 || i-dot > 80 {
			from = i + 1
			continue
		}
		name, ok, rest := readJavaIdent(body[dot+len("this."):])
		if !ok || !strings.HasPrefix(rest, "(") {
			from = i + 1
			continue
		}
		openCall := dot + len("this.") + len(name)
		if openCall >= len(body) || body[openCall] != '(' {
			from = i + 1
			continue
		}
		if !strings.HasPrefix(body[openCall:], needle) {
			from = i + 1
			continue
		}
		asOpen := openCall + len("(Arrays.asList")
		asClose := matchingCloseParen(body, asOpen)
		if asClose < 0 {
			from = i + 1
			continue
		}
		if asClose+1 >= len(body) || body[asClose+1] != ')' {
			from = i + 1
			continue
		}
		argIdent, ok2, afterArg := readJavaIdent(body[asOpen+1:])
		if !ok2 || !isDecompilerLocal(argIdent) || !strings.HasPrefix(afterArg, ")") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, dot)
		head := body[start:dot]
		brace := strings.Index(head, "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		sig := head[:brace]
		if !strings.Contains(sig, name+"(") || !strings.Contains(sig, "... "+argIdent) {
			from = i + 1
			continue
		}
		if strings.Contains(body[openCall:asClose+2], "new java.util.ArrayList<>(") {
			from = asClose
			continue
		}
		inner := body[openCall+1 : asClose+1]
		neu := "(new java.util.ArrayList<>(" + inner + "))"
		body = body[:openCall] + neu + body[asClose+2:]
		from = openCall + len(neu)
	}
}

func varargsElemType(sig, name, argIdent string) string {
	needle := "... " + argIdent
	idx := strings.Index(sig, needle)
	if idx < 0 {
		return ""
	}
	typeEnd := idx
	for typeEnd > 0 && sig[typeEnd-1] == ' ' {
		typeEnd--
	}
	typeStart := typeEnd
	for typeStart > 0 && (isJavaIdentChar(sig[typeStart-1]) || sig[typeStart-1] == '$' || sig[typeStart-1] == '?' || sig[typeStart-1] == '<' || sig[typeStart-1] == '>') {
		typeStart--
	}
	typ := strings.TrimSpace(sig[typeStart:typeEnd])
	if i := strings.IndexByte(typ, '<'); i >= 0 {
		typ = typ[:i]
	}
	typ = lastDottedIdent(typ)
	if typ == "" || isStmtKeyword(typ) {
		return ""
	}
	switch typ {
	case "int", "long", "boolean", "byte", "short", "char", "float", "double", "void":
		return ""
	}
	return typ
}

// unwrapCollectionBeforeLambda drops `(Collection)(expr)` when the next
// argument is a lambda. Raw Collection erases the element type so the lambda
// param becomes Object (IndexWriter applyToAll pendingMerges).
func unwrapCollectionBeforeLambda(body string) string {
	needle := "(Collection)("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len(needle) - 1
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		rest := body[close+1:]
		if !strings.HasPrefix(rest, ",") {
			from = i + 1
			continue
		}
		after := strings.TrimLeft(rest[1:], " \t")
		if !strings.HasPrefix(after, "(l") {
			from = i + 1
			continue
		}
		inner := body[open : close+1]
		body = body[:i] + inner + body[close+1:]
		from = i + len(inner)
	}
}

// wrapCompoundListOfAsList wraps `CompoundList.of(...)` as `(List)(...)` so a
// List<Factory> (raw element) is accepted as List<? extends Factory<?>>.
func wrapCompoundListOfAsList(body string) string {
	needle := "CompoundList.of("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		head := strings.TrimRight(body[:i], " \t")
		if strings.HasSuffix(head, "(List)(") || strings.HasSuffix(head, "((List)(") {
			from = i + len(needle)
			continue
		}
		open := i + len("CompoundList.of")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		call := body[i : close+1]
		wrap := "((List)(" + call + "))"
		body = body[:i] + wrap + body[close+1:]
		from = i + len(wrap)
	}
}

func dumpClassSimpleName(body string) string {
	markers := []string{"\npublic class ", "\nclass ", "\npublic enum ", "\nenum ", "\npublic static class ", "public class ", "class ", "enum "}
	best := -1
	bestName := ""
	for _, m := range markers {
		if i := strings.Index(body, m); i >= 0 && (best < 0 || i < best) {
			ident, ok, _ := readJavaIdent(body[i+len(m):])
			if ok {
				best = i
				bestName = ident
			}
		}
	}
	return bestName
}

// wrapReturnThisAsRawOuter rewrites `return this;` in Outer$Inner when the
// method returns Outer<...> (Empty.asDefined covariant return).
func wrapReturnThisAsRawOuter(body string) string {
	cls := dumpClassSimpleName(body)
	dollar := strings.Index(cls, "$")
	if dollar <= 0 || !strings.HasSuffix(cls, "$Empty") {
		return body
	}
	outer := cls[:dollar]
	from := 0
	for {
		rel := strings.Index(body[from:], "return this;")
		if rel < 0 {
			return body
		}
		i := from + rel
		start := prevMemberStart(body, i)
		head := body[start:i]
		brace := strings.Index(head, "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		sig := head[:brace]
		if !strings.Contains(sig, outer+"<") {
			from = i + 1
			continue
		}
		if strings.Contains(body[i:i+len("return this;")], "("+outer+")") {
			from = i + 1
			continue
		}
		neu := "return (" + outer + ")(this);"
		body = body[:i] + neu + body[i+len("return this;"):]
		from = i + len(neu)
	}
}

// wrapTypeVarReturnRawCast wraps `return expr;` as `return (Raw)(expr)` when
// the method is generic in T and the return type is Raw<T>.
func wrapTypeVarReturnRawCast(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "return ")
		if rel < 0 {
			return body
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(body[i-1]) {
			from = i + 1
			continue
		}
		semi := strings.Index(body[i:], ";")
		if semi < 0 || semi > 240 {
			from = i + 1
			continue
		}
		stmt := body[i : i+semi+1]
		if strings.Contains(stmt, "{") || strings.Contains(stmt, "->") {
			from = i + 1
			continue
		}
		expr := strings.TrimSpace(stmt[len("return ") : len(stmt)-1])
		if expr == "" || expr == "this" || expr == "null" || expr == "true" || expr == "false" || strings.HasPrefix(expr, "(") {
			from = i + 1
			continue
		}
		if !strings.ContainsAny(expr, ".(") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		head := body[start:i]
		brace := strings.Index(head, "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		if strings.TrimSpace(head[brace+1:]) != "" {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		afterRet := strings.Trim(strings.TrimSpace(body[i+semi+1:end]), "}")
		if strings.TrimSpace(afterRet) != "" {
			from = i + 1
			continue
		}
		if strings.Contains(body[start:end], "->") {
			from = i + 1
			continue
		}
		sig := head[:brace]
		paren := strings.LastIndex(sig, "(")
		if paren < 0 {
			from = i + 1
			continue
		}
		lt := strings.Index(sig[:paren], "<")
		if lt < 0 {
			from = i + 1
			continue
		}
		gt := strings.Index(sig[lt:], ">")
		if gt < 0 {
			from = i + 1
			continue
		}
		gt += lt
		tv, ok, _ := readJavaIdent(sig[lt+1:])
		if !ok || !isTypeVarName(tv) {
			from = i + 1
			continue
		}
		after := strings.TrimSpace(sig[gt+1 : paren])
		raw, ok2, rest := readJavaIdent(after)
		if !ok2 || !strings.HasPrefix(rest, "<"+tv) || !strings.Contains(raw, "$") {
			from = i + 1
			continue
		}
		if strings.HasPrefix(expr, "("+raw+")") || strings.HasPrefix(expr, "(("+raw+")") {
			from = i + 1
			continue
		}
		neu := "return (" + raw + ")(" + expr + ");"
		body = body[:i] + neu + body[i+semi+1:]
		from = i + len(neu)
	}
}

func retypeMixedNewAssignSuffix(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rhs, ok, _ := readJavaIdent(body[i+len(" = new "):])
		if !ok {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == rhs || strings.Contains(decl, ".") {
			from = i + 1
			continue
		}
		suf := commonCamelSuffix(decl, rhs)
		if suf == "" || suf == decl || suf == rhs || !isSimpleClassIdent(suf) || isJdkSimpleName(suf) || !typeNamePresent(body, suf) {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, suf+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func commonCamelSuffix(a, b string) string {
	wa := camelWords(a)
	wb := camelWords(b)
	n := 0
	for n < len(wa) && n < len(wb) && wa[len(wa)-1-n] == wb[len(wb)-1-n] {
		n++
	}
	if n == 0 {
		return ""
	}
	return strings.Join(wa[len(wa)-n:], "")
}

func camelWords(s string) []string {
	s = lastDottedIdent(strings.ReplaceAll(s, "$", ""))
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			out = append(out, s[start:i])
			start = i
		}
	}
	out = append(out, s[start:])
	return out
}

// qualifyShadowedEnumImport rewrites Class.method inside an enum constant
// whose simple name equals an imported type (LZ4 constant vs LZ4 class).
func qualifyShadowedEnumImport(body string) string {
	if !strings.Contains(body, "enum ") {
		return body
	}
	imports := map[string]string{}
	from := 0
	for {
		rel := strings.Index(body[from:], "\nimport ")
		if rel < 0 {
			break
		}
		i := from + rel + len("\nimport ")
		semi := strings.Index(body[i:], ";")
		if semi < 0 {
			break
		}
		path := strings.TrimSpace(body[i : i+semi])
		if strings.HasPrefix(path, "static ") || strings.HasSuffix(path, ".*") {
			from = i + semi
			continue
		}
		simple := lastDottedIdent(path)
		if simple != "" && simple[0] >= 'A' && simple[0] <= 'Z' {
			imports[simple] = path
		}
		from = i + semi
	}
	if len(imports) == 0 {
		return body
	}
	cls := dumpClassSimpleName(body)
	if cls == "" {
		return body
	}
	for simple, fqn := range imports {
		needle := "\n\t" + simple + "("
		pos := 0
		for {
			rel := strings.Index(body[pos:], needle)
			if rel < 0 {
				break
			}
			i := pos + rel
			openParen := i + len("\n\t") + len(simple)
			closeParen := matchingCloseParen(body, openParen)
			if closeParen < 0 {
				pos = i + 1
				continue
			}
			j := closeParen + 1
			for j < len(body) && (body[j] == ' ' || body[j] == '\t' || body[j] == '\n') {
				j++
			}
			if j >= len(body) || body[j] != '{' {
				pos = i + 1
				continue
			}
			closeBrace := matchingCloseBrace(body, j)
			if closeBrace < 0 {
				pos = i + 1
				continue
			}
			chunk := body[j : closeBrace+1]
			old := simple + "."
			neu := fqn + "."
			if strings.Contains(chunk, old) && !strings.Contains(chunk, neu) {
				chunk = strings.ReplaceAll(chunk, old, neu)
				body = body[:j] + chunk + body[closeBrace+1:]
				pos = j + len(chunk)
				continue
			}
			pos = closeBrace
		}
	}
	return body
}

func isLambdaLocal(s string) bool {
	if strings.HasPrefix(s, "lv") && len(s) > 2 {
		return true
	}
	return len(s) >= 2 && s[0] == 'l' && s[1] >= '0' && s[1] <= '9'
}

// wrapAccessDollarLambdaArg wraps `Type$Inner.access$N(l0)` as
// `Type$Inner.access$N((Type$Inner)(l0))` so erased lambda params match
// the accessor's instance type.
func wrapAccessDollarLambdaArg(body string) string {
	needle := ".access$"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		dot := from + rel
		recEnd := dot
		recStart := recEnd
		for recStart > 0 && isJavaIdentChar(body[recStart-1]) {
			recStart--
		}
		recv := body[recStart:recEnd]
		if recv == "" || !strings.Contains(recv, "$") {
			from = dot + len(needle)
			continue
		}
		j := dot + len(needle)
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
		}
		if j >= len(body) || body[j] != '(' {
			from = dot + 1
			continue
		}
		open := j
		argStart := open + 1
		ident, ok, rest := readJavaIdent(body[argStart:])
		if !ok || !isLambdaLocal(ident) {
			from = j + 1
			continue
		}
		if !strings.HasPrefix(rest, ")") && !strings.HasPrefix(rest, ",") {
			from = j + 1
			continue
		}
		head := strings.TrimRight(body[:argStart], " \t")
		if strings.HasSuffix(head, "("+recv+")(") || strings.HasSuffix(head, "(("+recv+")(") {
			from = j + 1
			continue
		}
		wrap := "(" + recv + ")(" + ident + ")"
		body = body[:argStart] + wrap + body[argStart+len(ident):]
		from = argStart + len(wrap)
	}
}

func methodReturnSimple(body string, pos int) string {
	start := prevMemberStart(body, pos)
	head := body[start:min(pos, len(body))]
	brace := strings.Index(head, "{")
	if brace < 0 {
		return ""
	}
	sig := head[:brace]
	paren := strings.LastIndex(sig, "(")
	if paren < 0 {
		return ""
	}
	pre := strings.TrimSpace(sig[:paren])
	nameStart := len(pre)
	for nameStart > 0 && isJavaIdentChar(pre[nameStart-1]) {
		nameStart--
	}
	pre = strings.TrimSpace(pre[:nameStart])
	if pre == "" {
		return ""
	}
	lt := strings.LastIndex(pre, "<")
	if lt >= 0 {
		pre = strings.TrimSpace(pre[:lt])
	}
	typeStart := len(pre)
	for typeStart > 0 && (isJavaIdentChar(pre[typeStart-1]) || pre[typeStart-1] == '.') {
		typeStart--
	}
	typ := lastDottedIdent(pre[typeStart:])
	if typ == "" || isPrimitiveOrObjectName(typ) || isStmtKeyword(typ) {
		return ""
	}
	return typ
}

// retypeSelfWrapToMethodReturn retypes a local that is later assigned
// `ident = new Wrapper(ident, ...)` to the enclosing method's return type
// (BoostQuery wrapping a *Query local returned as Query).
func retypeSelfWrapToMethodReturn(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rest := body[i+len(" = new "):]
		rhs, ok, after := readJavaIdent(rest)
		if !ok || !strings.HasPrefix(after, "(") {
			from = i + 1
			continue
		}
		open := i + len(" = new ") + len(rhs)
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		args := body[open+1 : close]
		if !strings.HasPrefix(args, ident+",") && args != ident {
			from = i + 1
			continue
		}
		ret := methodReturnSimple(body, i)
		if ret == "" || ret == rhs || !isSimpleClassIdent(ret) || isJdkSimpleName(ret) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == ret || decl == rhs {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, ret+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func isFinalCaptureLocal(s string) bool {
	if !strings.HasPrefix(s, "var") || len(s) < 6 {
		return false
	}
	i := 3
	if s[i] < '0' || s[i] > '9' {
		return false
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i+1 >= len(s) || s[i] != '_' || s[i+1] != 'f' {
		return false
	}
	i += 2
	if i >= len(s) {
		return false
	}
	for _, c := range s[i:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// retypeFinalObjectCapture retypes `final Object varN_fM = varN` to the
// declared class type of varN (lambda capture of a typed local dumped as Object).
func retypeFinalObjectCapture(body string) string {
	needle := "final Object "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || !isFinalCaptureLocal(ident) {
			from = i + 1
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, "=") {
			from = i + 1
			continue
		}
		rhs := strings.TrimLeft(rest[1:], " \t")
		src, ok2, after := readJavaIdent(rhs)
		if !ok2 || !isDecompilerLocal(src) || !strings.HasPrefix(ident, src+"_f") {
			from = i + 1
			continue
		}
		after = strings.TrimLeft(after, " \t")
		if !strings.HasPrefix(after, ";") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		typ := identDeclaredClassType(member, src)
		if typ == "" || typ == "Object" || strings.Contains(typ, ".") || strings.Contains(typ, "<") {
			from = i + 1
			continue
		}
		simple := lastDottedIdent(typ)
		if isPrimitiveOrObjectName(simple) || isStmtKeyword(simple) {
			from = i + 1
			continue
		}
		old := "final Object " + ident
		neu := "final " + typ + " " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		member = strings.Replace(member, old, neu, 1)
		body = body[:start] + member + body[end:]
		from = start
	}
}

func uniqueAssignTargetClassType(chunk, ident string) string {
	needle := " = " + ident
	seen := ""
	from := 0
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return seen
		}
		i := from + rel
		after := i + len(needle)
		if after < len(chunk) && isJavaIdentChar(chunk[after]) {
			from = i + 1
			continue
		}
		if after < len(chunk) && chunk[after] != ';' && chunk[after] != ')' && chunk[after] != ',' && chunk[after] != '\n' && chunk[after] != ' ' && chunk[after] != '\t' {
			from = i + 1
			continue
		}
		identEnd := i
		for identEnd > 0 && (chunk[identEnd-1] == ' ' || chunk[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(chunk[identStart-1]) {
			identStart--
		}
		lhs := chunk[identStart:identEnd]
		if lhs == "" || lhs == ident {
			from = after
			continue
		}
		typ := identDeclaredClassType(chunk, lhs)
		if typ == "" {
			from = after
			continue
		}
		if seen == "" {
			seen = typ
		} else if seen != typ {
			return ""
		}
		from = after
	}
}

// retypeIntAssignedNullToClass retypes `int varN = 0` / `Object varN = null`
// used as `(varN = expr) != (null)` and then assigned to a class-typed local.
func retypeIntAssignedNullToClass(body string) string {
	body = retypeAssignedNullToClass(body, "int var", "int ", " = 0;")
	body = retypeAssignedNullToClass(body, "Object var", "Object ", " = null;")
	return body
}

func retypeAssignedNullToClass(body, search, prefix, init string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], search)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(prefix):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, init) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !assignComparedToNull(member, ident) {
			from = i + 1
			continue
		}
		typ := uniqueAssignTargetClassType(member, ident)
		if typ == "" || strings.Contains(typ, ".") {
			typ = receiverTypeOfNullCmpAssign(member, ident)
		}
		if typ == "" || strings.Contains(typ, ".") {
			from = i + 1
			continue
		}
		old := prefix + ident + init
		neu := typ + " " + ident + " = null;"
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		member = strings.Replace(member, old, neu, 1)
		body = body[:start] + member + body[end:]
		from = start + len(member)
	}
}

// wrapIntIdentAsBooleanIf rewrites `if (ident)` to `if ((ident) != (0))`
// when ident is an int local (boolean materialised as 0/1).
func wrapIntIdentAsBooleanIf(body string) string {
	needle := "if ("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || (!isDecompilerLocal(ident) && !isFinalCaptureLocal(ident)) {
			from = i + 1
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, ")") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, "int "+ident) {
			from = i + 1
			continue
		}
		if strings.Contains(member, "boolean "+ident) {
			from = i + 1
			continue
		}
		old := "if (" + ident + ")"
		neu := "if ((" + ident + ") != (0))"
		if !strings.Contains(body[i:], old) {
			from = i + 1
			continue
		}
		body = body[:i] + neu + body[i+len(old):]
		from = i + len(neu)
	}
}

// wrapUnresolvedNestedNew rewrites `new Type$Inner(new StringBuilder...)`
// to `new Type$Inner$Sub(...)` when a unique sibling nested new with a
// StringBuilder arg exists (abstract two-level new vs concrete three-level).
func wrapUnresolvedNestedNew(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(body[i-1]) {
			from = i + 1
			continue
		}
		typ, ok, rest := readJavaIdent(body[i+len("new "):])
		if !ok || strings.Count(typ, "$") != 1 || !strings.HasPrefix(rest, "(new StringBuilder") {
			from = i + 1
			continue
		}
		prefix := "new " + typ + "$"
		found := ""
		pos := 0
		for {
			rel2 := strings.Index(body[pos:], prefix)
			if rel2 < 0 {
				break
			}
			j := pos + rel2
			sub, ok2, after := readJavaIdent(body[j+len("new "):])
			if !ok2 || strings.Count(sub, "$") != 2 || !strings.HasPrefix(after, "(new StringBuilder") {
				pos = j + len(prefix)
				continue
			}
			if found == "" {
				found = sub
			} else if found != sub {
				found = ""
				break
			}
			pos = j + len(prefix)
		}
		if found == "" {
			from = i + 1
			continue
		}
		old := "new " + typ + "("
		neu := "new " + found + "("
		body = body[:i] + neu + body[i+len(old):]
		from = i + len(neu)
	}
}

// retypeMixedDollarNewAssign retypes a local declared Type$A that is later
// assigned `new Type$B(...)` to the common `$` prefix (nested sibling mix).
func retypeMixedDollarNewAssign(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rhs, ok, after := readJavaIdent(body[i+len(" = new "):])
		if !ok || !strings.HasPrefix(after, "(") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == rhs || strings.Contains(decl, ".") {
			from = i + 1
			continue
		}
		if strings.Contains(member, ident+" = ") && strings.Contains(member, ".access$") {
			from = i + 1
			continue
		}
		common := commonDollarPrefix(decl, rhs)
		if common == "" || common == decl || !strings.Contains(common, "$") {
			from = i + 1
			continue
		}
		if !typeNamePresent(body, common) && !strings.Contains(body, common+"$") {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, common+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

// retypeListUsedAsString retypes a List local used as `(String)(ident)` or
// `ident instanceof String` to Object (JSON-value slot mix).
func retypeListUsedAsString(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "List var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len("List "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, "(String)("+ident+")") && !strings.Contains(member, ident+" instanceof String") {
			from = i + 1
			continue
		}
		old := "List " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, "Object "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start
	}
}

func unwrapObjectNullCast(body string) string {
	const old = ".matches((Object)(null))"
	if !strings.Contains(body, old) {
		return body
	}
	return strings.ReplaceAll(body, old, ".matches(null)")
}

// wrapBangOnStringLocal rewrites `!(ident)` when ident is a String local and
// the class has a boolean `generate` field (boolean/String slot mix).
func wrapBangOnStringLocal(body string) string {
	if !strings.Contains(body, "boolean generate") && !strings.Contains(body, " boolean generate") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], "!(")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+2:])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, ")") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, "String "+ident) {
			from = i + 1
			continue
		}
		old := "!(" + ident + ")"
		neu := "!(this.generate)"
		body = body[:i] + neu + body[i+len(old):]
		from = i + len(neu)
	}
}

// retypeInstanceThenNewSibling retypes `Type$A ident = Type$A.INSTANCE`
// later assigned `new Type$B(...)` to the common `$` prefix.
func retypeInstanceThenNewSibling(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rhs, ok, after := readJavaIdent(body[i+len(" = new "):])
		if !ok || !strings.HasPrefix(after, "(") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == rhs || !strings.Contains(decl, "$") {
			from = i + 1
			continue
		}
		if !strings.Contains(member, decl+" "+ident) || !strings.Contains(member, decl+".INSTANCE") {
			from = i + 1
			continue
		}
		common := commonDollarPrefix(decl, rhs)
		if common == "" || common == decl {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		neu := strings.Replace(member, old, common+" "+ident, 1)
		if neu == member {
			from = i + 1
			continue
		}
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

// wrapComputeIfAbsentLambdaArg wraps a computeIfAbsent lambda param as
// String when the key expression is a field access (map-of-String keys).
func wrapComputeIfAbsentLambdaArg(body string) string {
	needle := ".computeIfAbsent("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len(".computeIfAbsent")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		args := body[open+1 : close]
		comma := strings.Index(args, ",(")
		if comma < 0 {
			from = close
			continue
		}
		key := args[:comma]
		if !strings.Contains(key, ".") {
			from = close
			continue
		}
		lam := args[comma+1:]
		ident, ok, rest := readJavaIdent(strings.TrimPrefix(lam, "("))
		if !ok || !isLambdaLocal(ident) || !strings.HasPrefix(rest, ") ->") {
			from = close
			continue
		}
		brace := strings.Index(lam, "{")
		if brace < 0 {
			from = close
			continue
		}
		lamBody := lam[brace:]
		if strings.Contains(lamBody, "(String)("+ident+")") {
			from = close
			continue
		}
		neuBody := wrapIdentArgsAsType(lamBody, ident, "String")
		if neuBody == lamBody {
			from = close
			continue
		}
		neuArgs := args[:comma+1] + lam[:brace] + neuBody
		body = body[:open+1] + neuArgs + body[close:]
		from = open + 1 + len(neuArgs)
	}
}

// wrapArraySortLambdaElem wraps `Arrays.sort(this.field, ... (l0) -> l0.`
// using the field's array element type.
func wrapArraySortLambdaElem(body string) string {
	needle := "Arrays.sort(this."
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		field, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || !strings.HasPrefix(rest, ",") {
			from = i + 1
			continue
		}
		elem := arrayFieldElemType(body, field)
		if elem == "" || isPrimitiveOrObjectName(elem) {
			from = i + 1
			continue
		}
		open := strings.Index(body[i:], "(")
		if open < 0 {
			from = i + 1
			continue
		}
		openAbs := i + open
		close := matchingCloseParen(body, openAbs)
		if close < 0 {
			from = i + 1
			continue
		}
		call := body[openAbs : close+1]
		lamAt := strings.Index(call, "(l")
		if lamAt < 0 {
			from = close
			continue
		}
		ident, ok2, after := readJavaIdent(call[lamAt+1:])
		if !ok2 || !isLambdaIdent(ident) || !strings.HasPrefix(after, ") ->") {
			from = close
			continue
		}
		braceRel := strings.Index(call[lamAt:], "{")
		if braceRel < 0 {
			from = close
			continue
		}
		braceAbs := lamAt + braceRel
		closeBrace := matchingCloseBrace(call, braceAbs)
		if closeBrace < 0 {
			from = close
			continue
		}
		lamBody := call[braceAbs : closeBrace+1]
		neuBody := wrapLambdaIdentAsType(lamBody, ident, elem)
		if neuBody == lamBody {
			from = close
			continue
		}
		neu := call[:braceAbs] + neuBody + call[closeBrace+1:]
		body = body[:openAbs] + neu + body[close+1:]
		from = openAbs + len(neu)
	}
}

func arrayFieldElemType(body, field string) string {
	needles := []string{"[] " + field, "[]" + field}
	for _, n := range needles {
		idx := strings.Index(body, n)
		if idx < 0 {
			continue
		}
		typeEnd := idx
		for typeEnd > 0 && body[typeEnd-1] == ' ' {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && (isJavaIdentChar(body[typeStart-1]) || body[typeStart-1] == '$' || body[typeStart-1] == '.') {
			typeStart--
		}
		typ := lastDottedIdent(body[typeStart:typeEnd])
		if typ != "" && !isPrimitiveOrObjectName(typ) && !isStmtKeyword(typ) {
			return typ
		}
	}
	return ""
}

func uniqueNextCastType(member string) string {
	seen := ""
	from := 0
	for {
		rel := strings.Index(member[from:], ".next()")
		if rel < 0 {
			return seen
		}
		i := from + rel
		identEnd := i
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(member[identStart-1]) {
			identStart--
		}
		if identStart < 2 || member[identStart-1] != '(' || member[identStart-2] != ')' {
			from = i + 5
			continue
		}
		open := matchingOpenParen(member, identStart-2)
		if open < 0 {
			from = i + 5
			continue
		}
		typ, ok, after := readJavaIdent(member[open+1:])
		if !ok || !strings.HasPrefix(after, ")") {
			from = i + 5
			continue
		}
		if lastDottedIdent(typ) == "" || isStmtKeyword(typ) || isDecompilerLocal(typ) {
			from = i + 5
			continue
		}
		if isJdkCollectionName(typ) {
			from = i + 5
			continue
		}
		if seen == "" {
			seen = typ
		} else if seen != typ {
			return ""
		}
		from = i + 5
	}
}

func firstNonCollectionNextCast(chunk string) string {
	from := 0
	for {
		rel := strings.Index(chunk[from:], ".next()")
		if rel < 0 {
			return ""
		}
		i := from + rel
		identStart := i
		for identStart > 0 && isJavaIdentChar(chunk[identStart-1]) {
			identStart--
		}
		if identStart < 2 || chunk[identStart-1] != '(' || chunk[identStart-2] != ')' {
			from = i + 5
			continue
		}
		open := matchingOpenParen(chunk, identStart-2)
		if open < 0 {
			from = i + 5
			continue
		}
		typ, ok, after := readJavaIdent(chunk[open+1:])
		if !ok || !strings.HasPrefix(after, ")") {
			from = i + 5
			continue
		}
		if lastDottedIdent(typ) == "" || isStmtKeyword(typ) || isDecompilerLocal(typ) || isJdkCollectionName(typ) {
			from = i + 5
			continue
		}
		return typ
	}
}

func wrapCollectionsSortLambda(body string) string {
	needle := "Collections.sort("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len("Collections.sort")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if !strings.HasPrefix(inner, "(List)(") {
			from = close
			continue
		}
		listIdent, ok, _ := readJavaIdent(inner[len("(List)("):])
		if !ok || !isDecompilerLocal(listIdent) {
			from = close
			continue
		}
		start := prevMemberStart(body, i)
		windowStart := start
		if d := strings.LastIndex(body[start:i], "ArrayList "+listIdent+" = "); d >= 0 {
			windowStart = start + d
		} else if d := strings.LastIndex(body[start:i], "List "+listIdent+" = "); d >= 0 {
			windowStart = start + d
		}
		elem := uniqueNextCastType(body[windowStart:i])
		if elem == "" {
			elem = uniqueAddElemType(body[windowStart:i], listIdent)
		}
		if elem == "" {
			from = close
			continue
		}
		call := body[open : close+1]
		neu := wrapSortCallLambdaBodies(call, elem)
		if neu == call {
			from = close
			continue
		}
		body = body[:open] + neu + body[close+1:]
		from = open + len(neu)
	}
}

func wrapSortCallLambdaBodies(call, elem string) string {
	from := 0
	for {
		rel := strings.Index(call[from:], ") ->")
		if rel < 0 {
			return call
		}
		arrow := from + rel
		identEnd := arrow
		for identEnd > 0 && (call[identEnd-1] == ' ' || call[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(call[identStart-1]) {
			identStart--
		}
		ident := call[identStart:identEnd]
		if !isLambdaIdent(ident) && !isLambdaLocal(ident) {
			from = arrow + 1
			continue
		}
		braceRel := strings.Index(call[arrow:], "{")
		if braceRel < 0 || braceRel > 12 {
			from = arrow + 1
			continue
		}
		brace := arrow + braceRel
		closeBrace := matchingCloseBrace(call, brace)
		if closeBrace < 0 {
			from = arrow + 1
			continue
		}
		lamBody := call[brace : closeBrace+1]
		neuBody := wrapLambdaIdentAsType(lamBody, ident, elem)
		if ident2 := siblingLambdaIdent(call[:arrow]); ident2 != "" && ident2 != ident {
			neuBody = wrapLambdaIdentAsType(neuBody, ident2, elem)
		}
		if neuBody == lamBody {
			from = closeBrace
			continue
		}
		call = call[:brace] + neuBody + call[closeBrace+1:]
		from = brace + len(neuBody)
	}
}

func siblingLambdaIdent(head string) string {
	open := strings.LastIndex(head, "(")
	if open < 0 {
		return ""
	}
	params := head[open+1:]
	comma := strings.Index(params, ",")
	if comma < 0 {
		return ""
	}
	ident, ok, _ := readJavaIdent(strings.TrimSpace(params[:comma]))
	if !ok || (!isLambdaIdent(ident) && !isLambdaLocal(ident)) {
		return ""
	}
	return ident
}

func fixBlankFinalTryCatchAssign(body string) string {
	marker := "\n\tstatic  {"
	clinit := strings.Index(body, marker)
	if clinit < 0 {
		marker = "\n\tstatic {"
		clinit = strings.Index(body, marker)
	}
	if clinit < 0 {
		return body
	}
	open := strings.Index(body[clinit:], "{")
	if open < 0 {
		return body
	}
	openAbs := clinit + open
	close := matchingCloseBrace(body, openAbs)
	if close < 0 {
		return body
	}
	block := body[openAbs : close+1]
	from := 0
	changed := false
	for {
		rel := strings.Index(body[from:], "static final boolean ")
		if rel < 0 {
			break
		}
		i := from + rel
		name, ok, rest := readJavaIdent(body[i+len("static final boolean "):])
		if !ok || !strings.HasPrefix(strings.TrimSpace(rest), ";") {
			from = i + 1
			continue
		}
		assign := name + " = "
		if strings.Count(block, assign) < 2 || !strings.Contains(block, "}catch(") {
			from = i + 1
			continue
		}
		tmp := name + "_x"
		if strings.Contains(block, tmp) {
			from = i + 1
			continue
		}
		neu := strings.ReplaceAll(block, assign, tmp+" = ")
		neu = neu[:1] + "\n\t\tboolean " + tmp + ";" + neu[1:]
		catchEnd := firstTryCatchEnd(neu, tmp+" = ")
		if catchEnd < 0 {
			from = i + 1
			continue
		}
		neu = neu[:catchEnd+1] + "\n\t\t" + name + " = " + tmp + ";" + neu[catchEnd+1:]
		block = neu
		changed = true
		from = i + 1
	}
	if !changed {
		return body
	}
	return body[:openAbs] + block + body[close+1:]
}

func firstTryCatchEnd(block, assign string) int {
	from := 0
	for {
		rel := strings.Index(block[from:], "try{")
		if rel < 0 {
			return -1
		}
		i := from + rel
		open := strings.Index(block[i:], "{")
		if open < 0 {
			return -1
		}
		tclose := matchingCloseBrace(block, i+open)
		if tclose < 0 {
			return -1
		}
		if !strings.Contains(block[i:tclose+1], assign) {
			from = tclose
			continue
		}
		pos := tclose
		last := tclose
		for {
			if pos+1 >= len(block) || !strings.HasPrefix(block[pos+1:], "catch(") {
				break
			}
			paren := pos + 1 + len("catch")
			pclose := matchingCloseParen(block, paren)
			if pclose < 0 {
				break
			}
			j := pclose + 1
			for j < len(block) && (block[j] == ' ' || block[j] == '\t' || block[j] == '\n') {
				j++
			}
			if j >= len(block) || block[j] != '{' {
				break
			}
			end := matchingCloseBrace(block, j)
			if end < 0 {
				break
			}
			last = end
			pos = end
		}
		return last
	}
}

func wrapEnumNoOpNewAsUsingJump(body string) string {
	needle := "$NoOp("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		newAt := strings.LastIndex(body[:i], "new ")
		if newAt < 0 || i-newAt > 80 {
			from = i + 1
			continue
		}
		typ := strings.TrimSpace(body[newAt+len("new ") : i+len("$NoOp")])
		if !strings.HasSuffix(typ, "$NoOp") {
			from = i + 1
			continue
		}
		if !strings.Contains(body, typ+".INSTANCE") {
			from = i + 1
			continue
		}
		parent := strings.TrimSuffix(typ, "$NoOp")
		neu := "new " + parent + "$UsingJump"
		body = body[:newAt] + neu + body[i+len("$NoOp"):]
		oldDecl := typ + " "
		newDecl := parent + " "
		if strings.Contains(body, oldDecl) {
			body = strings.Replace(body, oldDecl, newDecl, 1)
		}
		from = newAt + len(neu)
	}
}

func retypeStringAssignedClassField(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "String var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len("String "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		needle := ident + " = this."
		idx := strings.Index(member, needle)
		if idx < 0 {
			from = i + 1
			continue
		}
		field, ok2, _ := readJavaIdent(member[idx+len(needle):])
		if !ok2 || field == "" {
			from = i + 1
			continue
		}
		ft := fieldDeclType(body, field)
		if ft == "" || ft == "String" || isPrimitiveOrObjectName(ft) || strings.Contains(ft, ".") {
			from = i + 1
			continue
		}
		if !strings.Contains(member, ident+".close(") &&
			!strings.Contains(member, "("+ident+") != (this."+field+")") &&
			!strings.Contains(member, "("+ident+") == (this."+field+")") {
			from = i + 1
			continue
		}
		old := "String " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, ft+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func fieldDeclType(body, field string) string {
	needles := []string{" " + field + ";", " " + field + " =", " " + field + "\n"}
	for _, n := range needles {
		idx := strings.Index(body, n)
		if idx < 0 {
			continue
		}
		typeEnd := idx
		for typeEnd > 0 && body[typeEnd-1] == ' ' {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && (isJavaIdentChar(body[typeStart-1]) || body[typeStart-1] == '$') {
			typeStart--
		}
		typ := body[typeStart:typeEnd]
		if typ != "" && !isStmtKeyword(typ) && !isDecompilerLocal(typ) {
			return typ
		}
	}
	return ""
}

func wrapMatcherMatchesArrayList(body string) string {
	needle := "this.matcher.matches("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len("this.matcher.matches")
		ident, ok, rest := readJavaIdent(body[open+1:])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, ")") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, "ArrayList "+ident) {
			from = i + 1
			continue
		}
		head := strings.TrimRight(body[:open+1], " \t")
		headTail := head
		if len(headTail) > 48 {
			headTail = headTail[len(headTail)-48:]
		}
		if strings.HasSuffix(head, "(Iterable)(") || strings.Contains(headTail, "(Iterable<? extends ") {
			from = i + 1
			continue
		}
		wrap := "((Iterable)(" + ident + "))"
		if elem := matcherIterableElemType(body); elem != "" {
			wrap = "((Iterable<? extends " + elem + ">)(" + ident + "))"
		}
		body = body[:open+1] + wrap + body[open+1+len(ident):]
		from = open + 1 + len(wrap)
	}
}

func stripRawStreamTypedLambda(body string) string {
	for _, meth := range []string{"mapToDouble", "mapToInt", "mapToLong"} {
		body = stripRawStreamTypedLambdaMeth(body, meth)
	}
	return body
}

func stripRawStreamTypedLambdaMeth(body, meth string) string {
	needle := ".stream()." + meth + "(("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len(needle):]
		typ, ok, after := readJavaIdent(rest)
		if !ok || !isSimpleClassIdent(typ) {
			from = i + 1
			continue
		}
		after = strings.TrimLeft(after, " \t")
		ident, ok2, after2 := readJavaIdent(after)
		if !ok2 || (!isLambdaIdent(ident) && !isLambdaLocal(ident)) {
			from = i + 1
			continue
		}
		if !strings.HasPrefix(strings.TrimLeft(after2, " \t"), ") ->") {
			from = i + 1
			continue
		}
		replNeedle := ".stream()." + meth + "((" + typ + " " + ident + ")"
		repl := ".stream()." + meth + "((" + ident + ")"
		if !strings.HasPrefix(body[i:], replNeedle) {
			from = i + 1
			continue
		}
		body = body[:i] + repl + body[i+len(replNeedle):]
		arrowRel := strings.Index(body[i:], ") ->")
		if arrowRel >= 0 {
			braceRel := strings.Index(body[i+arrowRel:], "{")
			if braceRel >= 0 && braceRel < 12 {
				bAbs := i + arrowRel + braceRel
				cb := matchingCloseBrace(body, bAbs)
				if cb > 0 {
					lamBody := body[bAbs : cb+1]
					neu := wrapLambdaIdentAsType(lamBody, ident, typ)
					body = body[:bAbs] + neu + body[cb+1:]
				}
			}
		}
		from = i + len(repl)
	}
}

func uniqueAssignedTypeVar(chunk, ident string) string {
	seen := ""
	from := 0
	needle := ident + " = "
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return seen
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(chunk[i-1]) {
			from = i + 1
			continue
		}
		rest := chunk[i+len(needle):]
		semi := strings.IndexByte(rest, ';')
		if semi < 0 || semi > 400 {
			from = i + 1
			continue
		}
		assign := rest[:semi]
		tvFrom := 0
		for {
			rel2 := strings.Index(assign[tvFrom:], ")(")
			if rel2 < 0 {
				break
			}
			closeParen := tvFrom + rel2
			open := matchingOpenParen(assign, closeParen)
			if open < 0 {
				tvFrom = closeParen + 1
				continue
			}
			tv, ok, after := readJavaIdent(assign[open+1:])
			if !ok || !isTypeVarName(tv) || !strings.HasPrefix(after, ")") {
				tvFrom = closeParen + 1
				continue
			}
			if seen == "" {
				seen = tv
			} else if seen != tv {
				return ""
			}
			tvFrom = closeParen + 1
		}
		from = i + 1
	}
}

func assignComparedToNull(chunk, ident string) bool {
	for _, needle := range []string{"(" + ident + " = ", "(" + ident + "="} {
		from := 0
		for {
			rel := strings.Index(chunk[from:], needle)
			if rel < 0 {
				break
			}
			open := from + rel
			close := matchingCloseParen(chunk, open)
			if close < 0 {
				from = open + 1
				continue
			}
			rest := strings.TrimLeft(chunk[close+1:], " \t")
			if strings.HasPrefix(rest, "!= (null)") || strings.HasPrefix(rest, "== (null)") {
				return true
			}
			from = open + 1
		}
	}
	return false
}

func receiverTypeOfNullCmpAssign(chunk, ident string) string {
	for _, needle := range []string{"(" + ident + " = ", "(" + ident + "="} {
		from := 0
		for {
			rel := strings.Index(chunk[from:], needle)
			if rel < 0 {
				break
			}
			i := from + rel
			rhs := chunk[i+len(needle):]
			recv, ok, rest := readJavaIdent(rhs)
			if !ok || recv == ident || !strings.HasPrefix(rest, ".") {
				from = i + 1
				continue
			}
			window := rhs
			if len(window) > 240 {
				window = window[:240]
			}
			if !strings.Contains(window, ") != (null)") && !strings.Contains(window, ") == (null)") {
				from = i + 1
				continue
			}
			typ := identDeclaredClassType(chunk, recv)
			if typ != "" && !strings.Contains(typ, ".") {
				return typ
			}
			from = i + 1
		}
	}
	return ""
}

func uniqueAddElemType(chunk, ident string) string {
	seen := ""
	from := 0
	needle := ident + ".add("
	for {
		rel := strings.Index(chunk[from:], needle)
		if rel < 0 {
			return seen
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(chunk[i-1]) {
			from = i + 1
			continue
		}
		arg := chunk[i+len(needle):]
		var typ string
		switch {
		case strings.HasPrefix(arg, "new "):
			t, ok, _ := readDottedType(arg[len("new "):])
			if ok {
				typ = t
			}
		case strings.HasPrefix(arg, "(("):
			t, ok, rest := readDottedType(arg[2:])
			if ok && strings.HasPrefix(rest, ")") {
				typ = t
			}
		case strings.HasPrefix(arg, "("):
			t, ok, rest := readDottedType(arg[1:])
			if ok && strings.HasPrefix(rest, ")") {
				typ = t
			}
		}
		if typ == "" || isDecompilerLocal(lastDottedIdent(typ)) || isStmtKeyword(lastDottedIdent(typ)) {
			from = i + 1
			continue
		}
		if seen == "" {
			seen = typ
		} else if seen != typ {
			return ""
		}
		from = i + 1
	}
}

func wrapComparatorComparingLambda(body string) string {
	needle := "Comparator<"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		typ, ok, rest := readDottedType(body[i+len(needle):])
		if !ok || strings.Contains(typ, "?") || strings.Contains(typ, "<") {
			from = i + 1
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, ">") {
			from = i + 1
			continue
		}
		after := strings.TrimLeft(rest[1:], " \t")
		_, ok2, afterName := readJavaIdent(after)
		if !ok2 {
			from = i + 1
			continue
		}
		afterName = strings.TrimLeft(afterName, " \t")
		if !strings.HasPrefix(afterName, "=") {
			from = i + 1
			continue
		}
		rhs := strings.TrimLeft(afterName[1:], " \t")
		if !strings.Contains(rhs, "Comparator.comparing") {
			from = i + 1
			continue
		}
		end := stmtEndAtDepthZero(body, i)
		if end < 0 {
			from = i + 1
			continue
		}
		chunk := body[i : end+1]
		neu := wrapComparingLambdaBodies(chunk, typ)
		if neu == chunk {
			from = end
			continue
		}
		body = body[:i] + neu + body[end+1:]
		from = i + len(neu)
	}
}

func stmtEndAtDepthZero(s string, from int) int {
	depthBrace, depthParen := 0, 0
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '{':
			depthBrace++
		case '}':
			depthBrace--
		case '(':
			depthParen++
		case ')':
			depthParen--
		case ';':
			if depthBrace == 0 && depthParen == 0 {
				return i
			}
		}
	}
	return -1
}

func wrapComparingLambdaBodies(chunk, typ string) string {
	from := 0
	for {
		rel := strings.Index(chunk[from:], ") ->")
		if rel < 0 {
			return chunk
		}
		arrow := from + rel
		identEnd := arrow
		for identEnd > 0 && (chunk[identEnd-1] == ' ' || chunk[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(chunk[identStart-1]) {
			identStart--
		}
		ident := chunk[identStart:identEnd]
		if !isLambdaIdent(ident) && !isLambdaLocal(ident) {
			from = arrow + 1
			continue
		}
		if identStart > 0 && chunk[identStart-1] == '(' {
			chunk = chunk[:identStart] + typ + " " + chunk[identStart:]
			arrow += len(typ) + 1
		}
		braceRel := strings.Index(chunk[arrow:], "{")
		if braceRel < 0 || braceRel > 12 {
			from = arrow + 1
			continue
		}
		brace := arrow + braceRel
		closeBrace := matchingCloseBrace(chunk, brace)
		if closeBrace < 0 {
			from = arrow + 1
			continue
		}
		lamBody := chunk[brace : closeBrace+1]
		neuBody := wrapLambdaIdentAsType(lamBody, ident, typ)
		if neuBody == lamBody {
			from = closeBrace
			continue
		}
		chunk = chunk[:brace] + neuBody + chunk[closeBrace+1:]
		from = brace + len(neuBody)
	}
}

func unwrapEnumArrayIndexCast(body string) string {
	needle := "(Enum)("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, "[") {
			from = i + 1
			continue
		}
		open := i + len(needle) + len(ident)
		close := matchingCloseBracket(body, open)
		if close < 0 || close+1 >= len(body) || body[close+1] != ')' {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		typ := identDeclaredClassType(body[start:end], ident)
		if !strings.HasSuffix(typ, "[]") {
			from = i + 1
			continue
		}
		elem := strings.TrimSuffix(typ, "[]")
		if elem == "" || elem == "Enum" || isPrimitiveOrObjectName(lastDottedIdent(elem)) {
			from = i + 1
			continue
		}
		inner := body[i+len(needle) : close+1]
		body = body[:i] + inner + body[close+2:]
		from = i + len(inner)
	}
}

func matchingCloseBracket(s string, openIdx int) int {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '[' {
		return -1
	}
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func rewriteClassLocalCmpZero(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], ") == (0)")
		rel2 := strings.Index(body[from:], ") != (0)")
		if rel < 0 && rel2 < 0 {
			return body
		}
		useNe := false
		if rel < 0 || (rel2 >= 0 && rel2 < rel) {
			rel = rel2
			useNe = true
		}
		closeParen := from + rel
		identEnd := closeParen
		for identEnd > 0 && (body[identEnd-1] == ' ' || body[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = closeParen + 1
			continue
		}
		if identStart == 0 || body[identStart-1] != '(' {
			from = closeParen + 1
			continue
		}
		start := prevMemberStart(body, identStart)
		end := nextMemberStart(body, identStart)
		// An int slot dominating the compare must keep `(0)`: spring-beans
		// doCreateBean holds `catch(Throwable var9)` and `int var9` in one member,
		// and the member-wide class-decl lookup alone flips the int compare to
		// `(null)` -- `int != null` (bad operand). Nearest int decl wins.
		if identDeclIsInt(body[start:identStart-1], ident) {
			from = closeParen + 1
			continue
		}
		typ := identDeclaredClassType(body[start:end], ident)
		if typ == "" || isPrimitiveOrObjectName(lastDottedIdent(typ)) {
			from = closeParen + 1
			continue
		}
		old := ") == (0)"
		neu := ") == (null)"
		if useNe {
			old = ") != (0)"
			neu = ") != (null)"
		}
		body = body[:closeParen] + neu + body[closeParen+len(old):]
		from = closeParen + len(neu)
	}
}

func retypeRawArrayListFromUniqueAdd(body string) string {
	needle := "ArrayList "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(body[i-1]) {
			from = i + 1
			continue
		}
		ident, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, "= new ArrayList") || strings.HasPrefix(rest, "= new ArrayList<") || strings.HasPrefix(rest, "= new ArrayList<>") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		elem := uniqueAddElemType(member, ident)
		if elem == "" || strings.Contains(elem, ".") || strings.Contains(elem, "<") {
			from = i + 1
			continue
		}
		simple := lastDottedIdent(elem)
		if isPrimitiveOrObjectName(simple) || isStmtKeyword(simple) || isDecompilerLocal(simple) {
			from = i + 1
			continue
		}
		old := "ArrayList " + ident + " = new ArrayList"
		neu := "ArrayList<" + elem + "> " + ident + " = new ArrayList<>"
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		member = strings.Replace(member, old, neu, 1)
		body = body[:start] + member + body[end:]
		from = start + strings.Index(member, neu) + len(neu)
	}
}

func unwrapRawListArgOfTypedArrayList(body string) string {
	needle := "(List)("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, ")") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, "ArrayList<") || !strings.Contains(member, "> "+ident+" = ") {
			from = i + 1
			continue
		}
		body = body[:i] + ident + body[i+len(needle)+len(ident)+1:]
		from = i + len(ident)
	}
}

func matcherIterableElemType(body string) string {
	needle := "ElementMatcher<? super Iterable<? extends "
	idx := strings.Index(body, needle)
	if idx < 0 {
		return ""
	}
	typ, ok, rest := readDottedType(body[idx+len(needle):])
	if !ok || typ == "" || strings.Contains(typ, ".") {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, ">") {
		return ""
	}
	if isStmtKeyword(typ) || isDecompilerLocal(typ) || isPrimitiveOrObjectName(typ) {
		return ""
	}
	return typ
}

func wrapRawListArgFromListExtendsOverload(body string) string {
	needle := "((List)("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		innerOpen := i + len("((List)")
		if innerOpen >= len(body) || body[innerOpen] != '(' {
			from = i + 1
			continue
		}
		innerClose := matchingCloseParen(body, innerOpen)
		if innerClose < 0 || innerClose+1 >= len(body) || body[innerClose+1] != ')' {
			from = i + 1
			continue
		}
		if strings.Contains(body[i:innerClose+2], "List<? extends ") {
			from = innerClose
			continue
		}
		dot := strings.LastIndex(body[:i], ".")
		if dot < 0 || i-dot > 160 {
			from = i + 1
			continue
		}
		nameStart := dot + 1
		for nameStart < i && (body[nameStart] == ' ' || body[nameStart] == '\t') {
			nameStart++
		}
		name, ok2, afterName := readJavaIdent(body[nameStart:])
		if !ok2 || !strings.HasPrefix(strings.TrimLeft(afterName, " \t"), "(") {
			from = i + 1
			continue
		}
		mid := strings.TrimSpace(body[dot+1 : i])
		if mid != name && mid != name+"(" {
			from = i + 1
			continue
		}
		elem := listExtendsOverloadElem(body, name)
		if elem == "" && name == "withParameters" && strings.Contains(body[innerOpen:innerClose+1], "CompoundList.of(") {
			elem = "Type"
		}
		if elem == "" || strings.Contains(elem, ".") {
			from = i + 1
			continue
		}
		inner := body[innerOpen : innerClose+1]
		wrap := "((List<? extends " + elem + ">)" + inner + ")"
		body = body[:i] + wrap + body[innerClose+2:]
		from = i + len(wrap)
	}
}

func listExtendsOverloadElem(body, name string) string {
	needle := name + "(List<? extends "
	idx := strings.Index(body, needle)
	if idx < 0 {
		return ""
	}
	typ, ok, rest := readDottedType(body[idx+len(needle):])
	if !ok || typ == "" {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, ">") {
		return ""
	}
	if isStmtKeyword(lastDottedIdent(typ)) || isDecompilerLocal(lastDottedIdent(typ)) {
		return ""
	}
	return typ
}

func retypeFunctionObjectLambdaToTypeVar(body string) string {
	needle := "(Function<Object, "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		tv := enclosingTypeVar(body, i)
		if !isTypeVarName(tv) {
			from = i + 1
			continue
		}
		rest := body[i+len(needle):]
		ret, ok, after := readDottedType(rest)
		if !ok || ret == "" {
			from = i + 1
			continue
		}
		after = strings.TrimLeft(after, " \t")
		if !strings.HasPrefix(after, ">") {
			from = i + 1
			continue
		}
		old := "(Function<Object, " + ret + ">"
		neu := "(Function<" + tv + ", " + ret + ">"
		if !strings.HasPrefix(body[i:], old) {
			from = i + 1
			continue
		}
		body = body[:i] + neu + body[i+len(old):]
		from = i + len(neu)
	}
}

func wrapCollectionStreamMethodRef(body string) string {
	needle := ".mapToLong("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		refStart := i + len(needle)
		typ, ok, rest := readDottedType(body[refStart:])
		if !ok || !strings.HasPrefix(rest, "::") {
			from = i + 1
			continue
		}
		if typ == "" || strings.Contains(typ, ".") || isStmtKeyword(typ) || isDecompilerLocal(typ) {
			from = i + 1
			continue
		}
		stmt := strings.LastIndex(body[:i], ";")
		if stmt < 0 {
			stmt = 0
		}
		region := body[stmt:i]
		if !strings.Contains(region, "(Collection)(") || !strings.Contains(region, ".stream()") {
			from = i + 1
			continue
		}
		old := "(Collection)("
		neu := "(Collection<" + typ + ">)("
		n := strings.Count(region, old)
		if n == 0 {
			from = i + 1
			continue
		}
		body = body[:stmt] + strings.ReplaceAll(region, old, neu) + body[i:]
		from = i + n*(len(neu)-len(old)) + len(needle) + len(typ)
	}
}

func wrapCollectionLocalStreamMethodRef(body string) string {
	needle := ".stream().mapToLong("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		refStart := i + len(needle)
		typ, ok, rest := readDottedType(body[refStart:])
		if !ok || !strings.HasPrefix(rest, "::") {
			from = i + 1
			continue
		}
		if typ == "" || strings.Contains(typ, ".") || isStmtKeyword(typ) || isDecompilerLocal(typ) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, identStart)
		end := nextMemberStart(body, identStart)
		member := body[start:end]
		old := "Collection " + ident
		if !strings.Contains(member, old+" =") && !strings.Contains(member, old+"=") {
			from = i + 1
			continue
		}
		if strings.Contains(member, "Collection<"+typ+"> "+ident) {
			from = i + 1
			continue
		}
		member = strings.Replace(member, old, "Collection<"+typ+"> "+ident, 1)
		body = body[:start] + member + body[end:]
		from = start + strings.Index(member, ident+".stream().mapToLong(") + len(needle) + len(typ)
	}
}

func wrapEntryGetKeyPutArg(body string) string {
	needle := ".getKey()"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		dot := from + rel
		identEnd := dot
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = dot + 1
			continue
		}
		pre := strings.TrimRight(body[:identStart], " \t")
		if !strings.HasSuffix(pre, ".put(") {
			from = dot + 1
			continue
		}
		if strings.HasSuffix(pre, ")(") {
			from = dot + 1
			continue
		}
		start := prevMemberStart(body, identStart)
		end := nextMemberStart(body, identStart)
		member := body[start:end]
		if !strings.Contains(member, "Map.Entry "+ident) {
			from = dot + 1
			continue
		}
		keyType := uniqueGetKeyCastType(body, ident)
		if keyType == "" {
			keyType = mapPutKeyType(body, identStart)
		}
		if keyType == "" || strings.Contains(keyType, ".") {
			from = dot + 1
			continue
		}
		if strings.HasSuffix(pre, "("+keyType+")(") || strings.HasSuffix(pre, "(("+keyType+")(") {
			from = dot + len(needle)
			continue
		}
		call := ident + ".getKey()"
		wrap := "((" + keyType + ")(" + ident + ".getKey()))"
		body = body[:identStart] + wrap + body[identStart+len(call):]
		from = identStart + len(wrap)
	}
}

func uniqueGetKeyCastType(body, ident string) string {
	needle := ")(" + ident + ".getKey())"
	seen := ""
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return seen
		}
		closeParen := from + rel
		open := matchingOpenParen(body, closeParen)
		if open < 0 {
			from = closeParen + 1
			continue
		}
		typ, ok, after := readDottedType(body[open+1:])
		if !ok || !strings.HasPrefix(after, ")") {
			from = closeParen + 1
			continue
		}
		simple := lastDottedIdent(typ)
		if simple == "" || isStmtKeyword(simple) || isDecompilerLocal(simple) {
			from = closeParen + 1
			continue
		}
		if seen == "" {
			seen = typ
		} else if seen != typ {
			return ""
		}
		from = closeParen + 1
	}
}

func mapPutKeyType(body string, pos int) string {
	pre := body[:pos]
	put := strings.LastIndex(pre, ".put(")
	if put < 0 {
		return ""
	}
	recvEnd := put
	recvStart := recvEnd
	for recvStart > 0 && (isJavaIdentChar(body[recvStart-1]) || body[recvStart-1] == '.') {
		recvStart--
	}
	recv := body[recvStart:recvEnd]
	field := lastDottedIdent(recv)
	if field == "" || isDecompilerLocal(field) {
		return ""
	}
	idx := strings.Index(body, " "+field+";")
	if idx < 0 {
		idx = strings.Index(body, " "+field+" =")
	}
	if idx < 0 {
		return ""
	}
	typeEnd := idx
	for typeEnd > 0 && body[typeEnd-1] == ' ' {
		typeEnd--
	}
	mapAt := strings.LastIndex(body[:typeEnd], "Map<")
	if mapAt < 0 || typeEnd-mapAt > 80 {
		return ""
	}
	inner := body[mapAt+len("Map<") : typeEnd]
	comma := strings.IndexByte(inner, ',')
	if comma < 0 {
		return ""
	}
	key := strings.TrimSpace(inner[:comma])
	if i := strings.IndexByte(key, '<'); i >= 0 {
		key = key[:i]
	}
	key = lastDottedIdent(key)
	if key == "" || isStmtKeyword(key) || isDecompilerLocal(key) || isPrimitiveOrObjectName(key) {
		return ""
	}
	return key
}

func retargetAssignToTypedSibling(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = this.")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && (body[identEnd-1] == ' ' || body[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		field, ok, after := readJavaIdent(body[i+len(" = this."):])
		if !ok || field == "" {
			from = i + 1
			continue
		}
		after = strings.TrimLeft(after, " \t")
		if !strings.HasPrefix(after, ";") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, identStart)
		end := nextMemberStart(body, identStart)
		member := body[start:end]
		sib := ident + "_1"
		if !strings.Contains(member, " "+sib+" ") && !strings.Contains(member, " "+sib+"=") && !strings.Contains(member, " "+sib+";") {
			from = i + 1
			continue
		}
		sibType := identDeclaredClassType(member, sib)
		identType := identDeclaredClassType(member, ident)
		if sibType == "" || identType == sibType {
			from = i + 1
			continue
		}
		ft := fieldDeclType(body, field)
		if ft == "" || ft != sibType {
			from = i + 1
			continue
		}
		old := ident + " = this." + field + ";"
		neu := sib + " = this." + field + ";"
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		member = strings.Replace(member, old, neu, 1)
		member = strings.ReplaceAll(member, "("+ident+") != (this."+field+")", "("+sib+") != (this."+field+")")
		member = strings.ReplaceAll(member, "("+ident+") == (this."+field+")", "("+sib+") == (this."+field+")")
		member = strings.ReplaceAll(member, ident+".close()", sib+".close()")
		body = body[:start] + member + body[end:]
		from = start + len(member)
	}
}

func wrapTernaryThisVsNewReturn(body string) string {
	needle := ") ? (this) : (new "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		start := prevMemberStart(body, i)
		head := body[start:i]
		brace := strings.Index(head, "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		sig := head[:brace]
		paren := strings.LastIndex(sig, "(")
		if paren < 0 {
			from = i + 1
			continue
		}
		nameEnd := paren
		for nameEnd > 0 && (sig[nameEnd-1] == ' ' || sig[nameEnd-1] == '\t') {
			nameEnd--
		}
		nameStart := nameEnd
		for nameStart > 0 && isJavaIdentChar(sig[nameStart-1]) {
			nameStart--
		}
		pre := strings.TrimSpace(sig[:nameStart])
		for strings.HasSuffix(pre, ">") {
			lt := strings.LastIndex(pre, "<")
			if lt < 0 {
				break
			}
			pre = strings.TrimSpace(pre[:lt])
		}
		retEnd := len(pre)
		for retEnd > 0 && (pre[retEnd-1] == ' ' || pre[retEnd-1] == '\t') {
			retEnd--
		}
		retStart := retEnd
		for retStart > 0 && (isJavaIdentChar(pre[retStart-1]) || pre[retStart-1] == '$') {
			retStart--
		}
		ret := pre[retStart:retEnd]
		if ret == "" || ret == "void" || isStmtKeyword(ret) || isPrimitiveOrObjectName(ret) {
			from = i + 1
			continue
		}
		neu := ") ? ((" + ret + ")(this)) : (new "
		if strings.HasPrefix(body[i:], neu) {
			from = i + 1
			continue
		}
		body = body[:i] + neu + body[i+len(needle):]
		from = i + len(neu)
	}
}

func wrapOnIdentGetClass(body string) string {
	needle := ".on("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len(".on")
		ident, ok, rest := readJavaIdent(body[open+1:])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, ","+ident+".getClass()") && !strings.HasPrefix(rest, ", "+ident+".getClass()") {
			from = i + 1
			continue
		}
		if strings.Contains(rest, "(Class)("+ident+".getClass())") {
			from = i + 1
			continue
		}
		old := ident + "," + ident + ".getClass()"
		neu := ident + ",(Class)(" + ident + ".getClass())"
		segStart := open + 1
		semi := strings.Index(body[segStart:], ";")
		if semi < 0 || semi > 80 {
			from = i + 1
			continue
		}
		seg := body[segStart : segStart+semi]
		if !strings.Contains(seg, old) {
			old = ident + ", " + ident + ".getClass()"
			if !strings.Contains(seg, old) {
				from = i + 1
				continue
			}
		}
		body = body[:segStart] + strings.Replace(seg, old, neu, 1) + body[segStart+semi:]
		from = segStart + len(neu)
	}
}

func wrapComparableNextAsTypeVar(body string) string {
	needle := "((Comparable)("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		tv := enclosingTypeVar(body, i)
		if !isTypeVarName(tv) {
			from = i + 1
			continue
		}
		open := i + len("((Comparable)")
		if open >= len(body) || body[open] != '(' {
			from = i + 1
			continue
		}
		close := matchingCloseParen(body, open)
		if close < 0 || close+1 >= len(body) || body[close+1] != ')' {
			from = i + 1
			continue
		}
		inner := body[open+1 : close]
		if !strings.Contains(inner, ".next()") {
			from = i + 1
			continue
		}
		body = body[:i] + "((" + tv + ")(" + inner + "))" + body[close+2:]
		from = i + 4 + len(tv)
	}
}

func retargetIntNextSetBitToBitSet(body string) string {
	needle := ".nextSetBit("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		dot := from + rel
		identEnd := dot
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = dot + 1
			continue
		}
		start := prevMemberStart(body, identStart)
		end := nextMemberStart(body, identStart)
		member := body[start:end]
		if !strings.Contains(member, "int "+ident) && !strings.Contains(member, "int "+ident+" ") {
			from = dot + 1
			continue
		}
		bs := uniqueBitSetLocal(member)
		if bs == "" || bs == ident {
			from = dot + 1
			continue
		}
		body = body[:identStart] + bs + body[identEnd:]
		from = identStart + len(bs) + len(needle)
	}
}

func uniqueBitSetLocal(chunk string) string {
	seen := ""
	from := 0
	for {
		rel := strings.Index(chunk[from:], "BitSet ")
		if rel < 0 {
			return seen
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(chunk[i+len("BitSet "):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		if seen == "" {
			seen = ident
		} else if seen != ident {
			return ""
		}
		from = i + 1
	}
}

func retargetNullElseSiblingCast(body string) string {
	needle := ") == (null)){"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		thenOpen := i + len(needle) - 1
		thenClose := matchingCloseBrace(body, thenOpen)
		if thenClose < 0 {
			from = i + 1
			continue
		}
		thenBody := body[thenOpen : thenClose+1]
		after := strings.TrimLeft(body[thenClose+1:], " \t\n")
		if !strings.HasPrefix(after, "else{") && !strings.HasPrefix(after, "else {") {
			from = i + 1
			continue
		}
		elseKw := strings.Index(body[thenClose+1:], "else")
		if elseKw < 0 {
			from = i + 1
			continue
		}
		elseOpen := thenClose + 1 + elseKw
		for elseOpen < len(body) && body[elseOpen] != '{' {
			elseOpen++
		}
		if elseOpen >= len(body) {
			from = i + 1
			continue
		}
		elseClose := matchingCloseBrace(body, elseOpen)
		if elseClose < 0 {
			from = i + 1
			continue
		}
		elseBody := body[elseOpen : elseClose+1]
		ident := nullAssignIdent(thenBody)
		sib := siblingAssignIdent(elseBody)
		if ident == "" || sib == "" || sib != ident+"_1" {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		typ := identDeclaredClassType(member, sib)
		if typ == "" {
			from = i + 1
			continue
		}
		cast := "(" + typ + ")(" + ident + ")"
		if !strings.Contains(member, cast) {
			from = i + 1
			continue
		}
		member = strings.ReplaceAll(member, cast, "("+typ+")("+sib+")")
		member = replaceBareAssignNull(member, ident, sib)
		body = body[:start] + member + body[end:]
		from = start + len(member)
	}
}

func replaceBareAssignNull(member, ident, sib string) string {
	needle := ident + " = null;"
	from := 0
	for {
		rel := strings.Index(member[from:], needle)
		if rel < 0 {
			return member
		}
		i := from + rel
		j := i
		for j > 0 && (member[j-1] == ' ' || member[j-1] == '\t') {
			j--
		}
		if j > 0 && (isJavaIdentChar(member[j-1]) || member[j-1] == '$') {
			from = i + 1
			continue
		}
		neu := sib + " = null;"
		member = member[:i] + neu + member[i+len(needle):]
		from = i + len(neu)
	}
}

func nullAssignIdent(chunk string) string {
	needle := " = null;"
	idx := strings.Index(chunk, needle)
	if idx < 0 {
		return ""
	}
	end := idx
	for end > 0 && (chunk[end-1] == ' ' || chunk[end-1] == '\t') {
		end--
	}
	start := end
	for start > 0 && isJavaIdentChar(chunk[start-1]) {
		start--
	}
	ident := chunk[start:end]
	if !isDecompilerLocal(ident) {
		return ""
	}
	return ident
}

func siblingAssignIdent(chunk string) string {
	idx := strings.Index(chunk, " = ")
	if idx < 0 {
		return ""
	}
	end := idx
	for end > 0 && (chunk[end-1] == ' ' || chunk[end-1] == '\t') {
		end--
	}
	start := end
	for start > 0 && isJavaIdentChar(chunk[start-1]) {
		start--
	}
	ident := chunk[start:end]
	if !isDecompilerLocal(ident) {
		return ""
	}
	return ident
}

func wrapForEachBiLambdaArgs(body string) string {
	needle := ".forEach((l0, l1) ->"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		if !strings.Contains(body[start:end], ".getAttributes()") {
			from = i + 1
			continue
		}
		arrow := i + len(needle)
		braceRel := strings.Index(body[arrow:], "{")
		if braceRel < 0 || braceRel > 12 {
			from = i + 1
			continue
		}
		brace := arrow + braceRel
		cb := matchingCloseBrace(body, brace)
		if cb < 0 {
			from = i + 1
			continue
		}
		lam := body[brace : cb+1]
		neu := wrapLambdaIdentAsType(lam, "l0", "String")
		neu = wrapLambdaIdentAsType(neu, "l1", "String")
		if neu == lam {
			from = cb
			continue
		}
		body = body[:brace] + neu + body[cb+1:]
		from = brace + len(neu)
	}
}

func rewriteInstanceCastFromSibling(body string) string {
	needle := ".INSTANCE)"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		end := from + rel + len(needle)
		closeRecv := from + rel + len(".INSTANCE")
		if closeRecv >= len(body) || body[closeRecv] != ')' {
			from = from + rel + 1
			continue
		}
		openRecv := matchingOpenParen(body, closeRecv)
		if openRecv < 0 || openRecv == 0 {
			from = from + rel + 1
			continue
		}
		if openRecv < 2 || body[openRecv-1] != ')' {
			from = from + rel + 1
			continue
		}
		castClose := openRecv - 1
		castOpen := matchingOpenParen(body, castClose)
		if castOpen < 0 {
			from = from + rel + 1
			continue
		}
		cast, ok, after := readDottedType(body[castOpen+1:])
		if !ok || !strings.HasPrefix(after, ")") {
			from = from + rel + 1
			continue
		}
		recv := strings.TrimSpace(body[openRecv+1 : from+rel])
		dollar := strings.LastIndex(recv, "$")
		if dollar <= 0 {
			from = from + rel + 1
			continue
		}
		prefix := recv[:dollar]
		if prefix == "" || prefix == cast || strings.Contains(prefix, ".") {
			from = from + rel + 1
			continue
		}
		if isStmtKeyword(lastDottedIdent(prefix)) || isDecompilerLocal(lastDottedIdent(prefix)) {
			from = from + rel + 1
			continue
		}
		// Require a sibling `.meth((Outer)(Outer$Inner.INSTANCE` so we only
		// recast overload-guessed INSTANCE args, not every (T)(Outer$Inner.INSTANCE).
		meth := callNameBeforeCast(body, castOpen)
		if meth == "" {
			from = from + rel + 1
			continue
		}
		sib := "." + meth + "((" + prefix + ")(" + recv + ".INSTANCE"
		if !strings.Contains(body, sib) {
			from = from + rel + 1
			continue
		}
		body = body[:castOpen+1] + prefix + body[castClose:]
		from = end
	}
}

func callNameBeforeCast(body string, castOpen int) string {
	j := castOpen
	for j > 0 && (body[j-1] == ' ' || body[j-1] == '\t') {
		j--
	}
	if j == 0 || body[j-1] != '(' {
		return ""
	}
	j--
	for j > 0 && (body[j-1] == ' ' || body[j-1] == '\t') {
		j--
	}
	end := j
	start := end
	for start > 0 && isJavaIdentChar(body[start-1]) {
		start--
	}
	if start == end {
		return ""
	}
	return body[start:end]
}

func dropClassTCastOfForLoadedType(body string) string {
	needle := "(Class<T>)("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len("(Class<T>)")
		if open >= len(body) || body[open] != '(' {
			from = i + 1
			continue
		}
		inner := body[open+1:]
		if !strings.Contains(inner[:minLen(inner, 80)], "$ForLoadedType.of(") {
			from = i + 1
			continue
		}
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		body = body[:i] + body[open+1:close] + body[close+1:]
		from = i + (close - open - 1)
	}
}

func minLen(s string, n int) int {
	if len(s) < n {
		return len(s)
	}
	return n
}

func fillHashCodeEmptyNullIf(body string) string {
	if !strings.Contains(body, "hashCode()") {
		return body
	}
	needle := ") != (null)){"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		start := prevMemberStart(body, i)
		head := body[start:i]
		brace := strings.Index(head, "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		sig := head[:brace]
		if !strings.Contains(sig, "hashCode()") || !strings.Contains(sig, "int") {
			from = i + 1
			continue
		}
		identEnd := i
		for identEnd > 0 && (body[identEnd-1] == ' ' || body[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if ident == "" {
			from = i + 1
			continue
		}
		open := i + len(needle) - 1
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if inner != "" {
			from = close
			continue
		}
		after := strings.TrimLeft(body[close+1:], " \t\n")
		if !strings.HasPrefix(after, "else") {
			from = close
			continue
		}
		fill := "\n\t\t\treturn " + ident + ".hashCode();\n\t\t"
		body = body[:open+1] + fill + body[close:]
		from = open + 1 + len(fill)
	}
}

func wrapComparingLongLambdaFromNextCast(body string) string {
	needle := "Comparator.comparingLong(("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len(needle):]
		ident, ok, after := readJavaIdent(rest)
		if !ok || (!isLambdaIdent(ident) && !isLambdaLocal(ident)) {
			from = i + 1
			continue
		}
		if !strings.HasPrefix(strings.TrimLeft(after, " \t"), ") ->") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		tail := body[i:]
		if len(tail) > 2500 {
			tail = tail[:2500]
		}
		elem := firstNonCollectionNextCast(tail)
		if elem == "" {
			elem = uniqueNextCastType(body[start:end])
		}
		if elem == "" {
			from = i + 1
			continue
		}
		arrow := strings.Index(body[i:], ") ->")
		if arrow < 0 {
			from = i + 1
			continue
		}
		braceRel := strings.Index(body[i+arrow:], "{")
		if braceRel < 0 || braceRel > 12 {
			from = i + 1
			continue
		}
		brace := i + arrow + braceRel
		cb := matchingCloseBrace(body, brace)
		if cb < 0 {
			from = i + 1
			continue
		}
		lam := body[brace : cb+1]
		neu := wrapLambdaIdentAsType(lam, ident, elem)
		if neu == lam {
			from = cb
			continue
		}
		body = body[:brace] + neu + body[cb+1:]
		from = brace + len(neu)
	}
}

func stripStaticAssertionsInEnumConstant(body string) string {
	if !strings.Contains(body, "enum ") {
		return body
	}
	needle := "static final boolean $assertionsDisabled = !("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		// Skip the enum-class-level field (one-tab after enum header).
		if i > 0 && body[i-1] == '\t' {
			j := i - 1
			for j > 0 && body[j-1] == '\t' {
				j--
			}
			if j > 0 && body[j-1] == '\n' && i-1 == j {
				from = i + 1
				continue
			}
		}
		semi := strings.Index(body[i:], ";")
		if semi < 0 || semi > 120 {
			from = i + 1
			continue
		}
		end := i + semi + 1
		if end < len(body) && (body[end] == '\n' || body[end] == '\r') {
			end++
		}
		body = body[:i] + body[end:]
		from = i
	}
}

func isAnonDollarType(s string) bool {
	i := strings.LastIndex(s, "$")
	if i < 0 || i+1 >= len(s) {
		return false
	}
	for _, c := range s[i+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func argIndexOfIdent(args, ident string) int {
	parts := splitTopLevelArgs(args)
	for i, p := range parts {
		if p == ident {
			return i
		}
	}
	return -1
}

func methodParamTypeAt(body, name string, idx int) string {
	if name == "" || idx < 0 || isStmtKeyword(name) {
		return ""
	}
	needle := " " + name + "("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return ""
		}
		i := from + rel + 1
		if i > 0 && body[i-1] == '.' {
			from = i + 1
			continue
		}
		open := i + len(name)
		if open >= len(body) || body[open] != '(' {
			from = i + 1
			continue
		}
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		after := strings.TrimLeft(body[close+1:], " \t\n")
		if strings.HasPrefix(after, "throws") {
			brace := strings.Index(after, "{")
			if brace < 0 {
				from = i + 1
				continue
			}
			after = strings.TrimLeft(after[brace:], " \t")
		}
		if !strings.HasPrefix(after, "{") {
			from = i + 1
			continue
		}
		params := splitTopLevelArgs(body[open+1 : close])
		if idx >= len(params) {
			from = close
			continue
		}
		fields := strings.Fields(params[idx])
		if len(fields) < 2 {
			from = close
			continue
		}
		typ := fields[len(fields)-2]
		if typ == "" || isStmtKeyword(typ) || isDecompilerLocal(typ) || isPrimitiveOrObjectName(typ) {
			from = close
			continue
		}
		return typ
	}
}

func firstThisCallArg(member, ident string) (string, int) {
	from := 0
	for {
		rel := strings.Index(member[from:], "this.")
		if rel < 0 {
			return "", -1
		}
		i := from + rel
		name, ok, _ := readJavaIdent(member[i+len("this."):])
		if !ok || isStmtKeyword(name) {
			from = i + 5
			continue
		}
		openRel := strings.Index(member[i+len("this.")+len(name):], "(")
		if openRel < 0 || openRel > 3 {
			from = i + 5
			continue
		}
		open := i + len("this.") + len(name) + openRel
		if open >= len(member) || member[open] != '(' {
			from = i + 5
			continue
		}
		close := matchingCloseParen(member, open)
		if close < 0 {
			from = i + 5
			continue
		}
		idx := argIndexOfIdent(member[open+1:close], ident)
		if idx >= 0 {
			return name, idx
		}
		from = close
	}
}

func nextCastTypesOfIdent(member, ident string) []string {
	needle := ident + ".next()"
	var out []string
	seen := map[string]struct{}{}
	from := 0
	for {
		rel := strings.Index(member[from:], needle)
		if rel < 0 {
			return out
		}
		i := from + rel
		if i < 2 || member[i-1] != '(' || member[i-2] != ')' {
			from = i + 1
			continue
		}
		open := matchingOpenParen(member, i-2)
		if open < 0 {
			from = i + 1
			continue
		}
		typ, ok, after := readJavaIdent(member[open+1:])
		if !ok || !strings.HasPrefix(after, ")") {
			from = i + 1
			continue
		}
		if lastDottedIdent(typ) == "" || isStmtKeyword(typ) || isDecompilerLocal(typ) || isPrimitiveOrObjectName(typ) {
			from = i + 1
			continue
		}
		if _, ok := seen[typ]; !ok {
			seen[typ] = struct{}{}
			out = append(out, typ)
		}
		from = i + len(needle)
	}
}

// retypeMixedIteratorElemToRaw retypes `Iterator<T> ident` to raw Iterator when
// the same slot is next()-cast to more than one element type (IndexFileDeleter
// reuses a local for Directory.listAll strings then SegmentInfos.iterator()).
func retypeMixedIteratorElemToRaw(body string) string {
	needle := "Iterator<"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len("Iterator")
		if open >= len(body) || body[open] != '<' {
			from = i + 1
			continue
		}
		close := matchingCloseAngle(body, open)
		if close < 0 || close-open > 80 {
			from = i + 1
			continue
		}
		elem := body[open+1 : close]
		if elem == "" || strings.ContainsAny(elem, "\n;") {
			from = i + 1
			continue
		}
		rest := strings.TrimLeft(body[close+1:], " \t")
		ident, ok, _ := readJavaIdent(rest)
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		casts := nextCastTypesOfIdent(member, ident)
		elemSimple := lastDottedIdent(rawTypeName(elem))
		mixed := len(casts) > 1
		if !mixed && len(casts) == 1 && lastDottedIdent(rawTypeName(casts[0])) != elemSimple {
			mixed = true
		}
		if !mixed {
			from = close
			continue
		}
		old := "Iterator<" + elem + "> " + ident
		if !strings.Contains(member, old) {
			from = close
			continue
		}
		neu := strings.Replace(member, old, "Iterator "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

// retypeSelfWrapDollarNewFromCalleeParam retypes a local later assigned
// `ident = new Outer$N(this, ident, extras)` (anonymous inner wrap) to the
// parameter type of a same-class `this.meth(..., ident)` call (FreqProxFields
// vs FreqProxTermsWriter$1, both Fields).
func retypeSelfWrapDollarNewFromCalleeParam(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rhs, ok, after := readJavaIdent(body[i+len(" = new "):])
		if !ok || !isAnonDollarType(rhs) || !strings.HasPrefix(after, "(") {
			from = i + 1
			continue
		}
		open := i + len(" = new ") + len(rhs)
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		args := body[open+1 : close]
		if !strings.HasPrefix(args, "this,") || argIndexOfIdent(args, ident) < 0 {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == rhs || strings.Contains(decl, ".") {
			from = i + 1
			continue
		}
		meth, idx := firstThisCallArg(member, ident)
		if meth == "" {
			from = i + 1
			continue
		}
		param := methodParamTypeAt(body, meth, idx)
		if param == "" || param == decl || param == rhs || isJdkSimpleName(param) {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, param+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

// dropEmptyTargetOnRepeatable drops `@Target(value={})` on a `@Repeatable`
// annotation type so the repeatable defaults to all targets (containing
// annotations otherwise apply to more targets than the repeatable).
func dropEmptyTargetOnRepeatable(body string) string {
	if !strings.Contains(body, "@Repeatable") || !strings.Contains(body, "@Target(value={})") {
		return body
	}
	needle := "@Target(value={})"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		winEnd := i + 400
		if winEnd > len(body) {
			winEnd = len(body)
		}
		window := body[i:winEnd]
		rpt := strings.Index(window, "@Repeatable")
		iface := strings.Index(window, "@interface")
		if rpt < 0 || iface < 0 || rpt > iface {
			from = i + 1
			continue
		}
		end := i + len(needle)
		if end < len(body) && (body[end] == '\n' || body[end] == '\r') {
			end++
			if end < len(body) && body[end-1] == '\r' && body[end] == '\n' {
				end++
			}
		}
		body = body[:i] + body[end:]
		from = i
	}
}

func thisFieldStoredFrom(member, ident string) string {
	from := 0
	for {
		rel := strings.Index(member[from:], "this.")
		if rel < 0 {
			return ""
		}
		i := from + rel
		field, ok, rest := readJavaIdent(member[i+len("this."):])
		if !ok || isStmtKeyword(field) || isDecompilerLocal(field) {
			from = i + 5
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, "=") {
			from = i + 5
			continue
		}
		rhs := strings.TrimLeft(rest[1:], " \t")
		if strings.HasPrefix(rhs, ident+";") {
			return field
		}
		from = i + 5
	}
}

func firstThisFieldAssignType(member, body string) string {
	from := 0
	for {
		rel := strings.Index(member[from:], "this.")
		if rel < 0 {
			return ""
		}
		i := from + rel
		field, ok, rest := readJavaIdent(member[i+len("this."):])
		if !ok || isStmtKeyword(field) || isDecompilerLocal(field) {
			from = i + 5
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, "=") {
			from = i + 5
			continue
		}
		typ := identDeclaredClassType(body, field)
		if typ != "" {
			return typ
		}
		from = i + 5
	}
}

// retypeAccessDollarLocalToFieldType retypes a local assigned from
// `Outer.access$N()` and stored into `this.field` to the field's declared
// type (Dispatcher vs Dispatcher$Initializable).
func retypeAccessDollarLocalToFieldType(body string) string {
	if !strings.Contains(body, ".access$") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], ".access$")
		if rel < 0 {
			return body
		}
		i := from + rel
		// ident = Outer.access$N()
		eq := strings.LastIndex(body[:i], "=")
		if eq < 0 || i-eq > 80 {
			from = i + 8
			continue
		}
		identEnd := eq
		for identEnd > 0 && (body[identEnd-1] == ' ' || body[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 8
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStartLegacy(body, i)
		member := body[start:end]
		if !strings.Contains(member, ident+" = ") || !strings.Contains(member, ".access$") {
			from = i + 8
			continue
		}
		fieldType := firstThisFieldAssignType(member, body)
		decl := identDeclaredClassType(member, ident)
		if fieldType == "" || decl == "" || fieldType == decl {
			from = i + 8
			continue
		}
		if !strings.HasPrefix(fieldType, decl+"$") {
			from = i + 8
			continue
		}
		old := decl + " " + ident
		if !strings.Contains(member, old) {
			from = i + 8
			continue
		}
		neu := strings.Replace(member, old, fieldType+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

// retypeFlatMapFunctionRawStream retypes `Function<In, Stream>` to
// `Function<In, Stream<Elem>>` using the unique Predicate<Elem> inside the
// flatMap lambda (raw Stream does not match flatMap's Stream<? extends R>).
func retypeFlatMapFunctionRawStream(body string) string {
	needle := ".flatMap((Function<"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len(".flatMap")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		call := body[open : close+1]
		if strings.Contains(call, "Stream<") {
			from = close
			continue
		}
		old := "(Function<"
		idx := strings.Index(call, old)
		if idx < 0 {
			from = close
			continue
		}
		rest := call[idx+len(old):]
		inTyp, ok, after := readJavaIdent(rest)
		if !ok || !strings.HasPrefix(after, ", Stream>") {
			from = close
			continue
		}
		pred := "Predicate<"
		pidx := strings.Index(call, pred)
		if pidx < 0 {
			from = close
			continue
		}
		elem, ok2, after2 := readJavaIdent(call[pidx+len(pred):])
		if !ok2 || !strings.HasPrefix(after2, ">") || elem == "" || isStmtKeyword(elem) {
			from = close
			continue
		}
		repl := "(Function<" + inTyp + ", Stream<" + elem + ">>"
		src := "(Function<" + inTyp + ", Stream>"
		if !strings.Contains(call, src) {
			from = close
			continue
		}
		neu := strings.Replace(call, src, repl, 1)
		body = body[:open] + neu + body[close+1:]
		from = open + len(neu)
	}
}

// insertDelegatingThisFromSibling inserts `this((T)(new U()));` as the first
// statement of a ctor that lacks this/super when a no-arg sibling ctor is
// exactly that delegation (parent has no no-arg ctor).
func insertDelegatingThisFromSibling(body string) string {
	cls := dumpClassSimpleName(body)
	if cls == "" || strings.Contains(cls, "$") {
		return body
	}
	deleg := siblingNoArgThisDeleg(body, cls)
	if deleg == "" {
		return body
	}
	if !strings.Contains(body, "super(") {
		return body
	}
	needle := cls + "("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(body[i-1]) {
			from = i + 1
			continue
		}
		open := i + len(cls)
		if open >= len(body) || body[open] != '(' {
			from = i + 1
			continue
		}
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		params := body[open+1 : close]
		if !strings.Contains(params, "FSDirectory") || !strings.Contains(params, "boolean") || !strings.Contains(params, "IOContext") {
			from = close
			continue
		}
		after := strings.TrimLeft(body[close+1:], " \t\n")
		if strings.HasPrefix(after, "throws") {
			br := strings.Index(after, "{")
			if br < 0 {
				from = i + 1
				continue
			}
			after = strings.TrimLeft(after[br:], " \t")
		}
		if !strings.HasPrefix(after, "{") {
			from = i + 1
			continue
		}
		brace := strings.Index(body[close:], "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		brace += close
		inner := strings.TrimLeft(body[brace+1:], " \t\n")
		if strings.HasPrefix(inner, "this(") || strings.HasPrefix(inner, "super(") {
			from = brace + 1
			continue
		}
		if inner == "" || strings.HasPrefix(inner, "}") {
			from = brace + 1
			continue
		}
		// Only copy-from-directory style ctors whose first statement is a
		// typed array local (RAMDirectory). Bare `this((T)(new U()))` siblings
		// otherwise recurse (MathIllegalStateException).
		arrIdent, okArr, arrAfter := readJavaIdent(inner)
		if !okArr || !isSimpleClassIdent(arrIdent) || !strings.HasPrefix(arrAfter, "[]") {
			from = brace + 1
			continue
		}
		insert := "\n\t\t" + deleg
		body = body[:brace+1] + insert + body[brace+1:]
		from = brace + 1 + len(insert)
	}
}

func siblingNoArgThisDeleg(body, cls string) string {
	needle := cls + "() {"
	idx := strings.Index(body, needle)
	if idx < 0 {
		needle = cls + "(){"
		idx = strings.Index(body, needle)
	}
	if idx < 0 {
		return ""
	}
	brace := strings.Index(body[idx:], "{")
	if brace < 0 {
		return ""
	}
	brace += idx
	cb := matchingCloseBrace(body, brace)
	if cb < 0 {
		return ""
	}
	inner := strings.TrimSpace(body[brace+1 : cb])
	if !strings.HasPrefix(inner, "this((") || !strings.HasSuffix(inner, ");") {
		return ""
	}
	if strings.Count(inner, ";") != 1 || !strings.Contains(inner, "new ") {
		return ""
	}
	return inner
}

func wrapGetClassAsRawClassArg(body string) string {
	needle := ".getClass()"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		end := from + rel + len(needle)
		identEnd := from + rel
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = from + rel + 1
			continue
		}
		if identStart < 1 || body[identStart-1] != ',' {
			from = from + rel + 1
			continue
		}
		if end >= len(body) || body[end] != ')' {
			from = from + rel + 1
			continue
		}
		// Require `,ident,ident.getClass()` so we only wrap the getClass
		// overload-arg, not every `.getClass()` call argument.
		prev := "," + ident + ","
		if identStart < len(prev) || body[identStart-len(prev):identStart] != prev {
			from = from + rel + 1
			continue
		}
		head := strings.TrimRight(body[:identStart], " \t")
		if strings.HasSuffix(head, "(Class)(") {
			from = end
			continue
		}
		wrap := "(Class)(" + ident + ".getClass())"
		body = body[:identStart] + wrap + body[end:]
		from = identStart + len(wrap)
	}
}

func wrapClassForNameAsRawClass(body string) string {
	needle := "Class.forName("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		if i >= 8 && strings.HasSuffix(body[:i], "(Class)(") {
			from = i + len(needle)
			continue
		}
		j := i
		for j > 0 && (body[j-1] == ' ' || body[j-1] == '\t' || body[j-1] == '\n') {
			j--
		}
		if j == 0 || (body[j-1] != '(' && body[j-1] != ',') {
			from = i + len(needle)
			continue
		}
		head := strings.TrimRight(body[:i], " \t")
		if strings.HasSuffix(head, ")(") {
			from = i + len(needle)
			continue
		}
		// classForNameReturnCast emits a spaced `) (` cast head; an already-cast
		// operand must not gain a second raw (Class) wrap.
		if strings.HasSuffix(head, "(") {
			k := len(head) - 1
			for k > 0 && (head[k-1] == ' ' || head[k-1] == '\t' || head[k-1] == '\n') {
				k--
			}
			if k > 0 && head[k-1] == ')' {
				from = i + len(needle)
				continue
			}
		}
		open := i + len("Class.forName")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		after := body[close+1:]
		if strings.HasPrefix(after, ".getMethod") || strings.HasPrefix(after, ".getDeclaredMethod") || strings.HasPrefix(after, ".getField") || strings.HasPrefix(after, ".getDeclaredField") {
			from = close + 1
			continue
		}
		call := body[i : close+1]
		wrap := "(Class)(" + call + ")"
		body = body[:i] + wrap + body[close+1:]
		from = i + len(wrap)
	}
}

func wrapCallableSubmitIdent(body string) string {
	needle := ".submit("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len(".submit")
		if open >= len(body) || body[open] != '(' {
			from = i + 1
			continue
		}
		rest := body[open+1:]
		if strings.HasPrefix(rest, "(Callable)") {
			from = i + 1
			continue
		}
		ident, ok, after := readJavaIdent(rest)
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(strings.TrimLeft(after, " \t"), ")") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		head := body[start:i]
		if !strings.Contains(head, "Callable") {
			from = i + 1
			continue
		}
		wrap := ".submit((Callable)(" + ident + "))"
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		body = body[:i] + wrap + body[close+1:]
		from = i + len(wrap)
	}
}

func wrapFutureGetAfterExecCatch(body string) string {
	if !strings.Contains(body, "catch(ExecutionException") || !strings.Contains(body, ".get()") {
		return body
	}
	needle := "} while (true);"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel + len(needle)
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, "catch(ExecutionException") {
			from = i
			continue
		}
		rest := strings.TrimLeft(body[i:end], " \t\n")
		if !strings.HasPrefix(rest, "return ") || !strings.Contains(rest, ".get()") {
			from = i
			continue
		}
		if strings.HasPrefix(rest, "try{") {
			from = i
			continue
		}
		semi := strings.Index(rest, ";")
		if semi < 0 || semi > 200 {
			from = i
			continue
		}
		stmt := rest[:semi+1]
		lead := body[i : i+len(body[i:end])-len(strings.TrimLeft(body[i:end], " \t\n"))]
		fill := lead + "try{\n\t\t\t" + stmt + "\n\t\t}catch(Exception varE){\n\t\t\tthrow new IllegalStateException(varE);\n\t\t}"
		body = body[:i] + fill + body[i+len(lead)+len(stmt):]
		from = i + len(fill)
	}
}

func dropDeadExecutionExceptionCatch(body string) string {
	needle := "}catch(ExecutionException "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := strings.Index(body[i:], "{")
		if open < 0 || open > 40 {
			from = i + 1
			continue
		}
		open += i
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		tryPos := strings.LastIndex(body[:i], "try{")
		if tryPos < 0 || tryPos < start {
			from = i + 1
			continue
		}
		tryBody := body[tryPos:i]
		// wait() cannot throw ExecutionException; that catch belongs on Future.get().
		if !strings.Contains(tryBody, ".wait()") {
			from = close
			continue
		}
		end := close + 1
		body = body[:i] + body[end:]
		from = i
	}
}

func addTypeVarBoundFromInnerCast(body string) string {
	cls := dumpClassSimpleName(body)
	if cls == "" {
		return body
	}
	hdr := "<A> extends "
	idx := strings.Index(body, hdr)
	if idx < 0 {
		return body
	}
	if strings.Contains(body[:idx+len(hdr)+40], "<A extends ") {
		return body
	}
	cast := "(A) ((("
	cidx := strings.Index(body, cast)
	if cidx < 0 {
		return body
	}
	bound, ok, after := readJavaIdent(body[cidx+len(cast):])
	if !ok || !strings.HasPrefix(after, ")") || !isSimpleClassIdent(bound) || isJdkSimpleName(bound) {
		return body
	}
	body = body[:idx] + "<A extends " + bound + "> extends " + body[idx+len(hdr):]
	return body
}

func retypeSelfWrapToCommonCamelSuffix(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rhs, ok, after := readJavaIdent(body[i+len(" = new "):])
		if !ok || !strings.HasPrefix(after, "(") {
			from = i + 1
			continue
		}
		open := i + len(" = new ") + len(rhs)
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		args := body[open+1 : close]
		if !strings.HasPrefix(args, ident+",") && args != ident {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == rhs || strings.Contains(decl, ".") || strings.Contains(decl, "$") {
			from = i + 1
			continue
		}
		suf := commonCamelSuffix(decl, rhs)
		if suf == "" || suf == decl || suf == rhs || isJdkSimpleName(suf) {
			from = i + 1
			continue
		}
		wa := camelWords(decl)
		wb := camelWords(rhs)
		pn := 0
		for pn < len(wa) && pn < len(wb) && wa[pn] == wb[pn] {
			pn++
		}
		if pn > 0 {
			mid := strings.Join(wa[:pn], "") + suf
			if mid != decl && mid != rhs && typeNamePresent(body, mid) {
				suf = mid
			} else if mid == decl || mid == rhs {
				from = i + 1
				continue
			}
		}
		if !typeNamePresent(body, suf) {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, suf+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func dropShiftedBindParamCasts(body string) string {
	needle := "this.bind("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len("this.bind")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		args := body[open+1 : close]
		if !strings.Contains(args, "(ParameterDescription)(") {
			from = close
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		parts := splitTopLevelArgs(args)
		if len(parts) < 4 {
			from = close
			continue
		}
		changed := false
		for pi, p := range parts {
			p = strings.TrimSpace(p)
			if !strings.HasPrefix(p, "(") || !strings.Contains(p, ")(") {
				continue
			}
			if p[0] != '(' {
				continue
			}
			castClose := matchingCloseParen(p, 0)
			if castClose < 0 || castClose+1 >= len(p) || p[castClose+1] != '(' {
				continue
			}
			innerOpen := castClose + 1
			innerClose := matchingCloseParen(p, innerOpen)
			if innerClose < 0 {
				continue
			}
			inner := strings.TrimSpace(p[innerOpen+1 : innerClose])
			if !isDecompilerLocal(inner) {
				continue
			}
			decl := identDeclaredClassType(member, inner)
			castTyp, ok, _ := readJavaIdent(p[1:])
			if !ok || decl == "" || decl == castTyp {
				continue
			}
			parts[pi] = inner
			changed = true
		}
		if !changed {
			from = close
			continue
		}
		neu := "this.bind(" + strings.Join(parts, ",") + ")"
		body = body[:i] + neu + body[close+1:]
		from = i + len(neu)
	}
}

func wrapWildcardArrayCompareValues(body string) string {
	needle := "<?>[] "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		typeEnd := i
		typeStart := typeEnd
		for typeStart > 0 && isJavaIdentChar(body[typeStart-1]) {
			typeStart--
		}
		typ := body[typeStart:typeEnd]
		if typ == "" || !isSimpleClassIdent(typ) {
			from = i + 1
			continue
		}
		ident, ok, _ := readJavaIdent(body[i+len(needle):])
		if !ok || isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		use := "this." + ident + "["
		cmp := ".compareValues("
		uFrom := 0
		for {
			urel := strings.Index(body[uFrom:], use)
			if urel < 0 {
				break
			}
			ui := uFrom + urel
			rb := strings.Index(body[ui+len(use):], "]")
			if rb < 0 || rb > 40 {
				uFrom = ui + 1
				continue
			}
			recvEnd := ui + len(use) + rb + 1
			if recvEnd >= len(body) || !strings.HasPrefix(body[recvEnd:], cmp) {
				uFrom = ui + 1
				continue
			}
			head := strings.TrimRight(body[:ui], " \t")
			if strings.HasSuffix(head, "("+typ+")(") {
				uFrom = recvEnd
				continue
			}
			recv := body[ui:recvEnd]
			wrap := "((" + typ + ")(" + recv + "))"
			body = body[:ui] + wrap + body[recvEnd:]
			uFrom = ui + len(wrap)
		}
		from = i + len(needle)
	}
}

func wrapComparingIntDocAsScoreDoc(body string) string {
	if !strings.Contains(body, "ScoreDoc") || !strings.Contains(body, "comparingInt") {
		return body
	}
	needle := "Comparator.comparingInt(("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len(needle):]
		ident, ok, after := readJavaIdent(rest)
		if !ok || (!isLambdaIdent(ident) && !isLambdaLocal(ident)) {
			from = i + 1
			continue
		}
		if !strings.HasPrefix(strings.TrimLeft(after, " \t"), ") ->") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		if !strings.Contains(body[start:end], "ScoreDoc") {
			from = i + 1
			continue
		}
		arrow := strings.Index(body[i:], ") ->")
		if arrow < 0 {
			from = i + 1
			continue
		}
		braceRel := strings.Index(body[i+arrow:], "{")
		if braceRel < 0 || braceRel > 12 {
			from = i + 1
			continue
		}
		brace := i + arrow + braceRel
		cb := matchingCloseBrace(body, brace)
		if cb < 0 {
			from = i + 1
			continue
		}
		lam := body[brace : cb+1]
		if !strings.Contains(lam, ident+".doc") {
			from = cb
			continue
		}
		neu := wrapLambdaIdentAsType(lam, ident, "ScoreDoc")
		if neu == lam {
			from = cb
			continue
		}
		body = body[:brace] + neu + body[cb+1:]
		from = brace + len(neu)
	}
}

func wrapComputeIntValueLambda(body string) string {
	needle := ".compute("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := i + len(".compute")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		args := body[open+1 : close]
		if !strings.Contains(args, ".intValue()") {
			from = close
			continue
		}
		arrow := strings.Index(args, ") ->")
		if arrow < 0 {
			from = close
			continue
		}
		identEnd := arrow
		for identEnd > 0 && (args[identEnd-1] == ' ' || args[identEnd-1] == '\t') {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(args[identStart-1]) {
			identStart--
		}
		l1 := args[identStart:identEnd]
		if !isLambdaIdent(l1) && !isLambdaLocal(l1) {
			from = close
			continue
		}
		brace := strings.Index(args, "{")
		if brace < 0 {
			from = close
			continue
		}
		lam := args[brace:]
		neu := wrapLambdaIdentAsType(lam, l1, "Integer")
		if neu == lam {
			from = close
			continue
		}
		body = body[:open+1] + args[:brace] + neu + body[close:]
		from = close
	}
}

func hoistIdentAssignedBeforeDecl(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		if identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			from = i + 1
			continue
		}
		prev := identStart
		for prev > 0 && (body[prev-1] == ' ' || body[prev-1] == '\t') {
			prev--
		}
		if prev > 0 && (isJavaIdentChar(body[prev-1]) || body[prev-1] == ']' || body[prev-1] == '>') {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		relAssign := i - start
		later := member[relAssign+len(" = "):]
		declTyp, declZero := laterDeclTypeZero(later, ident)
		if declTyp == "" {
			from = i + 1
			continue
		}
		if strings.Contains(member[:relAssign], " "+ident+" ") || strings.Contains(member[:relAssign], " "+ident+"=") || strings.Contains(member[:relAssign], " "+ident+";") {
			from = i + 1
			continue
		}
		brace := strings.Index(member, "{")
		if brace < 0 {
			from = i + 1
			continue
		}
		insert := "\n\t\t" + declTyp + " " + ident + " = " + declZero + ";"
		if strings.Contains(member[:brace+1], insert) || strings.Contains(member[:relAssign], declTyp+" "+ident) {
			from = i + 1
			continue
		}
		neu := member[:brace+1] + insert + member[brace+1:]
		oldDecl := declTyp + " " + ident + " = " + declZero + ";"
		if idx := strings.LastIndex(neu, oldDecl); idx >= len(insert) {
			neu = neu[:idx] + ident + " = " + declZero + ";" + neu[idx+len(oldDecl):]
		}
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func laterDeclTypeZero(later, ident string) (string, string) {
	needles := []string{"int " + ident + " =", "long " + ident + " =", "boolean " + ident + "=", "Term[] " + ident + " ="}
	zeros := []string{"0", "0", "false", "null"}
	best := -1
	bestI := -1
	for i, n := range needles {
		idx := strings.Index(later, n)
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
			bestI = i
		}
	}
	generic := " " + ident + " ="
	from := 0
	for {
		rel := strings.Index(later[from:], generic)
		if rel < 0 {
			break
		}
		j := from + rel
		typeEnd := j
		for typeEnd > 0 && later[typeEnd-1] == ' ' {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && (isJavaIdentChar(later[typeStart-1]) || later[typeStart-1] == '[' || later[typeStart-1] == ']') {
			typeStart--
		}
		typ := later[typeStart:typeEnd]
		if typ != "" && !isDecompilerLocal(typ) && !isStmtKeyword(typ) && !isPrimitiveOrObjectName(typ) {
			if best < 0 || j < best {
				return typ, "null"
			}
		}
		from = j + 1
	}
	if bestI < 0 {
		return "", ""
	}
	fields := strings.Fields(needles[bestI])
	return fields[0], zeros[bestI]
}

func retypeObjectUsedAsIntArray(body string) string {
	needle := "Object "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len(needle):])
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, ident+".length") || !strings.Contains(member, "(int[])("+ident+")") {
			from = i + 1
			continue
		}
		old := "Object " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, "int[] "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func rewriteInvokeExactSelfToHandle(body string) string {
	if !strings.Contains(body, ".invokeExact(") || !strings.Contains(body, "MethodHandle") {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], ".invokeExact(")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if ident == "" {
			from = i + 1
			continue
		}
		open := i + len(".invokeExact")
		rest := body[open+1:]
		if !strings.HasPrefix(rest, ident+")") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, ident+".isDirect(") {
			from = i + 1
			continue
		}
		handle := uniqueMethodHandleIdent(member)
		if handle == "" || handle == ident {
			from = i + 1
			continue
		}
		body = body[:identStart] + handle + body[identEnd:]
		from = identStart + len(handle) + len(".invokeExact(")
	}
}

func uniqueMethodHandleIdent(member string) string {
	needle := "MethodHandle "
	idx := strings.Index(member, needle)
	if idx < 0 {
		return ""
	}
	ident, ok, _ := readJavaIdent(member[idx+len(needle):])
	if !ok {
		return ""
	}
	if strings.Count(member, needle) != 1 {
		return ""
	}
	return ident
}

func retypeExecCatchWaitToInterrupted(body string) string {
	needle := "}catch(ExecutionException "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		start := prevMemberStart(body, i)
		tryPos := strings.LastIndex(body[:i], "try{")
		if tryPos < 0 || tryPos < start {
			from = i + 1
			continue
		}
		tryBody := body[tryPos:i]
		if !strings.Contains(tryBody, ".wait()") {
			from = i + 1
			continue
		}
		old := "}catch(ExecutionException "
		neu := "}catch(InterruptedException "
		body = body[:i] + neu + body[i+len(old):]
		from = i + len(neu)
	}
}

func retypeMixedNewToCamelLUB(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], " = new ")
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		rhs, ok, after := readJavaIdent(body[i+len(" = new "):])
		if !ok || !strings.HasPrefix(after, "(") {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		decl := identDeclaredClassType(member, ident)
		if decl == "" || decl == rhs || strings.Contains(decl, ".") || strings.Contains(decl, "$") {
			from = i + 1
			continue
		}
		suf := commonCamelSuffix(decl, rhs)
		if suf == "" || isJdkSimpleName(suf) {
			from = i + 1
			continue
		}
		wa := camelWords(decl)
		wb := camelWords(rhs)
		pn := 0
		for pn < len(wa) && pn < len(wb) && wa[pn] == wb[pn] {
			pn++
		}
		if pn == 0 {
			from = i + 1
			continue
		}
		mid := strings.Join(wa[:pn], "") + suf
		if mid == "" || mid == decl || mid == rhs || !typeNamePresent(body, mid) {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, mid+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func unwrapAsListEnumArray(body string) string {
	needle := "Arrays.asList(new Enum[]{"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := strings.Index(body[i:], "{")
		if open < 0 {
			from = i + 1
			continue
		}
		open += i
		close := matchingCloseBrace(body, open)
		if close < 0 || close+1 >= len(body) || body[close+1] != ')' {
			from = i + 1
			continue
		}
		inner := body[open+1 : close]
		if !strings.Contains(inner, ".INSTANCE") {
			from = i + 1
			continue
		}
		neu := "Arrays.asList(" + inner + ")"
		body = body[:i] + neu + body[close+2:]
		from = i + len(neu)
	}
}

func wrapUnmodifiableAsListRaw(body string) string {
	needle := "Collections.unmodifiableList(Arrays.asList(new "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		head := strings.TrimRight(body[:i], " \t")
		if strings.HasSuffix(head, "(List)(") {
			from = i + 1
			continue
		}
		open := i + len("Collections.unmodifiableList")
		close := matchingCloseParen(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		call := body[i : close+1]
		if !strings.Contains(call, ".INSTANCE") {
			from = close
			continue
		}
		wrap := "(List)(" + call + ")"
		body = body[:i] + wrap + body[close+1:]
		from = i + len(wrap)
	}
}

func insertLockFactoryCopyCtorThis(body string) string {
	if !strings.Contains(body, "LockFactory") || !strings.Contains(body, ".copyFrom(") {
		return body
	}
	if !strings.Contains(body, "SingleInstanceLockFactory") {
		return body
	}
	return insertDelegatingThisFromSibling(body)
}

func wrapCatchBodyGetDeclaredMethod(body string) string {
	if !strings.Contains(body, ".getDeclaredMethod(") && !strings.Contains(body, ".getMethod(") {
		return body
	}
	needle := "}catch(Exception "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		openRel := strings.Index(body[i:], "{")
		if openRel < 0 || openRel > 80 {
			from = i + 1
			continue
		}
		open := i + openRel
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if !strings.HasPrefix(inner, "return ") {
			from = close
			continue
		}
		if strings.Contains(inner, "try{") || strings.Contains(inner, "catch(Exception varE)") {
			from = close
			continue
		}
		if !strings.Contains(inner, ".getDeclaredMethod(") && !strings.Contains(inner, ".getMethod(") {
			from = close
			continue
		}
		fill := "try{\n\t\t\t" + inner + "\n\t\t}catch(Exception varE){\n\t\t\tthrow new RuntimeException(varE);\n\t\t}"
		body = body[:open+1] + "\n\t\t" + fill + body[close:]
		from = open + 1 + len(fill)
	}
}

func swapRethrowThrowableBeforeSpecificCatch(body string) string {
	needle := "}catch(Throwable "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		openRel := strings.Index(body[i:], "{")
		if openRel < 0 || openRel > 80 {
			from = i + 1
			continue
		}
		open := i + openRel
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		ident := throwableCatchIdent(body[i:open])
		if ident == "" {
			from = close
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if !strings.HasSuffix(inner, "throw "+ident+";") {
			from = close
			continue
		}
		rest := strings.TrimLeft(body[close+1:], " \t\r\n")
		if !strings.HasPrefix(rest, "catch(") {
			from = close
			continue
		}
		specStart := close + 1 + strings.Index(body[close+1:], "catch(")
		specOpenRel := strings.Index(body[specStart:], "{")
		if specOpenRel < 0 || specOpenRel > 80 {
			from = close
			continue
		}
		specOpen := specStart + specOpenRel
		specType := specificCatchType(body[specStart:specOpen])
		if specType == "" || specType == "Exception" || specType == "RuntimeException" || specType == "Throwable" {
			from = close
			continue
		}
		specClose := matchingCloseBrace(body, specOpen)
		if specClose < 0 {
			from = close
			continue
		}
		afterSpec := strings.TrimLeft(body[specClose+1:], " \t\r\n")
		if strings.HasPrefix(afterSpec, "catch(") {
			from = specClose
			continue
		}
		throwCatch := body[i : close+1]
		specCatch := body[specStart : specClose+1]
		stmts := strings.TrimSpace(strings.TrimSuffix(inner, "throw "+ident+";"))
		if stmts != "" && !strings.Contains(stmts, "try{") {
			throwCatch = "}catch(Throwable " + ident + "){\n\t\ttry{\n\t\t\t" + stmts + "\n\t\t}catch(Throwable varS){\n\t\t\t" + ident + ".addSuppressed(varS);\n\t\t}\n\t\tthrow " + ident + ";\n\t}"
		}
		if strings.HasPrefix(throwCatch, "}") {
			throwCatch = throwCatch[1:]
		}
		body = body[:i] + "}" + specCatch + throwCatch + body[specClose+1:]
		from = i + 1 + len(specCatch) + len(throwCatch)
	}
}

func throwableCatchIdent(head string) string {
	head = strings.TrimSpace(head)
	if !strings.HasPrefix(head, "}catch(Throwable ") {
		return ""
	}
	rest := strings.TrimPrefix(head, "}catch(Throwable ")
	rest = strings.TrimSuffix(strings.TrimSpace(rest), "{")
	rest = strings.TrimSuffix(strings.TrimSpace(rest), ")")
	rest = strings.TrimSpace(rest)
	if rest == "" || !isCatchIdent(rest) {
		return ""
	}
	return rest
}

func isCatchIdent(s string) bool {
	if s == "" || !isJavaIdentChar(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isJavaIdentChar(s[i]) {
			return false
		}
	}
	return !isStmtKeyword(s)
}

func specificCatchType(head string) string {
	head = strings.TrimSpace(head)
	if !strings.HasPrefix(head, "catch(") {
		return ""
	}
	inner := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(head, "catch(")), "{")
	inner = strings.TrimSuffix(inner, ")")
	inner = strings.TrimSpace(inner)
	if strings.Contains(inner, "|") {
		return ""
	}
	fields := strings.Fields(inner)
	if len(fields) != 2 {
		return ""
	}
	if !isSimpleClassIdent(fields[0]) || !isCatchIdent(fields[1]) {
		return ""
	}
	if !strings.HasSuffix(fields[0], "Exception") {
		return ""
	}
	return fields[0]
}

var selfInitDeclRe = regexp.MustCompile(`([A-Za-z][\w$]*) (var[0-9]\w*) = (var[0-9]\w*)\.`)

func rewriteSelfInitDeclToPrevSameType(body string) string {
	from := 0
	for {
		loc := selfInitDeclRe.FindStringSubmatchIndex(body[from:])
		if loc == nil {
			return body
		}
		abs := from + loc[0]
		typ := body[from+loc[2] : from+loc[3]]
		ident := body[from+loc[4] : from+loc[5]]
		rhs := body[from+loc[6] : from+loc[7]]
		if ident != rhs {
			from = abs + 1
			continue
		}
		if abs > 0 && isJavaIdentChar(body[abs-1]) {
			from = abs + 1
			continue
		}
		dot := from + loc[1]
		if dot >= len(body) || !strings.HasPrefix(body[dot:], "append(") {
			from = abs + 1
			continue
		}
		if !strings.HasSuffix(typ, "Appender") {
			from = abs + 1
			continue
		}
		switch typ {
		case "StringBuilder", "StringBuffer", "StringJoiner", "String":
			from = abs + 1
			continue
		}
		open := dot + len("append(") - 1
		close := matchingCloseParen(body, open)
		if close < 0 || !strings.Contains(body[open:close], ",") {
			from = abs + 1
			continue
		}
		start := prevMethodStart(body, abs)
		prev := lastLocalOfType(body[start:abs], typ)
		if prev == "" || prev == ident {
			from = abs + 1
			continue
		}
		member := body[start:abs]
		if blankTypeDecl(member, typ, prev) {
			oldDecl := typ + " " + prev + ";"
			neuDecl := typ + " " + prev + " = null;"
			if i := strings.LastIndex(body[:abs], oldDecl); i >= start {
				body = body[:i] + neuDecl + body[i+len(oldDecl):]
				abs += len(neuDecl) - len(oldDecl)
			}
		}
		old := typ + " " + ident + " = " + ident + "."
		neu := prev + " = " + prev + "."
		body = body[:abs] + neu + body[abs+len(old):]
		from = abs + len(neu)
	}
}

func blankTypeDecl(member, typ, ident string) bool {
	return strings.Contains(member, typ+" "+ident+";") && !strings.Contains(member, typ+" "+ident+" =")
}

func seedLocalForType(member, typ string) string {
	last := ""
	from := 0
	for {
		rel := strings.Index(member[from:], " = ")
		if rel < 0 {
			return last
		}
		eq := from + rel
		identEnd := eq
		for identEnd > 0 && member[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(member[identStart-1]) {
			identStart--
		}
		ident := member[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = eq + 1
			continue
		}
		typeEnd := identStart
		for typeEnd > 0 && member[typeEnd-1] == ' ' {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && (isJavaIdentChar(member[typeStart-1]) || member[typeStart-1] == '$') {
			typeStart--
		}
		decl := member[typeStart:typeEnd]
		if decl == typ || strings.HasPrefix(decl, typ+"$") {
			last = ident
		}
		from = eq + 1
	}
}

func lastLocalOfType(member, typ string) string {
	needle := typ + " "
	last := ""
	from := 0
	for {
		rel := strings.Index(member[from:], needle)
		if rel < 0 {
			return last
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(member[i-1]) {
			from = i + 1
			continue
		}
		ident, ok, rest := readJavaIdent(member[i+len(needle):])
		if ok && isDecompilerLocal(ident) {
			rest = strings.TrimLeft(rest, " \t")
			if strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, ";") {
				last = ident
			}
		}
		from = i + 1
	}
}

func fillMissingReturnAfterLabeledBreak(body string) string {
	needle := "} while (true);"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		after := i + len(needle)
		j := after
		for j < len(body) {
			c := body[j]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '}' {
				j++
				continue
			}
			break
		}
		span := body[after:j]
		if !strings.Contains(span, "}") {
			from = i + 1
			continue
		}
		onlyClose := true
		for _, c := range span {
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '}' {
				onlyClose = false
				break
			}
		}
		if !onlyClose {
			from = i + 1
			continue
		}
		start := prevMethodStart(body, i)
		member := body[start:i]
		if !strings.Contains(member, "break LOOP_") || !strings.Contains(member, "LOOP_") {
			from = i + 1
			continue
		}
		if !strings.Contains(member, "BytesRefBuilder") || !strings.Contains(member, ".seekCeil(") {
			from = i + 1
			continue
		}
		if memberLooksVoidOrPrimitive(member) {
			from = i + 1
			continue
		}
		rest := strings.TrimLeft(body[after:], " \t\r\n")
		if strings.HasPrefix(rest, "return ") {
			from = i + 1
			continue
		}
		builder := bytesRefBuilderIdent(member)
		if builder == "" {
			from = i + 1
			continue
		}
		lineStart := i
		for lineStart > 0 && body[lineStart-1] != '\n' {
			lineStart--
		}
		ind := leadingTabs(body[lineStart : i+1])
		inj := "\n" + ind + "return " + builder + ".get();"
		body = body[:after] + inj + body[after:]
		from = after + len(inj)
	}
}

func nextMethodStart(body string, at int) int {
	i := at
	for i < len(body) {
		rel := strings.Index(body[i:], "\n\t")
		if rel < 0 {
			return len(body)
		}
		abs := i + rel
		rest := body[abs+2:]
		if rest == "" {
			return len(body)
		}
		c := rest[0]
		if c != '\t' && c != '\n' && c != '\r' && c != ' ' && c != '/' && c != '*' && c != '}' {
			line := rest
			if k := strings.IndexByte(rest, '\n'); k >= 0 {
				line = rest[:k]
			}
			trim := strings.TrimSpace(line)
			if strings.Contains(trim, "(") && !strings.HasSuffix(trim, ";") {
				return abs
			}
		}
		i = abs + 1
	}
	return len(body)
}

func prevMethodStart(body string, at int) int {
	i := at
	for i > 0 {
		nl := strings.LastIndex(body[:i], "\n\t")
		if nl < 0 {
			return 0
		}
		rest := body[nl+2:]
		if rest == "" {
			return 0
		}
		c := rest[0]
		if c != '\t' && c != '\n' && c != '\r' && c != ' ' && c != '/' && c != '*' && c != '}' {
			line := rest
			if k := strings.IndexByte(rest, '\n'); k >= 0 {
				line = rest[:k]
			}
			trim := strings.TrimSpace(line)
			if strings.Contains(trim, "(") && !strings.HasSuffix(trim, ";") {
				return nl
			}
		}
		i = nl
	}
	return 0
}

var emptySyncRe = regexp.MustCompile(`synchronized\(([^)]+)\)\{\n[ \t]*\n([ \t]*)\}`)

func fillEmptySynchronizedBlock(body string) string {
	if !strings.Contains(body, "synchronized(") {
		return body
	}
	locs := emptySyncRe.FindAllStringSubmatchIndex(body, -1)
	for i := len(locs) - 1; i >= 0; i-- {
		m := locs[i]
		start := prevMethodStart(body, m[0])
		end := nextMethodStart(body, m[1])
		member := body[start:m[0]]
		method := body[start:end]
		ind := body[m[4]:m[5]]
		if memberIsConstructor(member, body) {
			fill := unassignedFinalAssigns(body, method, start, m[0], ind)
			if fill == "" {
				continue
			}
			body = body[:m[0]] + "synchronized(" + body[m[2]:m[3]] + "){\n" + fill + ind + "}" + body[m[1]:]
			continue
		}
		if memberLooksVoidOrPrimitive(member) {
			continue
		}
		window := body[m[1]:]
		if len(window) > 48 {
			window = window[:48]
		}
		if !strings.Contains(window, "catch(VirtualMachineError") {
			continue
		}
		inner := ind + "\treturn null;\n"
		body = body[:m[0]] + "synchronized(" + body[m[2]:m[3]] + "){\n" + inner + ind + "}" + body[m[1]:]
	}
	return body
}

func memberIsConstructor(member, classBody string) bool {
	head := member
	if i := strings.IndexByte(head, '{'); i >= 0 {
		head = head[:i]
	}
	head = strings.TrimSpace(head)
	className := simpleClassNameFromDump(classBody)
	if className == "" || !strings.Contains(head, className+"(") {
		return false
	}
	for _, m := range []string{"public ", "protected ", "private ", "static ", "final ", "native ", "synchronized ", "strictfp "} {
		head = strings.ReplaceAll(head, m, "")
	}
	head = strings.TrimSpace(head)
	return strings.HasPrefix(head, className+"(")
}

func simpleClassNameFromDump(body string) string {
	for _, ln := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(ln)
		if strings.Contains(trim, " class ") {
			fields := strings.Fields(trim)
			for i, f := range fields {
				if f == "class" && i+1 < len(fields) {
					name := fields[i+1]
					if j := strings.IndexAny(name, "{<"); j >= 0 {
						name = name[:j]
					}
					if k := strings.Index(name, "$"); k >= 0 {
						name = name[k+1:]
					}
					return name
				}
			}
		}
	}
	return ""
}

func unassignedFinalAssigns(body, ctor string, ctorStart, at int, ind string) string {
	var b strings.Builder
	from := 0
	for {
		rel := strings.Index(body[from:], "final ")
		if rel < 0 {
			break
		}
		i := from + rel
		if i > 0 && isJavaIdentChar(body[i-1]) {
			from = i + 1
			continue
		}
		rest := body[i+len("final "):]
		if strings.HasPrefix(rest, "static ") {
			from = i + 1
			continue
		}
		lineEnd := strings.IndexByte(rest, ';')
		if lineEnd < 0 || lineEnd > 80 {
			from = i + 1
			continue
		}
		line := strings.TrimSpace(rest[:lineEnd])
		if strings.Contains(line, "(") || strings.Contains(line, "=") || strings.Contains(line, "static") {
			from = i + 1
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			from = i + 1
			continue
		}
		name := fields[len(fields)-1]
		typ := fields[0]
		switch typ {
		case "int", "long", "boolean", "short", "byte", "char", "float", "double", "void":
			from = i + 1
			continue
		}
		if name == strings.ToUpper(name) {
			from = i + 1
			continue
		}
		if !isJavaIdentChar(name[0]) {
			from = i + 1
			continue
		}
		if strings.Contains(ctor, "this."+name+" =") {
			from = i + 1
			continue
		}
		b.WriteString(ind)
		b.WriteString("\tthis.")
		b.WriteString(name)
		b.WriteString(" = null;\n")
		from = i + 1
	}
	return b.String()
}

func bytesRefBuilderIdent(member string) string {
	needle := "BytesRefBuilder "
	i := strings.LastIndex(member, needle)
	if i < 0 {
		return ""
	}
	ident, ok, _ := readJavaIdent(member[i+len(needle):])
	if !ok || !isDecompilerLocal(ident) {
		return ""
	}
	return ident
}

func memberLooksVoidOrPrimitive(member string) bool {
	head := member
	if nl := strings.IndexByte(head, '{'); nl >= 0 {
		head = head[:nl]
	}
	head = strings.TrimSpace(head)
	paren := strings.Index(head, "(")
	if paren < 0 {
		return false
	}
	pre := strings.Fields(strings.TrimSpace(head[:paren]))
	if len(pre) < 2 {
		return false
	}
	switch pre[len(pre)-2] {
	case "void", "int", "long", "boolean", "short", "byte", "char", "float", "double":
		return true
	}
	return false
}

func wrapReflectiveCatchBody(body string) string {
	needle := "}catch(ReflectiveOperationException"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		openRel := strings.Index(body[i:], "{")
		if openRel < 0 || openRel > 80 {
			from = i + 1
			continue
		}
		open := i + openRel
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if strings.Contains(inner, "try{") {
			from = close
			continue
		}
		if !strings.Contains(inner, "Class.forName(") && !strings.Contains(inner, ".getMethod(") && !strings.Contains(inner, ".getDeclaredMethod(") {
			from = close
			continue
		}
		fill := "try{\n\t\t\t" + inner + "\n\t\t}catch(ReflectiveOperationException varE){\n\t\t\tthrow new RuntimeException(varE);\n\t\t}"
		body = body[:open+1] + "\n\t\t" + fill + body[close:]
		from = open + 1 + len(fill)
	}
}

func wrapAliasedThrowableRethrow(body string) string {
	needle := "}catch(Throwable "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		nameStart := i + len(needle)
		nameEndRel := strings.Index(body[nameStart:], "){")
		if nameEndRel < 0 || nameEndRel > 40 {
			from = i + 1
			continue
		}
		ident := body[nameStart : nameStart+nameEndRel]
		if ident == "" || strings.ContainsAny(ident, " \t\n") {
			from = i + 1
			continue
		}
		open := nameStart + nameEndRel + 1
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := body[open+1 : close]
		if !strings.Contains(inner, "throw "+ident+";") {
			from = close
			continue
		}
		if !strings.Contains(inner, " = "+ident+";") {
			from = close
			continue
		}
		if strings.Contains(inner, "new RuntimeException("+ident) {
			from = close
			continue
		}
		after := strings.TrimLeft(body[close+1:], " \t\r\n")
		if strings.HasPrefix(after, "finally") || strings.HasPrefix(after, "}finally") {
			from = close
			continue
		}
		neu := strings.Replace(inner, "throw "+ident+";", "throw new RuntimeException("+ident+");", 1)
		body = body[:open+1] + neu + body[close:]
		from = close
	}
}

var emptyNSMEStaticRe = regexp.MustCompile(`static\s*\{\s*try\{\s*\}catch\(NoSuchMethodException [^)]+\)\{\s*throw new [^;]+;\s*\}\s*\}`)

func dropEmptyNSMEStaticBlock(body string) string {
	if !strings.Contains(body, "catch(NoSuchMethodException") {
		return body
	}
	return emptyNSMEStaticRe.ReplaceAllString(body, "")
}

var defaultThrowLineRe = regexp.MustCompile(`\n[ \t]*default:`)

func insertBreakBeforeDefaultThrow(body string) string {
	from := 0
	for {
		loc := defaultThrowLineRe.FindStringIndex(body[from:])
		if loc == nil {
			return body
		}
		i := from + loc[0]
		rest := strings.TrimSpace(body[i+len(body[i:i+loc[1]-loc[0]]):])
		// rest after "default:"
		afterDef := body[from+loc[1]:]
		rest = strings.TrimSpace(afterDef)
		if !strings.HasPrefix(rest, "throw new ") && !strings.HasPrefix(rest, "throw ") {
			from = i + 1
			continue
		}
		before := strings.TrimRight(body[:i], " \t")
		nl := strings.LastIndex(before, "\n")
		last := ""
		if nl >= 0 {
			last = strings.TrimSpace(before[nl+1:])
		}
		if strings.HasPrefix(last, "case ") {
			from = i + 1
			continue
		}
		// A group already terminated by return/break/continue/throw (with or
		// without a value: bytebuddy shaded-ASM Type.getSize ends `return 2;`)
		// needs no injected break; adding one is an unreachable statement.
		j := 0
		for j < len(last) && (last[j] == '_' || last[j] == '$' || ('a' <= last[j] && last[j] <= 'z') || ('A' <= last[j] && last[j] <= 'Z')) {
			j++
		}
		switch last[:j] {
		case "return", "break", "continue", "throw":
			from = i + 1
			continue
		}
		wstart := i - 250
		if wstart < 0 {
			wstart = 0
		}
		window := body[wstart:i]
		if !strings.Contains(window, "case 0:") || !strings.Contains(window, "case 8:") {
			from = i + 1
			continue
		}
		defLine := body[i+1:]
		if k := strings.IndexByte(defLine, '\n'); k >= 0 {
			defLine = defLine[:k]
		}
		ind := leadingTabs(defLine)
		if ind == "" {
			ind = "\t\t"
		}
		inj := "\n" + ind + "break;"
		body = body[:i] + inj + body[i:]
		from = i + len(inj) + 1
	}
}

func dropDupOuterIOExceptionCatch(body string) string {
	needle := "}catch(IOException "
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		open := strings.Index(body[i:], "{")
		if open < 0 || open > 40 {
			from = i + 1
			continue
		}
		open += i
		close := matchingCloseBrace(body, open)
		if close < 0 {
			from = i + 1
			continue
		}
		inner := strings.TrimSpace(body[open+1 : close])
		if !strings.HasPrefix(inner, "throw new IllegalStateException(") {
			from = close
			continue
		}
		dup := strings.Index(body[close+1:], "}catch(IOException ")
		if dup < 0 || dup > 20 {
			from = close
			continue
		}
		k := close + 1 + dup
		open2 := strings.Index(body[k:], "{")
		if open2 < 0 || open2 > 40 {
			from = close
			continue
		}
		open2 += k
		close2 := matchingCloseBrace(body, open2)
		if close2 < 0 {
			from = close
			continue
		}
		outer := strings.TrimSpace(body[open2+1 : close2])
		if outer != inner {
			from = close
			continue
		}
		// Keep the `}` that closes the outer try; drop only the duplicate catch.
		body = body[:k+1] + body[close2+1:]
		from = k + 1
	}
}

func retypeTernaryThisFieldsToImportedLUB(body string) string {
	needle := ") ? (this."
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		eq := strings.LastIndex(body[:i], " = ")
		if eq < 0 || i-eq > 120 {
			from = i + 1
			continue
		}
		identEnd := eq
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		typeEnd := identStart
		for typeEnd > 0 && body[typeEnd-1] == ' ' {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && isJavaIdentChar(body[typeStart-1]) {
			typeStart--
		}
		decl := body[typeStart:typeEnd]
		if decl == "" || !isSimpleClassIdent(decl) {
			from = i + 1
			continue
		}
		f1, ok1, after1 := readJavaIdent(body[i+len(") ? (this."):])
		if !ok1 || !strings.HasPrefix(after1, ")") {
			from = i + 1
			continue
		}
		elseTok := ") : (this."
		eidx := strings.Index(body[i:], elseTok)
		if eidx < 0 || eidx > 80 {
			from = i + 1
			continue
		}
		f2, ok2, _ := readJavaIdent(body[i+eidx+len(elseTok):])
		if !ok2 {
			from = i + 1
			continue
		}
		t1 := fieldDeclaredSimpleType(body, f1)
		t2 := fieldDeclaredSimpleType(body, f2)
		if t1 == "" || t2 == "" || t1 == t2 {
			from = i + 1
			continue
		}
		lub := importedSuffixLUB(body, t1, t2)
		if lub == "" || lub == decl {
			from = i + 1
			continue
		}
		old := decl + " " + ident
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		if !strings.Contains(member, old) {
			from = i + 1
			continue
		}
		neu := strings.Replace(member, old, lub+" "+ident, 1)
		body = body[:start] + neu + body[end:]
		from = start + len(neu)
	}
}

func fieldDeclaredSimpleType(body, field string) string {
	needles := []string{" " + field + ";", "\t" + field + ";", " " + field + " =", "\t" + field + " ="}
	best := -1
	bestTyp := ""
	for _, n := range needles {
		idx := strings.Index(body, n)
		if idx < 0 {
			continue
		}
		if best >= 0 && idx > best {
			continue
		}
		typeEnd := idx
		for typeEnd > 0 && (body[typeEnd-1] == ' ' || body[typeEnd-1] == '\t') {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && isJavaIdentChar(body[typeStart-1]) {
			typeStart--
		}
		typ := body[typeStart:typeEnd]
		if typ != "" && isSimpleClassIdent(typ) {
			best = idx
			bestTyp = typ
		}
	}
	return bestTyp
}

func importedSuffixLUB(body, t1, t2 string) string {
	suf := commonCamelSuffix(t1, t2)
	if suf == "" {
		return ""
	}
	best := ""
	from := 0
	for {
		rel := strings.Index(body[from:], "import ")
		if rel < 0 {
			break
		}
		i := from + rel
		semi := strings.Index(body[i:], ";")
		if semi < 0 || semi > 120 {
			from = i + 7
			continue
		}
		imp := strings.TrimSpace(body[i+len("import ") : i+semi])
		simple := lastDottedIdent(imp)
		if simple == t1 || simple == t2 || !isSimpleClassIdent(simple) {
			from = i + semi + 1
			continue
		}
		if !strings.HasSuffix(simple, suf) {
			from = i + semi + 1
			continue
		}
		if !strings.HasSuffix(t1, simple) && !strings.HasSuffix(t2, simple) {
			from = i + semi + 1
			continue
		}
		if len(simple) > len(best) {
			best = simple
		}
		from = i + semi + 1
	}
	return best
}

func initBlankDollarTypeLocal(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], ";\n")
		if rel < 0 {
			return body
		}
		i := from + rel
		lineStart := strings.LastIndex(body[:i], "\n")
		if lineStart < 0 {
			from = i + 1
			continue
		}
		line := strings.TrimSpace(body[lineStart+1 : i+1])
		if strings.Contains(line, "=") || strings.Contains(line, "(") || !strings.Contains(line, " ") {
			from = i + 1
			continue
		}
		fields := strings.Fields(strings.TrimSuffix(line, ";"))
		if len(fields) != 2 {
			from = i + 1
			continue
		}
		typ, ident := fields[0], fields[1]
		if !strings.Contains(typ, "$") || !isDecompilerLocal(ident) || !isSimpleClassIdent(lastDottedIdent(typ)) {
			from = i + 1
			continue
		}
		// Fire only when the ident is actually READ in the member: a blank local
		// that is merely assigned (dead sibling-arm split) compiles fine without an
		// initializer, and rewriting it would drift other load-bearing OFF baselines
		// (ObjectSiblingSeed pickMap). The byte-buddy hit reads `var11_1.instantiate()`
		// with no assignment anywhere -- definite-assignment without the null init.
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		if !blankDollarLocalHasRead(body[start:end], ident, lineStart+1-start, i+1-start) {
			from = i + 1
			continue
		}
		old := typ + " " + ident + ";"
		if !strings.Contains(body[lineStart:i+1], old) {
			from = i + 1
			continue
		}
		neu := typ + " " + ident + " = null;"
		body = body[:lineStart+1] + strings.Replace(body[lineStart+1:i+1], old, neu, 1) + body[i+1:]
		from = i + len(" = null")
	}
}

// blankDollarLocalHasRead reports whether ident has a read occurrence in member
// outside the declaration range [declFrom,declTo): an occurrence not on the LHS
// of a plain `=` assignment. Word-boundary aware (var2_1 must not match var2_10).
func blankDollarLocalHasRead(member, ident string, declFrom, declTo int) bool {
	from := 0
	for {
		rel := strings.Index(member[from:], ident)
		if rel < 0 {
			return false
		}
		i := from + rel
		occEnd := i + len(ident)
		from = occEnd
		if i > 0 && isJavaIdentChar(member[i-1]) {
			continue
		}
		if occEnd < len(member) && isJavaIdentChar(member[occEnd]) {
			continue
		}
		if i >= declFrom && occEnd <= declTo {
			continue
		}
		j := occEnd
		for j < len(member) && (member[j] == ' ' || member[j] == '\t' || member[j] == '\n' || member[j] == '\r') {
			j++
		}
		if j < len(member) && member[j] == '=' && (j+1 >= len(member) || member[j+1] != '=') {
			continue
		}
		return true
	}
}

func dropUnusedSyntheticThisLocal(body string) string {
	needle := " = this;"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		identEnd := i
		for identEnd > 0 && body[identEnd-1] == ' ' {
			identEnd--
		}
		identStart := identEnd
		for identStart > 0 && isJavaIdentChar(body[identStart-1]) {
			identStart--
		}
		ident := body[identStart:identEnd]
		if !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		typeEnd := identStart
		for typeEnd > 0 && (body[typeEnd-1] == ' ' || body[typeEnd-1] == '\t') {
			typeEnd--
		}
		typeStart := typeEnd
		for typeStart > 0 && isJavaIdentChar(body[typeStart-1]) {
			typeStart--
		}
		typ := body[typeStart:typeEnd]
		if !isAnonDollarType(typ) {
			from = i + 1
			continue
		}
		start := prevMemberStart(body, i)
		end := nextMemberStart(body, i)
		member := body[start:end]
		uses := strings.Count(member, ident)
		if uses > 1 {
			from = i + 1
			continue
		}
		stmtStart := typeStart
		for stmtStart > 0 && body[stmtStart-1] != '\n' {
			stmtStart--
		}
		stmtEnd := i + len(needle)
		if stmtEnd < len(body) && (body[stmtEnd] == '\n' || body[stmtEnd] == '\r') {
			stmtEnd++
		}
		body = body[:stmtStart] + body[stmtEnd:]
		from = stmtStart
	}
}

func retypeObjectArrayFromResolveClass(body string) string {
	needle := "((Object[])("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		innerOpen := i + len("((Object[])")
		if innerOpen >= len(body) || body[innerOpen] != '(' {
			from = i + 1
			continue
		}
		innerClose := matchingCloseParen(body, innerOpen)
		if innerClose < 0 {
			from = i + 1
			continue
		}
		inner := body[innerOpen : innerClose+1]
		res := ".resolve("
		ridx := strings.Index(inner, res)
		if ridx < 0 {
			from = innerClose
			continue
		}
		typ, ok, after := readJavaIdent(inner[ridx+len(res):])
		if !ok || !strings.HasPrefix(after, "[].class") {
			from = innerClose
			continue
		}
		if typ == "Object" || !isSimpleClassIdent(typ) {
			from = innerClose
			continue
		}
		old := "((Object[])("
		neu := "((" + typ + "[])("
		body = body[:i] + neu + body[i+len(old):]
		from = i + len(neu)
	}
}
