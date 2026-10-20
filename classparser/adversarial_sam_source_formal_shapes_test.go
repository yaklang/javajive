package javaclassparser

import "testing"

func TestAdversarialSAMSourceFormalsPreserveNestedVarianceAndArrays(t *testing.T) {
	const source = `import java.util.*;import java.util.function.*;import java.lang.reflect.*;
class FormalShapeProbe {static int calls;static Object list,map,array,predicate;static boolean check(String name,List<? extends CharSequence> values,Map<String,? super String> index,List<String>[] batches,Predicate<List<String>> accept){calls++;list=values;map=index;array=batches;predicate=accept;if(values==null||index==null||batches==null||accept==null)return false;index.put("seen",name);return name!=null&&values.size()==2&&batches.length==1&&accept.test(batches[0]);}}
public class UncapturedLambdaNames {public static GenericShapeContract factory(String name){return (values,index,batches,accept)->FormalShapeProbe.check(name,values,index,batches,accept);}}
class LambdaParameterDriver {public static void main(String[]args)throws Exception{Method implementation=null;for(Method m:UncapturedLambdaNames.class.getDeclaredMethods())if(m.isSynthetic()){if(implementation!=null||!Arrays.equals(m.getParameterTypes(),new Class<?>[]{String.class,List.class,Map.class,List[].class,Predicate.class}))throw new AssertionError("physical erased words");implementation=m;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);int rows=0;StringBuilder trace=new StringBuilder();for(String name:new String[]{null,"","left","right"})for(int mask=0;mask<16;mask++)for(int path=0;path<2;path++){List<CharSequence> list=(mask&1)==0?null:Arrays.<CharSequence>asList(name,"right");Map<String,Object> index=(mask&2)==0?null:new LinkedHashMap<String,Object>();List<String>[] batches=(mask&4)==0?null:(List<String>[])new List<?>[]{Arrays.asList(name,"right")};Predicate<List<String>> accept=(mask&8)==0?null:values->values.size()==2;FormalShapeProbe.calls=0;GenericShapeContract subject=UncapturedLambdaNames.factory(name);if(FormalShapeProbe.calls!=0)throw new AssertionError("eager callback");boolean actual=path==0?subject.matches(list,index,batches,accept):(Boolean)implementation.invoke(null,name,list,index,batches,accept);boolean expected=mask==15&&name!=null;if(actual!=expected||FormalShapeProbe.calls!=1||FormalShapeProbe.list!=list||FormalShapeProbe.map!=index||FormalShapeProbe.array!=batches||FormalShapeProbe.predicate!=accept)throw new AssertionError("value/binding "+rows);if(mask==15&&(!index.containsKey("seen")||!Objects.equals(index.get("seen"),name)))throw new AssertionError("effect/order");trace.append(actual?'T':'F');rows++;}if(rows!=128)throw new AssertionError(rows);System.out.println("128:uncaptured:lambda:parameters:"+trace);}}
`
	extras := map[string]string{"GenericShapeContract.java": `import java.util.*;import java.util.function.*;public interface GenericShapeContract{boolean matches(List<? extends CharSequence> values,Map<String,? super String> index,List<String>[] batches,Predicate<List<String>> accept);}`}
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) {
			testUncapturedLambdaParameterMetadataRoundTripWithSources(t, source, extras, 1, "128", named)
		})
	}
}
