package javaclassparser

import (
	"strings"
	"testing"
)

const scalarPacketFixture = `class ScalarTarget{final int value;ScalarTarget(int value){this.value=value;}ScalarTarget(long value){throw new AssertionError("wrong widened target");}}
class ScalarOwner{class Part extends ScalarTarget{Part(int word){super((word^Integer.MIN_VALUE)>>>3);}Object owner(){return ScalarOwner.this;}}Part build(int word){return new Part(word);}}
class ScalarDriver{public static void main(String[]args){ScalarOwner owner=new ScalarOwner();int rows=0;for(int word:new int[]{Integer.MIN_VALUE,Integer.MIN_VALUE+1,-536870912,-1,0,1,7,8,15,536870912,Integer.MAX_VALUE-1,Integer.MAX_VALUE}){ScalarOwner.Part part=owner.build(word);int expected=(int)(((long)word+2147483648L)/8L);if(part.value!=expected||part.owner()!=owner)throw new AssertionError("scalar computation/binding/outer identity");rows++;}System.out.println(rows+":scalar:constructor:identity");}}`

func TestAdversarialConstructorScalarPacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, scalarPacketFixture, []string{"ScalarOwner"}, "ScalarDriver", "12:scalar:constructor:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorScalarPacketRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(scalarPacketFixture, "Scalar", "RotatedWord")
	testNativeIndependentFamilyFixture(t, f, []string{"RotatedWordOwner"}, "RotatedWordDriver", "12:scalar:constructor:identity\n", nativeLexicalExactSignatures)
}

const longScalarPacketFixture = `class WideTarget{final long value;WideTarget(long value){this.value=value;}WideTarget(double value){throw new AssertionError("wrong floating target");}}
class WideOwner{class Part extends WideTarget{Part(long word){super((word^Long.MIN_VALUE)>>>63);}Object owner(){return WideOwner.this;}}Part build(long word){return new Part(word);}}
class WideDriver{public static void main(String[]args){WideOwner owner=new WideOwner();int rows=0;for(long word:new long[]{Long.MIN_VALUE,Long.MIN_VALUE+1,-1,0,1,Long.MAX_VALUE}){WideOwner.Part part=owner.build(word);if(part.value!=(word<0?0:1)||part.owner()!=owner)throw new AssertionError("wide word/count category/binding/outer identity");rows++;}System.out.println(rows+":wide:constructor:identity");}}`

func TestAdversarialConstructorScalarWidePacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, longScalarPacketFixture, []string{"WideOwner"}, "WideDriver", "6:wide:constructor:identity\n", nativeLexicalExactSignatures)
}

const divisionScalarPacketFixture = `class DivideTarget{final int value;DivideTarget(int value){this.value=value;}DivideTarget(long value){throw new AssertionError("wrong widened target");}}
class DivideOwner{class Part extends DivideTarget{Part(int denominator){super(96/denominator);}Object owner(){return DivideOwner.this;}}Part build(int denominator){return new Part(denominator);}}
class DivideDriver{public static void main(String[]args){DivideOwner owner=new DivideOwner();int rows=0;for(int denominator:new int[]{Integer.MIN_VALUE,-7,-1,0,1,7,Integer.MAX_VALUE}){try{DivideOwner.Part part=owner.build(denominator);if(denominator==0)throw new AssertionError("lost original division failure");long positive=denominator<0?-(long)denominator:(long)denominator;int expected=(int)(96L/positive);if(denominator<0)expected=-expected;if(part.value!=expected||part.owner()!=owner)throw new AssertionError("quotient/binding/outer identity");}catch(ArithmeticException failure){if(denominator!=0)throw failure;}rows++;}System.out.println(rows+":division:constructor:failure");}}`

func TestAdversarialConstructorScalarDivisionPacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, divisionScalarPacketFixture, []string{"DivideOwner"}, "DivideDriver", "7:division:constructor:failure\n", nativeLexicalExactSignatures)
}

