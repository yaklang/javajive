package javaclassparser

import "testing"

// A completed constructor expression owns only its own operands. A nested
// ordinary call still needs its independent array declaration/source-view proof.
// Repeated conditional varargs and a loop stress distinct stack-backed bindings.
func TestAdversarialConstructorNestedVarargsKeepsArrayBindingsRoundTrip(t *testing.T) {
	const f = `import java.io.*;import java.util.*;class NestedVarargsReport{long count,bytes;long[] frequencies;NestedVarargsReport(long c,long b,long[]f){count=c;bytes=b;frequencies=f;}public String toString(){ByteArrayOutputStream out=new ByteArrayOutputStream();PrintStream p=null;try{p=new PrintStream(out,false,"UTF-8");p.println(new StringBuilder().append("bytes:").append(bytes).append(count!=0?new StringBuilder().append("/").append(String.format(Locale.ROOT,"%.1f",new Object[]{Double.valueOf((double)bytes/(double)count)})).toString():"").toString());StringBuilder s=new StringBuilder();for(int i=0;i<frequencies.length;i++){if(frequencies[i]!=0){if(s.length()>0)s.append(",");s.append(i);s.append(":");s.append(frequencies[i]);}}p.println(new StringBuilder().append("ratio:").append(count!=0?new StringBuilder().append(String.format(Locale.ROOT,"%.2f",new Object[]{Double.valueOf((double)bytes/(double)count)})).append("/").append((Object)s).toString():"").toString());p.println(new StringBuilder().append("tail:").append(count!=0?String.format(Locale.ROOT,"%.1f",new Object[]{Double.valueOf((double)bytes/(double)count)}):"").toString());}catch(IOException e){throw new IllegalStateException(e);}finally{if(p!=null)p.close();}try{return out.toString("UTF-8");}catch(UnsupportedEncodingException e){throw new IllegalStateException(e);}}}public class NestedVarargsDriver{public static void main(String[]args){for(long n:new long[]{0,2,7})System.out.print(new NestedVarargsReport(n,17,new long[]{0,3,0,9}));}}`
	roundTripGenericFlowUnits(t, "NestedVarargsDriver", f, nil, []string{"NestedVarargsReport"}, Precision, Compatibility, "legacy")
}
