package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Ordinary external static targets retain their JVM identity across source
// reconstruction. The unchanged binary driver also observes the original
// compiler's deserializer on malformed SerializedLambda tuples: rejection,
// acceptance and exception order are part of this compiler protocol.
const serializableStaticReferenceFixture = `
class SerializationState { static String trace=""; }
class ExternalOperations {
 static { SerializationState.trace+="Z"; }
 static String Aa(String value) { SerializationState.trace+="A";return value==null?"null:A":value+":A"; }
 static String BB(String value) { SerializationState.trace+="B";return value==null?"null:B":value+":B"; }
}
class SerializableReferences {
 static java.util.function.Function<String,String> first() {
  return (java.util.function.Function<String,String> & java.io.Serializable)ExternalOperations::Aa;
 }
 static java.util.function.Function<String,String> second() {
  return (java.util.function.Function<String,String> & java.io.Serializable)ExternalOperations::BB;
 }
}
class SerializationDriver {
 static java.lang.invoke.SerializedLambda form(Object value) throws Exception {
  java.lang.reflect.Method method=value.getClass().getDeclaredMethod("writeReplace");
  method.setAccessible(true);return (java.lang.invoke.SerializedLambda)method.invoke(value);
 }
 static Object copy(Object value) throws Exception {
  java.io.ByteArrayOutputStream bytes=new java.io.ByteArrayOutputStream();
  try(java.io.ObjectOutputStream out=new java.io.ObjectOutputStream(bytes)){out.writeObject(value);}
  try(java.io.ObjectInputStream in=new java.io.ObjectInputStream(new java.io.ByteArrayInputStream(bytes.toByteArray()))){return in.readObject();}
 }
 static String observe(java.lang.invoke.SerializedLambda original,int change) throws Exception {
  String iface=original.getFunctionalInterfaceClass(),name=original.getFunctionalInterfaceMethodName();
  String sam=original.getFunctionalInterfaceMethodSignature(),owner=original.getImplClass();
  String impl=original.getImplMethodName(),descriptor=original.getImplMethodSignature(),instantiated=original.getInstantiatedMethodType();
  int kind=original.getImplMethodKind();Object[]captured=new Object[0];
  switch(change){case 1:iface="missing/Interface";break;case 2:name="missing";break;case 3:sam="()V";break;
   case 4:owner="missing/Owner";break;case 5:impl="missing";break;case 6:descriptor="()V";break;
   case 7:kind=5;break;case 8:instantiated="()V";break;case 9:captured=new Object[]{"extra"};break;
   case 10:iface=null;break;case 11:name=null;break;case 12:sam=null;break;case 13:owner=null;break;
   case 14:impl=null;break;case 15:descriptor=null;break;case 16:impl=impl.equals("Aa")?"BB":"Aa";break;}
  java.lang.invoke.SerializedLambda value=new java.lang.invoke.SerializedLambda(SerializableReferences.class,iface,name,sam,kind,owner,impl,descriptor,instantiated,captured);
  java.lang.reflect.Method method=SerializableReferences.class.getDeclaredMethod("$deserializeLambda$",java.lang.invoke.SerializedLambda.class);
  method.setAccessible(true);
  try {Object result=method.invoke(null,value);return "ok:"+form(result).getImplMethodName();}
  catch(java.lang.reflect.InvocationTargetException wrapped){return "fail:"+wrapped.getCause().getClass().getName();}
 }
 public static void main(String[]args) throws Exception {
  java.util.function.Function<String,String>a=SerializableReferences.first(),b=SerializableReferences.second();
  if(!SerializationState.trace.equals(""))throw new AssertionError("target initialized at capture");
  int rows=0;
  for(java.util.function.Function<String,String>f:new java.util.function.Function[]{a,b}){
   java.lang.invoke.SerializedLambda original=form(f);String impl=original.getImplMethodName();
   if(original.getImplMethodKind()!=6||!original.getImplClass().equals("ExternalOperations")||!original.getFunctionalInterfaceClass().equals("java/util/function/Function")||!original.getFunctionalInterfaceMethodName().equals("apply")||!original.getFunctionalInterfaceMethodSignature().equals("(Ljava/lang/Object;)Ljava/lang/Object;")||!original.getImplMethodSignature().equals("(Ljava/lang/String;)Ljava/lang/String;")||!original.getInstantiatedMethodType().equals("(Ljava/lang/String;)Ljava/lang/String;")||original.getCapturedArgCount()!=0)throw new AssertionError("original serialized identity");
   java.util.function.Function<String,String>next=(java.util.function.Function<String,String>)copy(f);
   if(!form(next).getImplMethodName().equals(impl))throw new AssertionError("serialization target");
   for(String input:new String[]{null,"","x","\u0000","\u20ac\ud83d\ude42"}){
    String expected=(input==null?"null":input)+(impl.equals("Aa")?":A":":B");
    if(!expected.equals(f.apply(input))||!expected.equals(next.apply(input)))throw new AssertionError("functional target/binding");rows++;
   }
   for(int change=0;change<=16;change++){System.out.println(impl+":"+change+":"+observe(original,change));rows++;}
  }
  if(!SerializationState.trace.equals("ZAAAAAAAAAABBBBBBBBBB"))throw new AssertionError("target initialization/effect order:"+SerializationState.trace);
  System.out.println(rows+":serializable:static:identity:protocol");
 }
}
`

func TestAdversarialSerializableStaticReferencesRetainOriginalDeserializerProtocol(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, renamed := range []bool{false, true} {
		fixture, owner, label := serializableStaticReferenceFixture, "SerializableReferences", "original"
		if renamed {
			label, owner = "renamed", "SeparateFactories"
			fixture = strings.NewReplacer("SerializableReferences", owner, "ExternalOperations", "SeparateOperations").Replace(fixture)
		}
		t.Run(label, func(t *testing.T) {
			// The protocol oracle is emitted by an original binary driver, not
			// inferred from the decompiler's source or bootstrap diagnostics.
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(string(profile), func(t *testing.T) {
					compiler := javac
					if profile == NativeJavac8 {
						compiler = os.Getenv("JAVA8_JAVAC")
						if compiler == "" {
							t.Fatal("JAVA8_JAVAC is required")
						}
					}
					compile := func(debug string) map[string][]byte {
						if profile == NativeJavac8 {
							return nativePrivateEnumCompile(t, fixture, owner, debug)
						}
						return nativeCompileDebugClasses(t, fixture, debug)
					}
					files := compile("none")
					_, java := t04Tools(t)
					oracle := t04RunJava(t, java, adversarialAssertionClasses(t, files), "SerializationDriver")
					if !strings.HasSuffix(oracle, "44:serializable:static:identity:protocol\n") {
						t.Fatal("original complete protocol oracle", oracle)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{owner}, "SerializationDriver", oracle, nil, nil, []string{"normal", "no-core-cleanups"}, nativeLexicalExactSignatures)
				})
			}
		})
	}
}
