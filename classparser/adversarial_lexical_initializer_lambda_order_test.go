package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialLexicalInitializerLambdaOrderKeepsPrivateBridgeAndMembers(t *testing.T) {
	fixture := lexicalInitializerLambdaFixture
	testInterfaceLambdaInitializerFamily(t, fixture, "10:lexical:initializer:lambda:bridge:identity\n")
}

const lexicalInitializerLambdaFixture = `interface InitPacketOperation{long read();}
class InitPacketEffects{static String trace="";static InitPacketOperation bind(String tag,InitPacketOperation operation){trace+=tag;return operation;}}
class InitPacketContract{
 private static final class Cell{private final Object value;private Cell(Object value){this.value=value;}}
 static final InitPacketOperation ZERO=InitPacketEffects.bind("A",()->0L);
 static final InitPacketOperation ONE=InitPacketEffects.bind("B",()->1L);
 static InitPacketOperation captured(long value){return ()->value;}
 static Object echo(Object value){return new Cell(value).value;}
}
public class InitPacketDriver{public static void main(String[]args){
 if(!InitPacketEffects.trace.equals(""))throw new AssertionError("premature initialization");
 InitPacketOperation zero=InitPacketContract.ZERO,one=InitPacketContract.ONE;
 if(!InitPacketEffects.trace.equals("AB")||zero==one||zero!=InitPacketContract.ZERO)throw new AssertionError("initializer once/order/identity");
 int rows=0;Object identity=new Object();for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(Object token:new Object[]{null,identity}){
 if(zero.read()!=0L||one.read()!=1L||InitPacketContract.captured(value).read()!=value||InitPacketContract.echo(token)!=token||!InitPacketEffects.trace.equals("AB"))throw new AssertionError("source order/private bridge/capture identity");rows++;}
 Class<?>[] members=InitPacketContract.class.getDeclaredClasses();if(members.length<1)throw new AssertionError("private member absent");
 System.out.println(rows+":lexical:initializer:lambda:bridge:identity");}}`

func TestAdversarialLexicalInitializerLambdaOrderKeepsAnonymousCounter(t *testing.T) {
	source := strings.Replace(lexicalInitializerLambdaFixture, "class InitPacketContract{", `final class InitPacketContract{
 private final java.util.function.Function<Object,Object> transfer=new java.util.function.Function<Object,Object>(){public Object apply(Object value){return value;}};
 static Object anonymous(Object token){return new InitPacketContract().transfer.apply(token);}`, 1)
	source = strings.Replace(source, "InitPacketContract.echo(token)!=token||", "InitPacketContract.echo(token)!=token||InitPacketContract.anonymous(token)!=token||", 1)
	testInterfaceLambdaInitializerFamily(t, source, "10:lexical:initializer:lambda:bridge:identity\n")
}
