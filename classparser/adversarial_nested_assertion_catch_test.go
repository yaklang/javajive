package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The independent caller checks evaluation, exception identity and cleanup rather
// than generated text. Its original target is executed before rebuilding it.
const assertionCatchProtocolFixture = `class AssertionCatchEffects {
 static String trace="";static boolean left,right,closeFail;static int phase;
 static java.io.IOException operationToken=new java.io.IOException("operation"),closeToken=new java.io.IOException("close");
 static RuntimeException conditionToken=new IllegalStateException("condition"),messageToken=new IllegalArgumentException("message");
 static void operation()throws java.io.IOException{trace+="O";if(phase==4)return;throw operationToken;}
 static boolean first(){trace+="L";if(phase==1)throw conditionToken;return left;}
 static boolean second(){trace+="R";if(phase==2)throw conditionToken;return right;}
 static Object message(Object value){trace+="M";if(phase==3)throw messageToken;return value;}
}
class AssertionCatchResource implements java.io.Closeable{AssertionCatchResource(){AssertionCatchEffects.trace+="A";}public void close()throws java.io.IOException{AssertionCatchEffects.trace+="C";if(AssertionCatchEffects.closeFail)throw AssertionCatchEffects.closeToken;}}
class AssertionCatchOwner {static void execute(boolean swallow,Object message)throws java.io.IOException{
 RESOURCE_START
 try{AssertionCatchEffects.operation();}catch(java.io.IOException e){if(swallow){assert PREDICATE:AssertionCatchEffects.message(message);return;}throw e;}
 RESOURCE_END
}}
class AssertionCatchDriver {public static void main(String[]a)throws Exception{
 boolean enabled=ENABLED,resource=RESOURCE;ClassLoader.getSystemClassLoader().setClassAssertionStatus("AssertionCatchOwner",enabled);
 int checked=0;Throwable payload=new IllegalArgumentException("payload");
 for(int bits=0;bits<4;bits++)for(int take=0;take<2;take++)for(int phase=0;phase<5;phase++)for(int closing=0;closing<2;closing++){
 AssertionCatchEffects.left=(bits&1)!=0;AssertionCatchEffects.right=(bits&2)!=0;AssertionCatchEffects.phase=phase;AssertionCatchEffects.closeFail=closing!=0;AssertionCatchEffects.trace="";AssertionCatchEffects.operationToken=new java.io.IOException("operation");AssertionCatchEffects.closeToken=new java.io.IOException("close");AssertionCatchEffects.conditionToken=new IllegalStateException("condition");AssertionCatchEffects.messageToken=new IllegalArgumentException("message");
 Throwable expected=null;boolean assertion=false;String trace=resource?"AO":"O";
 if(phase!=4&&take==0)expected=AssertionCatchEffects.operationToken;
 else if(phase!=4&&enabled){trace+="L";if(phase==1)expected=AssertionCatchEffects.conditionToken;
 else{boolean failure=AssertionCatchEffects.left;if(!failure){trace+="R";if(phase==2)expected=AssertionCatchEffects.conditionToken;else failure=AssertionCatchEffects.right;}
 if(expected==null&&failure){trace+="M";if(phase==3)expected=AssertionCatchEffects.messageToken;else assertion=true;}}}
 if(resource)trace+="C";boolean suppressed=resource&&closing!=0&&(expected!=null||assertion);
 if(resource&&closing!=0&&expected==null&&!assertion)expected=AssertionCatchEffects.closeToken;
 Throwable observed=null;try{AssertionCatchOwner.execute(take!=0,payload);}catch(Throwable e){observed=e;}
 if(assertion){if(!(observed instanceof AssertionError)||observed.getCause()!=payload)throw new AssertionError("assertion payload",observed);}
 else if(observed!=expected)throw new AssertionError("exception identity phase="+phase+" bits="+bits,observed);
 if(!AssertionCatchEffects.trace.equals(trace))throw new AssertionError("evaluation order:"+trace+"/"+AssertionCatchEffects.trace);
 if(observed!=null){Throwable[] saved=observed.getSuppressed();if(suppressed){if(saved.length!=1||saved[0]!=AssertionCatchEffects.closeToken)throw new AssertionError("close suppression",observed);}else if(saved.length!=0)throw new AssertionError("unexpected suppression",observed);}
 checked++;
 }
 java.lang.reflect.Field flag=AssertionCatchOwner.class.getDeclaredField("$assertionsDisabled");flag.setAccessible(true);if(!flag.isSynthetic()||flag.getModifiers()!=0x1018||flag.getBoolean(null)==enabled)throw new AssertionError("compiler assertion status");
 System.out.println("assertion-catch:"+checked+":order:identity:suppression");}}
`

