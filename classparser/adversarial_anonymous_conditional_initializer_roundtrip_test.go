package javaclassparser

import (
	"strings"
	"testing"
)

// The constructor's branch chooses an initialization computation, not a value
// that can be speculated before SUPER or evaluated on both arms. The unchanged
// driver observes defaults from SUPER and ordered partial stores on failure.
const anonymousConditionalInitializerFixture = `
class ConditionalInitEffects {
 static String trace="";static int fail;static ConditionalInitParent published;
 static final RuntimeException error=new RuntimeException("original");
 static Object arm(Object value,int branch){trace+=branch==1?"L":"R";if(fail==branch)throw error;return value;}
}
abstract class ConditionalInitParent {
 final Object seed;
 ConditionalInitParent(Object seed){ConditionalInitEffects.trace+="P"+first();if(seed==null)throw ConditionalInitEffects.error;this.seed=seed;ConditionalInitEffects.published=this;}
 abstract int first();abstract Object selected();abstract boolean done();
}
class ConditionalInitOwner {
 ConditionalInitParent make(Object seed,final Object selector,final Object left,final Object right){return new ConditionalInitParent(seed){
  int before=17;Object input=selector;
  final Object chosen=input==null?ConditionalInitEffects.arm(left,1):ConditionalInitEffects.arm(right,2);
  boolean after=true;
  int first(){return before;}Object selected(){return chosen;}boolean done(){return after;}
 };}
}
class ConditionalInitDriver {
 public static void main(String[]args)throws Exception {
  Object seed=new Object(),left=new Object(),right=new Object();int rows=0;
  for(Object selector:new Object[]{null,seed})for(int fail:new int[]{0,1,2}){
   int branch=selector==null?1:2;String trace="P0"+(branch==1?"L":"R");
   ConditionalInitEffects.trace="";ConditionalInitEffects.fail=fail;ConditionalInitEffects.published=null;
   try{
    ConditionalInitParent p=new ConditionalInitOwner().make(seed,selector,left,right);
    if(fail==branch||p.seed!=seed||p.first()!=17||p.selected()!=(branch==1?left:right)||!p.done())throw new AssertionError("chosen arm/value/store");
   }catch(RuntimeException error){
    ConditionalInitParent p=ConditionalInitEffects.published;
    if(fail!=branch||error!=ConditionalInitEffects.error||p==null||p.seed!=seed||p.first()!=17||p.selected()!=null||p.done())throw new AssertionError("failure identity/partial stores",error);
   }
   ConditionalInitParent p=ConditionalInitEffects.published;
   if(!ConditionalInitEffects.trace.equals(trace)||!p.getClass().isAnonymousClass()||p.getClass().getEnclosingClass()!=ConditionalInitOwner.class||!p.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("branch order/anonymous identity");
   java.lang.reflect.Field input=p.getClass().getDeclaredField("input");input.setAccessible(true);
   java.lang.reflect.Field chosen=p.getClass().getDeclaredField("chosen");
   if(input.get(p)!=selector||chosen.getModifiers()!=java.lang.reflect.Modifier.FINAL)throw new AssertionError("earlier store/field modifiers");rows++;
  }
  ConditionalInitEffects.trace="";ConditionalInitEffects.fail=0;ConditionalInitEffects.published=null;
  try{new ConditionalInitOwner().make(null,null,left,right);throw new AssertionError("missing parent failure");}
  catch(RuntimeException error){if(error!=ConditionalInitEffects.error||!ConditionalInitEffects.trace.equals("P0")||ConditionalInitEffects.published!=null)throw new AssertionError("parent failure precedes condition");rows++;}
  System.out.println(rows+":conditional:initializer:branch:partial:identity");
 }
}`

func TestAdversarialAnonymousConditionalInitializerPreservesChosenArm(t *testing.T) {
	testNativeIndependentFamilyFixture(t, anonymousConditionalInitializerFixture, []string{"ConditionalInitOwner"}, "ConditionalInitDriver", "7:conditional:initializer:branch:partial:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousConditionalInitializerPreservesInversePredicate(t *testing.T) {
	f := strings.Replace(anonymousConditionalInitializerFixture, "input==null?ConditionalInitEffects.arm(left,1):ConditionalInitEffects.arm(right,2)", "input!=null?ConditionalInitEffects.arm(right,2):ConditionalInitEffects.arm(left,1)", 1)
	f = strings.ReplaceAll(f, "ConditionalInitOwner", "SeparateBranchScope")
	testNativeIndependentFamilyFixture(t, f, []string{"SeparateBranchScope"}, "ConditionalInitDriver", "7:conditional:initializer:branch:partial:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousConditionalInitializerPreservesPredicateCall(t *testing.T) {
	f := strings.Replace(anonymousConditionalInitializerFixture, "input==null?ConditionalInitEffects.arm", "chooseInput()==null?ConditionalInitEffects.arm", 1)
	f = strings.Replace(f, "int first(){return before;}", "Object chooseInput(){ConditionalInitEffects.trace+=\"C\";return input;}int first(){return before;}", 1)
	f = strings.Replace(f, "String trace=\"P0\"+", "String trace=\"P0C\"+", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"ConditionalInitOwner"}, "ConditionalInitDriver", "7:conditional:initializer:branch:partial:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousConditionalInitializerKeepsSuccessiveDiamonds(t *testing.T) {
	f := strings.Replace(anonymousConditionalInitializerFixture, "boolean after=true;", "final Object twice=input==null?ConditionalInitEffects.arm(left,1):ConditionalInitEffects.arm(right,2);boolean after=true;", 1)
	f = strings.Replace(f, "String trace=\"P0\"+(branch==1?\"L\":\"R\");", "String trace=\"P0\"+(branch==1?\"L\":\"R\")+(fail==branch?\"\":(branch==1?\"L\":\"R\"));", 1)
	f = strings.Replace(f, "java.lang.reflect.Field input=p.getClass()", "java.lang.reflect.Field twice=p.getClass().getDeclaredField(\"twice\");twice.setAccessible(true);if(twice.get(p)!=(fail==branch?null:(branch==1?left:right)))throw new AssertionError(\"second diamond result/order\");java.lang.reflect.Field input=p.getClass()", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"ConditionalInitOwner"}, "ConditionalInitDriver", "7:conditional:initializer:branch:partial:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousConditionalInitializerKeepsFieldArmIdentity(t *testing.T) {
	f := strings.Replace(anonymousConditionalInitializerFixture, "final Object chosen=input==null?ConditionalInitEffects.arm(left,1):ConditionalInitEffects.arm(right,2);", "Object leftSaved=left;Object rightSaved=right;final Object chosen=input==null?leftSaved:rightSaved;", 1)
	f = strings.Replace(f, "new int[]{0,1,2}", "new int[]{0}", 1)
	f = strings.Replace(f, "String trace=\"P0\"+(branch==1?\"L\":\"R\");", "String trace=\"P0\";", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"ConditionalInitOwner"}, "ConditionalInitDriver", "3:conditional:initializer:branch:partial:identity\n", nativeLexicalExactSignatures)
}
