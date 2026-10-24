package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const nativePrivateArgumentFixture = `class ArgumentOwner {
 private int first=7,last=13;void update(int value){first=value;}int current(){return first;}
 java.util.function.IntSupplier make(final ArgumentBox value){return new java.util.function.IntSupplier(){
  private int helper(ArgumentBox box)throws java.io.IOException{ArgumentEffects.trace+="H";if(ArgumentEffects.mode==2)throw ArgumentEffects.failure;return box.value+first;}
  public int getAsInt(){return new java.util.function.IntSupplier(){public int getAsInt(){try{return helper(ARG);}catch(java.io.IOException error){throw new IllegalStateException(error);}}}.getAsInt();}
 };}
 class Tail{int get(){return last;}}Tail tail(){return new Tail();}
}
class ArgumentBox {int value;ArgumentBox(int value){this.value=value;}}
class ArgumentEffects {
 static String trace="";static int mode;
 static final java.io.IOException failure=new java.io.IOException("checked private argument");
 static final IllegalArgumentException argumentFailure=new IllegalArgumentException("argument producer");
 static ArgumentBox pick(ArgumentBox value){trace+="A";if(mode==1)throw argumentFailure;return value;}
}
class ArgumentDriver {public static void main(String[]args){
 for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int mode:new int[]{0,1,2}){
  ArgumentOwner owner=new ArgumentOwner();ArgumentBox box=new ArgumentBox(seed);java.util.function.IntSupplier delayed=owner.make(box);owner.update(17);box.value=seed+1;
  ArgumentEffects.trace="";ArgumentEffects.mode=mode;
  try{int result=delayed.getAsInt();if(mode==2||EFFECTS&&mode==1||result!=box.value+owner.current())throw new AssertionError("private arguments/delayed state");}
  catch(IllegalArgumentException error){if(!EFFECTS||mode!=1||error!=ArgumentEffects.argumentFailure)throw new AssertionError("argument exception identity",error);}
  catch(IllegalStateException error){if(mode!=2||error.getCause()!=ArgumentEffects.failure)throw new AssertionError("checked cause identity",error);}
  if(!ArgumentEffects.trace.equals(EFFECTS?(mode==1?"A":"AH"):"H")||owner.tail().get()!=13)throw new AssertionError("argument evaluation order/count");
 }System.out.println("private-argument:checked:verified");
}}`

