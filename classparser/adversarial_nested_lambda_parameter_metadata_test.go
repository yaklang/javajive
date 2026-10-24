package javaclassparser

import "testing"

// The nested map body has no captures; the predicate captures its enclosing
// lambda's SAM formal. The outer factory captures an ordinary parameter. These
// are distinct source scopes even though all implementations are static JVM
// methods. The unchanged driver models stream laziness, faults and short cuts.
const nestedLambdaParameterFixture = `
import java.util.*;import java.util.function.*;
public class UncapturedLambdaNames {
 public static Predicate<Object> nested(Collection<?> types){return(Object item)->types.stream().map((Object type)->type instanceof Class?((Class<?>)type).getName():type.toString()).anyMatch((String type)->type.equals(item.getClass().getName()));}
}
class NestedNameProbe {
 final String text,tag;final boolean fail;int calls;static String trace="";static final RuntimeException error=new RuntimeException("original");
 NestedNameProbe(String text,String tag,boolean fail){this.text=text;this.tag=tag;this.fail=fail;}
 public String toString(){calls++;trace+=tag;if(fail)throw error;return text;}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception {
  NestedNameProbe good=new NestedNameProbe("java.lang.String","G",false),bad=new NestedNameProbe("never","B",true);
  Object[][] banks={null,{}, {String.class,Integer.class},{"java.lang.Integer",good},{bad,String.class},{Object.class,bad},{null,good},{Integer.class,good,bad}};
  int rows=0;
  for(Object[] bank:banks)for(Object item:new Object[]{null,"value",Integer.valueOf(7),new Object(),String.class})for(boolean mutate:new boolean[]{false,true}){
   List<Object> types=bank==null?null:new ArrayList<Object>(Arrays.asList(bank));Predicate<Object> operation=UncapturedLambdaNames.nested(types);
   if(mutate&&types!=null)types.add(0,"java.lang.String");
   boolean expected=false;Class<?> errorType=null;Throwable errorIdentity=null;String expectedTrace="";int goodCalls=0,badCalls=0;
   if(types==null){errorType=NullPointerException.class;}else for(Object type:types){
    String name=null;
    if(type==null){errorType=NullPointerException.class;break;}
    if(type instanceof NestedNameProbe){NestedNameProbe probe=(NestedNameProbe)type;expectedTrace+=probe.tag;if(probe==good)goodCalls++;else badCalls++;if(probe.fail){errorIdentity=NestedNameProbe.error;break;}name=probe.text;}
    else if(type instanceof Class){name=((Class<?>)type).getName();}else{name=(String)type;}
    if(item==null){errorType=NullPointerException.class;break;}
    if(name.equals(item.getClass().getName())){expected=true;break;}
   }
   NestedNameProbe.trace="";good.calls=bad.calls=0;boolean actual=false;Throwable caught=null;try{actual=operation.test(item);}catch(Throwable error){caught=error;}
   if(errorIdentity!=null?caught!=errorIdentity:errorType!=null?(caught==null||caught.getClass()!=errorType):caught!=null||actual!=expected)throw new AssertionError("world="+rows+" lazy map/predicate/failure",caught);
   if(!NestedNameProbe.trace.equals(expectedTrace)||good.calls!=goodCalls||bad.calls!=badCalls)throw new AssertionError("world="+rows+" deferred capture/order/once-only/short cut");rows++;
  }
  List<String> declarations=new ArrayList<String>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());
   for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));
  System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialNestedLambdaMetadataKeepsLexicalCaptureAndRegistration(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, nestedLambdaParameterFixture, 3, "80")
}

// Multiple lexical depths, a category-2 SAM capture and two identical nested
// factory trees constrain the same registration algorithm. The binary probe
// observes the original implementation names, not merely equal return values.
const nestedLambdaRegistrationForestFixture = `
import java.util.*;import java.util.function.*;
public class UncapturedLambdaNames {
 public static Function<String,Function<String,Function<String,String>>> chain(){return(String first)->(String second)->(String third)->String.valueOf(second)+String.valueOf(third);}
 public static LongFunction<LongPredicate> wide(){return(final long first)->(long second)->(first^second)==-1L;}
 public static Function<String,Predicate<String>> same(boolean branch){if(branch){return(String first)->(String second)->{NestedForestProbe.see();return first==second;};}else{return(String first)->(String second)->{NestedForestProbe.see();return first==second;};}}
}
class NestedForestProbe {
 static String last;
 static void see(){last=null;for(StackTraceElement frame:new Throwable().getStackTrace())if(frame.getClassName().equals("UncapturedLambdaNames")){last=frame.getMethodName();break;}if(last==null)throw new AssertionError("missing nested implementation frame");}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception {
  int rows=0;List<String> declarations=new ArrayList<String>();String[] values={null,"",new String("seed")};
  for(String first:values)for(String second:values)for(String third:values){Function<String,Function<String,Function<String,String>>> outer=UncapturedLambdaNames.chain();String actual=outer.apply(first).apply(second).apply(third);if(!actual.equals(String.valueOf(second)+String.valueOf(third)))throw new AssertionError("deep lexical binder");rows++;}
  long[] edges={0L,1L,-1L,Long.MIN_VALUE,Long.MAX_VALUE,1L<<32};for(long first:edges)for(long second:edges){LongPredicate predicate=UncapturedLambdaNames.wide().apply(first);if(predicate.test(second)!=((first^second)==-1L))throw new AssertionError("wide SAM capture");rows++;}
  for(boolean branch:new boolean[]{false,true})for(String first:values)for(String second:values){Predicate<String> predicate=UncapturedLambdaNames.same(branch).apply(first);if(predicate.test(second)!=(first==second))throw new AssertionError("same text distinct original nested factory");declarations.add("site:"+branch+":"+rows+":"+NestedForestProbe.last);rows++;}
  for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialNestedLambdaRegistrationKeepsFactoryForestAndWideSAM(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, nestedLambdaRegistrationForestFixture, 9, "81")
}
