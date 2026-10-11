package javaclassparser

import (
	"strings"
	"testing"
)

// Partial T[][] stores use AASTORE, not a source cast to T[]. The original
// caller checks exception precedence and evaluates every producer once. The
// lexical field assignment additionally requires T's enclosing first bound.
const nativeLexicalGenericArrayStoreFixture = `class RowEffects{static String trace="";static int fail;static final RuntimeException error=new IllegalStateException("original");static void mark(String s,int stage){trace+=s;if(fail==stage)throw error;}static int index(int n){mark("I",2);return n;}static Number[] row(Number[] n){mark("V",3);return n;}static Number number(Number n){mark("V",3);return n;}}
class RowOwner<T extends Number>{T[] outer;RowOwner(T[] outer){this.outer=outer;}class Reader{final T[][] rows;Reader(T[][] rows){this.rows=rows;}T[][] matrix(){RowEffects.mark("A",1);return rows;}T[] outerArray(){RowEffects.mark("A",1);return RowOwner.this.outer;}void writeRow(int index,Number[] row){RowEffects.mark("A",1);rows[RowEffects.index(index)]=(T[])RowEffects.row(row);}void writeOuter(int index,Number value){Object[] array=outerArray();array[RowEffects.index(index)]=RowEffects.number(value);}void replace(Number[] value){RowOwner.this.outer=(T[])RowEffects.row(value);}}}
class RowDriver{static void check(String error,String expected,String trace){if(!error.equals(expected)||!RowEffects.trace.equals(trace))throw new AssertionError(error+":"+expected+":"+RowEffects.trace+":"+trace);}public static void main(String[]args){int count=0;Number[][] replacements={null,new Integer[]{9},new Double[]{2.5},new Number[]{7}};Number[] values={null,Integer.valueOf(9),Double.valueOf(2.5)};for(int state=0;state<3;state++)for(int pos=-1;pos<=1;pos++)for(int fail=0;fail<4;fail++){for(int choice=0;choice<replacements.length;choice++){Integer[][] rows=state==0?null:state==1?new Integer[0][]:new Integer[][]{new Integer[]{3}};RowOwner<Integer> owner=new RowOwner<Integer>(null);RowOwner<Integer>.Reader reader=owner.new Reader(rows);RowEffects.fail=fail;RowEffects.trace="";String error="ok";try{reader.writeRow(pos,replacements[choice]);}catch(RuntimeException e){error=e==RowEffects.error?"original":e.getClass().getSimpleName();}String expected=fail!=0?"original":state==0?"NullPointerException":state==1||pos!=0?"ArrayIndexOutOfBoundsException":choice>1?"ArrayStoreException":"ok";check(error,expected,fail==1?"A":fail==2?"AI":"AIV");if(expected.equals("ok")&&rows[0]!=replacements[choice])throw new AssertionError("row identity");count++;}for(int choice=0;choice<values.length;choice++){Integer[] array=state==0?null:state==1?new Integer[0]:new Integer[]{3};RowOwner<Integer> owner=new RowOwner<Integer>(array);RowOwner<Integer>.Reader reader=owner.new Reader(null);RowEffects.fail=fail;RowEffects.trace="";String error="ok";try{reader.writeOuter(pos,values[choice]);}catch(RuntimeException e){error=e==RowEffects.error?"original":e.getClass().getSimpleName();}String expected=fail!=0?"original":state==0?"NullPointerException":state==1||pos!=0?"ArrayIndexOutOfBoundsException":choice==2?"ArrayStoreException":"ok";check(error,expected,fail==1?"A":fail==2?"AI":"AIV");if(expected.equals("ok")&&array[0]!=values[choice])throw new AssertionError("scalar identity");count++;}}for(Number[] value:replacements){RowOwner raw=new RowOwner(null);RowOwner.Reader reader=raw.new Reader(null);RowEffects.fail=0;RowEffects.trace="";reader.replace(value);if(raw.outer!=value||!RowEffects.trace.equals("V"))throw new AssertionError("lexical field identity or narrowing");count++;}System.out.println(count+":lexical:generic:arrays:original-order");}}`

func TestNativeLexicalGenericArrayStoresKeepOriginalChecks(t *testing.T) {
	for _, owner := range []string{"RowOwner", "DifferentMatrixScope"} {
		t.Run(owner, func(t *testing.T) {
			fixture := strings.ReplaceAll(nativeLexicalGenericArrayStoreFixture, "RowOwner", owner)
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, "RowDriver", "256:lexical:generic:arrays:original-order\n", nativeLexicalExactSignatures)
		})
	}
}
