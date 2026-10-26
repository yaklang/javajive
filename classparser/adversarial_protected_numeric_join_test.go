package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialProtectedNumericBranchesKeepJoinedValue(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class GuardedNumber {
 public static int reads,after;
 public static final RuntimeException TOKEN=new RuntimeException("original");
 public static int step(int value,int stage,int fault){reads++;if(stage==fault)throw TOKEN;return value;}
 public static int recover(int seed,boolean branch,int fault,boolean rethrow){int value=seed;
  try{if(branch)value=step(seed+3,1,fault);else value=step(seed-5,1,fault);value=step(value*7,2,fault);}
  catch(RuntimeException failure){if(failure!=TOKEN)throw new AssertionError("identity");if(rethrow)throw failure;return value;}
  finally{after++;}
  return value;
 }
 public static int parse(String text,int base){reads++;return Integer.parseInt(text,base);}
 public static long decode(String text,boolean hex){int value;
  try{if(hex)value=parse(text,16);else value=parse(text,10);}catch(NumberFormatException failure){return Long.MIN_VALUE;}
  after++;if(value>65535){char[] pair=Character.toChars(value);return ((long)pair[0]<<16)|pair[1];}else return value;
 }
}
class GuardedNumberDriver {public static void main(String[]args){int checks=0;
 String[] inputs={"0","1","-1","17","255","65535","65536","1114111","1114112","2147483647","2147483648","-2147483648","-2147483649","7fffffff","80000000","ffff","10ffff","110000","-ff","","xyz","+41","000123","FFFFFFFF"};
 for(String input:inputs)for(boolean hex:new boolean[]{false,true})for(int repeat=0;repeat<3;repeat++){
  int word=0;boolean parseFailure=false,rangeFailure=false;long expected=Long.MIN_VALUE;
  try{java.math.BigInteger parsed=new java.math.BigInteger(input,hex?16:10);if(parsed.compareTo(java.math.BigInteger.valueOf(Integer.MIN_VALUE))<0||parsed.compareTo(java.math.BigInteger.valueOf(Integer.MAX_VALUE))>0)parseFailure=true;else word=parsed.intValue();}catch(NumberFormatException e){parseFailure=true;}
  if(!parseFailure){rangeFailure=word>0x10ffff;if(!rangeFailure){if(word<=65535)expected=word;else{int offset=word-65536;expected=((long)(0xd800+(offset>>>10))<<16)|(0xdc00+(offset&1023));}}}
  GuardedNumber.reads=GuardedNumber.after=0;
  try{long actual=GuardedNumber.decode(input,hex);if(rangeFailure||actual!=expected)throw new AssertionError("joined value "+input+" "+hex+" "+actual+" "+expected);}
  catch(IllegalArgumentException e){if(!rangeFailure)throw new AssertionError("unexpected failure",e);}
  if(GuardedNumber.reads!=1||GuardedNumber.after!=(parseFailure?0:1))throw new AssertionError("protected effects");checks++;
 }
 for(int seed:new int[]{Integer.MIN_VALUE,-37,-1,0,1,29,Integer.MAX_VALUE})for(boolean branch:new boolean[]{false,true})for(int fault=0;fault<=2;fault++)for(boolean rethrow:new boolean[]{false,true}){
  GuardedNumber.reads=GuardedNumber.after=0;int first=branch?seed+3:seed-5;int expected=fault==1?seed:fault==2?first:first*7;
  try{int actual=GuardedNumber.recover(seed,branch,fault,rethrow);if((fault!=0&&rethrow)||actual!=expected)throw new AssertionError("handler input state");}
  catch(RuntimeException failure){if(fault==0||!rethrow||failure!=GuardedNumber.TOKEN)throw new AssertionError("original exception",failure);}
  if(GuardedNumber.after!=1||GuardedNumber.reads!=(fault==1?1:2))throw new AssertionError("exception effect order");checks++;
 }
 System.out.println(checks+":protected:numeric:join");}}
`
	for _, name := range []string{"GuardedNumber", "ParsedWord"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "GuardedNumber", name), name, name+"Driver", "228:protected:numeric:join\n")
		})
	}
}
