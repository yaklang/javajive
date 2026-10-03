package javaclassparser

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// ldc and ConstantValue are primitive constants. Their source expressions must
// not acquire a dependency on a same-scope class named Float or Double.
func TestAdversarialFloatingConstantsRemainPrimitiveInShadowedScopes(t *testing.T) {
	const source = `class Float{}class Double{}
class PrimitiveConstants {
 static final float FN=0.0F/0.0F,FP=1.0F/0.0F,FM=-1.0F/0.0F;
 static final double DN=0.0D/0.0D,DP=1.0D/0.0D,DM=-1.0D/0.0D;
 static float floating(int n){if(n==0)return 0.0F/0.0F;if(n>0)return 1.0F/0.0F;return -1.0F/0.0F;}
 static double decimal(int n){if(n==0)return 0.0D/0.0D;if(n>0)return 1.0D/0.0D;return -1.0D/0.0D;}
}
public class FloatingShadowDriver {public static void main(String[]args)throws Exception{
 for(int n:new int[]{-1,0,1}){System.out.println(Integer.toHexString(java.lang.Float.floatToRawIntBits(PrimitiveConstants.floating(n))));System.out.println(Long.toHexString(java.lang.Double.doubleToRawLongBits(PrimitiveConstants.decimal(n))));}
 for(String field:new String[]{"FN","FP","FM"})System.out.println(Integer.toHexString(java.lang.Float.floatToRawIntBits(PrimitiveConstants.class.getDeclaredField(field).getFloat(null))));
 for(String field:new String[]{"DN","DP","DM"})System.out.println(Long.toHexString(java.lang.Double.doubleToRawLongBits(PrimitiveConstants.class.getDeclaredField(field).getDouble(null))));
}}
`
	roundTripGenericFlowUnitsClasspath(t, "FloatingShadowDriver", source, nil, []string{"PrimitiveConstants"}, true, Precision, Compatibility, "legacy")
}

// Compile an independent hexadecimal source oracle, then compare shortest
// decimal regeneration over deterministic IEEE edge and varied mantissa values.
func TestAdversarialFiniteFloatingLiteralHexOracle(t *testing.T) {
	floats := []uint32{0, 0x80000000, 1, 0x80000001, 0x007fffff, 0x00800000, 0x3f800001, 0x3f7fffff, 0x7f7fffff, 0xff7fffff}
	doubles := []uint64{0, 0x8000000000000000, 1, 0x8000000000000001, 0x000fffffffffffff, 0x0010000000000000, 0x3ff0000000000001, 0x3fefffffffffffff, 0x7fefffffffffffff, 0xffefffffffffffff}
	state := uint64(0x59c07a31deeb052f)
	for i := 0; i < 32; i++ {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		if math.IsInf(math.Float64frombits(state), 0) || math.IsNaN(math.Float64frombits(state)) {
			continue
		}
		doubles = append(doubles, state)
		f := uint32(state)
		if !math.IsNaN(float64(math.Float32frombits(f))) && !math.IsInf(float64(math.Float32frombits(f)), 0) {
			floats = append(floats, f)
		}
	}
	fs, ds := []string{}, []string{}
	for _, bits := range floats {
		fs = append(fs, strconv.FormatFloat(float64(math.Float32frombits(bits)), 'x', -1, 32)+"F")
	}
	for _, bits := range doubles {
		ds = append(ds, strconv.FormatFloat(math.Float64frombits(bits), 'x', -1, 64)+"D")
	}
	source := `class FiniteConstants{static float[] floating(){return new float[]{` + strings.Join(fs, ",") + `};}static double[] decimal(){return new double[]{` + strings.Join(ds, ",") + `};}}
public class FiniteLiteralDriver{public static void main(String[]args){for(float value:FiniteConstants.floating())System.out.println(Integer.toHexString(Float.floatToRawIntBits(value)));for(double value:FiniteConstants.decimal())System.out.println(Long.toHexString(Double.doubleToRawLongBits(value)));}}`
	roundTripGenericFlowUnitsClasspath(t, "FiniteLiteralDriver", source, nil, []string{"FiniteConstants"}, true, Precision, Compatibility, "legacy")
}
