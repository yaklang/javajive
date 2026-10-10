package javaclassparser

import (
	"strings"
	"testing"
)

// Runtime reflection reads the original class-file constants independently of
// the generated source. In particular decimal rounding and an omitted float
// suffix must not change annotation elements or defaults.
func TestAdversarialAnnotationNumericConstantsPreserveBits(t *testing.T) {
	const fixture = `import java.lang.annotation.*;
@Retention(RetentionPolicy.RUNTIME) @interface NumericMark {float f() default Float.MIN_VALUE;double d() default Double.MIN_VALUE;float[] fs();double[] ds();}
enum NumericEnum {
@NumericMark(f=1.2345678F,d=1.2345678901234567D,fs={Float.MIN_VALUE,Float.NEGATIVE_INFINITY},ds={Double.MIN_VALUE,Double.NaN}) VALUE
}
@NumericMark(f=1.2345678F,d=1.2345678901234567D,fs={-0.0F,Float.MIN_VALUE,Float.MAX_VALUE,Float.NaN,Float.POSITIVE_INFINITY,Float.NEGATIVE_INFINITY},ds={-0.0D,Double.MIN_VALUE,Double.MAX_VALUE,Double.NaN,Double.POSITIVE_INFINITY,Double.NEGATIVE_INFINITY})
class AnnotationNumbers {
@NumericMark(f=1.2345678F,d=1.2345678901234567D,fs={-0.0F,Float.MIN_VALUE,Float.NaN},ds={-0.0D,Double.MIN_VALUE,Double.POSITIVE_INFINITY}) int field;
@NumericMark(f=1.2345678F,d=1.2345678901234567D,fs={-0.0F,Float.MAX_VALUE,Float.NEGATIVE_INFINITY},ds={-0.0D,Double.MAX_VALUE,Double.NaN}) void method(@NumericMark(f=1.2345678F,d=1.2345678901234567D,fs={Float.MIN_VALUE,Float.POSITIVE_INFINITY},ds={Double.MIN_VALUE,Double.NEGATIVE_INFINITY}) int n){}
}
public class AnnotationNumericDriver{public static void main(String[]args)throws Exception{for(NumericMark a:new NumericMark[]{AnnotationNumbers.class.getAnnotation(NumericMark.class),AnnotationNumbers.class.getDeclaredField("field").getAnnotation(NumericMark.class),AnnotationNumbers.class.getDeclaredMethod("method",int.class).getAnnotation(NumericMark.class),(NumericMark)AnnotationNumbers.class.getDeclaredMethod("method",int.class).getParameterAnnotations()[0][0],NumericEnum.class.getField("VALUE").getAnnotation(NumericMark.class)}){System.out.println(Integer.toHexString(Float.floatToRawIntBits(a.f())));System.out.println(Long.toHexString(Double.doubleToRawLongBits(a.d())));for(float v:a.fs())System.out.println(Integer.toHexString(Float.floatToRawIntBits(v)));for(double v:a.ds())System.out.println(Long.toHexString(Double.doubleToRawLongBits(v)));}System.out.println(Integer.toHexString(Float.floatToRawIntBits((Float)NumericMark.class.getMethod("f").getDefaultValue())));System.out.println(Long.toHexString(Double.doubleToRawLongBits((Double)NumericMark.class.getMethod("d").getDefaultValue())));}}`
	for _, variant := range []string{"ordinary scope", "wrapper names shadowed"} {
		source := fixture
		if variant == "wrapper names shadowed" {
			source = strings.NewReplacer("Float.", "java.lang.Float.", "Double.", "java.lang.Double.", "(Float)", "(java.lang.Float)", "(Double)", "(java.lang.Double)").Replace(source)
			source += "\nclass Float{}class Double{}"
		}
		t.Run(variant, func(t *testing.T) {
			roundTripGenericFlowUnitsClasspath(t, "AnnotationNumericDriver", source, nil, []string{"AnnotationNumbers", "NumericMark", "NumericEnum"}, true, Precision, Compatibility, "legacy")
		})
	}
}
