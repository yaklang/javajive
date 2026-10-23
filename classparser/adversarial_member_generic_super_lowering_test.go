package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialConstructorLoweringAlignsEnclosingAndGenericSuperParameters(t *testing.T) {
	for _, bound := range []string{"Object", "Runnable"} {
		for _, shape := range []string{"plain", "method formal", "wide slots", "renamed"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(bound+"/"+shape+"/"+debug, func(t *testing.T) {
					source := constructorEnclosingGenericFixture(bound)
					owners := []string{"SuffixOwner$Member", "SuffixOwner$Base"}
					switch shape {
					case "method formal":
						source = strings.Replace(source, "Base(T task)", "<U extends T> Base(U task)", 1)
					case "wide slots":
						source = strings.Replace(source, "Base(T task){", "Base(long first,T task,double last){if(first!=Long.MIN_VALUE||Double.doubleToRawLongBits(last)!=Long.MIN_VALUE)throw new AssertionError(\"wide ports\");", 1)
						source = strings.Replace(source, "Member(SuffixAction task){super(task);}", "Member(long first,SuffixAction task,double last){super(first,task,last);}", 1)
						source = strings.Replace(source, "new Member(action)", "new Member(Long.MIN_VALUE,action,-0.0d)", 1)
					case "renamed":
						source = strings.NewReplacer("Base", "GenericParent", "Member", "ConcreteChild", "<T extends", "<P extends", "final T task", "final P task", "GenericParent(T task)", "GenericParent(P task)").Replace(source)
						// Apply formal renaming after the class renaming independently.
						source = strings.Replace(source, "GenericParent(T task)", "GenericParent(P task)", 1)
						owners = []string{"SuffixOwner$ConcreteChild", "SuffixOwner$GenericParent"}
					}
					constructorSuffixFamilyRoundTrip(t, source, debug, owners, "6:generic:enclosing:capture:identity:failure\n")
				})
			}
		}
	}
}

func constructorEnclosingGenericFixture(bound string) string {
	return strings.ReplaceAll(`class SuffixWitness {static SuffixOwner outer;static SuffixOwner.Base escaped;static RuntimeException failure;}
class SuffixOwner {
 final Object token;SuffixOwner(Object token){this.token=token;}
 class Base<T extends BOUND> {final T task;Base(T task){SuffixWitness.escaped=this;if(captured()!=SuffixOwner.this.token)throw new AssertionError("pre-super capture");if(task==null)throw SuffixWitness.failure;this.task=task;}Object captured(){return null;}}
 class Member extends Base<SuffixAction> {Member(SuffixAction task){super(task);}Object captured(){return SuffixOwner.this.token;}}
 Member make(SuffixAction action){return new Member(action);}
}
class SuffixAction implements Runnable {public void run(){throw new AssertionError("construction must not invoke operand");}}
class SuffixDriver {public static void main(String[]args){int rows=0;
 java.lang.reflect.Constructor<?> constructor=SuffixOwner.Base.class.getDeclaredConstructors()[0];
 java.lang.reflect.Type[] generic=constructor.getGenericParameterTypes();
 if(generic.length+1!=constructor.getParameterTypes().length)throw new AssertionError("implicit generic port");
 int variables=0;for(java.lang.reflect.Type p:generic)if(p instanceof java.lang.reflect.TypeVariable)variables++;
 if(variables!=1)throw new AssertionError("generic parameter identity");
 for(Object token:new Object[]{null,"token",new Object()})for(boolean fail:new boolean[]{false,true}){
  SuffixOwner owner=new SuffixOwner(token);SuffixWitness.escaped=null;SuffixWitness.failure=new IllegalStateException("original");SuffixAction action=fail?null:new SuffixAction();SuffixOwner.Member result=null;Throwable error=null;
  try{result=owner.make(action);}catch(Throwable e){error=e;}
  if(SuffixWitness.escaped==null||SuffixWitness.escaped.captured()!=token||error!=(fail?SuffixWitness.failure:null)||!fail&&(result!=SuffixWitness.escaped||result.task!=action))throw new AssertionError("generic enclosing binding",error);rows++;
 }System.out.println(rows+":generic:enclosing:capture:identity:failure");}}
`, "BOUND", bound)
}
