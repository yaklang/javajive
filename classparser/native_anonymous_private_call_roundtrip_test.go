package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const anonymousPrivateCallFixture = `class PrivateCallOwner {
 private TYPE first=FIRST,last=LAST;
 SUPPLIER make(final TYPE seed){return new SUPPLIER(){
  private TYPE helper(){BODY}
  OVERLOAD
  public TYPE METHOD(){return new SUPPLIER(){public TYPE METHOD(){return helper();}}.METHOD();}
 };}
 class Tail {TYPE get(){return last;}}
 Tail tail(){return new Tail();}
 TYPE firstValue(){return first;} void firstValue(TYPE value){first=value;} TYPE lastValue(){return last;}
}
class PrivateCallEffects {
 static String trace="";static boolean fail;
 static final IllegalArgumentException failure=new IllegalArgumentException("private helper");
}
class PrivateCallDriver {
 public static void main(String[]args){
  for(TYPE seed:new TYPE[]{SEEDS})for(boolean fail:new boolean[]{false,true}){
   PrivateCallOwner owner=new PrivateCallOwner();PrivateCallEffects.trace="";PrivateCallEffects.fail=fail;
   SUPPLIER delayed=owner.make(seed);owner.firstValue(CHANGED);
   try {TYPE result=delayed.METHOD();
    if(EFFECTS&&fail||result!=EXPECTED)throw new AssertionError("private target/identity/overflow");
   } catch(IllegalArgumentException error){
    if(!EFFECTS||!fail||error!=PrivateCallEffects.failure)throw new AssertionError("private exception identity",error);
   }
   if(!PrivateCallEffects.trace.equals(EFFECTS?"H":"")||owner.tail().get()!=owner.lastValue())
    throw new AssertionError("private call once/order/shared named accessor");
  }
  System.out.println("private-call:lexical:verified");
 }
}`

func TestNativeAnonymousPrivateCallLexicalRoundTrip(t *testing.T) {
	for _, shape := range []struct{ name, typ, supplier, method, first, last, changed, seeds, body, expected string }{
		{"int", "int", "java.util.function.IntSupplier", "getAsInt", "7", "13", "17", "Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE", "return seed+first;", "seed+owner.firstValue()"},
		{"long", "long", "java.util.function.LongSupplier", "getAsLong", "7L", "13L", "17L", "Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE", "return seed+first;", "seed+owner.firstValue()"},
		{"reference", "Object", "java.util.function.Supplier<Object>", "get", "new Object()", "new Object()", "new Object()", "null,new Object(),new Object()", "return seed==null?first:seed;", "(seed==null?owner.firstValue():seed)"},
	} {
		t.Run(shape.name, func(t *testing.T) {
			for _, variant := range []string{"plain", "effects", "overload", "captured names", "renamed"} {
				t.Run(variant, func(t *testing.T) {
					body, overload, effect := shape.body, "", "false"
					if variant == "effects" {
						body = `PrivateCallEffects.trace+="H";if(PrivateCallEffects.fail)throw PrivateCallEffects.failure;` + body
						effect = "true"
					}
					if variant == "overload" {
						overload = `private ` + shape.typ + ` helper(int unused){throw new AssertionError("wrong overload");}`
					}
					source := strings.NewReplacer("BODY", body, "OVERLOAD", overload, "EFFECTS", effect, "EXPECTED", shape.expected, "SEEDS", shape.seeds, "SUPPLIER", shape.supplier, "METHOD", shape.method, "TYPE", shape.typ, "FIRST", shape.first, "LAST", shape.last, "CHANGED", shape.changed).Replace(anonymousPrivateCallFixture)
					if variant == "captured names" {
						before := "return new " + shape.supplier + "(){public " + shape.typ + " " + shape.method + "(){return helper();}}." + shape.method + "();"
						after := "final " + shape.typ + " helper=seed;return new " + shape.supplier + "(){public " + shape.typ + " " + shape.method + "(){" + shape.typ + " value=helper();if(value==helper)return helper;return value;}}." + shape.method + "();"
						changed := strings.Replace(source, before, after, 1)
						if changed == source {
							t.Fatal("captured-name variant did not change original source")
						}
						source = changed
					}
					owner := "PrivateCallOwner"
					if variant == "renamed" {
						owner = "RenamedPrivateCallScope"
						source = strings.NewReplacer("PrivateCallOwner", owner, "helper", "dispatch", "Tail", "Side").Replace(source)
					}
					t.Run("modern", func(t *testing.T) {
						testNativePrivateSetterFixture(t, source, owner, "PrivateCallDriver", "private-call:lexical:verified\n")
					})
					t.Run("native8", func(t *testing.T) {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "PrivateCallDriver", "private-call:lexical:verified\n", nil, nativeLexicalExactSignatures)
					})
				})
			}
		})
	}
}

func TestNativeAnonymousPrivateVoidCallKeepsDelayedEffectsAndExceptionIdentity(t *testing.T) {
	fixture := `class PrivateVoidOwner {
 private int first=7,last=13;
 void firstValue(int value){first=value;}
 Runnable make(){return new Runnable(){
  private void helper(){PrivateVoidEffects.trace+="H"+first;if(PrivateVoidEffects.fail)throw PrivateVoidEffects.failure;}
  public void run(){new Runnable(){public void run(){helper();}}.run();}
 };}
 class Tail{int get(){return last;}} Tail tail(){return new Tail();}
}
class PrivateVoidEffects {static String trace="";static boolean fail;static final IllegalArgumentException failure=new IllegalArgumentException("private void");}
class PrivateVoidDriver {public static void main(String[]args){
 for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){
  PrivateVoidOwner owner=new PrivateVoidOwner();Runnable delayed=owner.make();owner.firstValue(value);
  PrivateVoidEffects.trace="";PrivateVoidEffects.fail=fail;
  try{delayed.run();if(fail)throw new AssertionError("lost exception");}
  catch(IllegalArgumentException error){if(!fail||error!=PrivateVoidEffects.failure)throw new AssertionError("exception identity",error);}
  if(!PrivateVoidEffects.trace.equals("H"+value)||owner.tail().get()!=13)throw new AssertionError("void call once/delayed read/shared accessor");
 }System.out.println("private-void:lexical:verified");
}}`
	for _, renamed := range []bool{false, true} {
		owner, source := "PrivateVoidOwner", fixture
		if renamed {
			owner = "OtherVoidScope"
			source = strings.NewReplacer("PrivateVoidOwner", owner, "helper", "dispatch", "Tail", "Side").Replace(source)
		}
		t.Run(owner, func(t *testing.T) {
			t.Run("modern", func(t *testing.T) {
				testNativePrivateSetterFixture(t, source, owner, "PrivateVoidDriver", "private-void:lexical:verified\n")
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "PrivateVoidDriver", "private-void:lexical:verified\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}
