package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const interfaceHandleProtocolFixture = `class InterfaceHandleOwner {
 public interface Callback{void receive(java.util.Collection<String> packet);void receive(Object packet);}
 static java.util.function.Consumer<java.util.Collection<String>> bind(Callback callback){InterfaceHandleDriver.trace+="F";return callback::receive;}
}
class InterfaceHandleDriver{
 static Object observed;static int phase;static String trace="";static final IllegalArgumentException error=new IllegalArgumentException("identity");
 static void apply(java.util.Collection<String> packet){trace+="R";observed=packet;if(phase==1)throw error;packet.add("x");}
 static class Receiver implements InterfaceHandleOwner.Callback{public void receive(java.util.Collection<String> packet){apply(packet);}public void receive(Object packet){throw new AssertionError("wrong physical overload");}}
 public static void main(String[]args){Receiver receiver=new Receiver();java.util.function.Consumer<java.util.Collection<String>> first=InterfaceHandleOwner.bind(receiver),second=InterfaceHandleOwner.bind(receiver);java.util.List<String> a=new java.util.ArrayList<String>(),b=new java.util.ArrayList<String>();a.add("a");b.add("b");first.accept(a);if(observed!=a||!a.toString().equals("[a, x]"))throw new AssertionError("packet binding/effect");phase=1;try{second.accept(b);throw new AssertionError("missing deferred throw");}catch(IllegalArgumentException e){if(e!=error||observed!=b||!b.toString().equals("[b]"))throw new AssertionError("deferred identity/effects");}phase=0;second.accept(b);try{InterfaceHandleOwner.bind(null);throw new AssertionError("missing eager null receiver");}catch(NullPointerException expected){}if(!trace.equals("FFRRRF")||!b.toString().equals("[b, x]"))throw new AssertionError("receiver/order");if(!InterfaceHandleOwner.Callback.class.isMemberClass()||InterfaceHandleOwner.Callback.class.getDeclaringClass()!=InterfaceHandleOwner.class)throw new AssertionError("original interface scope");System.out.println("3:interface:binding:dispatch:scope");}}
`

func interfaceHandleProtocolVariant(generic, defaults, deep, rename bool) (string, string) {
	s, owner := interfaceHandleProtocolFixture, "InterfaceHandleOwner"
	if defaults {
		s = strings.Replace(s, "void receive(java.util.Collection<String> packet);", "default void receive(java.util.Collection<String> packet){InterfaceHandleDriver.apply(packet);}", 1)
		s = strings.Replace(s, "public void receive(java.util.Collection<String> packet){apply(packet);}", "", 1)
	}
	if !generic {
		s = strings.ReplaceAll(s, "void receive(java.util.Collection<String> packet)", "void receive(java.util.Collection packet)")
	}
	if deep {
		s = strings.Replace(s, "class InterfaceHandleOwner {", "class InterfaceHandleOwner {static class Holder {", 1)
		s = strings.Replace(s, "}\nclass InterfaceHandleDriver", "}}\nclass InterfaceHandleDriver", 1)
		s = strings.ReplaceAll(s, "InterfaceHandleOwner.Callback", "InterfaceHandleOwner.Holder.Callback")
		s = strings.ReplaceAll(s, "InterfaceHandleOwner.bind", "InterfaceHandleOwner.Holder.bind")
		s = strings.ReplaceAll(s, "!=InterfaceHandleOwner.class", "!=InterfaceHandleOwner.Holder.class")
	}
	if rename {
		owner = "OriginalInvocationScope"
		s = strings.NewReplacer("InterfaceHandleOwner", owner, "Callback", "ConsumerView", "receive", "deliver").Replace(s)
	}
	return s, owner
}
func TestAdversarialOwnedInterfaceHandleProtocol(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, generic := range []bool{false, true} {
		for _, defaults := range []bool{false, true} {
			for _, deep := range []bool{false, true} {
				for _, rename := range []bool{false, true} {
					t.Run(fmt.Sprintf("generic=%t/default=%t/deep=%t/rename=%t", generic, defaults, deep, rename), func(t *testing.T) {
						s, owner := interfaceHandleProtocolVariant(generic, defaults, deep, rename)
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": s}, debug, "8")
						}, ModernJavac, javac, []string{owner}, "InterfaceHandleDriver", "3:interface:binding:dispatch:scope\n", nil, nativeLexicalExactSignatures)
					})
				}
			}
		}
	}
}
func TestAdversarialOwnedInterfaceHandleConcreteParent(t *testing.T) {
	javac, _ := t04Tools(t)
	s, owner := interfaceHandleProtocolVariant(true, true, true, true)
	s = strings.Replace(s, "public interface ConsumerView{", "public interface ConsumerView extends InterfaceHandleMarker<String>{", 1) + "\ninterface InterfaceHandleMarker<T>{}\n"
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": s}, debug, "8")
	}, ModernJavac, javac, []string{owner}, "InterfaceHandleDriver", "3:interface:binding:dispatch:scope\n", nil, nativeLexicalExactSignatures)
}

func TestAdversarialOwnedInterfaceHandleNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, generic := range []bool{false, true} {
		for _, defaults := range []bool{false, true} {
			t.Run(fmt.Sprintf("generic=%t/default=%t", generic, defaults), func(t *testing.T) {
				s, owner := interfaceHandleProtocolVariant(generic, defaults, false, false)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, s, owner, debug) }, NativeJavac8, javac, []string{owner}, "InterfaceHandleDriver", "3:interface:binding:dispatch:scope\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}
