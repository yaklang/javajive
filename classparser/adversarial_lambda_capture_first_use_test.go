package javaclassparser

import "testing"

// Inlining the real field-read result into set(dest, get(src)) preserves JVM
// effects, but changes javac's lexical capture discovery from src,dest to
// dest,src. The original binary caller also observes those implementation
// parameters. A repair must retain the real data dependency, not insert unused
// reads merely to steer compiler numbering.
const lambdaCaptureFirstUseFixture = `
import java.lang.reflect.*;import java.util.*;
interface CaptureOperation{void apply(Field field)throws IllegalAccessException;}
public class UncapturedLambdaNames {
 public static CaptureOperation copy(Object src,Object dest){return(Field field)->{field.setAccessible(true);Object value=field.get(src);field.set(dest,value);};}
}
class CaptureCell{public Object value;public int number;}
class LambdaParameterDriver{
 public static void main(String[]args)throws Exception{
  int rows=0;Object token=new Object();Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||method.getModifiers()!=4106||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{Object.class,Object.class,Field.class}))throw new AssertionError("original physical capture implementation");implementation=method;}if(implementation==null)throw new AssertionError("missing original implementation");implementation.setAccessible(true);
  for(int path=0;path<2;path++)for(int payload=0;payload<2;payload++)for(String name:new String[]{"value","number"})for(int s=0;s<5;s++)for(int d=0;d<5;d++){
   CaptureCell left=new CaptureCell(),right=new CaptureCell();left.value=payload==0?null:token;right.value=new Object();left.number=Integer.MIN_VALUE;right.number=Integer.MAX_VALUE;
   Object[] bank={null,left,right,new Object(),"wrong"};Object src=bank[s],dest=bank[d];Object expected=src instanceof CaptureCell?((CaptureCell)src).value:null;int numeric=src instanceof CaptureCell?((CaptureCell)src).number:0;
   Object leftBefore=left.value,rightBefore=right.value;int leftNumber=left.number,rightNumber=right.number;
   Field field=CaptureCell.class.getDeclaredField(name);if(field.isAccessible())throw new AssertionError("fresh original field");
   CaptureOperation operation=UncapturedLambdaNames.copy(src,dest);if(field.isAccessible()||left.value!=leftBefore||right.value!=rightBefore||left.number!=leftNumber||right.number!=rightNumber)throw new AssertionError("factory must defer all field effects");
   Class<?> failure=src==null?NullPointerException.class:!(src instanceof CaptureCell)?IllegalArgumentException.class:dest==null?NullPointerException.class:!(dest instanceof CaptureCell)?IllegalArgumentException.class:null;
   Throwable caught=null;try{if(path==0)operation.apply(field);else implementation.invoke(null,src,dest,field);}catch(InvocationTargetException error){caught=error.getCause();}catch(Throwable error){caught=error;}
   if(!field.isAccessible()||(failure==null?caught!=null:caught==null||caught.getClass()!=failure))throw new AssertionError("original source-before-destination fault order "+rows,caught);
   if(failure!=null){if(left.value!=leftBefore||right.value!=rightBefore||left.number!=leftNumber||right.number!=rightNumber)throw new AssertionError("failed copy wrote state");}
   else{CaptureCell target=(CaptureCell)dest;if(name.equals("value")?target.value!=expected:target.number!=numeric)throw new AssertionError("captured source/destination identity "+rows);CaptureCell other=target==left?right:left;if(other==left?(other.value!=leftBefore||other.number!=leftNumber):(other.value!=rightBefore||other.number!=rightNumber))throw new AssertionError("copy changed other receiver");}
   rows++;
  }
  List<String> declarations=new ArrayList<>();for(Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialLambdaCaptureDiscoveryRetainsRealProducerBeforeItsConsumer(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, lambdaCaptureFirstUseFixture, 1, "200")
}
