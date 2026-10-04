package javaclassparser

import "testing"

func TestAdversarialTypedCatchSharedFinallyRoundTrip(t *testing.T) {
	t.Parallel()
	// Independently authored model of a catch-all cleanup region covering both
	// the protected body and its typed handlers. A sibling catch(Throwable)
	// cannot replace finally: it does not protect throws from another handler.
	roundTripGenericFlow(t, "TypedCatchSharedFinallyReview", `
class ReviewConversionFailure extends RuntimeException {ReviewConversionFailure(String message){super(message);}ReviewConversionFailure(Throwable cause){super(cause);}}
class ReviewSecurityFailure extends RuntimeException {}
public class TypedCatchSharedFinallyReview {
 static java.util.ArrayList<Object> stack=new java.util.ArrayList<Object>();static String trace="";static int pops;static RuntimeException cleanup;
 static Object work(int mode,Object result,ReviewConversionFailure conversion,ReviewSecurityFailure security,RuntimeException runtime,AssertionError error){
  stack.add(result);trace+="push;";
  try{trace+="body;";if(mode==1)throw conversion;if(mode==2)throw security;if(mode==3)throw runtime;if(mode==4)throw error;return result;}
  catch(ReviewConversionFailure caught){trace+="conversion;";throw caught;}
  catch(ReviewSecurityFailure caught){trace+="security;";throw caught;}
  catch(RuntimeException caught){ReviewConversionFailure wrapped=new ReviewConversionFailure(caught);trace+="wrap;";throw wrapped;}
  finally{stack.remove(stack.size()-1);pops++;trace+="pop;";if(cleanup!=null)throw cleanup;}
 }
 static String run(int mode,boolean fail){stack.clear();trace="";pops=0;cleanup=fail?new IllegalStateException("cleanup"):null;Object result=new Object();ReviewConversionFailure conversion=new ReviewConversionFailure("conversion");ReviewSecurityFailure security=new ReviewSecurityFailure();RuntimeException runtime=new IllegalArgumentException("runtime");AssertionError error=new AssertionError("error");String outcome;
  try{outcome="ok:"+(work(mode,result,conversion,security,runtime,error)==result);}catch(Throwable caught){outcome=caught.getClass().getName()+":"+(caught==conversion)+":"+(caught==security)+":"+(caught==error)+":"+(caught.getCause()==runtime)+":"+(caught==cleanup);}
  String first=outcome+":"+stack.size()+":"+pops+":"+trace;cleanup=null;Object again=work(0,result,conversion,security,runtime,error);return first+":"+(again==result)+":"+stack.size()+":"+pops;
 }
 public static void main(String[] args){for(int mode=0;mode<5;mode++)for(boolean fail:new boolean[]{false,true})System.out.println(run(mode,fail));}
}

`, Precision, Compatibility, "legacy")
}

