package javaclassparser

import (
	"strings"
	"testing"
)

const guardedFinallyLambdaFixture = `import java.util.function.LongConsumer;import java.util.function.LongUnaryOperator;
public class ArtifactLambdaOwner {
 public static String trace="";
 public static final RuntimeException workError=new RuntimeException("work"),cleanupError=new RuntimeException("cleanup");
 public long state;public int closed;
 public ArtifactLambdaOwner(long seed){state=seed;}
 public static void consume(long[] values,LongConsumer action){for(long value:values)action.accept(value);}
 public long run(long value,int failAt,final boolean cleanupFails){
  LongUnaryOperator prepare=x->{trace+="P";return x+5L;};long prepared=prepare.applyAsLong(value);long[] entries=new long[]{value,-value};boolean failed=true;
  try{
   for(int i=0;i<2;i++){
    LongUnaryOperator work=x->{trace+="W";return state*31L+x;};state=work.applyAsLong(prepared+i);
    if(i==failAt)throw workError;
   }
   failed=false;
  }finally{
   if(failed)consume(entries,n->{trace+="C";closed++;state+=n;if(cleanupFails)throw cleanupError;});
  }
  return state;
 }
 EXTRA_NESTED_METHOD
}
class ArtifactLambdaDriver {
 static long add(long a,long b){return java.math.BigInteger.valueOf(a).add(java.math.BigInteger.valueOf(b)).longValue();}
 static long step(long a,long b){return java.math.BigInteger.valueOf(a).multiply(java.math.BigInteger.valueOf(31L)).add(java.math.BigInteger.valueOf(b)).longValue();}
 public static void main(String[]args){int rows=0;
  for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(int failAt:new int[]{-1,0,1})for(boolean cleanupFails:new boolean[]{false,true}){
   ArtifactLambdaOwner owner=new ArtifactLambdaOwner(17L);ArtifactLambdaOwner.trace="";Throwable observed=null;long result=0L;
   try{result=owner.run(value,failAt,cleanupFails);}catch(Throwable t){observed=t;}
   long expected=17L,prepared=add(value,5L);String trace="P";int closed=0;
   for(int i=0;i<2;i++){expected=step(expected,add(prepared,i));trace+="W";if(i==failAt)break;}
   if(failAt>=0){expected=add(expected,value);closed++;trace+="C";if(!cleanupFails){expected=add(expected,java.math.BigInteger.valueOf(value).negate().longValue());closed++;trace+="C";}}
   Throwable required=failAt<0?null:cleanupFails?ArtifactLambdaOwner.cleanupError:ArtifactLambdaOwner.workError;
   if(observed!=required||observed!=null&&observed.getSuppressed().length!=0||owner.closed!=closed||owner.state!=expected||!ArtifactLambdaOwner.trace.equals(trace)||required==null&&result!=expected)throw new AssertionError("guarded finally state/order/exception identity "+failAt+":"+cleanupFails+":"+ArtifactLambdaOwner.trace,observed);
   EXTRA_NESTED_CHECK
   rows++;
  }
  System.out.println(rows+":guarded-finally:single-factory:overflow:alias:exception");
 }
}`

func TestAdversarialGuardedFinallyKeepsSingleLambdaSourceFactory(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "flat context"
		method, check := "", ""
		if nested {
			name = "nested context"
			method = "public static LongUnaryOperator nested(final long delta){return x->((LongUnaryOperator)(y->y+delta)).applyAsLong(x);}"
			check = "if(ArtifactLambdaOwner.nested(7L).applyAsLong(value)!=add(value,7L))throw new AssertionError(\"independent nested context\");"
		}
		t.Run(name, func(t *testing.T) {
			fixture := strings.ReplaceAll(guardedFinallyLambdaFixture, "EXTRA_NESTED_METHOD", method)
			fixture = strings.ReplaceAll(fixture, "EXTRA_NESTED_CHECK", check)
			testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "30:guarded-finally:single-factory:overflow:alias:exception\n")
		})
	}
}
