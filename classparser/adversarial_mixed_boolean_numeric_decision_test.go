package javaclassparser

import "testing"

func TestAdversarialMixedBooleanNumericDecisionRoundTrip(t *testing.T) {
	const fixture = `
class MixedWords {
 static boolean field;
 static int between(boolean first,boolean second,int a,int value,int b){boolean low=first?!greater(a,value):less(a,value);boolean high=second?!greater(value,b):less(value,b);int bits=0;if(low)bits+=1;if(high)bits+=2;if(low&&high)return bits+20;return bits;}

 static boolean greater(int a,int b){return a>b;}static boolean less(int a,int b){return a<b;}
 static boolean inclusive(boolean yes,int a,int b){boolean word=yes?!greater(a,b):less(a,b);return word;}
 static int mark(boolean b){return b?71:29;}static int mark(int n){return n+1000;}
 static int overloaded(boolean yes,int a,int b){return mark(yes?!greater(a,b):less(a,b));}
 static boolean stored(boolean yes,int a,int b){field=yes?!greater(a,b):less(a,b);return field;}
 static boolean boxed(boolean yes,boolean first,Boolean second){return yes?!first:second.booleanValue();}
 static int numeric(boolean yes,boolean first,boolean second){int word=yes?(first?1:0):(second?1:0);return word;}
}
class MixedWordsDriver {public static void main(String[]args){int rows=0;for(int a:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int b:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean yes:new boolean[]{false,true}){long delta=(long)a-(long)b;boolean lo=yes?(long)a<=0:(long)a<0;boolean hi=yes?(long)b>=0:(long)b>0;int bits=(lo?1:0)+(hi?2:0);if(MixedWords.between(yes,yes,a,0,b)!=bits+(lo&&hi?20:0))throw new AssertionError("materialized decision words");boolean expected=yes?delta<=0:delta<0;if(MixedWords.inclusive(yes,a,b)!=expected||MixedWords.stored(yes,a,b)!=expected||MixedWords.overloaded(yes,a,b)!=(expected?71:29))throw new AssertionError("mixed predicate/overload");rows++;}for(boolean yes:new boolean[]{false,true})for(boolean x:new boolean[]{false,true})for(boolean y:new boolean[]{false,true}){if(MixedWords.boxed(yes,x,Boolean.valueOf(y))!=(yes?!x:y)||MixedWords.numeric(yes,x,y)!=(yes?(x?1:0):(y?1:0)))throw new AssertionError("numeric view");rows++;}if(!MixedWords.boxed(true,false,null))throw new AssertionError("unused null");rows++;try{MixedWords.boxed(false,false,null);throw new AssertionError("missing null fault");}catch(NullPointerException expected){rows++;}if(rows!=60)throw new AssertionError("coverage "+rows);System.out.println("60:mixed:predicate:word:overload:field:null");}}
`
	testNativeIndependentFamilyFixture(t, fixture, []string{"MixedWords"}, "MixedWordsDriver", "60:mixed:predicate:word:overload:field:null\n", nativeLexicalExactSignatures)
}