func TestAdversarialLifecycleAndCatchBindingRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LifecycleCatchBindingReview", `
class ReviewConnectionFailure extends RuntimeException {ReviewConnectionFailure(String message,Throwable cause){super(message,cause);}}
public class LifecycleCatchBindingReview {
 static String trace="";static int cleanups;static boolean broken;
 static void step(String name,Throwable failure)throws Throwable{trace+=name+";";if(failure!=null)throw failure;}
 static void lifecycle(Throwable setup,Throwable body,Throwable cleanup)throws Throwable{
  Throwable saved=null;step("setup",setup);
  try{step("body",body);}catch(Throwable caught){saved=caught;}
  finally{try{cleanups++;step("cleanup",cleanup);}catch(Throwable caught){if(saved==null)saved=caught;}}
  if(saved!=null)throw saved;
 }
 static String lifecycleCase(boolean s,boolean b,boolean c){trace="";cleanups=0;Throwable setup=new AssertionError("setup"),body=new IllegalArgumentException("body"),cleanup=new AssertionError("cleanup");String result;
  try{lifecycle(s?setup:null,b?body:null,c?cleanup:null);result="ok";}catch(Throwable caught){result=(caught==setup)+":"+(caught==body)+":"+(caught==cleanup);}
  return result+":"+cleanups+":"+trace;
 }
 static String readLine(int mode){trace+="read;";if(mode==2)throw new IllegalStateException("read");return mode==0?"":"server";}
 static void send(boolean fail,ReviewConnectionFailure original,int mode){
  try{trace+="send;";if(fail)throw original;return;}
  catch(ReviewConnectionFailure caught){try{String line=readLine(mode);if(line!=null&&line.length()>0)caught=new ReviewConnectionFailure(line,caught.getCause());}catch(RuntimeException ignored){}broken=true;throw caught;}
 }
 static String sendCase(boolean fail,int mode){trace="";broken=false;Throwable root=new IllegalArgumentException("root");ReviewConnectionFailure original=new ReviewConnectionFailure("original",root);String result;
  try{send(fail,original,mode);result="ok";}catch(Throwable caught){result=caught.getClass().getName()+":"+(caught==original)+":"+(caught.getCause()==root)+":"+caught.getMessage();}
  return result+":"+broken+":"+trace;
 }
 public static void main(String[] args){for(boolean s:new boolean[]{false,true})for(boolean b:new boolean[]{false,true})for(boolean c:new boolean[]{false,true})System.out.println(lifecycleCase(s,b,c));for(boolean fail:new boolean[]{false,true})for(int mode=0;mode<3;mode++)System.out.println(sendCase(fail,mode));}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedReflectionHandlersRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowUnits(t, "NestedReflectionHandlersReview", `
interface ReviewPolicy {}
class ReviewPolicyOk implements ReviewPolicy {public ReviewPolicyOk(){}}
class ReviewPolicyWrong {public ReviewPolicyWrong(){}}
class ReviewPolicyMissingCtor implements ReviewPolicy {public ReviewPolicyMissingCtor(String ignored){}}
class ReviewPolicyThrow implements ReviewPolicy {public ReviewPolicyThrow(){throw NestedReflectionHandlersReview.constructorFailure;}}
class ReviewMethodCacheGood {static final java.lang.reflect.Method A;static final java.lang.reflect.Method B;static{try{NestedReflectionHandlersReview.trace+="a;";A=NestedReflectionHandlersReview.class.getMethod("alpha",new Class[0]);NestedReflectionHandlersReview.trace+="b;";B=NestedReflectionHandlersReview.class.getMethod("beta",new Class[0]);}catch(NoSuchMethodException caught){throw new IllegalStateException("cache",caught);}}}
class ReviewMethodCacheFail {static final java.lang.reflect.Method A;static final java.lang.reflect.Method B;static{try{NestedReflectionHandlersReview.trace+="a;";A=NestedReflectionHandlersReview.class.getMethod("alpha",new Class[0]);NestedReflectionHandlersReview.trace+="b;";B=NestedReflectionHandlersReview.class.getMethod("missing",new Class[0]);}catch(NoSuchMethodException caught){throw new IllegalStateException("cache",caught);}}}
public class NestedReflectionHandlersReview {
 static String trace="";static final RuntimeException constructorFailure=new IllegalArgumentException("ctor");public static void alpha(){}public static void beta(){}
 static String reflect(String first,String second,boolean legacy){try{try{trace+="fast;";return NestedReflectionHandlersReview.class.getDeclaredMethod(first,new Class[0]).getName();}catch(Exception caught){trace+="fallback;";return legacy?"legacy":NestedReflectionHandlersReview.class.getDeclaredMethod(second,new Class[0]).getName();}}catch(Exception caught){trace+="outer;";return caught.getClass().getSimpleName();}}
 static ReviewPolicy create(String name)throws ClassNotFoundException,NoSuchMethodException,InstantiationException,IllegalAccessException,java.lang.reflect.InvocationTargetException{trace+=name+";";return (ReviewPolicy)Class.forName(name).getConstructor(new Class[0]).newInstance(new Object[0]);}
 static ReviewPolicy policy(String first,String second){try{try{return create(first);}catch(ClassCastException|ClassNotFoundException caught){return create(second);}}catch(ClassCastException caught){throw new IllegalArgumentException("wrong");}catch(ClassNotFoundException|InstantiationException|IllegalAccessException|java.lang.reflect.InvocationTargetException|NoSuchMethodException caught){throw new IllegalArgumentException("reflection",caught);}}
 static String deepLayers(int mode){RuntimeException first=new IllegalArgumentException("first"),second=new IllegalStateException("second");try{try{try{trace+="inner;";if(mode==1)throw first;if(mode==2)throw second;return "ok";}catch(IllegalArgumentException caught){trace+="inner-catch;";throw new IllegalStateException(caught);}}catch(IllegalStateException caught){trace+="middle-catch;";throw new UnsupportedOperationException(caught);}}catch(UnsupportedOperationException caught){trace+="outer-catch;";return (caught.getCause()==second)+":"+(caught.getCause().getCause()==first);}}
 static String policyCase(String first,String second){trace="";try{return policy(first,second).getClass().getSimpleName()+":"+trace;}catch(Throwable caught){Throwable cause=caught.getCause();return caught.getMessage()+":"+(cause==null?"none":cause.getClass().getSimpleName())+":"+(cause!=null&&cause.getCause()==constructorFailure)+":"+trace;}}
 static String cacheGood(){trace="";java.lang.reflect.Method a=ReviewMethodCacheGood.A;java.lang.reflect.Method b=ReviewMethodCacheGood.B;String first=trace;return a.getName()+":"+b.getName()+":"+(a==ReviewMethodCacheGood.A)+":"+(b==ReviewMethodCacheGood.B)+":"+first+":"+trace;}
 static String cacheFail(){trace="";Throwable failure=null;try{java.lang.reflect.Method b=ReviewMethodCacheFail.B;}catch(Throwable caught){failure=caught;}String first=trace;Throwable again=null;try{java.lang.reflect.Method b=ReviewMethodCacheFail.B;}catch(Throwable caught){again=caught;}return failure.getClass().getSimpleName()+":"+failure.getCause().getClass().getSimpleName()+":"+failure.getCause().getCause().getClass().getSimpleName()+":"+again.getClass().getSimpleName()+":"+first+":"+trace;}
 public static void main(String[] args){for(String first:new String[]{"alpha","missing"})for(String second:new String[]{"beta","missing"})for(boolean legacy:new boolean[]{false,true}){trace="";System.out.println(reflect(first,second,legacy)+":"+trace);}for(String first:new String[]{"ReviewPolicyOk","ReviewPolicyWrong","missing","ReviewPolicyMissingCtor","ReviewPolicyThrow"})for(String second:new String[]{"ReviewPolicyOk","ReviewPolicyWrong","missing","ReviewPolicyMissingCtor","ReviewPolicyThrow"})System.out.println(policyCase(first,second));for(int mode=0;mode<3;mode++){trace="";System.out.println(deepLayers(mode)+":"+trace);}System.out.println(cacheGood());System.out.println(cacheFail());}
}
`, nil, []string{"ReviewMethodCacheGood", "ReviewMethodCacheFail"}, Precision, Compatibility, "legacy")
}

