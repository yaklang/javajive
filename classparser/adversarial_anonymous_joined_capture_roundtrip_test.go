package javaclassparser

import (
	"strings"
	"testing"
)

const nativeJoinedCaptureFixture = `interface JoinedValue{long apply(long delta);Object token();}
class JoinedEffects{static String trace="";static int fail;static final RuntimeException error=new RuntimeException("exact failure");static long word(long input,int branch){trace+=branch;if(fail==branch)throw error;return branch==1?input+17:input-31;}}
class JoinedOwner{JoinedValue make(boolean choose,long input,final Object token){final long selected;if(choose){selected=JoinedEffects.word(input,1);}else{selected=JoinedEffects.word(input,2);}JoinedEffects.trace+="C";return new JoinedValue(){public long apply(long delta){return selected^delta;}public Object token(){return token;}};}}
class JoinedDriver{public static void main(String[]args){Object token=new Object();int rows=0;for(boolean choose:new boolean[]{false,true})for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object identity:new Object[]{null,token}){JoinedEffects.fail=0;JoinedEffects.trace="";JoinedValue value=new JoinedOwner().make(choose,input,identity);java.math.BigInteger selected=java.math.BigInteger.valueOf(input);selected=choose?selected.add(java.math.BigInteger.valueOf(17)):selected.subtract(java.math.BigInteger.valueOf(31));if(value.apply(delta)!=selected.xor(java.math.BigInteger.valueOf(delta)).longValue()||value.token()!=identity||!JoinedEffects.trace.equals(choose?"1C":"2C"))throw new AssertionError("joined word/identity/effects");JoinedEffects.trace="";JoinedEffects.fail=choose?1:2;try{new JoinedOwner().make(choose,input,identity);throw new AssertionError("missing failure");}catch(RuntimeException e){if(e!=JoinedEffects.error||!JoinedEffects.trace.equals(choose?"1":"2"))throw new AssertionError("exact exception/partial effects");}rows++;}System.out.println(rows+":joined:capture:word:identity:order:failure");}}
`

func TestAdversarialAnonymousJoinedCaptureRoundTrip(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		fixture, owner, driver, name := nativeJoinedCaptureFixture, "JoinedOwner", "JoinedDriver", "ordinary"
		fixture = strings.Replace(fixture, "if(value.apply(delta)", "if(!value.getClass().isAnonymousClass()||value.getClass().getEnclosingClass()!=JoinedOwner.class||!value.getClass().getEnclosingMethod().getName().equals(\"make\"))throw new AssertionError(\"original lexical anonymous declaration\");if(value.apply(delta)", 1)
		if renamed {
			fixture = strings.NewReplacer("JoinedOwner", "IndependentJoinScope", "JoinedDriver", "IndependentJoinDriver", "selected", "keptWord").Replace(fixture)
			owner, driver, name = "IndependentJoinScope", "IndependentJoinDriver", "renamed"
		}
		t.Run(name, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, "100:joined:capture:word:identity:order:failure\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousJoinedCaptureAbruptPredecessorRoundTrip(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		fixture := strings.Replace(nativeJoinedCaptureFixture, "else{selected=JoinedEffects.word(input,2);}", "else if(input==0){return null;}else{selected=JoinedEffects.word(input,2);}", 1)
		fixture = strings.Replace(fixture, "if(value.apply(delta)", "if(!choose&&input==0){if(value!=null||!JoinedEffects.trace.equals(\"\"))throw new AssertionError(\"abrupt predecessor effects\");rows++;continue;}if(!value.getClass().isAnonymousClass()||value.getClass().getEnclosingClass()!=JoinedOwner.class||!value.getClass().getEnclosingMethod().getName().equals(\"make\"))throw new AssertionError(\"lexical declaration\");if(value.apply(delta)", 1)
		owner, driver, name := "JoinedOwner", "JoinedDriver", "ordinary"
		if renamed {
			fixture = strings.NewReplacer("JoinedOwner", "AbruptJoinScope", "JoinedDriver", "AbruptJoinDriver", "selected", "retained").Replace(fixture)
			owner, driver, name = "AbruptJoinScope", "AbruptJoinDriver", "renamed"
		}
		t.Run(name, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, "100:joined:capture:word:identity:order:failure\n", nativeLexicalExactSignatures)
		})
	}
}

const nativeJoinedReferenceCaptureFixture = `interface JoinedReference{CharSequence token();long value(long delta);}
class JoinedReferenceEffects{static String trace="";static int fail;static final RuntimeException error=new RuntimeException("reference failure");static <Q>Q pick(Q input,int branch){trace+=branch;if(fail==branch)throw error;return input;}}
class JoinedReferenceOwner<T extends CharSequence>{JoinedReference make(boolean choose,T left,T right,final long seed){final T selected;if(choose){selected=JoinedReferenceEffects.pick(left,1);}else{selected=JoinedReferenceEffects.pick(right,2);}JoinedReferenceEffects.trace+="C";return new JoinedReference(){public CharSequence token(){return selected;}public long value(long delta){return seed+delta;}};}}
class JoinedReferenceDriver{public static void main(String[]args){CharSequence left=new StringBuilder("left"),right=new String("right");int rows=0;for(boolean choose:new boolean[]{false,true})for(CharSequence first:new CharSequence[]{null,left})for(CharSequence second:new CharSequence[]{null,right})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){JoinedReferenceEffects.fail=0;JoinedReferenceEffects.trace="";JoinedReference result=new JoinedReferenceOwner<CharSequence>().make(choose,first,second,seed);if(result.token()!=(choose?first:second)||result.value(delta)!=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(delta)).longValue()||!JoinedReferenceEffects.trace.equals(choose?"1C":"2C")||!result.getClass().isAnonymousClass()||result.getClass().getEnclosingClass()!=JoinedReferenceOwner.class)throw new AssertionError("generic join identity/word/order/declaration");JoinedReferenceEffects.trace="";JoinedReferenceEffects.fail=choose?1:2;try{new JoinedReferenceOwner<CharSequence>().make(choose,first,second,seed);throw new AssertionError("missing reference failure");}catch(RuntimeException e){if(e!=JoinedReferenceEffects.error||!JoinedReferenceEffects.trace.equals(choose?"1":"2"))throw new AssertionError("reference exception/effects");}rows++;}System.out.println(rows+":generic:joined:capture:identity:word:order:failure");}}
`

func TestAdversarialAnonymousJoinedGenericReferenceCaptureRoundTrip(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		fixture, owner, driver, name := nativeJoinedReferenceCaptureFixture, "JoinedReferenceOwner", "JoinedReferenceDriver", "ordinary"
		if renamed {
			fixture = strings.NewReplacer("JoinedReferenceOwner", "IndependentGenericJoin", "JoinedReferenceDriver", "IndependentGenericDriver", "selected", "keptToken").Replace(fixture)
			owner, driver, name = "IndependentGenericJoin", "IndependentGenericDriver", "renamed"
		}
		t.Run(name, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, "200:generic:joined:capture:identity:word:order:failure\n", nativeLexicalExactSignatures)
		})
	}
}
