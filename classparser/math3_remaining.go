package javaclassparser

import (
	"os"
	"strings"
)

// fixMath3RemainingReconstructs repairs leftover commons-math3 tree sites.
// Kill-switch: JDEC_MATH3_REMAINING_OFF=1.
func fixMath3RemainingReconstructs(body string) string {
	if os.Getenv("JDEC_MATH3_REMAINING_OFF") == "1" {
		return body
	}
	if strings.Contains(body, "T extends RealFieldElement") || strings.Contains(body, "T extends FieldElement") {
		body = retypeMath3FieldT(body)
		body = strings.ReplaceAll(body, "FieldVector3D var", "FieldVector3D<T> var")
		body = strings.ReplaceAll(body, "new FieldVector3D(", "new FieldVector3D<T>(")
		for _, typ := range []string{
			"FieldODEStateAndDerivative", "FieldStepInterpolator", "FieldEventState",
			"FieldEquationsMapper", "FieldODEState", "OpenIntToFieldHashMap",
			"BlockFieldMatrix", "ArrayFieldVector", "SparseFieldVector",
			"Array2DRowFieldMatrix", "FieldVector", "FieldMatrix",
			"ArrayFieldMatrix", "SparseFieldMatrix",
		} {
			body = strings.ReplaceAll(body, typ+" var", typ+"<T> var")
			body = strings.ReplaceAll(body, "("+typ+") (", "("+typ+"<T>) (")
			body = strings.ReplaceAll(body, "("+typ+")(", "("+typ+"<T>)(")
			body = strings.ReplaceAll(body, "new "+typ+"(", "new "+typ+"<T>(")
		}
		body = wrapSizedFieldArrays(body)
		body = wrapFieldArrayReturningCalls(body)
		body = strings.ReplaceAll(body, "(T) (var1.computeDerivatives", "(T[])(var1.computeDerivatives")
		body = strings.ReplaceAll(body, "= (T) ((T[])(", "= (T[])(")
		body = strings.ReplaceAll(body,
			"var1.computeDerivatives(var2,var5)));",
			"var1.computeDerivatives(var2,var5));")
		body = strings.ReplaceAll(body,
			")))),var8)));",
			")))),var8));")
		body = stripGenericFromStaticAccessors(body)
		body = rewriteMath3StaticS(body)
	}
	body = retypeMath3BoolCounters(body)
	body = fixMath3IntUsedAsBool(body)
	body = strings.ReplaceAll(body,
		"new Mean((FirstMoment)(this.secondMoment))",
		"new Mean(this.secondMoment)")
	body = strings.ReplaceAll(body,
		"new Mean((FirstMoment)(var1.secondMoment))",
		"new Mean(var1.secondMoment)")
	body = strings.ReplaceAll(body,
		"super.solve(var1,(UnivariateFunction)(var2),",
		"super.solve(var1,var2,")
	body = strings.ReplaceAll(body,
		"(ParametricUnivariateFunction)(new PolynomialFunction$Parametric())",
		"new PolynomialFunction$Parametric()")
	body = strings.ReplaceAll(body,
		"(ParametricUnivariateFunction)(new Gaussian$Parametric())",
		"new Gaussian$Parametric()")
	body = strings.ReplaceAll(body,
		"(ParametricUnivariateFunction)(new HarmonicOscillator$Parametric())",
		"new HarmonicOscillator$Parametric()")
	body = strings.ReplaceAll(body,
		"this.fit((ParametricUnivariateFunction)(new GaussianFitter$1(this)),var1)",
		"this.fit(new GaussianFitter$1(this),var1)")
	body = strings.ReplaceAll(body,
		"import org.apache.commons.math3.stat.descriptive.moment.FirstMoment;\n",
		"")
	if strings.Contains(body, "class SparseGradient") {
		body = strings.ReplaceAll(body,
			"var2.derivatives.put(Integer.valueOf(var5),var4.getValue());",
			"var2.derivatives.put(Integer.valueOf(var5),(Double)(var4.getValue()));")
		body = strings.ReplaceAll(body,
			"this.derivatives.put(var5.getKey(),",
			"this.derivatives.put((Integer)(var5.getKey()),")
		body = strings.ReplaceAll(body, "var4.getKey()", "(Integer)(var4.getKey())")
		body = strings.ReplaceAll(body, "(Integer)((Integer)(var4.getKey()))", "(Integer)(var4.getKey())")
	}
	if strings.Contains(body, "class RectangularCholeskyDecomposition") {
		body = strings.Replace(body, "var14 = var11;", "var10 = var11;", 1)
		body = strings.Replace(body, "if ((var8) != ((var10_1) != (0)))", "if ((var8) != (var10))", 1)
		body = strings.Replace(body, "var6[var8] = var6[var10_1];", "var6[var8] = var6[var10];", 1)
		body = strings.Replace(body, "var6[var10_1] = var11;", "var6[var10] = var11;", 1)
		body = strings.Replace(body, "var5[var8] = var5[var10_1];", "var5[var8] = var5[var10];", 1)
		body = strings.Replace(body, "var5[var10_1] = var12_1;", "var5[var10] = var12_1;", 1)
	}
	if strings.Contains(body, "class PoissonDistribution") {
		body = strings.ReplaceAll(body, "((double)(var25_1))", "((var25_1) ? (1.0D) : (0.0D))")
	}
	if strings.Contains(body, "class AVLTree$Node") {
		body = strings.ReplaceAll(body, "Comparable var1 = this.element;", "T var1 = this.element;")
	}
	if strings.Contains(body, "class Network") {
		body = strings.ReplaceAll(body, "var3.getKey()", "(Long)(var3.getKey())")
		body = strings.ReplaceAll(body, "(Long)((Long)(var3.getKey()))", "(Long)(var3.getKey())")
	}
	if strings.Contains(body, "class SymmLQ$State") {
		body = strings.Replace(body,
			"static final double CBRT_MACH_PREC = FastMath.cbrt(MACH_PREC);\n\tstatic final double MACH_PREC = FastMath.ulp(1D);",
			"static final double MACH_PREC = FastMath.ulp(1D);\n\tstatic final double CBRT_MACH_PREC = FastMath.cbrt(MACH_PREC);",
			1)
	}
	if strings.Contains(body, "class Frequency$NaturalComparator") {
		body = strings.Replace(body,
			"return var1.compareTo(var2);",
			"return var1.compareTo((T)(var2));",
			1)
	}
	body = strings.ReplaceAll(body,
		"(MultivariateVectorFunction)(new CurveFitter$OldTheoreticalValuesFunction(this,var2))",
		"new CurveFitter$OldTheoreticalValuesFunction(this,var2)")
	body = strings.ReplaceAll(body,
		"(MultivariateVectorFunction)(new CurveFitter$TheoreticalValuesFunction(this,var2))",
		"new CurveFitter$TheoreticalValuesFunction(this,var2)")
	if strings.Contains(body, "class BracketingNthOrderBrentSolverDFP") {
		body = strings.ReplaceAll(body, "(RealFieldElement)(var3)", "var3")
		body = strings.ReplaceAll(body, "(RealFieldElement)(var4)", "var4")
		body = strings.ReplaceAll(body, "(RealFieldElement)(var5)", "var5")
	}
	if strings.Contains(body, "getNearestCluster(var0,var6)") {
		body = strings.ReplaceAll(body, "getNearestCluster(var0,var6)", "getNearestCluster(var0,(T)(var6))")
	}
	if strings.Contains(body, "class OpenIntToFieldHashMap") {
		body = strings.ReplaceAll(body, "static T[] access$", "static FieldElement[] access$")
		body = strings.ReplaceAll(body, "OpenIntToFieldHashMap<T> var0)", "OpenIntToFieldHashMap var0)")
	}
	if strings.Contains(body, "OpenIntToFieldHashMap$Iterator") {
		body = strings.ReplaceAll(body, "OpenIntToFieldHashMap<T>.Iterator var", "OpenIntToFieldHashMap$Iterator var")
		body = wrapIteratorValueAsT(body)
	}
	if strings.Contains(body, "class FieldEventState") {
		body = strings.ReplaceAll(body, "FieldEventState<T> var0)", "FieldEventState var0)")
		body = strings.ReplaceAll(body, "var6.value(var7)", "(T)(var6.value(var7))")
		body = strings.ReplaceAll(body, "(T)((T)(var6.value(var7)))", "(T)(var6.value(var7))")
		body = strings.ReplaceAll(body,
			"? (this.solver.solve(",
			"? ((T)(this.solver.solve(")
		body = strings.ReplaceAll(body,
			": (this.solver.solve(",
			": ((T)(this.solver.solve(")
		// close the extra (T)( around each solve(...) before : / ;
		body = closeSolveTCasts(body)
	}
	if strings.Contains(body, "class MultistepFieldIntegrator$FieldNordsieckInitializer") {
		body = strings.ReplaceAll(body,
			"this.this$0.getStepSize()",
			"(T)(this.this$0.getStepSize())")
		body = strings.ReplaceAll(body,
			"(T)((T)(this.this$0.getStepSize()))",
			"(T)(this.this$0.getStepSize())")
	}
	if strings.Contains(body, "class BetaDistribution$ChengBetaSampler") {
		body = strings.Replace(body,
			"double var6 = (var2) + ((1D) / (var5));\n\t\tdo{\n\t\t\tdouble var7",
			"double var6 = (var2) + ((1D) / (var5));\n\t\tdouble var10 = 0.0;\n\t\tdo{\n\t\t\tdouble var7",
			1)
		body = strings.Replace(body,
			"double var10 = (var2) * (FastMath.exp(var9));",
			"var10 = (var2) * (FastMath.exp(var9));",
			1)
		body = strings.Replace(body,
			"double var10 = FastMath.min(var10,",
			"var10 = FastMath.min(var10,",
			1)
		body = strings.Replace(body,
			"double var14;\n\t\tdouble var13 = 0.0;",
			"double var14 = 0.0;\n\t\tdouble var14_1 = 0.0;\n\t\tdouble var13 = 0.0;",
			1)
		body = strings.Replace(body,
			"var14 = (var2) * (FastMath.exp(var13));",
			"var14_1 = (var2) * (FastMath.exp(var13));",
			1)
		body = strings.Replace(body,
			"double var14_1 = (var2) * (FastMath.exp(var13));",
			"var14_1 = (var2) * (FastMath.exp(var13));",
			1)
		body = strings.Replace(body,
			"double var14_1 = FastMath.min(var14_1,",
			"var14_1 = FastMath.min(var14_1,",
			1)
	}
	if strings.Contains(body, "class ResizableDoubleArray") {
		body = strings.Replace(body,
			"ResizableDoubleArray var2 = this;\n\t\t\t\tsynchronized(this){\n\n\t\t\t\t}",
			"ResizableDoubleArray var2 = this;\n\t\t\t\tsynchronized(this){\n\t\t\t\t\tResizableDoubleArray var3 = ((ResizableDoubleArray)(var1));\n\t\t\t\t\treturn ((this.numElements) == (var3.numElements)) && ((this.startIndex) == (var3.startIndex));\n\t\t\t\t}",
			1)
	}
	if strings.Contains(body, "class Erf") {
		body = strings.Replace(body,
			"if ((var0) < (-0.4769362762044697D)){\n\n\t\t\t}else{",
			"if ((var0) < (-0.4769362762044697D)){\n\t\t\t\treturn ((var1) < (0D)) ? ((erfc(-var1)) - (erfc(-var0))) : ((erfc(-var0)) - (erfc(var1)));\n\t\t\t}else{",
			1)
	}
	if strings.Contains(body, "class AdamsNordsieckFieldTransformer") {
		body = strings.Replace(body,
			"this.update = new Array2DRowFieldMatrix<T>(var5.solve((FieldMatrix<T>)(new Array2DRowFieldMatrix<T>(var7,false))).getData());",
			"this.update = new Array2DRowFieldMatrix<T>((T[][])(var5.solve((FieldMatrix<T>)(new Array2DRowFieldMatrix<T>(var7,false))).getData()));",
			1)
	}
	return body
}