func TestAdversarialSwitchAccumulatorLabeledExitRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SwitchAccumulatorLabeledExitReview", `
class ReviewImmutableAppender {final String value;ReviewImmutableAppender(String value){this.value=value;}ReviewImmutableAppender append(String next){return new ReviewImmutableAppender(value+next);}}
class ReviewLexCursor {final byte[][] entries;final boolean unsupported;ReviewLexCursor(byte[][] entries,boolean unsupported){this.entries=entries;this.unsupported=unsupported;}void seekExact(long index){if(unsupported)throw new UnsupportedOperationException();}byte[] term(){return entries[entries.length-1];}byte[] next(){return entries.length==0?null:entries[0];}boolean end(byte[] prefix){for(byte[] entry:entries)if(compare(entry,prefix)>=0)return false;return true;}static int compare(byte[] a,byte[] b){for(int i=0;i<Math.min(a.length,b.length);i++){int d=(a[i]&255)-(b[i]&255);if(d!=0)return d;}return a.length-b.length;}}
class ReviewByteBuilder {byte[] data=new byte[1];int length;void append(byte value){grow(length+1);data[length++]=value;}void setByteAt(int index,byte value){data[index]=value;}int length(){return length;}void setLength(int length){this.length=length;}void grow(int required){if(required>data.length)data=java.util.Arrays.copyOf(data,Math.max(required,data.length*2));}byte[] get(){return java.util.Arrays.copyOf(data,length);}}
public class SwitchAccumulatorLabeledExitReview {
 static String trace="";static int reads;
 static long scan(int[] widths,long[] offsets,RuntimeException invalid){long sum=0;for(int index=0;index<widths.length;index++){trace+="w"+widths[index]+";";switch(widths[index]){case 0:case 1:case 2:case 4:case 8:sum+=offsets[index];reads++;continue;default:throw invalid;}}return sum;}
 static String scanCase(int[] widths){trace="";reads=0;long[] values=new long[widths.length];for(int i=0;i<values.length;i++)values[i]=i+10;RuntimeException failure=new IllegalArgumentException("width");String result;try{result="sum:"+scan(widths,values,failure);}catch(Throwable caught){result="throw:"+(caught==failure);}return result+":"+reads+":"+trace;}
 static String append(String[] first,String[] second,int start){ReviewImmutableAppender original=new ReviewImmutableAppender("");ReviewImmutableAppender current=original;for(int index=start;index<first.length;index++){current=current.append(index+":"+first[index]+";");}for(String item:second){current=current.append(item+";");}return original.value+":"+current.value+":"+(original==current);}
 static byte[] highest(byte[][] entries,long size,boolean unsupported){if(size==0)return null;if(size>=0){try{ReviewLexCursor cursor=new ReviewLexCursor(entries,unsupported);cursor.seekExact(size-1);return cursor.term();}catch(UnsupportedOperationException ignored){}}
  ReviewLexCursor cursor=new ReviewLexCursor(entries,unsupported);byte[] first=cursor.next();if(first==null)return first;ReviewByteBuilder builder=new ReviewByteBuilder();builder.append((byte)0);
  outer:while(true){int low=0,high=256;while(low!=high){int middle=(low+high)>>>1;builder.setByteAt(builder.length()-1,(byte)middle);if(cursor.end(builder.get())){if(middle==0)break outer;high=middle;}else{if(low==middle)break;low=middle;}}builder.setLength(builder.length()+1);builder.grow(builder.length());}
  builder.setLength(builder.length()-1);return builder.get();
 }
 public static void main(String[] args){for(int[] widths:new int[][]{new int[0],{0,1,2,4,8},{8,3,1},{-1},{9},{1,2,0}})System.out.println(scanCase(widths));for(int start=0;start<4;start++)System.out.println(append(new String[]{"A","B","C"},new String[]{"x","y"},start));System.out.println(append(new String[0],new String[0],0));for(byte[][] entries:new byte[][][]{new byte[0][],{new byte[0]},{{0}},{{(byte)255}},{{0,0},{0,1},{1}},{{1},{1,0},{1,0,0}}})for(boolean unsupported:new boolean[]{false,true}){System.out.println(java.util.Arrays.toString(highest(entries,-1,unsupported)));System.out.println(java.util.Arrays.toString(highest(entries,entries.length,unsupported)));}}
}
`, Precision, Compatibility, "legacy")
}
