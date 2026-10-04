package javaclassparser

import "testing"

func TestAdversarialInterfaceInitializerViewsRoundTrip(t *testing.T) {
	t.Parallel()
	// Rebuild the interfaces too: keeping them on the original helper classpath
	// would conceal a missing <clinit>. Shared singleton identity, live array
	// views, ordered multiple fields, failure once and circular reads are observed.
	roundTripGenericFlowUnits(t, "InterfaceInitViewReview", `
import java.util.*;
interface ReviewedInterfaceTable<T> {
 List<ReviewedInterfaceTable<?>> DEFAULTS=Collections.unmodifiableList(Arrays.asList(new ReviewedInterfaceTable<?>[]{ReviewedInterfaceInitState.FIRST,ReviewedInterfaceInitState.SECOND}));
 Object[] BACKING=ReviewedInterfaceInitState.backing();
 List<Object> VIEW=Collections.unmodifiableList(Arrays.asList(BACKING));
 int TOKEN=ReviewedInterfaceInitState.mark("T");
 Object LAST=ReviewedInterfaceInitState.finish(VIEW);
 String name();
}
class ReviewedFirstHandler implements ReviewedInterfaceTable<String>{public String name(){return "first";}}
class ReviewedSecondHandler implements ReviewedInterfaceTable<Integer>{public String name(){return "second";}}
interface ReviewedInterfaceFailure {Object FIRST=ReviewedInterfaceInitState.beforeFailure();Object FAIL=ReviewedInterfaceInitState.fail();Object AFTER=ReviewedInterfaceInitState.afterFailure();}
interface ReviewedInterfaceCycle {int FIRST=ReviewedInterfaceInitState.cycle();int LATER=ReviewedInterfaceInitState.seven();}
class ReviewedInterfaceInitState {
 static StringBuilder trace=new StringBuilder();static int calls;static final IllegalStateException failure=new IllegalStateException("init-failure");
 static final ReviewedInterfaceTable<String> FIRST=new ReviewedFirstHandler();static final ReviewedInterfaceTable<Integer> SECOND=new ReviewedSecondHandler();
 static Object[] backing(){trace.append("B");calls++;return new Object[]{"old",null};}
 static int mark(String s){trace.append(s);return ++calls;}
 static Object finish(Object value){trace.append("F");calls++;return value;}
 static Object beforeFailure(){trace.append("X");calls++;return new Object();}
 static Object fail(){trace.append("!");calls++;throw failure;}
 static Object afterFailure(){trace.append("Z");calls++;return new Object();}
 static int cycle(){trace.append("C");calls++;return ReviewedInterfaceCycle.LATER;}
 static int seven(){trace.append("L");calls++;return 7;}
}
public class InterfaceInitViewReview {
 public static void main(String[] args){
  List<ReviewedInterfaceTable<?>> values=ReviewedInterfaceTable.DEFAULTS;
  System.out.println(values.size()+":"+(values.get(0)==ReviewedInterfaceInitState.FIRST)+":"+(values.get(1)==ReviewedInterfaceInitState.SECOND)+":"+values.get(0).name()+":"+values.get(1).name());
  System.out.println(values==ReviewedInterfaceTable.DEFAULTS);
  try{values.add(ReviewedInterfaceInitState.FIRST);}catch(Throwable e){System.out.println(e.getClass().getName());}
  System.out.println(ReviewedInterfaceTable.VIEW+":"+(ReviewedInterfaceTable.LAST==ReviewedInterfaceTable.VIEW));
  ReviewedInterfaceTable.BACKING[0]="new";System.out.println(ReviewedInterfaceTable.VIEW);
  System.out.println(ReviewedInterfaceInitState.trace+":"+ReviewedInterfaceInitState.calls+":"+ReviewedInterfaceTable.TOKEN);
  for(int i=0;i<2;i++)try{System.out.println(ReviewedInterfaceFailure.FAIL);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e.getCause()==ReviewedInterfaceInitState.failure));}
  System.out.println(ReviewedInterfaceInitState.trace+":"+ReviewedInterfaceInitState.calls);
  System.out.println(ReviewedInterfaceCycle.FIRST+":"+ReviewedInterfaceCycle.LATER);
  System.out.println(ReviewedInterfaceCycle.FIRST+":"+ReviewedInterfaceCycle.LATER);
  System.out.println(ReviewedInterfaceInitState.trace+":"+ReviewedInterfaceInitState.calls);
 }
}`, nil, []string{"ReviewedInterfaceTable", "ReviewedInterfaceFailure", "ReviewedInterfaceCycle"}, Precision, Compatibility, "legacy")
}