func wrapIteratorValueAsT(body string) string {
	from := 0
	needle := ".value()"
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		dot := from + rel
		if dot >= 3 && body[dot-3:dot] == "(T)" {
			from = dot + len(needle)
			continue
		}
		start := scanBackJavaRecv(body, dot)
		pre := body[max(0, start-4):start]
		if strings.Contains(pre, "(T)") {
			from = dot + len(needle)
			continue
		}
		end := dot + len(needle)
		call := body[start:end]
		if strings.HasPrefix(body[end:], ".") {
			body = body[:start] + "((T)(" + call + "))" + body[end:]
			from = start + len("((T)(") + len(call) + 2
		} else {
			body = body[:start] + "(T)(" + call + ")" + body[end:]
			from = start + len("(T)(") + len(call) + 1
		}
	}
}

func closeSolveTCasts(body string) string {
	// After wrapping `? ((T)(this.solver.solve(` the original call already has a
	// closing `)` before `:` / `;`. Insert one more `)` to close the (T)(.
	body = strings.ReplaceAll(body,
		",AllowedSolution.RIGHT_SIDE))",
		",AllowedSolution.RIGHT_SIDE)))")
	body = strings.ReplaceAll(body,
		",AllowedSolution.LEFT_SIDE));",
		",AllowedSolution.LEFT_SIDE)));")
	return body
}

