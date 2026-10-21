package javaclassparser

import "testing"

func TestAdversarialInterfaceOrdinaryReferencesKeepDeferredEffectsExceptionsAndCompleteABI(t *testing.T) {
	testInterfaceLambdaInitializerFamily(t, `import java.util.function.*;import java.math.BigInteger;import java.io.*;
class InitPacketEffects implements Closeable {
 static int calls;static final IOException failure=new IOException("original cause");
 final int value;final boolean fail;
 InitPacketEffects(int value,boolean fail){this.value=value;this.fail=fail;}
 int read(){calls++;return value^0x6b347819;}
 static String format(Object value){calls++;return String.valueOf(value);}
 static String format(String value){throw new AssertionError("wrong overload");}
 public void close()throws IOException{calls++;if(fail)throw failure;}
}
interface InitPacketAction{void accept(Closeable value)throws IOException;}
interface InitPacketContract {
 IntUnaryOperator WORD=value->value+1;
 Function<Object,String> FORMAT=InitPacketEffects::format;
 ToIntFunction<InitPacketEffects> READ=InitPacketEffects::read;
 InitPacketAction CLOSE=Closeable::close;
}
public class InitPacketDriver {public static void main(String[]args)throws Exception {
 int rows=0;
 if(InitPacketEffects.calls!=0)throw new AssertionError("premature invocation");
 Object[] factories={InitPacketContract.WORD,InitPacketContract.FORMAT,InitPacketContract.READ,InitPacketContract.CLOSE};
 if(factories.length!=4||InitPacketEffects.calls!=0)throw new AssertionError("reference creation ran deferred target");
 for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  if(InitPacketContract.WORD.applyAsInt(value)!=BigInteger.valueOf(value).add(BigInteger.ONE).intValue())throw new AssertionError("word model");rows++;
 }
 for(Object value:new Object[]{null,"λ",Integer.valueOf(7)}){
  if(!InitPacketContract.FORMAT.apply(value).equals(value==null?"null":value.toString()))throw new AssertionError("overload target");rows++;
 }
 for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  int expected=BigInteger.valueOf(value).xor(BigInteger.valueOf(0x6b347819)).intValue();
  if(InitPacketContract.READ.applyAsInt(new InitPacketEffects(value,false))!=expected)throw new AssertionError("unbound receiver model");rows++;
 }
 InitPacketContract.CLOSE.accept(new InitPacketEffects(7,false));rows++;
 try{InitPacketContract.CLOSE.accept(new InitPacketEffects(9,true));throw new AssertionError("lost checked failure");}catch(IOException failure){if(failure!=InitPacketEffects.failure)throw new AssertionError("failure identity");}rows++;
 try{InitPacketContract.CLOSE.accept(null);throw new AssertionError("lost null receiver check");}catch(NullPointerException expected){}rows++;
 if(InitPacketEffects.calls!=10)throw new AssertionError("changed deferred call count "+InitPacketEffects.calls);
 System.out.println(rows+":ordinary-reference:effects:checked:null:overload");
}}`, "16:ordinary-reference:effects:checked:null:overload\n")
}
