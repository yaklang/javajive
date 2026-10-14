package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const constructorReferenceCapabilityFixture = `class ReferenceParent {final Object[] values;ReferenceParent(Object[] values){this.values=values;}}
class ReferenceOwner {class Child extends ReferenceParent {Child(){this(new TYPE[]{LEFT,RIGHT});}Child(Object[] values){super(values);}ReferenceOwner root(){return ReferenceOwner.this;}Object at(int i){return values[i];}}Child child(){return new Child();}}
class ReferenceDriver {public static void main(String[]args){ReferenceOwner owner=new ReferenceOwner();ReferenceOwner.Child child=owner.child();if(child.root()!=owner||child.at(0)!=LEFT||child.at(1)!=RIGHT)throw new AssertionError("constructor interface-array identity/order");System.out.println("reference:delegation:2");}}`

func TestNativeConstructorReferenceCapabilitiesRoundTrip(t *testing.T) {
	for _, shape := range []struct{ name, typ, left, right string }{{"interface", "java.nio.file.OpenOption", "java.nio.file.StandardOpenOption.READ", "java.nio.file.StandardOpenOption.WRITE"}, {"generic-interface", "java.lang.Comparable<?> ", "java.util.concurrent.TimeUnit.SECONDS", "java.util.concurrent.TimeUnit.DAYS"}, {"temporal-interface", "java.time.temporal.TemporalUnit", "java.time.temporal.ChronoUnit.SECONDS", "java.time.temporal.ChronoUnit.DAYS"}} {
		t.Run(shape.name, func(t *testing.T) {
			for _, variant := range []string{"array", "conditional nullable", "later covariant array", "effects and failure", "overload binding", "renamed"} {
				t.Run(variant, func(t *testing.T) {
					owner, source := "ReferenceOwner", constructorReferenceCapabilityFixture
					if variant == "conditional nullable" {
						source = strings.Replace(source, "Child(){this(new TYPE[]{LEFT,RIGHT});}", "Child(){this(false);}Child(boolean flip){this(new TYPE[]{flip?RIGHT:LEFT,flip?LEFT:null});}", 1)
						source = source[:strings.Index(source, "class ReferenceDriver")] + `class ReferenceDriver {public static void main(String[]args)throws Exception{
 for(boolean flip:new boolean[]{false,true})for(ReferenceOwner owner:new ReferenceOwner[]{null,new ReferenceOwner()}){
  java.lang.reflect.Constructor<ReferenceOwner.Child> c=ReferenceOwner.Child.class.getDeclaredConstructor(ReferenceOwner.class,boolean.class);c.setAccessible(true);
  ReferenceOwner.Child child=c.newInstance(owner,flip);
  if(child.root()!=owner||child.at(0)!=(flip?RIGHT:LEFT)||child.at(1)!=(flip?LEFT:null))throw new AssertionError("nullable enclosing/merged interface producer");
 }System.out.println("reference:delegation:2");}}`
					} else if variant == "later covariant array" {
						source = strings.Replace(source, "Child(){this(new TYPE[]{LEFT,RIGHT});}Child(Object[] values){super(values);}", "private final Object[] later;Child(){this(false);}Child(boolean flip){this(new TYPE[]{flip?RIGHT:LEFT,null},new TYPE[]{LEFT,RIGHT});}Child(Object[] values,Object[] extra){super(values);later=extra;}Object later(int i){return later[i];}", 1)
						source = source[:strings.Index(source, "class ReferenceDriver")] + `class ReferenceDriver {public static void main(String[]args)throws Exception{
 for(boolean flip:new boolean[]{false,true})for(ReferenceOwner owner:new ReferenceOwner[]{null,new ReferenceOwner()}){
  java.lang.reflect.Constructor<ReferenceOwner.Child> c=ReferenceOwner.Child.class.getDeclaredConstructor(ReferenceOwner.class,boolean.class);c.setAccessible(true);
  ReferenceOwner.Child child=c.newInstance(owner,flip);
  if(child.root()!=owner||child.at(0)!=(flip?RIGHT:LEFT)||child.at(1)!=null||child.later(0)!=LEFT||child.later(1)!=RIGHT)throw new AssertionError("two covariant operands/allocation identity");
 }System.out.println("reference:delegation:2");}}`
					} else if variant == "effects and failure" {
						source = strings.Replace(source, "new TYPE[]{LEFT,RIGHT}", "new TYPE[]{ReferenceEffects.pick(false),ReferenceEffects.pick(true)}", 1)
						source = source[:strings.Index(source, "class ReferenceDriver")] + `class ReferenceEffects {
 static String trace="";static boolean fail;static final IllegalArgumentException failure=new IllegalArgumentException("second operand");
 static TYPE pick(boolean second){trace+=second?"R":"L";if(second&&fail)throw failure;return second?RIGHT:LEFT;}
}
class ReferenceDriver {public static void main(String[]args){
 for(boolean fail:new boolean[]{false,true}){ReferenceEffects.trace="";ReferenceEffects.fail=fail;ReferenceOwner owner=new ReferenceOwner();
  try{ReferenceOwner.Child child=owner.child();if(fail||child.root()!=owner||child.at(0)!=LEFT||child.at(1)!=RIGHT)throw new AssertionError("success/identity");}
  catch(IllegalArgumentException e){if(!fail||e!=ReferenceEffects.failure)throw new AssertionError("exception identity",e);}
  if(!ReferenceEffects.trace.equals("LR"))throw new AssertionError("operand order/count");
 }System.out.println("reference:delegation:2");}}`
					} else if variant == "overload binding" {
						source = strings.Replace(source, "Child(){this(new TYPE[]{LEFT,RIGHT});}", "Child(){this((Object[])new TYPE[]{LEFT,RIGHT});}Child(TYPE[] wrong){super(new Object[]{null,null});}", 1)
					} else if variant == "renamed" {
						owner = "OtherReferenceScope"
						source = strings.ReplaceAll(source, "ReferenceOwner", owner)
						source = strings.ReplaceAll(source, "Child", "Worker")
					}
					source = strings.NewReplacer("TYPE", shape.typ, "LEFT", shape.left, "RIGHT", shape.right).Replace(source)
					t.Run("modern", func(t *testing.T) {
						testNativePrivateSetterFixture(t, source, owner, "ReferenceDriver", "reference:delegation:2\n")
					})
					t.Run("native8", func(t *testing.T) {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "ReferenceDriver", "reference:delegation:2\n", nil, nativeLexicalExactSignatures)
					})
				})
			}
		})
	}
}
