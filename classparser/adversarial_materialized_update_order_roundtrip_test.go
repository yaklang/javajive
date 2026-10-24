package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A materialized producer updates a local before the invocation reads its
// first argument. Substitution into a later argument must not move that write
// behind the earlier read. The binary-only observer checks both arguments and
// exception identity; the driver computes unsigned bytes independently.
const materializedUpdateOrderFixture = `
class UpdateOrderOwner {
 static long increment(byte[] input,int position,int step) {
  int value=input[position++]&255;
  return UpdateOrderObserver.accept(position,value);
 }
 static long decrement(byte[] input,int position,int step) {
  int value=input[position--]&255;
  return UpdateOrderObserver.accept(position,value);
 }
 static long compound(byte[] input,int position,int step) {
  int value=input[position+=step]&255;
  return UpdateOrderObserver.accept(position,value);
 }
 static long assignment(byte[] input,int position,int step) {
  int value=input[position=step]&255;
  return UpdateOrderObserver.accept(position,value);
 }
}
class UpdateOrderObserver {
 static final RuntimeException failure=new IllegalArgumentException("callback");
 static int calls,position,value;
 static boolean fail;
 static long accept(int p,int v) {
  calls++;position=p;value=v;
  if(fail)throw failure;
  return ((long)p<<32)|(v&0xffffffffL);
 }
}
class UpdateOrderDriver {
 static long invoke(int mode,byte[] input,int position,int step) {
  switch(mode) {
   case 0:return UpdateOrderOwner.increment(input,position,step);
   case 1:return UpdateOrderOwner.decrement(input,position,step);
   case 2:return UpdateOrderOwner.compound(input,position,step);
   default:return UpdateOrderOwner.assignment(input,position,step);
  }
 }
 static void check(int mode,byte[] input,int position,int step,boolean fail) {
  int read=mode<2?position:mode==2?(int)((long)position+step):step;
  int after=mode==0?position+1:mode==1?position-1:read;
  boolean valid=input!=null&&read>=0&&read<input.length;
  int value=valid?Math.floorMod((int)input[read],256):0;
  UpdateOrderObserver.calls=0;UpdateOrderObserver.fail=fail;
  try {
   long result=invoke(mode,input,position,step);
   long expected=((long)after<<32)|(long)value;
   if(!valid||fail||result!=expected)throw new AssertionError("result/order");
  }catch(RuntimeException error) {
   if(!valid) {
    if(input==null?!(error instanceof NullPointerException):!(error instanceof ArrayIndexOutOfBoundsException))throw new AssertionError("array failure",error);
   }else if(!fail||error!=UpdateOrderObserver.failure)throw new AssertionError("callback identity",error);
  }
  if(UpdateOrderObserver.calls!=(valid?1:0))throw new AssertionError("call count/array before callback");
  if(valid&&(UpdateOrderObserver.position!=after||UpdateOrderObserver.value!=value))throw new AssertionError("earlier argument/unsigned byte");
 }
 public static void main(String[] args) {
  int rows=0;
  for(int word=0;word<256;word++) {
   byte[] input={(byte)word,(byte)(word-17),(byte)(127-word),(byte)(word^128)};
   byte[] before=input.clone();
   for(int mode=0;mode<4;mode++)for(int position:new int[]{-1,0,1,3,4})for(int step:new int[]{-2,0,2})for(boolean fail:new boolean[]{false,true}) {
    check(mode,input,position,step,fail);rows++;
   }
   if(!java.util.Arrays.equals(input,before))throw new AssertionError("input mutation");
  }
  for(int mode=0;mode<4;mode++)for(boolean fail:new boolean[]{false,true}) {
   check(mode,null,Integer.MAX_VALUE,Integer.MIN_VALUE,fail);rows++;
  }
  System.out.println(rows+":updates:arguments:unsigned:identity:bounds");
 }
}`

func TestAdversarialMaterializedUpdatesPreserveEarlierArguments(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"UpdateOrderOwner", "DifferentStorageName"} {
			t.Run(compiler.name+"/"+owner, func(t *testing.T) {
				source := strings.ReplaceAll(materializedUpdateOrderFixture, "UpdateOrderOwner", owner)
				compile := func(debug string) map[string][]byte {
					if compiler.profile == NativeJavac8 {
						return nativePrivateEnumCompile(t, source, owner, debug)
					}
					return nativeCompileDebugClasses(t, source, debug)
				}
				testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{owner}, "UpdateOrderDriver", "30728:updates:arguments:unsigned:identity:bounds\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}
