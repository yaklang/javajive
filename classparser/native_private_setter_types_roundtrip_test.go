package javaclassparser

import "testing"

const nativePrivateSetterTypesFixture = `abstract class TypedSetterParent{final Object observed;TypedSetterParent(){observed=observe();}abstract Object observe();}
class TypedSetterOwner<T> {
 final Object seed;TypedSetterOwner(Object seed){this.seed=seed;}
 class Layer extends TypedSetterParent {
  private volatile T token;private boolean flag;private byte small;private char character;private short narrow;private int integer;private long wide;private float single;private double precision;private Object[] array;
  Object observe(){return TypedSetterOwner.this.seed;}
  class Leaf {
   T token(Layer o,T v){return o.token=v;}T token(Layer o){return o.token;}
   boolean flag(Layer o,boolean v){return o.flag=v;}boolean flag(Layer o){return o.flag;}
   byte small(Layer o,byte v){return o.small=v;}byte small(Layer o){return o.small;}
   char character(Layer o,char v){return o.character=v;}char character(Layer o){return o.character;}
   short narrow(Layer o,short v){return o.narrow=v;}short narrow(Layer o){return o.narrow;}
   int integer(Layer o,int v){return o.integer=v;}int integer(Layer o){return o.integer;}
   long wide(Layer o,long v){return o.wide=v;}long wide(Layer o){return o.wide;}
   float single(Layer o,float v){return o.single=v;}float single(Layer o){return o.single;}
   double precision(Layer o,double v){return o.precision=v;}double precision(Layer o){return o.precision;}
   Object[] array(Layer o,Object[] v){return o.array=v;}Object[] array(Layer o){return o.array;}
  }
 }
}
class TypedSetterDriver {public static void main(String[]args){
 Object token=new Object();TypedSetterOwner<Object> owner=new TypedSetterOwner<Object>(token);TypedSetterOwner<Object>.Layer layer=owner.new Layer();TypedSetterOwner<Object>.Layer.Leaf leaf=layer.new Leaf();int rows=0;
 if(layer.observed!=token)throw new AssertionError("capture before callback");
 for(Object v:new Object[]{null,token}){if(leaf.token(layer,v)!=v||leaf.token(layer)!=v)throw new AssertionError("generic identity");rows++;}
 for(boolean v:new boolean[]{false,true}){if(leaf.flag(layer,v)!=v||leaf.flag(layer)!=v)throw new AssertionError("boolean");rows++;}
 for(byte v:new byte[]{Byte.MIN_VALUE,-1,0,Byte.MAX_VALUE}){if(leaf.small(layer,v)!=v||leaf.small(layer)!=v)throw new AssertionError("byte");rows++;}
 for(char v:new char[]{0,1,0xD800,0xFFFF}){if(leaf.character(layer,v)!=v||leaf.character(layer)!=v)throw new AssertionError("char");rows++;}
 for(short v:new short[]{Short.MIN_VALUE,-1,0,Short.MAX_VALUE}){if(leaf.narrow(layer,v)!=v||leaf.narrow(layer)!=v)throw new AssertionError("short");rows++;}
 for(int v:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}){if(leaf.integer(layer,v)!=v||leaf.integer(layer)!=v)throw new AssertionError("int");rows++;}
 for(long v:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){if(leaf.wide(layer,v)!=v||leaf.wide(layer)!=v)throw new AssertionError("long");rows++;}
 for(int bits:new int[]{0,0x80000000,0x7F800000,0xFF800000,1,0x7FC00001}){float v=Float.intBitsToFloat(bits);if(Float.floatToRawIntBits(leaf.single(layer,v))!=bits||Float.floatToRawIntBits(leaf.single(layer))!=bits)throw new AssertionError("float bits");rows++;}
 for(long bits:new long[]{0L,0x8000000000000000L,0x7FF0000000000000L,0xFFF0000000000000L,1L,0x7FF8000000000001L}){double v=Double.longBitsToDouble(bits);if(Double.doubleToRawLongBits(leaf.precision(layer,v))!=bits||Double.doubleToRawLongBits(leaf.precision(layer))!=bits)throw new AssertionError("double bits");rows++;}
 for(Object[] v:new Object[][]{null,new Object[]{token}}){if(leaf.array(layer,v)!=v||leaf.array(layer)!=v)throw new AssertionError("array identity");rows++;}
 System.out.println(rows+":typed-bits:identity");
}}`

func TestNativePrivateSetterTypesKeepRawValuesAndErasure(t *testing.T) {
	testNativePrivateSetterFixture(t, nativePrivateSetterTypesFixture, "TypedSetterOwner", "TypedSetterDriver", "38:typed-bits:identity\n")
}
