package javaclassparser

import "testing"

// Reuse the original-first compiler/metadata harness. The historical output
// prefix is only its wire format: these factories deliberately capture outer
// formals, so physical implementation parameters are not the source SAM list.
const capturedLambdaParameterFixture = `
import java.util.function.*;
public class UncapturedLambdaNames {
 public static LongUnaryOperator integer(final long bias,Object token){return(final long input)->bias^input^(token==null?0:1);}
 public static Supplier<Object> reference(Object token){return()->token;}
 public static LongSupplier reordered(long left,long right){return()->right^left;}
 public static DoubleUnaryOperator floating(double bias){return(double sample)->sample+bias;}
 public static IntUnaryOperator mutation(int[] box){return(int delta)->{int before=box[0];box[0]=before+delta;return before;};}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception {
  int rows=0;Object identity=new Object();long[] words={Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE};
  for(long bias:words)for(long input:words)for(Object token:new Object[]{null,identity}){
   LongUnaryOperator operation=UncapturedLambdaNames.integer(bias,token);
   if(operation.applyAsLong(input)!=(bias^input^(token==null?0:1)))throw new AssertionError("wide capture/SAM words or order");
   if(UncapturedLambdaNames.reordered(bias,input).getAsLong()!=(input^bias))throw new AssertionError("capture order follows original factory");rows++;
  }
  for(Object token:new Object[]{null,identity,"same",Integer.valueOf(7)}){if(UncapturedLambdaNames.reference(token).get()!=token)throw new AssertionError("captured identity");rows++;}
  double[] reals={Double.NEGATIVE_INFINITY,-Double.MAX_VALUE,-1.0,-0.0,0.0,1.0,Double.MAX_VALUE,Double.POSITIVE_INFINITY,Double.NaN};
  for(double bias:reals)for(double sample:reals){if(Double.doubleToLongBits(UncapturedLambdaNames.floating(bias).applyAsDouble(sample))!=Double.doubleToLongBits(sample+bias))throw new AssertionError("floating capture/SAM order");rows++;}
  for(int initial:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int delta:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   int[] box={initial};IntUnaryOperator operation=UncapturedLambdaNames.mutation(box);box[0]=initial^17;
   int before=box[0];if(operation.applyAsInt(delta)!=before||box[0]!=before+delta)throw new AssertionError("deferred mutable object identity/once-only store");rows++;
  }
  java.util.List<String> declarations=new java.util.ArrayList<String>();
  for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());
   for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());
   declarations.add(row.toString());
  }
  java.util.Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));
  System.out.println(rows+":uncaptured:lambda:parameters:"+java.util.Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialCapturedLambdaParameterMetadataKeepsPhysicalAndSAMRoles(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, capturedLambdaParameterFixture, 5, "160")
}
