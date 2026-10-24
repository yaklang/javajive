package javaclassparser

import "testing"

// Two distinct original factories can have identical source bodies and names
// for every captured/SAM parameter. Their actual compiler registration remains
// observable through the unchanged binary probe's stack frames. A separate
// method exercises a later generic-result cast outside the lambda packet.
const lambdaRegistrationOccurrenceFixture = `
import java.util.*;import java.util.function.*;
public class UncapturedLambdaNames {
 public static Function<String,String> choose(boolean branch,String capture){if(branch){return(String item)->{LambdaOccurrenceProbe.see();return capture+item;};}else{return(String item)->{LambdaOccurrenceProbe.see();return capture+item;};}}
 public static String binding(String capture,List<String> values){Predicate<String> gate=(String item)->item==capture;String result=values.size()==1?values.get(0):pick(values);return gate.test(capture)?result:null;}
 private static <T>T pick(List<T> values){return values.get(values.size()-1);}
}
class LambdaOccurrenceProbe {
 static String last;
 static void see(){last=null;for(StackTraceElement frame:new Throwable().getStackTrace())if(frame.getClassName().equals("UncapturedLambdaNames")){last=frame.getMethodName();break;}if(last==null)throw new AssertionError("missing original lambda frame");}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception {
  List<String> declarations=new ArrayList<String>();int rows=0;
  for(boolean branch:new boolean[]{false,true})for(String capture:new String[]{null,"", "seed"})for(String item:new String[]{null,"", "item"}){
   Function<String,String> operation=UncapturedLambdaNames.choose(branch,capture);String actual=operation.apply(item);if(!actual.equals(String.valueOf(capture)+String.valueOf(item)))throw new AssertionError("capture/SAM identity");
   declarations.add("site:"+branch+":"+rows+":"+LambdaOccurrenceProbe.last);rows++;
  }
  for(String capture:new String[]{null,"", "seed"})for(List<String> values:Arrays.asList(Arrays.asList((String)null),Arrays.asList("first"),Arrays.asList("first","last"))){if(UncapturedLambdaNames.binding(capture,values)!=values.get(values.size()-1))throw new AssertionError("selected generic return identity");rows++;}
  for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());
   for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialLambdaRegistrationDistinguishesIdenticalFactoryOccurrences(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, lambdaRegistrationOccurrenceFixture, 3, "27")
}