const floatingScalarPacketFixture = `class FloatingTarget{final double value;FloatingTarget(double value){this.value=value;}FloatingTarget(long value){throw new AssertionError("wrong integer target");}}
class FloatingOwner{class Part extends FloatingTarget{Part(double word){super(word<0?1.0:Double.isNaN(word)?2.0:3.0);}Object owner(){return FloatingOwner.this;}}Part build(double word){return new Part(word);}}
class FloatingDriver{public static void main(String[]args){FloatingOwner owner=new FloatingOwner();int rows=0;for(double word:new double[]{Double.NaN,Double.POSITIVE_INFINITY,Double.NEGATIVE_INFINITY,-0.0,0.0,-1.0,1.0}){long bits=Double.doubleToRawLongBits(word);boolean nan=(bits&0x7ff0000000000000L)==0x7ff0000000000000L&&(bits&0x000fffffffffffffL)!=0;double expected=nan?2.0:bits<0&&(bits&0x7fffffffffffffffL)!=0?1.0:3.0;FloatingOwner.Part part=owner.build(word);if(part.value!=expected||part.owner()!=owner)throw new AssertionError("NaN/signed zero/infinity/original comparator/binding/outer identity");rows++;}System.out.println(rows+":floating:constructor:identity");}}`

func TestAdversarialConstructorScalarFloatingPacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, floatingScalarPacketFixture, []string{"FloatingOwner"}, "FloatingDriver", "7:floating:constructor:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorScalarFloatingOppositePacketRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(floatingScalarPacketFixture, "word<0?1.0:Double.isNaN(word)?2.0:3.0", "word>0?1.0:word==0?2.0:Double.isNaN(word)?3.0:4.0")
	f = strings.ReplaceAll(f, "double expected=nan?2.0:bits<0&&(bits&0x7fffffffffffffffL)!=0?1.0:3.0", "double expected=nan?3.0:(bits&0x7fffffffffffffffL)==0?2.0:bits<0?4.0:1.0")
	testNativeIndependentFamilyFixture(t, f, []string{"FloatingOwner"}, "FloatingDriver", "7:floating:constructor:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorScalarOriginalWideningPacketRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(scalarPacketFixture, "final int value;ScalarTarget(int value)", "final long value;ScalarTarget(long value)")
	f = strings.ReplaceAll(f, "ScalarTarget(long value){throw", "ScalarTarget(double value){throw")
	f = strings.ReplaceAll(f, "super((word^Integer.MIN_VALUE)>>>3)", "super((long)word*32L+(word>>>5))")
	f = strings.ReplaceAll(f, "int expected=(int)(((long)word+2147483648L)/8L)", "long unsigned=word<0?(long)word+4294967296L:(long)word;long expected=(long)word*32L+unsigned/32L")
	testNativeIndependentFamilyFixture(t, f, []string{"ScalarOwner"}, "ScalarDriver", "12:scalar:constructor:identity\n", nativeLexicalExactSignatures)
}

const variableDivisionPacketFixture = `class QuotientTarget{final int value;QuotientTarget(int value){this.value=value;}QuotientTarget(long value){throw new AssertionError("wrong widened target");}}
class QuotientOwner{class Part extends QuotientTarget{Part(int numerator,int denominator){super(numerator/denominator);}Object owner(){return QuotientOwner.this;}}Part build(int numerator,int denominator){return new Part(numerator,denominator);}}
class QuotientDriver{public static void main(String[]args){QuotientOwner owner=new QuotientOwner();int rows=0;for(int numerator:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int denominator:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){try{QuotientOwner.Part part=owner.build(numerator,denominator);if(denominator==0)throw new AssertionError("lost original zero divisor");int expected=java.math.BigInteger.valueOf(numerator).divide(java.math.BigInteger.valueOf(denominator)).intValue();if(part.value!=expected||part.owner()!=owner)throw new AssertionError("int overflow/quotient/binding/outer identity");}catch(ArithmeticException failure){if(denominator!=0)throw failure;}rows++;}System.out.println(rows+":division:overflow:failure");}}`

func TestAdversarialConstructorScalarVariableDivisionPacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, variableDivisionPacketFixture, []string{"QuotientOwner"}, "QuotientDriver", "25:division:overflow:failure\n", nativeLexicalExactSignatures)
}