func assertionCatchProtocolVariant(enabled, resources, materialized, rename bool) (string, string) {
	predicate := "!(AssertionCatchEffects.first()||AssertionCatchEffects.second())"
	if materialized {
		predicate = "(AssertionCatchEffects.first()||AssertionCatchEffects.second())==false"
	}
	start, end := "", ""
	if resources {
		start = "try(AssertionCatchResource r=new AssertionCatchResource()){"
		end = "}"
	}
	s := strings.NewReplacer("RESOURCE_START", start, "RESOURCE_END", end, "PREDICATE", predicate, "ENABLED", fmt.Sprint(enabled), "RESOURCE", fmt.Sprint(resources)).Replace(assertionCatchProtocolFixture)
	owner := "AssertionCatchOwner"
	if rename {
		owner = "RenamedHandlerScope"
		s = strings.NewReplacer("AssertionCatchOwner", owner, "first()", "predicateA()", "second()", "predicateB()").Replace(s)
	}
	return s, owner
}
func TestAdversarialNestedAssertionCatchProtocol(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, enabled := range []bool{false, true} {
		for _, resources := range []bool{false, true} {
			for _, materialized := range []bool{false, true} {
				for _, rename := range []bool{false, true} {
					t.Run(fmt.Sprintf("enabled=%t/resources=%t/materialized=%t/rename=%t", enabled, resources, materialized, rename), func(t *testing.T) {
						s, owner := assertionCatchProtocolVariant(enabled, resources, materialized, rename)
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": s}, debug, "8")
						}, ModernJavac, javac, []string{owner}, "AssertionCatchDriver", "assertion-catch:80:order:identity:suppression\n", nil, nativeLexicalExactSignatures)
					})
				}
			}
		}
	}
}
func TestAdversarialNestedAssertionCatchNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, enabled := range []bool{false, true} {
		for _, resources := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/resources=%t", enabled, resources), func(t *testing.T) {
				s, owner := assertionCatchProtocolVariant(enabled, resources, true, true)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, s, owner, debug) }, NativeJavac8, javac, []string{owner}, "AssertionCatchDriver", "assertion-catch:80:order:identity:suppression\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}

// A nullable entry value exercises the compiler's guarded normal and exceptional
// cleanup, including no close at all when the resource is null. Direct cleanup
// variants above must keep their different original evaluation domain.
func nullableAssertionCatchProtocolVariant(enabled, nilResource bool) (string, string) {
	s, owner := assertionCatchProtocolVariant(enabled, true, true, true)
	s = strings.Replace(s, "try(AssertionCatchResource r=new AssertionCatchResource()){", "try(AssertionCatchResource r="+fmt.Sprint(nilResource)+"?null:new AssertionCatchResource()){", 1)
	s = strings.Replace(s, "resource=true;", "resource="+fmt.Sprint(!nilResource)+";", 1)
	return s, owner
}
func TestAdversarialNullableNestedAssertionCatchProtocol(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, enabled := range []bool{false, true} {
		for _, nilResource := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/null=%t", enabled, nilResource), func(t *testing.T) {
				s, owner := nullableAssertionCatchProtocolVariant(enabled, nilResource)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": s}, debug, "8")
				}, ModernJavac, javac, []string{owner}, "AssertionCatchDriver", "assertion-catch:80:order:identity:suppression\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}
func TestAdversarialNullableNestedAssertionCatchNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, enabled := range []bool{false, true} {
		for _, nilResource := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/null=%t", enabled, nilResource), func(t *testing.T) {
				s, owner := nullableAssertionCatchProtocolVariant(enabled, nilResource)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, s, owner, debug) }, NativeJavac8, javac, []string{owner}, "AssertionCatchDriver", "assertion-catch:80:order:identity:suppression\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}