func stripGenericFromStaticAccessors(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "static ")
		if rel < 0 {
			return body
		}
		i := from + rel
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if !strings.Contains(chunk, "access$") {
			from = i + 7
			continue
		}
		neu := strings.ReplaceAll(chunk, "<T> var", " var")
		neu = strings.ReplaceAll(neu, "static T[] ", "static FieldElement[] ")
		body = body[:i] + neu + body[end:]
		from = i + len(neu)
	}
}

func retypeMath3FieldT(body string) string {
	// Widest array forms first so we do not leave `T[][]` half-rewritten as `T[]`.
	body = strings.ReplaceAll(body, "((RealFieldElement[][])(", "((T[][])(")
	body = strings.ReplaceAll(body, "((FieldElement[][])(", "((T[][])(")
	body = strings.ReplaceAll(body, "((RealFieldElement[])(", "((T[])(")
	body = strings.ReplaceAll(body, "((FieldElement[])(", "((T[])(")
	body = strings.ReplaceAll(body, "((RealFieldElement)(", "((T)(")
	body = strings.ReplaceAll(body, "((FieldElement)(", "((T)(")
	body = strings.ReplaceAll(body, "RealFieldElement[][] var", "T[][] var")
	body = strings.ReplaceAll(body, "FieldElement[][] var", "T[][] var")
	body = strings.ReplaceAll(body, "RealFieldElement[] var", "T[] var")
	body = strings.ReplaceAll(body, "FieldElement[] var", "T[] var")
	body = strings.ReplaceAll(body, "RealFieldElement var", "T var")
	body = strings.ReplaceAll(body, "FieldElement var", "T var")
	body = strings.ReplaceAll(body, "new RealFieldElement[]{", "(T[])(new RealFieldElement[]{")
	body = strings.ReplaceAll(body, "new FieldElement[]{", "(T[])(new FieldElement[]{")
	body = closeMath3ArrayCasts(body)
	body = replaceBareFieldArrays(body, "RealFieldElement[]", "T[]")
	body = replaceBareFieldArrays(body, "FieldElement[]", "T[]")
	body = strings.ReplaceAll(body, "(RealFieldElement)", "(T)")
	body = strings.ReplaceAll(body, "(FieldElement)", "(T)")
	return body
}