func TestNativeAnonymousPrivateArgumentPacketsRoundTrip(t *testing.T) {
	for _, variant := range []string{"field argument", "effectful argument", "overload erasure"} {
		t.Run(variant, func(t *testing.T) {
			source := strings.NewReplacer("ARG", "value", "EFFECTS", "false").Replace(nativePrivateArgumentFixture)
			if variant == "effectful argument" {
				source = strings.NewReplacer("ARG", "ArgumentEffects.pick(value)", "EFFECTS", "true").Replace(nativePrivateArgumentFixture)
			}
			if variant == "overload erasure" {
				source = strings.Replace(source, "helper(ArgumentBox box)", "helper(Object box)", 1)
				source = strings.Replace(source, "return box.value+first;", "return ((ArgumentBox)box).value+first;", 1)
				source = strings.Replace(source, "public int getAsInt(){return", `private int helper(ArgumentBox wrong){throw new AssertionError("wrong overload");}public int getAsInt(){return`, 1)
				source = strings.Replace(source, "return helper(value);", "return helper((Object)value);", 1)
			}
			t.Run("modern", func(t *testing.T) {
				testNativePrivateSetterFixture(t, source, "ArgumentOwner", "ArgumentDriver", "private-argument:checked:verified\n")
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"ArgumentOwner.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"ArgumentOwner"}, "ArgumentDriver", "private-argument:checked:verified\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}

const nativePrivateWideArgumentsFixture = `class WideArgumentOwner {
 private int first=7,last=13;void update(int value){first=value;}int current(){return first;}
 java.util.function.IntSupplier make(final WideArgumentBox value){return new java.util.function.IntSupplier(){
  private int helper(WideArgumentBox box,long wide,double decimal,float fraction,boolean flag,char letter,Object[] array)throws java.io.IOException{
   WideArgumentEffects.trace+="H";if(WideArgumentEffects.mode==2)throw WideArgumentEffects.failure;
   if(array!=box.array)throw new AssertionError("array identity/varargs");
   return box.value+first+(int)(wide^(wide>>>32))+(int)decimal+(int)fraction+(flag?1:0)+letter;
  }
  private int helper(WideArgumentBox wrong,long wide,double decimal,float fraction,boolean flag,char letter,Object array){throw new AssertionError("wrong array overload");}
  public int getAsInt(){return new java.util.function.IntSupplier(){public int getAsInt(){try{return helper(ARGS);}catch(java.io.IOException error){throw new IllegalStateException(error);}}}.getAsInt();}
 };}
 class Tail{int get(){return last;}}Tail tail(){return new Tail();}
}
class WideArgumentBox {int value;long wide;double decimal;float fraction;boolean flag;char letter;Object[] array;
 WideArgumentBox(int value){this.value=value;wide=((long)value<<32)|0xffffffffL;decimal=value==0?Double.NaN:Double.POSITIVE_INFINITY;fraction=Float.NEGATIVE_INFINITY;flag=value<0;letter=(char)value;array=value==0?null:new Object[]{this};}
}
class WideArgumentEffects {
 static String trace="";static int mode;static final java.io.IOException failure=new java.io.IOException("wide argument target");
 static final IllegalArgumentException argumentFailure=new IllegalArgumentException("wide argument producer");
 static Object box(Object value){trace+="A";return value;}
 static Object box(WideArgumentBox wrong){throw new AssertionError("wrong producer overload");}
 static long wide(WideArgumentBox value){trace+="B";if(mode==1)throw argumentFailure;return value.wide;}
 static double decimal(WideArgumentBox value){trace+="C";return value.decimal;}
 static float fraction(WideArgumentBox value){trace+="D";return value.fraction;}
 static boolean flag(WideArgumentBox value){trace+="E";return value.flag;}
 static char letter(WideArgumentBox value){trace+="F";return value.letter;}
 static Object[] array(WideArgumentBox value){trace+="G";return value.array;}
}
class WideArgumentDriver {public static void main(String[]args){
 for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int mode:new int[]{0,1,2}){
  WideArgumentOwner owner=new WideArgumentOwner();WideArgumentBox box=new WideArgumentBox(seed);java.util.function.IntSupplier delayed=owner.make(box);owner.update(17);box.value=seed+1;
  WideArgumentEffects.trace="";WideArgumentEffects.mode=mode;
  try{int result=delayed.getAsInt();int expected=box.value+owner.current()+(int)(box.wide^(box.wide>>>32))+(int)box.decimal+(int)box.fraction+(box.flag?1:0)+box.letter;if(mode==2||EFFECTS&&mode==1||result!=expected)throw new AssertionError("wide argument result");}
  catch(IllegalArgumentException error){if(!EFFECTS||mode!=1||error!=WideArgumentEffects.argumentFailure)throw new AssertionError("argument exception identity",error);}
  catch(IllegalStateException error){if(mode!=2||error.getCause()!=WideArgumentEffects.failure)throw new AssertionError("checked cause identity",error);}
  if(!WideArgumentEffects.trace.equals(EFFECTS?(mode==1?"AB":"ABCDEFGH"):"H")||owner.tail().get()!=13)throw new AssertionError("wide argument order/count");
 }System.out.println("wide-private-arguments:verified");
}}`

func TestNativeAnonymousPrivateWideArgumentsPreserveSlotsAndEffects(t *testing.T) {
	for _, effects := range []bool{false, true} {
		for _, renamed := range []bool{false, true} {
			name := "fields"
			arguments := "value,value.wide,value.decimal,value.fraction,value.flag,value.letter,value.array"
			effectText := "false"
			if effects {
				name = "effects"
				effectText = "true"
				arguments = "(WideArgumentBox)WideArgumentEffects.box((Object)value),WideArgumentEffects.wide(value),WideArgumentEffects.decimal(value),WideArgumentEffects.fraction(value),WideArgumentEffects.flag(value),WideArgumentEffects.letter(value),WideArgumentEffects.array(value)"
			}
			source := strings.NewReplacer("ARGS", arguments, "EFFECTS", effectText).Replace(nativePrivateWideArgumentsFixture)
			if renamed {
				name += " renamed"
				source = strings.NewReplacer("WideArgument", "OtherArgument", "helper", "calculate", "first", "leading", "last", "trailing", "value", "input").Replace(source)
			}
			root, driver := "WideArgumentOwner", "WideArgumentDriver"
			if renamed {
				root, driver = "OtherArgumentOwner", "OtherArgumentDriver"
			}
			t.Run(name, func(t *testing.T) {
				t.Run("modern", func(t *testing.T) {
					testNativePrivateSetterFixture(t, source, root, driver, "wide-private-arguments:verified\n")
				})
				t.Run("native8", func(t *testing.T) {
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
						return localGetterCompileJavac8(t, map[string]string{root + ".java": source}, debug)
					}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{root}, driver, "wide-private-arguments:verified\n", nil, nativeLexicalExactSignatures)
				})
			})
		}
	}
}
