package javaclassparser

import (
	"strings"
	"testing"
)

const arrayLoadPacketFixture = `class ElementTarget{final int value;ElementTarget(int value){this.value=value;}ElementTarget(long value){throw new AssertionError("wrong widened target");}}
class ElementOwner{class Part extends ElementTarget{Part(int[] words,int index){super(words[index]);}Object owner(){return ElementOwner.this;}}Part build(int[] words,int index){return new Part(words,index);}}
class ElementDriver{public static void main(String[]args){ElementOwner owner=new ElementOwner();int[] words={Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE};int rows=0;for(int index=0;index<words.length;index++){ElementOwner.Part part=owner.build(words,index);int expected=index==0?Integer.MIN_VALUE:index==1?-1:index==2?0:index==3?1:Integer.MAX_VALUE;if(part.value!=expected||part.owner()!=owner)throw new AssertionError("array word/binding/outer identity");rows++;}for(int index:new int[]{Integer.MIN_VALUE,-1,5,Integer.MAX_VALUE}){try{owner.build(words,index);throw new AssertionError("missing bounds failure");}catch(ArrayIndexOutOfBoundsException expected){}rows++;try{owner.build(null,index);throw new AssertionError("missing null failure");}catch(NullPointerException expected){}rows++;}System.out.println(rows+":array:constructor:identity:failure");}}`

func TestAdversarialConstructorArrayLoadPacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, arrayLoadPacketFixture, []string{"ElementOwner"}, "ElementDriver", "13:array:constructor:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorArrayLoadPacketRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(arrayLoadPacketFixture, "Element", "IndexedWord")
	testNativeIndependentFamilyFixture(t, f, []string{"IndexedWordOwner"}, "IndexedWordDriver", "13:array:constructor:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorArrayLoadPacketPrimitiveComponentsRoundTrip(t *testing.T) {
	for _, tc := range []struct{ name, typ, values, oracle, compare, wrong string }{
		{"byte", "byte", "-128,-1,0,1,127", "(byte)(index==0?-128:index==1?-1:index==2?0:index==3?1:127)", "part.value!=expected", "int"},
		{"short", "short", "-32768,-1,0,1,32767", "(short)(index==0?-32768:index==1?-1:index==2?0:index==3?1:32767)", "part.value!=expected", "int"},
		{"char", "char", "0,1,32767,32768,65535", "(char)(index==0?0:index==1?1:index==2?32767:index==3?32768:65535)", "part.value!=expected", "int"},
		{"boolean", "boolean", "false,true,false,true,false", "(index&1)!=0", "part.value!=expected", "Object"},
		{"long", "long", "Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE", "index==0?Long.MIN_VALUE:index==1?-1L:index==2?0L:index==3?1L:Long.MAX_VALUE", "part.value!=expected", "double"},
		{"float", "float", "Float.intBitsToFloat(0x7fc12345),-0.0f,0.0f,Float.NEGATIVE_INFINITY,Float.POSITIVE_INFINITY", "Float.intBitsToFloat(index==0?0x7fc12345:index==1?0x80000000:index==2?0:index==3?0xff800000:0x7f800000)", "Float.floatToRawIntBits(part.value)!=Float.floatToRawIntBits(expected)", "double"},
		{"double", "double", "Double.longBitsToDouble(0x7ff8123456789abcL),-0.0d,0.0d,Double.NEGATIVE_INFINITY,Double.POSITIVE_INFINITY", "Double.longBitsToDouble(index==0?0x7ff8123456789abcL:index==1?Long.MIN_VALUE:index==2?0L:index==3?0xfff0000000000000L:0x7ff0000000000000L)", "Double.doubleToRawLongBits(part.value)!=Double.doubleToRawLongBits(expected)", "long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := `class StorageTarget{final TYPE value;StorageTarget(TYPE value){this.value=value;}StorageTarget(WRONG value){throw new AssertionError("wrong component overload");}}
class StorageOwner{class Part extends StorageTarget{Part(TYPE[] words,int index){super(words[index]);}Object owner(){return StorageOwner.this;}}Part build(TYPE[] words,int index){return new Part(words,index);}}
class StorageDriver{public static void main(String[]args){StorageOwner owner=new StorageOwner();TYPE[] words={VALUES};int rows=0;for(int index=0;index<5;index++){StorageOwner.Part part=owner.build(words,index);TYPE expected=ORACLE;if(COMPARE||part.owner()!=owner)throw new AssertionError("component category/word bits/overload/outer identity");rows++;}for(int index:new int[]{Integer.MIN_VALUE,-1,5,Integer.MAX_VALUE}){try{owner.build(words,index);throw new AssertionError("missing bounds failure");}catch(ArrayIndexOutOfBoundsException expected){}rows++;try{owner.build(null,index);throw new AssertionError("missing null failure");}catch(NullPointerException expected){}rows++;}System.out.println(rows+":component:constructor:identity:failure");}}`
			f = strings.NewReplacer("TYPE", tc.typ, "WRONG", tc.wrong, "VALUES", tc.values, "ORACLE", tc.oracle, "COMPARE", tc.compare).Replace(f)
			testNativeIndependentFamilyFixture(t, f, []string{"StorageOwner"}, "StorageDriver", "13:component:constructor:identity:failure\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialConstructorArrayLoadPacketReferenceRoundTrip(t *testing.T) {
	f := `class ReferenceTarget{final Object value;ReferenceTarget(Object value){this.value=value;}ReferenceTarget(String value){throw new AssertionError("wrong narrow overload");}}
class ReferenceOwner{class Part extends ReferenceTarget{Part(Object[] words,int index){super(words[index]);}Object owner(){return ReferenceOwner.this;}}Part build(Object[] words,int index){return new Part(words,index);}}
class ReferenceDriver{public static void main(String[]args){ReferenceOwner owner=new ReferenceOwner();Object token=new Object();Object[] words={token,null,"text"};int rows=0;for(int index=0;index<3;index++){ReferenceOwner.Part part=owner.build(words,index);Object expected=index==0?token:index==1?null:"text";if(part.value!=expected||part.owner()!=owner)throw new AssertionError("reference identity/original broad overload/outer identity");rows++;}for(int index:new int[]{-1,3}){try{owner.build(words,index);throw new AssertionError("missing bounds failure");}catch(ArrayIndexOutOfBoundsException expected){}try{owner.build(null,index);throw new AssertionError("missing null failure");}catch(NullPointerException expected){}rows+=2;}System.out.println(rows+":reference:constructor:identity:failure");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"ReferenceOwner"}, "ReferenceDriver", "7:reference:constructor:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorArrayLoadPacketNestedDimensionsRoundTrip(t *testing.T) {
	f := `class MatrixTarget{final int value;MatrixTarget(int value){this.value=value;}MatrixTarget(long value){throw new AssertionError("wrong word overload");}}
class MatrixOwner{class Part extends MatrixTarget{Part(int[][] words,int row,int col){super(words[row][col]);}Object owner(){return MatrixOwner.this;}}Part build(int[][] words,int row,int col){return new Part(words,row,col);}}
class MatrixDriver{public static void main(String[]args){MatrixOwner owner=new MatrixOwner();int[][] words={{Integer.MIN_VALUE,Integer.MAX_VALUE},null,{}};int rows=0;for(int row:new int[]{-1,0,1,2,3})for(int col:new int[]{-1,0,1,2}){try{MatrixOwner.Part part=owner.build(words,row,col);if(row!=0||col<0||col>1||part.value!=(col==0?Integer.MIN_VALUE:Integer.MAX_VALUE)||part.owner()!=owner)throw new AssertionError("dimensions/word/identity");}catch(NullPointerException expected){if(row!=1)throw new AssertionError("inner-null priority",expected);}catch(ArrayIndexOutOfBoundsException expected){if(row==1||row==0&&col>=0&&col<2)throw new AssertionError("bounds priority",expected);}rows++;}System.out.println(rows+":dimensions:constructor:identity:failure");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"MatrixOwner"}, "MatrixDriver", "20:dimensions:constructor:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorArrayLoadPacketEffectsRoundTrip(t *testing.T) {
	f := `class ReadEffects{static String trace="";static final IllegalArgumentException error=new IllegalArgumentException();static int[] array(int[] words){trace+="A";return words;}static int index(int index){trace+="I";if(index==7)throw error;return index;}}
class ReadTarget{final int value;ReadTarget(int value){ReadEffects.trace+="P";this.value=value;}}
class ReadOwner{class Part extends ReadTarget{Part(int[] words,int index){super(ReadEffects.array(words)[ReadEffects.index(index)]);}Object owner(){return ReadOwner.this;}}Part build(int[] words,int index){return new Part(words,index);}}
class ReadDriver{public static void main(String[]args){ReadOwner owner=new ReadOwner();int rows=0;for(boolean nil:new boolean[]{false,true})for(int index:new int[]{-1,0,1,7}){ReadEffects.trace="";try{ReadOwner.Part part=owner.build(nil?null:new int[]{31},index);if(nil||index!=0||part.value!=31||part.owner()!=owner||!ReadEffects.trace.equals("AIP"))throw new AssertionError("read/parent/identity/order");}catch(IllegalArgumentException e){if(index!=7||e!=ReadEffects.error||!ReadEffects.trace.equals("AI"))throw new AssertionError("index exception priority/identity/order",e);}catch(NullPointerException e){if(!nil||index==7||!ReadEffects.trace.equals("AI"))throw new AssertionError("array null priority/order",e);}catch(ArrayIndexOutOfBoundsException e){if(nil||index==0||index==7||!ReadEffects.trace.equals("AI"))throw new AssertionError("array bounds priority/order",e);}rows++;}System.out.println(rows+":read:effects:failure:identity:order");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"ReadOwner"}, "ReadDriver", "8:read:effects:failure:identity:order\n", nativeLexicalExactSignatures)
}