func TestAdversarialInvocationErasureAndRebindingRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "InvocationViewReview", `
import java.util.*;import java.util.concurrent.*;
class InvocationViewBase {final Object value;InvocationViewBase(Object v){value=v;}Object read(){InvocationViewState.trace.append("D");return value;}}
class InvocationViewFirst extends InvocationViewBase {InvocationViewFirst(Object v){super(v);}}
class InvocationViewSecond extends InvocationViewBase {InvocationViewSecond(InvocationViewBase original){super(original.value);}}
class InvocationViewState {
 static StringBuilder trace=new StringBuilder();static final RuntimeException failure=new IllegalStateException("producer");static Object marker=new Object();
 static Object value(int mode){trace.append("V");if(mode==2)throw failure;return mode==0?null:marker;}
 static InvocationViewBase receiver(int mode){trace.append("R");if(mode==2)throw failure;return mode==0?null:new InvocationViewFirst(marker);}
 static String argument(int mode){trace.append("A");if(mode==2)throw failure;return "arg";}
 static Object invoke(InvocationViewBase receiver,String argument,Object value){trace.append("I");return receiver.read()==value?value:argument;}
 static Object unbounded(Object value){trace.append("U");return value;}
}
public class InvocationViewReview<T> {
 T erased(int mode){return (T)InvocationViewState.unbounded(InvocationViewState.value(mode));}
 Object bind(int receiver,int argument,int value){return InvocationViewState.invoke(InvocationViewState.receiver(receiver),InvocationViewState.argument(argument),InvocationViewState.value(value));}
 InvocationViewBase rebound(boolean second){InvocationViewBase result=new InvocationViewFirst(InvocationViewState.marker);if(second)result=new InvocationViewSecond(result);return result;}
 List<?> varargs(Object... values){return delegate(Arrays.asList(values));}List<?> delegate(List<?> values){return values;}
 Object classOf(Object input){return classify(input,input.getClass());}Object classify(Object input,Class<?> kind){return kind==input.getClass()?input:null;}
 Object future(Future<?> input){try{return input.get();}catch(ExecutionException e){throw new IllegalStateException("execution",e.getCause());}catch(Exception e){throw new IllegalStateException("other",e);}}
 static String run(int r,int a,int v){InvocationViewState.trace.setLength(0);try{return (new InvocationViewReview<Object>().bind(r,a,v)==InvocationViewState.marker)+":"+InvocationViewState.trace;}catch(Throwable e){return e.getClass().getName()+":"+(e==InvocationViewState.failure)+":"+InvocationViewState.trace;}}
 public static void main(String[] args){
  InvocationViewReview<Object> review=new InvocationViewReview<Object>();
  for(int r=0;r<3;r++)for(int a=0;a<3;a++)for(int v=0;v<3;v++)System.out.println(run(r,a,v));
  for(int mode=0;mode<3;mode++){InvocationViewState.trace.setLength(0);try{System.out.println((review.erased(mode)==InvocationViewState.marker)+":"+InvocationViewState.trace);}catch(Throwable e){System.out.println((e==InvocationViewState.failure)+":"+InvocationViewState.trace);}}
  for(boolean second:new boolean[]{false,true}){InvocationViewBase result=review.rebound(second);System.out.println(result.getClass().getName()+":"+(result.value==InvocationViewState.marker));}
  Object[] array=new Object[]{"old",null};List<?> view=review.varargs(array);array[0]="new";System.out.println(view);try{((List)view).add("x");}catch(Throwable e){System.out.println(e.getClass().getName());}
  for(Object input:new Object[]{null,InvocationViewState.marker,"value"})try{System.out.println(review.classOf(input)==input);}catch(Throwable e){System.out.println(e.getClass().getName());}
  CompletableFuture<Object> success=new CompletableFuture<Object>();success.complete(InvocationViewState.marker);System.out.println(review.future(success)==InvocationViewState.marker);
  CompletableFuture<Object> failed=new CompletableFuture<Object>();failed.completeExceptionally(InvocationViewState.failure);try{review.future(failed);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage()+":"+(e.getCause()==InvocationViewState.failure));}
  CompletableFuture<Object> canceled=new CompletableFuture<Object>();canceled.cancel(false);try{review.future(canceled);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage()+":"+e.getCause().getClass().getName());}
 }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialFlattenedNestPrivateInvocationBindingRoundTrip(t *testing.T) {
	t.Parallel()
	// A private call is statically bound. A preexisting same-named method in a
	// subclass is legal and must not intercept that call when nested units flatten.
	roundTripGenericFlow(t, "NestPrivateBindingReview", `
class ReviewedPrivateBindingChild extends NestPrivateBindingReview {public Object select(Object input){trace+="C";return "child";}}
public class NestPrivateBindingReview {
 static class NestMarker {}
 static String trace;
 private Object select(Object input){trace+="P";return input;}
 Object dispatch(Object input){trace+="D";return select(input);}
 public static void main(String[] args){Object marker=new Object();for(NestPrivateBindingReview receiver:new NestPrivateBindingReview[]{new NestPrivateBindingReview(),new ReviewedPrivateBindingChild()})for(Object value:new Object[]{null,marker,"text"}){trace="";Object result=receiver.dispatch(value);System.out.println((result==value)+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}