func replaceBareFieldArrays(body, old, neu string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], old)
		if rel < 0 {
			return body
		}
		i := from + rel
		if i >= 4 && (body[i-4:i] == "new " || body[i-4:i] == "Real") {
			from = i + len(old)
			continue
		}
		body = body[:i] + neu + body[i+len(old):]
		from = i + len(neu)
	}
}

// closeMath3ArrayCasts adds the extra ')' after `{...}` for (T[])(new RealFieldElement[]{...}).
func closeMath3ArrayCasts(body string) string {
	for _, head := range []string{"(T[])(new RealFieldElement[]{", "(T[])(new FieldElement[]{"} {
		from := 0
		for {
			rel := strings.Index(body[from:], head)
			if rel < 0 {
				break
			}
			i := from + rel + len(head)
			depth := 1
			j := i
			for j < len(body) && depth > 0 {
				switch body[j] {
				case '{':
					depth++
				case '}':
					depth--
				}
				j++
			}
			if depth != 0 {
				from = i
				continue
			}
			// Always close the (T[])( wrapper. The following byte is often the
			// call's ')', which must stay *after* the cast close.
			body = body[:j] + ")" + body[j:]
			from = j + 1
		}
	}
	return body
}

func retypeMath3BoolCounters(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "boolean var")
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len("boolean "):]
		ident, ok, after := readJavaIdent(rest)
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		after = strings.TrimLeft(after, " \t")
		if !strings.HasPrefix(after, "=") {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if !math3BoolLooksLikeCounter(chunk, ident) {
			from = i + 1
			continue
		}
		init := "0"
		if strings.Contains(after, "= true") {
			init = "1"
		}
		old := "boolean " + ident
		neu := "int " + ident
		body = body[:i] + strings.Replace(body[i:], old, neu, 1)
		// default inits false/true → 0/1 on this declaration only.
		declEnd := strings.Index(body[i:], ";")
		if declEnd > 0 && declEnd < 80 {
			seg := body[i : i+declEnd]
			seg2 := strings.Replace(seg, " = false", " = 0", 1)
			seg2 = strings.Replace(seg2, " = true", " = 1", 1)
			if seg2 != seg {
				body = body[:i] + seg2 + body[i+declEnd:]
			}
		}
		body = strings.ReplaceAll(body, "!("+ident+")", "("+ident+") == (0)")
		body = strings.ReplaceAll(body, ident+" = false;", ident+" = 0;")
		body = strings.ReplaceAll(body, ident+" = true;", ident+" = 1;")
		_ = init
		from = i + len(neu)
	}
}

