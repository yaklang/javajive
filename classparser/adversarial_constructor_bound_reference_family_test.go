package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const constructorBoundReferenceFamilyFixture = `
import java.util.function.*;import java.lang.reflect.*;
class BoundEffects {
 static int mode;static String trace="";static Object lastValue;static BoundProducer lastProducer;
 static final IllegalStateException failure=new IllegalStateException("same failure");
 static int argument(int tag){trace+="A"+tag+";";if(mode==1)throw failure;return tag;}
}
class BoundProducer {
 void accept(Object value){BoundEffects.trace+="B;";BoundEffects.lastProducer=this;BoundEffects.lastValue=value;if(BoundEffects.mode==3)throw BoundEffects.failure;}
}
public class BoundConstructorOwner {
 private static final class Parts {
  private final Consumer<Object> action;
  Parts(Consumer<Object> action,int tag){BoundEffects.trace+="C"+tag+";";if(BoundEffects.mode==2)throw BoundEffects.failure;this.action=action;}
  Parts(BoundProducer producer,int tag){this(producer::accept,BoundEffects.argument(tag));}
  Parts(BoundProducer producer){this(producer,11);}
  void send(Object value){action.accept(value);}
 }
 public static Object first(BoundProducer producer,Object value){Parts parts=new Parts(producer,7);parts.send(value);return BoundEffects.lastValue;}
 public static Object second(BoundProducer producer,Object value){Parts parts=new Parts(producer);parts.send(value);return BoundEffects.lastValue;}
}
class BoundConstructorDriver {
 public static void main(String[]args)throws Exception{
  BoundProducer first=new BoundProducer(),second=new BoundProducer();Object shared=new Object(),other=new Object();int rows=0;
  for(BoundProducer producer:new BoundProducer[]{null,first,second})for(Object value:new Object[]{null,shared,other})for(int mode=0;mode<4;mode++)for(int route=0;route<2;route++){
   BoundEffects.mode=mode;BoundEffects.trace="";BoundEffects.lastValue=null;BoundEffects.lastProducer=null;Object actual=null;Throwable caught=null;
   try{actual=route==0?BoundConstructorOwner.first(producer,value):BoundConstructorOwner.second(producer,value);}catch(Throwable error){caught=error;}
   int tag=route==0?7:11;String expected=producer==null?"":"A"+tag+";"+(mode==1?"":"C"+tag+";"+(mode==2?"":"B;"));
   if(!BoundEffects.trace.equals(expected))throw new AssertionError("constructor argument/nullcheck/order expected="+expected+" actual="+BoundEffects.trace,caught);
   if(producer==null){if(!(caught instanceof NullPointerException))throw new AssertionError("eager receiver NPE",caught);}
   else if(mode!=0){if(caught!=BoundEffects.failure)throw new AssertionError("failure identity",caught);}
   else if(caught!=null||actual!=value||BoundEffects.lastProducer!=producer)throw new AssertionError("bound receiver/value identity",caught);rows++;
  }
  Class<?>[] members=BoundConstructorOwner.class.getDeclaredClasses();if(members.length!=1||members[0].getDeclaringClass()!=BoundConstructorOwner.class||!Modifier.isPrivate(members[0].getModifiers())||!Modifier.isStatic(members[0].getModifiers())||!Modifier.isFinal(members[0].getModifiers())||!Modifier.isPrivate(members[0].getDeclaredField("action").getModifiers()))throw new AssertionError("complete private lexical member graph");
  System.out.println(rows+":bound:constructor:receiver:nullcheck:order:metadata");
 }
}`

func TestAdversarialBoundReferenceConstructorArgumentKeepsEagerCheckAndMemberFamily(t *testing.T) {
	testConstructorBoundReferenceFamily(t, constructorBoundReferenceFamilyFixture)
}

func TestAdversarialBoundInterfaceConstructorArgumentKeepsDispatchAndMemberFamily(t *testing.T) {
	source := strings.NewReplacer("class BoundProducer {", "interface BoundProducer { void accept(Object value); } class BoundImplementation implements BoundProducer {", "void accept(Object value){", "public void accept(Object value){", "new BoundProducer()", "new BoundImplementation()", "this(producer::accept,", "this((Consumer<Object>)producer::accept,").Replace(constructorBoundReferenceFamilyFixture)
	testConstructorBoundReferenceFamily(t, source)
}

func TestAdversarialBoundResultConstructorArgumentKeepsSAMAndReturnIdentity(t *testing.T) {
	source := strings.NewReplacer("Consumer<Object>", "Function<Object,Object>", "void accept(Object value){", "Object accept(Object value){", "BoundEffects.lastValue=value;if(BoundEffects.mode==3)throw BoundEffects.failure;}", "BoundEffects.lastValue=value;if(BoundEffects.mode==3)throw BoundEffects.failure;return value;}", "void send(Object value){action.accept(value);}", "Object send(Object value){return action.apply(value);}", "parts.send(value);return BoundEffects.lastValue;", "return parts.send(value);").Replace(constructorBoundReferenceFamilyFixture)
	testConstructorBoundReferenceFamily(t, source)
}

func testConstructorBoundReferenceFamily(t *testing.T, fixture string) {
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
			for _, owner := range []string{"BoundConstructorOwner", "RenamedBoundCtorFamily"} {
				t.Run(owner, func(t *testing.T) {
					for _, named := range []bool{false, true} {
						label := "absent-parameters"
						if named {
							label = "named-parameters"
						}
						t.Run(label, func(t *testing.T) {
							source := strings.ReplaceAll(fixture, "BoundConstructorOwner", owner)
							testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
								return compileLambdaParameterMetadataFixture(t, compiler, profile, source, owner, debug, named)
							}, profile, compiler, []string{owner}, "BoundConstructorDriver", "72:bound:constructor:receiver:nullcheck:order:metadata\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
								t.Helper()
								if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
									t.Fatalf("original parameter roles changed %s\n%s\n%s", name, x, y)
								}
								if x, y := constructorInvocationThrowsDeclarations(t, a), constructorInvocationThrowsDeclarations(t, b); x != y {
									t.Fatalf("extra constructor helper/declaration %s\n%s\n%s", name, x, y)
								}
							})
						})
					}
				})
			}
		})
	}
}
