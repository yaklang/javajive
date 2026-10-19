package javaclassparser

import (
	"strings"
	"testing"
)

// A local binder is neither an enclosing formal nor an arbitrary expression.
// Its typed reaching definition and the invokedynamic snapshot must survive
// source emission before that binder can reproduce a physical capture name.
const localLambdaParameterFixture = `
import java.util.*;import java.util.function.*;
public class UncapturedLambdaNames {
 public static IntUnaryOperator array(int initial){final int[] state=new int[]{initial};return(int delta)->{int previous=state[0];state[0]=previous+delta;return previous;};}
 public static LongSupplier wide(long initial){final long shifted=initial^17L;return()->shifted;}
 public static IntSupplier constant(){int captured=7;return()->captured;}
 public static Map<Object,Object> singles(Map<Object,List<Object>> source){Map<Object,Object> single=new LinkedHashMap<>();source.forEach((Object key,List<Object> values)->{if(values!=null&&!values.isEmpty())single.put(key,values.get(0));});return single;}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;int[] edges={Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE};
  if(UncapturedLambdaNames.constant().getAsInt()!=7||UncapturedLambdaNames.constant().getAsInt()!=7)throw new AssertionError("constant initializer must keep the original capture parameter");rows++;
  for(int initial:edges)for(int delta:edges){IntUnaryOperator operation=UncapturedLambdaNames.array(initial);if(operation.applyAsInt(delta)!=initial||operation.applyAsInt(~delta)!=initial+delta||operation.applyAsInt(0)!=initial+delta+~delta)throw new AssertionError("captured local identity and deferred writes");rows++;}
  for(long initial:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){LongSupplier operation=UncapturedLambdaNames.wide(initial);if(operation.getAsLong()!=(initial^17L)||operation.getAsLong()!=(initial^17L))throw new AssertionError("wide local definition");rows++;}
  Object token=new Object();List<Object> empty=Collections.emptyList(),one=Arrays.asList(token),nil=Arrays.asList((Object)null),many=Arrays.asList(token,new Object());
  List<?>[] banks={null,Collections.emptyList(),Arrays.asList((Object)null),Arrays.asList(empty),Arrays.asList(one),Arrays.asList(nil),Arrays.asList(many),Arrays.asList(empty,null,one,nil,many)};
  for(List<?> bank:banks){Map<Object,List<Object>> input=null;Map<Object,Object> expected=new LinkedHashMap<>();if(bank!=null){input=new LinkedHashMap<>();int i=0;for(Object entry:bank){@SuppressWarnings("unchecked")List<Object> values=(List<Object>)entry;input.put(i,values);if(values!=null&&values.size()>0)expected.put(i,values.get(0));i++;}}
   Map<Object,Object> actual=null;Throwable caught=null;try{actual=UncapturedLambdaNames.singles(input);}catch(Throwable error){caught=error;}
   if(input==null?(caught==null||caught.getClass()!=NullPointerException.class):(caught!=null||actual==null||actual.getClass()!=LinkedHashMap.class||!actual.keySet().equals(expected.keySet())))throw new AssertionError("capture map/short cut",caught);
   if(actual!=null)for(Object key:expected.keySet())if(actual.get(key)!=expected.get(key))throw new AssertionError("captured map first reference identity");rows++;
  }
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialLocalLambdaMetadataRequiresTypedReachingDefinitionAndSourceBinder(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, localLambdaParameterFixture, 4, "39")
}

func TestAdversarialLocalLambdaMetadataKeepsGeneratedAndFieldNamespaceBindings(t *testing.T) {
	source := strings.Replace(localLambdaParameterFixture, "public class UncapturedLambdaNames {", "public class UncapturedLambdaNames { static class Anchor {} public static int state=73;public static Map<Object,Object> single=new LinkedHashMap<>();", 1)
	source = strings.Replace(source, "  List<String> declarations=", "  if(UncapturedLambdaNames.state!=73||!UncapturedLambdaNames.single.isEmpty())throw new AssertionError(\"local capture cannot bind same-named own static field\");\n  List<String> declarations=", 1)
	source = strings.Replace(source, "wide(long initial){final long shifted=initial^17L;return()->shifted;}", "wide(long var1){final long var0=var1^17L;return()->var0;}", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 4, "39")
}
