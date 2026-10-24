package javaclassparser

import "testing"

func TestAdversarialNamedSAMFormalKeepsDeclaredGenericContract(t *testing.T) {
	const source = `import java.util.function.*;import java.lang.reflect.*;import java.util.*;

class FormalPredicate implements Predicate<String>{static int calls;static final RuntimeException FAILURE=new RuntimeException("original predicate");final int mode;FormalPredicate(int mode){this.mode=mode;}public boolean test(String value){calls++;if(mode==2)throw FAILURE;return mode==0?value==null:"left".equals(value);}}
public class UncapturedLambdaNames {
 public static GenericProfileContract factory(String name){return active->active.test(name);}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{String.class,Predicate.class}))throw new AssertionError("physical SAM");implementation=method;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);int rows=0;StringBuilder trace=new StringBuilder();for(String name:new String[]{null,"","left","right"})for(int mode=0;mode<4;mode++)for(int path=0;path<2;path++){FormalPredicate.calls=0;Predicate<String> predicate=mode==3?null:new FormalPredicate(mode);GenericProfileContract subject=UncapturedLambdaNames.factory(name);if(FormalPredicate.calls!=0)throw new AssertionError("eager call");Throwable caught=null;boolean result=false;try{result=path==0?subject.matches(predicate):(Boolean)implementation.invoke(null,name,predicate);}catch(Throwable error){caught=path==1&&error instanceof InvocationTargetException?error.getCause():error;}if(mode==2){if(caught!=FormalPredicate.FAILURE||FormalPredicate.calls!=1)throw new AssertionError("exception identity",caught);trace.append('E');}else if(mode==3){if(!(caught instanceof NullPointerException)||FormalPredicate.calls!=0)throw new AssertionError("null boundary",caught);trace.append('N');}else{boolean expected=mode==0?name==null:"left".equals(name);if(caught!=null||result!=expected||FormalPredicate.calls!=1)throw new AssertionError("value/order "+rows,caught);trace.append(result?'T':'F');}rows++;}if(rows!=32)throw new AssertionError(rows);System.out.println("32:uncaptured:lambda:parameters:"+trace);}
}`
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) {
			testUncapturedLambdaParameterMetadataRoundTripWithSources(t, source, map[string]string{"GenericProfileContract.java": `import java.util.function.Predicate; public interface GenericProfileContract { boolean matches(Predicate<String> active); }`}, 1, "32", named)
		})
	}
}
