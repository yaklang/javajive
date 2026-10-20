package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const interfaceLambdaInitializerFixture = `class InitPacketEffects{static String trace="";static InitPacketContract bind(String tag,InitPacketContract value){trace+=tag;return value;}}
interface InitPacketContract{
 InitPacketContract ZERO=InitPacketEffects.bind("A",()->0L);
 InitPacketContract ONE=InitPacketEffects.bind("B",()->1L);
 long read();
 static InitPacketContract captured(long value){return ()->value;}
}
public class InitPacketDriver{public static void main(String[]args){
 if(!InitPacketEffects.trace.equals(""))throw new AssertionError("premature interface initialization");
 InitPacketContract zero=InitPacketContract.ZERO,one=InitPacketContract.ONE;
 if(!InitPacketEffects.trace.equals("AB")||zero==one||zero!=InitPacketContract.ZERO||one!=InitPacketContract.ONE)throw new AssertionError("initializer order/identity");
 int rows=0;for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}){
  InitPacketContract result=InitPacketContract.captured(value);
  if(zero.read()!=0L||one.read()!=1L||result.read()!=value||!InitPacketEffects.trace.equals("AB"))throw new AssertionError("static/ordinary factory result");rows++;
 }System.out.println(rows+":interface:initializer:factory:order:identity");
}}`

// An interface has no source static block. Its original clinit factories must
// retain their own occurrences when assignments become field initializers;
// ordinary method factories still share the compiler's implementation counter.
func TestAdversarialInterfaceLambdaInitializerKeepsFactoryOrderAndIdentity(t *testing.T) {
	testInterfaceLambdaInitializerFamily(t, interfaceLambdaInitializerFixture, "5:interface:initializer:factory:order:identity\n")
}

func TestAdversarialInterfaceDirectLambdaInitializerKeepsCompleteMemberFamily(t *testing.T) {
	source := strings.NewReplacer(`InitPacketEffects.bind("A",()->0L)`, `()->0L`, `InitPacketEffects.bind("B",()->1L)`, `()->1L`, `trace.equals("AB")`, `trace.equals("")`).Replace(interfaceLambdaInitializerFixture)
	testInterfaceLambdaInitializerFamily(t, source, "5:interface:initializer:factory:order:identity\n")
}

func TestAdversarialInterfaceLambdaInitializerKeepsAbruptCompletionAndCause(t *testing.T) {
	source := `class InitPacketEffects{static String trace="";static final IllegalStateException failure=new IllegalStateException("same original cause");static InitPacketContract bind(InitPacketContract value){trace+="F";throw failure;}}
interface InitPacketContract{InitPacketContract FAILED=InitPacketEffects.bind(()->17L);long read();}
public class InitPacketDriver{public static void main(String[]args){Throwable first=null,second=null;Object original=InitPacketEffects.failure;
 try{Object unused=InitPacketContract.FAILED;}catch(Throwable e){first=e;}
 if(!(first instanceof ExceptionInInitializerError)||first.getCause()!=original||!InitPacketEffects.trace.equals("F"))throw new AssertionError("first initialization cause/order",first);
 try{Object unused=InitPacketContract.FAILED;}catch(Throwable e){second=e;}
 if(!(second instanceof NoClassDefFoundError)||!InitPacketEffects.trace.equals("F"))throw new AssertionError("initialization retried or cause category changed",second);
 System.out.println("interface:initializer:first-failure:identity:no-retry");}}`
	testInterfaceLambdaInitializerFamily(t, source, "interface:initializer:first-failure:identity:no-retry\n")
}

func testInterfaceLambdaInitializerFamily(t *testing.T, fixture, want string) {
	t.Helper()
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, prefix := range []string{"InitPacket", "FieldFactory"} {
				t.Run(prefix, func(t *testing.T) {
					source := fixture
					source = strings.ReplaceAll(source, "InitPacket", prefix)
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
						units := map[string]string{prefix + "Driver.java": source}
						if profile == NativeJavac8 {
							return localGetterCompileJavac8(t, units, debug)
						}
						return nativeCompileSourceReleaseClasses(t, units, debug, "8")
					}, profile, compiler, []string{prefix + "Contract"}, prefix+"Driver", want, nil, nativeLexicalExactSignatures)
				})
			}
		})
	}
}
