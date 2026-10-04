package javaclassparser

import "testing"

func TestAdversarialNumericArrayLoopContractsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NumericArrayLoopReview", `
public class NumericArrayLoopReview<T extends Number> {
 String consume(T[] values){return values.getClass().getComponentType().getName()+":"+values.length+":"+(values.length==0?"empty":values[0]);}
 String array(T first,T second,boolean left){return consume((T[])new Number[]{left?first:second});}
 static String loop(int n){int[] index=new int[n];for(int i=0;i<n;i++)index[i]=i;int rank=0;boolean active=n>0;StringBuilder trace=new StringBuilder();while(active){int chosen=rank;for(int scan=rank+1;scan<n;scan++)if(index[scan]>index[chosen])chosen=scan;trace.append(index[chosen]).append(',');rank++;active=rank<n;}return rank+":"+trace;}
 static String digits(String text){int count=0;boolean sign=false;for(int i=0;i<text.length();i++){char c=text.charAt(i);if(i==0&&(c=='+'||c=='-'))sign=true;else if(c>='0'&&c<='9')count++;else break;}return (!sign&&count==2)+":"+count;}
 public static void main(String[] args){NumericArrayLoopReview<Integer> p=new NumericArrayLoopReview<Integer>();for(int n=0;n<=6;n++)System.out.println(loop(n));for(String text:new String[]{"","1","12","123","+12","-12","12x","x12"})System.out.println(digits(text));System.out.println(p.array(7,9,true));System.out.println(p.array(7,9,false));System.out.println(p.array(null,9,true));System.out.println(p.consume((Integer[])new Integer[0]));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialSavedRetryExceptionIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	// The first pass saves one of two checked exception objects. A failing retry
	// must rethrow that ORIGINAL object, never its retry object or a null local.
	roundTripGenericFlow(t, "SavedRetryExceptionReview", reviewedSavedRetryFixtureSource, Precision, Compatibility, "legacy")
}

func TestAdversarialResourceThrowableIdentityAndSuppressionRoundTrip(t *testing.T) {
	t.Parallel()
	// Formatter.close cannot throw, while AutoCloseable.close can. Both must
	// execute once and retain the body throwable's identity; only the second
	// case may attach the close failure as a suppressed exception.
	roundTripGenericFlow(t, "ResourceThrowableReview", `
import java.util.*;
class ReviewedCloseResource implements AutoCloseable {
 static int closes;final Exception closeFailure;ReviewedCloseResource(Exception failure){closeFailure=failure;}
 public void close()throws Exception{closes++;if(closeFailure!=null)throw closeFailure;}
}
public class ResourceThrowableReview {
 static final RuntimeException runtime=new RuntimeException("body-runtime");static final AssertionError error=new AssertionError("body-error");
 static String format(Formatter input,int mode){try(Formatter local=input){if(mode==2)throw runtime;if(mode==3)throw error;local.format("%d",mode==1?new Object():Integer.valueOf(17));return local.toString();}}
 static String formatted(int mode){Formatter f=new Formatter();String result;try{result="ok:"+format(f,mode);}catch(Throwable e){result=e.getClass().getName()+":"+(e==runtime)+":"+(e==error)+":"+e.getSuppressed().length;}try{f.toString();result+=":open";}catch(FormatterClosedException closed){result+=":closed";}return result;}
 static String work(ReviewedCloseResource input,int mode,RuntimeException bodyRuntime,AssertionError bodyError)throws Exception{try(ReviewedCloseResource local=input){if(mode==2)throw bodyRuntime;if(mode==3)throw bodyError;return "ok";}}
 static String closed(int mode,boolean fail){ReviewedCloseResource.closes=0;Exception closing=new Exception("close");RuntimeException bodyRuntime=new IllegalStateException("work-runtime");AssertionError bodyError=new AssertionError("work-error");ReviewedCloseResource r=new ReviewedCloseResource(fail?closing:null);try{return work(r,mode,bodyRuntime,bodyError)+":"+ReviewedCloseResource.closes;}catch(Throwable e){Throwable[] suppressed=e.getSuppressed();return e.getClass().getName()+":"+e.getMessage()+":"+(e==bodyRuntime)+":"+(e==bodyError)+":"+(e==closing)+":"+suppressed.length+":"+(suppressed.length>0&&suppressed[0]==closing)+":"+ReviewedCloseResource.closes;}}
 public static void main(String[] args){for(int mode=0;mode<4;mode++){System.out.println(formatted(mode));System.out.println(closed(mode,false));System.out.println(closed(mode,true));}}
}`, Precision, Compatibility, "legacy")
}

const reviewedSavedRetryFixtureSource = `
class ReviewedFormatFailure extends Exception {ReviewedFormatFailure(String message){super(message);}}
class ReviewedChecksumFailure extends Exception {ReviewedChecksumFailure(String message){super(message);}}
public class SavedRetryExceptionReview {
 static final ReviewedFormatFailure firstFormat=new ReviewedFormatFailure("first-format");static final ReviewedFormatFailure retryFormat=new ReviewedFormatFailure("retry-format");
 static final ReviewedChecksumFailure firstChecksum=new ReviewedChecksumFailure("first-checksum");static final ReviewedChecksumFailure retryChecksum=new ReviewedChecksumFailure("retry-checksum");
 static String trace;
 static int attempt(int kind,boolean retry)throws ReviewedFormatFailure,ReviewedChecksumFailure{trace+=retry?"R":"F";if(kind==1)throw retry?retryFormat:firstFormat;if(kind==2)throw retry?retryChecksum:firstChecksum;return kind;}
 static int decode(int first,int retry)throws ReviewedFormatFailure,ReviewedChecksumFailure {
  ReviewedFormatFailure savedFormat=null;ReviewedChecksumFailure savedChecksum=null;
  try{return attempt(first,false);}catch(ReviewedFormatFailure caught){savedFormat=caught;}catch(ReviewedChecksumFailure caught){savedChecksum=caught;}
  try{return attempt(retry,true);}catch(ReviewedFormatFailure|ReviewedChecksumFailure ignored){if(savedFormat!=null)throw savedFormat;throw savedChecksum;}
 }
 static String run(int first,int retry){trace="";try{return decode(first,retry)+":"+trace;}catch(Throwable e){return e.getClass().getName()+":"+(e==firstFormat)+":"+(e==firstChecksum)+":"+(e==retryFormat)+":"+(e==retryChecksum)+":"+e.getMessage()+":"+trace;}}
 public static void main(String[] args){for(int first=0;first<3;first++)for(int retry=0;retry<3;retry++)System.out.println(run(first,retry));}
}`
