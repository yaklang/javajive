package javaclassparser

import "testing"

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

func TestPoissonBoolAsDoubleIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PoissonDistribution.class", "JDEC_MATH3_REMAINING_OFF",
		"((var25_1) ? (1.0D) : (0.0D))",
		"((double)(var25_1))")
}

func TestErfTwoArgEmptyIfReturnIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Erf.class", "JDEC_MATH3_REMAINING_OFF",
		"return ((var1) < (0D)) ? ((erfc(-var1)) - (erfc(-var0))) : ((erfc(-var0)) - (erfc(var1)));",
		"if ((var0) < (-0.4769362762044697D)){\n\n\t\t\t}else{")
}
