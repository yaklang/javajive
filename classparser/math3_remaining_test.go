package javaclassparser

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestLutherFieldTLocalIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/LutherFieldStepInterpolator.class", "JDEC_MATH3_REMAINING_OFF",
		"T var9 = ((T)(((T)(((T)(var1.getZero())).add(21D))).sqrt()))",
		"RealFieldElement var9 = ((RealFieldElement)(((RealFieldElement)(((RealFieldElement)(var1.getZero())).add(21D))).sqrt()))")
}

func TestNewtonRaphsonDropsUnivariateFunctionCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/NewtonRaphsonSolver.class", "JDEC_MATH3_REMAINING_OFF",
		"super.solve(var1,var2,",
		"super.solve(var1,(UnivariateFunction)(var2),")
}

func TestPolynomialFitterDropsParametricCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PolynomialFitter.class", "JDEC_MATH3_REMAINING_OFF",
		"this.fit(var1,new PolynomialFunction$Parametric(),var2)",
		"this.fit(var1,(ParametricUnivariateFunction)(new PolynomialFunction$Parametric()),var2)")
}

func TestSummaryStatisticsMeanDropsFirstMomentCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SummaryStatistics.class", "JDEC_MATH3_REMAINING_OFF",
		"new Mean(this.secondMoment)",
		"new Mean((FirstMoment)(this.secondMoment))")
}

func TestCholeskyBoolCounterIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/RectangularCholeskyDecomposition.class", "JDEC_MATH3_REMAINING_OFF",
		"int var8 = 0",
		"boolean var8 = false")
}

func TestSparseGradientPutDoubleCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SparseGradient.class", "JDEC_MATH3_REMAINING_OFF",
		"var2.derivatives.put(Integer.valueOf(var5),(Double)(var4.getValue()))",
		"var2.derivatives.put(Integer.valueOf(var5),var4.getValue())")
}

func TestEulerSizedFieldArrayCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/EulerFieldStepInterpolator.class", "JDEC_MATH3_REMAINING_OFF",
		"(T[])(new RealFieldElement[1])",
		"new RealFieldElement[1]")
}

func TestArrayFieldVectorTypedCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ArrayFieldVector.class", "JDEC_MATH3_REMAINING_OFF",
		"(ArrayFieldVector<T>)(var1)",
		"(ArrayFieldVector)(var1)")
}

func TestOpenIntToFieldHashMapAccessTIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/OpenIntToFieldHashMap.class", "JDEC_MATH3_REMAINING_OFF",
		"(T)(((T)(var1.getZero())))",
		"(T)(((FieldElement)(var1.getZero())))")
}

func TestNetworkPutLongKeyIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Network.class", "JDEC_MATH3_REMAINING_OFF",
		"(Long)(var3.getKey())",
		"var3.getKey()")
}

func TestSymmLQMachPrecOrderIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SymmLQ$State.class", "JDEC_MATH3_REMAINING_OFF",
		"static final double MACH_PREC = FastMath.ulp(1D);\n\tstatic final double CBRT_MACH_PREC",
		"static final double CBRT_MACH_PREC = FastMath.cbrt(MACH_PREC);\n\tstatic final double MACH_PREC")
}

func TestAdversarialPoissonBoolAsDoubleIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/PoissonDistribution.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_MATH3_REMAINING_OFF", "1")
	source, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "? (1) : (0)") {
		t.Fatalf("missing opcode-based bool/int conversion:\n%s", source)
	}
	for _, decl := range regexp.MustCompile(`boolean (var[0-9_]+)`).FindAllStringSubmatch(source, -1) {
		if strings.Contains(source, "(double)("+decl[1]+")") {
			t.Fatalf("illegal boolean-to-double cast: %s", decl[1])
		}
	}
}

func TestErfTwoArgEmptyIfReturnIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Erf.class", "JDEC_MATH3_REMAINING_OFF",
		"return ((var1) < (0D)) ? ((erfc(-var1)) - (erfc(-var0))) : ((erfc(-var0)) - (erfc(var1)));",
		"if ((var0) < (-0.4769362762044697D)){\n\n\t\t\t}else{")
}

func TestMathRecoveryPreservesLoopDeclarationIdentities(t *testing.T) {
	body := "package org.apache.commons.math3.distribution;\nclass BetaDistribution$ChengBetaSampler {\n double scan() {\n\t\tdouble var10 = 0.0;\n\t\tdouble var6 = (var2) + ((1D) / (var5));\n\t\tdo{\n\t\t\tdouble var7 = 2;\n var14 = (var2) * (FastMath.exp(var13));\n } while(true); } }"
	if got := fixMath3RemainingReconstructs(body); got != body {
		t.Fatalf("changed a proved declaration or its store:\n%s", got)
	}
}

func TestMathRemainingKeepsRawConstructorBinding(t *testing.T) {
	// The caller's T is not evidence for an allocated class's instantiation.
	// The exact raw constructor accepts its descriptor's FieldElement[][]; a
	// textual <T> adds a stricter constraint and then needs an invented check.
	in := `class Any<T extends RealFieldElement<T>> {void copy(){this.update = new Array2DRowFieldMatrix(matrix.getData());}}`
	got := fixMath3RemainingReconstructs(in)
	if !strings.Contains(got, "new Array2DRowFieldMatrix(matrix.getData())") || strings.Contains(got, "new Array2DRowFieldMatrix<T>") {
		t.Fatal(got)
	}
}
