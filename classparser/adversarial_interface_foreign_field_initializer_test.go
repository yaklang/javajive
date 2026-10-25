package javaclassparser

import "testing"

const interfaceForeignFieldInitializerFixture = `
class InitPacketEffects {static String trace="";static int step(String id,int n){trace+=id;return n;}}
class InitPacketSource {
 static final int I=InitPacketEffects.step("A",Integer.MIN_VALUE);
 static final long J=Long.parseLong("-9223372036854775808");
 static final float F=-Float.parseFloat("0.0");static final double D=-Double.parseDouble("0.0");
 static final byte B=(byte)InitPacketEffects.step("B",-128);static final short S=(short)InitPacketEffects.step("S",-32768);
 static final char C=(char)InitPacketEffects.step("C",65535);static final boolean Z=Boolean.parseBoolean("true");
 static final String TEXT=new String("字𝄞");static int mutable=InitPacketEffects.step("M",7);static volatile int visible=9;
}
interface InitPacketContract {int I=InitPacketSource.I;long J=InitPacketSource.J;float F=InitPacketSource.F;double D=InitPacketSource.D;byte B=InitPacketSource.B;short S=InitPacketSource.S;char C=InitPacketSource.C;boolean Z=InitPacketSource.Z;String TEXT=InitPacketSource.TEXT;int M=InitPacketSource.mutable;int V=InitPacketSource.visible;}
public class InitPacketDriver {public static void main(String[]args){
 if(InitPacketContract.class.getDeclaredMethods().length!=0||!InitPacketEffects.trace.equals(""))throw new AssertionError("ABI or eager initialization");
 if(InitPacketContract.I!=Integer.MIN_VALUE||InitPacketContract.J!=Long.MIN_VALUE||Float.floatToRawIntBits(InitPacketContract.F)!=0x80000000||Double.doubleToRawLongBits(InitPacketContract.D)!=0x8000000000000000L||InitPacketContract.B!=-128||InitPacketContract.S!=-32768||InitPacketContract.C!=65535||!InitPacketContract.Z||InitPacketContract.TEXT!=InitPacketSource.TEXT||InitPacketContract.M!=7||InitPacketContract.V!=9||!InitPacketEffects.trace.equals("ABSCM"))throw new AssertionError("field types order or identity");
 InitPacketSource.mutable=99;InitPacketSource.visible=100;if(InitPacketContract.M!=7||InitPacketContract.V!=9)throw new AssertionError("initializer read repeated");
 System.out.println("foreign:field:initialization:identity:abi");
}}
`

func TestAdversarialInterfaceForeignFieldReadsRetainInitializationAndABI(t *testing.T) {
	testInterfaceLambdaInitializerFamily(t, interfaceForeignFieldInitializerFixture, "foreign:field:initialization:identity:abi\n")
}

func TestAdversarialInterfaceForeignFieldFailureKeepsOriginalCauseAndNoRetry(t *testing.T) {
	testInterfaceLambdaInitializerFamily(t, `class InitPacketEffects {static String trace="";static final RuntimeException fail=new IllegalStateException("cause");static int first(){trace+="A";throw fail;}static int later(){trace+="B";return 7;}}
 class InitPacketSource {static final int first=InitPacketEffects.first();static final int later=InitPacketEffects.later();}
 interface InitPacketContract {int FIRST=InitPacketSource.first;int LATER=InitPacketSource.later;}
 public class InitPacketDriver {public static void main(String[]args){
  if(InitPacketContract.class.getDeclaredMethods().length!=0||!InitPacketEffects.trace.equals(""))throw new AssertionError("ABI or eager initialization");
  Throwable first=null,second=null;try{int n=InitPacketContract.FIRST;}catch(Throwable t){first=t;}try{int n=InitPacketContract.LATER;}catch(Throwable t){second=t;}
  if(!(first instanceof ExceptionInInitializerError)||first.getCause()!=InitPacketEffects.fail||!(second instanceof NoClassDefFoundError)||!InitPacketEffects.trace.equals("A"))throw new AssertionError("first failure or retry");
  System.out.println("foreign:field:first-failure:no-retry");
 }} `, "foreign:field:first-failure:no-retry\n")
}

func TestAdversarialInterfaceForeignFieldCyclesObserveDefaultValues(t *testing.T) {
	for _, sourceFirst := range []bool{false, true} {
		label, entry, early := "interface_first", "int ignored=InitPacketContract.EARLY;", "41"
		if sourceFirst {
			label, entry, early = "source_first", "int ignored=InitPacketSource.FIRST;", "0"
		}
		t.Run(label, func(t *testing.T) {
			fixture := `class InitPacketEffects {static String trace="";static int next(){trace+="S";return 41;}}
 class InitPacketSource {static final int FIRST=InitPacketContract.EARLY;static final int NEXT=InitPacketEffects.next();}
 interface InitPacketContract {int EARLY=InitPacketSource.NEXT;int LATE=InitPacketSource.FIRST;}
 public class InitPacketDriver {public static void main(String[]args){
  if(InitPacketContract.class.getDeclaredMethods().length!=0||!InitPacketEffects.trace.equals(""))throw new AssertionError("ABI or early initialization");
 ` + entry + `if(InitPacketContract.EARLY!=` + early + `||InitPacketContract.LATE!=0||InitPacketSource.FIRST!=0||InitPacketSource.NEXT!=41||!InitPacketEffects.trace.equals("S"))throw new AssertionError("cyclic initialization default read");
  System.out.println("foreign:field:cycle:default-values");
 }}`
			testInterfaceLambdaInitializerFamily(t, fixture, "foreign:field:cycle:default-values\n")
		})
	}
}
