package javaclassparser

import "testing"

// The ASM-shaped carrier updates a destination array even after the first true
// result. A short-circuit OR, copied receiver, lost store or replayed producer
// changes the independent trace, array state or original exception identity.
func TestAdversarialBooleanArrayMergeCarrierRoundTrip(t *testing.T) {
	t.Setenv("JDEC_BOOL_ZERO_LITERAL_OFF", "1")
	roundTripGenericFlow(t, "BooleanArrayMergeReview", `import java.util.*;
class MergeDestination {int[] values;}
class MergeEffects {static String trace;static int fault;static final RuntimeException failure=new IllegalArgumentException("original");}
public class BooleanArrayMergeReview {
 final int[] values;BooleanArrayMergeReview(int[] values){this.values=values;}
 static boolean changed(int[] source,int[] target,int index){MergeEffects.trace+=index;if(index==MergeEffects.fault)throw MergeEffects.failure;int old=target[index];if(old==source[index])return false;target[index]=source[index];return true;}
 boolean merge(MergeDestination destination,boolean extra){boolean updated=false;if(destination.values==null){destination.values=new int[values.length];updated=true;}for(int index=0;index<values.length;index++)updated|=changed(values,destination.values,index);if(extra){updated|=changed(values,destination.values,0);}return updated;}
 public static void main(String[]args){for(int shape=0;shape<5;shape++)for(int fault=-1;fault<3;fault++)for(boolean extra:new boolean[]{false,true}){int[] input=new int[]{4,0,7};MergeDestination destination=new MergeDestination();destination.values=shape==0?null:shape==1?input:shape==2?new int[]{4,0,7}:shape==3?new int[]{0,0,0}:new int[]{4};int[] old=destination.values;MergeEffects.trace="";MergeEffects.fault=fault;try{System.out.print(new BooleanArrayMergeReview(input).merge(destination,extra)+":");}catch(Throwable error){System.out.print(error.getClass().getName()+":"+(error==MergeEffects.failure)+":");}System.out.println((old==destination.values)+":"+(input==destination.values)+":"+Arrays.toString(destination.values)+":"+Arrays.toString(input)+":"+MergeEffects.trace);}}
}`, Precision, Compatibility, "legacy")
}

// The two switch defaults converge on false. Equality is deliberately effectful
// and can throw, so the oracle also pins case fallthrough order and once-only
// evaluation rather than just the final Boolean result.
func TestAdversarialSiblingSwitchDefaultEqualityIdentityRoundTrip(t *testing.T) {
	t.Setenv("JDEC_COLLECTIONS4_REMAINING_OFF", "1")
	roundTripGenericFlow(t, "SwitchDefaultIdentityReview", `class EqualityProbe {static String trace;static int fault;static final RuntimeException failure=new IllegalStateException("original");final int wanted;EqualityProbe(int wanted){this.wanted=wanted;}public boolean equals(Object value){int candidate=((Integer)value).intValue();trace+=candidate;if(candidate==fault)throw failure;return candidate==wanted;}}
public class SwitchDefaultIdentityReview {
 Object first,second,third;int size;
 boolean contains(Object needle){if(needle==null){switch(size){case 3:if(third==null)return true;case 2:if(second==null)return true;case 1:if(first==null)return true;}}else{switch(size){case 3:if(needle.equals(third))return true;case 2:if(needle.equals(second))return true;case 1:if(needle.equals(first))return true;}}return false;}
 public static void main(String[]args){for(int size=-1;size<5;size++)for(int wanted=0;wanted<4;wanted++)for(int fault=-1;fault<4;fault++){SwitchDefaultIdentityReview receiver=new SwitchDefaultIdentityReview();receiver.first=Integer.valueOf(1);receiver.second=Integer.valueOf(2);receiver.third=Integer.valueOf(3);receiver.size=size;Object first=receiver.first,second=receiver.second,third=receiver.third;EqualityProbe.trace="";EqualityProbe.fault=fault;try{System.out.print(receiver.contains(new EqualityProbe(wanted))+":");}catch(Throwable error){System.out.print((error==EqualityProbe.failure)+":");}System.out.println((first==receiver.first)+":"+(second==receiver.second)+":"+(third==receiver.third)+":"+EqualityProbe.trace);}for(int size=-1;size<5;size++)for(int absent=0;absent<4;absent++){SwitchDefaultIdentityReview receiver=new SwitchDefaultIdentityReview();receiver.size=size;receiver.first=absent==1?null:Integer.valueOf(1);receiver.second=absent==2?null:Integer.valueOf(2);receiver.third=absent==3?null:Integer.valueOf(3);System.out.println(size+":"+absent+":"+receiver.contains(null));}}
}`, Precision, Compatibility, "legacy")
}
