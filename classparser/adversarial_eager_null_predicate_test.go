package javaclassparser

import (
	"os"
	"testing"
)

func TestAdversarialEagerNullPredicatesKeepStateAndEffects(t *testing.T) {
	const fixture = `class PendingToken{} public class PredicateState {public Object value;public int writes;public static int effects;public static boolean fail;public static final RuntimeException FAILURE=new IllegalStateException("same");public static boolean right(boolean value){effects++;if(fail)throw FAILURE;return value;}
 public static boolean orNull(Object value){return (value==null)|(value instanceof PendingToken);}public static boolean andNotNull(Object value){return (value!=null)&(value instanceof PendingToken);}public static boolean xorNull(Object value){return (value==null)^(value instanceof PendingToken);}
 public static boolean orCall(Object value,boolean other){return (value==null)|right(other);}public static boolean andCall(Object value,boolean other){return (value==null)&right(other);}public static boolean xorCall(Object value,boolean other){return (value==null)^right(other);}
 public static boolean ordered(int a,int b){return (a<b)|right(b!=0);}public static boolean floating(double a,double b){return (a<b)^right(false);}
 public boolean move(Object next){Object current=value;boolean moved=false;if((current==null)|(current instanceof PendingToken)){value=next;writes++;moved=true;}return moved;}}
class PredicateDriver {public static void main(String[]args){Object[] inputs={null,new PendingToken(),new Object()};int count=0;for(int i=0;i<inputs.length;i++){Object value=inputs[i];if(PredicateState.orNull(value)!=(i<2)||PredicateState.andNotNull(value)!=(i==1)||PredicateState.xorNull(value)!=(i<2))throw new AssertionError("predicate "+i);count+=3;
 PredicateState state=new PredicateState();state.value=value;Object next=new Object();boolean moved=state.move(next);if(moved!=(i<2)||state.value!=(i<2?next:value)||state.writes!=(i<2?1:0))throw new AssertionError("state "+i);count++;
 for(boolean other:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true})for(int kind=0;kind<3;kind++){PredicateState.effects=0;PredicateState.fail=fail;boolean result=false;Throwable error=null;try{result=kind==0?PredicateState.orCall(value,other):kind==1?PredicateState.andCall(value,other):PredicateState.xorCall(value,other);}catch(Throwable t){error=t;}boolean want=kind==0?i==0||other:kind==1?i==0&&other:(i==0)!=other;if(PredicateState.effects!=1||(fail?error!=PredicateState.FAILURE:error!=null||result!=want))throw new AssertionError("eager "+i+":"+kind+":"+other+":"+fail,error);count++;}}
 PredicateState.fail=false;for(int a:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})for(int b:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}){PredicateState.effects=0;if(PredicateState.ordered(a,b)!=(a<b||b!=0)||PredicateState.effects!=1)throw new AssertionError("ordered");count++;}for(double a:new double[]{Double.NaN,Double.NEGATIVE_INFINITY,-0.0,0.0,Double.POSITIVE_INFINITY})for(double b:new double[]{Double.NaN,Double.NEGATIVE_INFINITY,-0.0,0.0,Double.POSITIVE_INFINITY}){PredicateState.effects=0;if(PredicateState.floating(a,b)!=(a<b)||PredicateState.effects!=1)throw new AssertionError("floating");count++;}
 System.out.println(count+":null:eager:state:effects:exception-identity");}}
`
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "PredicateState", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"PredicateState"}, "PredicateDriver", "89:null:eager:state:effects:exception-identity\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}
