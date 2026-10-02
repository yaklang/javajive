package javaclassparser

import "testing"

func TestAdversarialCachedBooleanGuardContinuationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CachedBooleanGuard", `
class CachedGuardWitness {static Class<?> cached;static String trace="";static int fail;static final RuntimeException failure=new RuntimeException("same");static Class<?> load(){trace+="L";if(fail==1)throw failure;return Void.class;}static boolean boxed(Class<?> type){trace+="B";if(fail==2)throw failure;return type==Integer.class;}}
public class CachedBooleanGuard {
 static boolean accept(Class<?> type){return (type!=null && type!=Void.TYPE && type!=(CachedGuardWitness.cached==null?(CachedGuardWitness.cached=CachedGuardWitness.load()):CachedGuardWitness.cached) && type.isPrimitive()) || CachedGuardWitness.boxed(type);}
 static boolean range(Class<?> type,int n){return (n>=0 && n<2 && type!=(CachedGuardWitness.cached==null?(CachedGuardWitness.cached=CachedGuardWitness.load()):CachedGuardWitness.cached)) || CachedGuardWitness.boxed(type);}
 static boolean negated(Class<?> type){return !((type==null || type==Void.TYPE || type==(CachedGuardWitness.cached==null?(CachedGuardWitness.cached=CachedGuardWitness.load()):CachedGuardWitness.cached)) && !CachedGuardWitness.boxed(type));}
 public static void main(String[] args){for(Class<?> initial:new Class<?>[]{null,Void.class,String.class})for(Class<?> type:new Class<?>[]{null,Void.TYPE,Void.class,Integer.TYPE,Integer.class,String.class,int[].class})for(int fail=0;fail<3;fail++)for(int variant=0;variant<3;variant++)for(int n=-1;n<=2;n++){CachedGuardWitness.cached=initial;CachedGuardWitness.trace="";CachedGuardWitness.fail=fail;try{System.out.println((variant==0?accept(type):variant==1?range(type,n):negated(type))+":"+CachedGuardWitness.trace+":"+(CachedGuardWitness.cached==initial));}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e==CachedGuardWitness.failure)+":"+CachedGuardWitness.trace+":"+(CachedGuardWitness.cached==initial));}}}
}`, Precision, Compatibility, "legacy")
}
