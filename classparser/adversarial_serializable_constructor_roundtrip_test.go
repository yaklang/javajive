package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The delegating constructor owns a serializable factory while the destination
// constructor owns an ordinary null fallback. Rebuilding either one's target,
// capture timing or transient field behavior must not alter the other.
func TestAdversarialSerializableConstructorPreservesNullFallbackAndTransientField(t *testing.T) {
	fixture := strings.Replace(serializableStaticReferenceFixture, "class SerializableReferences {", `class SerializableReferences implements java.io.Serializable {
 private static final long serialVersionUID=37L;
 final transient java.util.function.Function<String,String> mapper;
 SerializableReferences(){this((java.util.function.Function<String,String>&java.io.Serializable)ExternalOperations::Aa);}
 SerializableReferences(java.util.function.Function<String,String> mapper){this.mapper=mapper==null?ExternalOperations::BB:mapper;}
 String apply(String value){return mapper.apply(value);}
`, 1) + `
class ConstructorSerializationDriver {
 public static void main(String[]args)throws Exception{
  SerializableReferences standard=new SerializableReferences();
  SerializableReferences fallback=new SerializableReferences(null);
  SerializableReferences supplied=new SerializableReferences(ExternalOperations::BB);
  if(!SerializationState.trace.equals(""))throw new AssertionError("eager constructor target initialization");
  if(!(standard.mapper instanceof java.io.Serializable)||fallback.mapper instanceof java.io.Serializable)throw new AssertionError("factory serialization protocols conflated");
  java.lang.invoke.SerializedLambda form=SerializationDriver.form(standard.mapper);
  if(!form.getCapturingClass().equals("SerializableReferences")||!form.getImplClass().equals("ExternalOperations")||!form.getImplMethodName().equals("Aa")||!form.getImplMethodSignature().equals("(Ljava/lang/String;)Ljava/lang/String;")||form.getCapturedArgCount()!=0)throw new AssertionError("constructor implementation identity");
  java.util.function.Function<String,String> roundtrip=(java.util.function.Function<String,String>)SerializationDriver.copy(standard.mapper);
  for(String value:new String[]{null,"","x","\u0000","\u20ac\ud83d\ude42"}){
   String raw=value==null?"null":value;
   if(!(raw+":A").equals(standard.apply(value))||!(raw+":A").equals(roundtrip.apply(value))||!(raw+":B").equals(fallback.apply(value))||!(raw+":B").equals(supplied.apply(value)))throw new AssertionError("constructor mapper algorithm");
  }
  if(!SerializationState.trace.equals("ZAABBAABBAABBAABBAABB"))throw new AssertionError("constructor mapper effect order:"+SerializationState.trace);
  SerializableReferences restored=(SerializableReferences)SerializationDriver.copy(standard);
  if(restored.mapper!=null)throw new AssertionError("transient final mapper was invented during deserialization");
  try{restored.apply("x");throw new AssertionError("transient mapper must remain absent");}catch(NullPointerException expected){}
  System.out.println("constructor:default:null-fallback:serializable:transient:25");
 }
}
`
	javac, java := t04Tools(t)
	for _, renamed := range []bool{false, true} {
		owner, source := "SerializableReferences", fixture
		if renamed {
			owner = "SeparateConstructorFactories"
			source = strings.NewReplacer("SerializableReferences", owner, "ExternalOperations", "SeparateOperations").Replace(source)
		}
		for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
			t.Run(owner+"/"+string(profile), func(t *testing.T) {
				compiler := javac
				if profile == NativeJavac8 {
					compiler = os.Getenv("JAVA8_JAVAC")
					if compiler == "" {
						t.Fatal("JAVA8_JAVAC is required")
					}
				}
				compile := func(debug string) map[string][]byte {
					if profile == NativeJavac8 {
						return nativePrivateEnumCompile(t, source, owner, debug)
					}
					return nativeCompileDebugClasses(t, source, debug)
				}
				files := compile("none")
				oracle := t04RunJava(t, java, adversarialAssertionClasses(t, files), "ConstructorSerializationDriver")
				if oracle != "constructor:default:null-fallback:serializable:transient:25\n" {
					t.Fatal("original constructor/serialization oracle", oracle)
				}
				testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{owner}, "ConstructorSerializationDriver", oracle, nil, nil, []string{"normal", "no-core-cleanups"}, nativeLexicalExactSignatures)
			})
		}
	}
}