func fixMath3IntUsedAsBool(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "int var")
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len("int "):]
		ident, ok, _ := readJavaIdent(rest)
		if !ok || !isDecompilerLocal(ident) {
			from = i + 1
			continue
		}
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		if strings.Contains(chunk, "!("+ident+")") {
			body = body[:i] + strings.ReplaceAll(chunk, "!("+ident+")", "("+ident+") == (0)") + body[end:]
			end = nextMemberStart(body, i)
			chunk = body[i:end]
		}
		if strings.Contains(chunk, ident+" = false;") {
			body = body[:i] + strings.ReplaceAll(chunk, ident+" = false;", ident+" = 0;") + body[end:]
			end = nextMemberStart(body, i)
			chunk = body[i:end]
		}
		if strings.Contains(chunk, ident+" = true;") {
			body = body[:i] + strings.ReplaceAll(chunk, ident+" = true;", ident+" = 1;") + body[end:]
		}
		from = i + 4
	}
}

func wrapSizedFieldArrays(body string) string {
	for _, head := range []string{"new RealFieldElement[", "new FieldElement["} {
		from := 0
		for {
			rel := strings.Index(body[from:], head)
			if rel < 0 {
				break
			}
			i := from + rel
			if i >= 6 && body[i-6:i] == "(T[])(" {
				from = i + len(head)
				continue
			}
			rb := strings.Index(body[i+len(head):], "]")
			if rb < 0 {
				from = i + 1
				continue
			}
			j := i + len(head) + rb + 1
			body = body[:i] + "(T[])(" + body[i:j] + ")" + body[j:]
			from = j + len("(T[])()")
		}
	}
	return body
}

func wrapFieldArrayReturningCalls(body string) string {
	for _, meth := range []string{".mapState(", ".mapDerivative(", ".computeDerivatives(", ".extractEquationData("} {
		body = wrapReceiverCallAsTArray(body, meth)
	}
	return body
}

func wrapReceiverCallAsTArray(body, meth string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], meth)
		if rel < 0 {
			return body
		}
		dot := from + rel
		start := scanBackJavaRecv(body, dot)
		pre := body[max(0, start-8):start]
		if strings.Contains(pre, "(T[])") || strings.HasSuffix(strings.TrimRight(pre, " \t"), "(T)") {
			from = dot + len(meth)
			continue
		}
		paren := dot + len(meth) - 1
		n := skipBalanced(body[paren:], '(', ')')
		if n < 2 {
			from = dot + 1
			continue
		}
		end := paren + n
		call := body[start:end]
		body = body[:start] + "(T[])(" + call + ")" + body[end:]
		from = start + len("(T[])(") + len(call) + 1
	}
}

func scanBackJavaRecv(body string, dot int) int {
	i := dot
	for i > 0 {
		i--
		c := body[i]
		if isIdentChar(c) || c == '.' {
			continue
		}
		if c == ')' {
			depth := 1
			for i > 0 && depth > 0 {
				i--
				switch body[i] {
				case ')':
					depth++
				case '(':
					depth--
				}
			}
			continue
		}
		if c == ']' {
			depth := 1
			for i > 0 && depth > 0 {
				i--
				switch body[i] {
				case ']':
					depth++
				case '[':
					depth--
				}
			}
			continue
		}
		return i + 1
	}
	return 0
}

func rewriteMath3StaticS(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "static <S extends")
		if rel < 0 {
			return body
		}
		i := from + rel
		end := nextMemberStart(body, i)
		chunk := body[i:end]
		neu := chunk
		neu = strings.ReplaceAll(neu, "((T[][])(", "((S[][])(")
		neu = strings.ReplaceAll(neu, "((T[])(", "((S[])(")
		neu = strings.ReplaceAll(neu, "((T)(", "((S)(")
		neu = strings.ReplaceAll(neu, "T[][] var", "S[][] var")
		neu = strings.ReplaceAll(neu, "T[] var", "S[] var")
		neu = strings.ReplaceAll(neu, "T var", "S var")
		neu = strings.ReplaceAll(neu, "<T>", "<S>")
		body = body[:i] + neu + body[end:]
		from = i + len(neu)
	}
}

func math3BoolLooksLikeCounter(chunk, ident string) bool {
	if strings.Contains(chunk, ident+"++") || strings.Contains(chunk, ident+"--") ||
		strings.Contains(chunk, "++"+ident) || strings.Contains(chunk, "--"+ident) {
		return true
	}
	if strings.Contains(chunk, "["+ident+"]") || strings.Contains(chunk, "("+ident+") +") ||
		strings.Contains(chunk, "("+ident+") <") || strings.Contains(chunk, "("+ident+") >") {
		return true
	}
	return false
}
