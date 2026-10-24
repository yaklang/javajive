package javaclassparser

import (
	"fmt"
	"os"
	"testing"
)

func testNativeAnonymousCovariantSuperReturnCalls(t *testing.T, native bool) {
	for _, row := range []struct {
		name, iface, implementation, typ, values, bridgeType string
		rows                                                 int
	}{
		{"number_subclass", `package p;public interface Supply<R>{R produce();}`, "p.Supply<Number>", "Integer", `new Integer[]{null,Integer.valueOf(Integer.MIN_VALUE),Integer.valueOf(0),Integer.valueOf(Integer.MAX_VALUE)}`, "Object", 4},
		{"array_components", `package p;public interface Supply<R>{R[] produce();}`, "p.Supply<Number>", "Integer[]", `new Integer[][]{null,new Integer[0],new Integer[]{null,Integer.valueOf(-1)}}`, "Object[]", 3},
		{"own_class_hierarchy", `package p;public interface Supply<K,V>{V produce();}`, "p.Supply<String,p.ParentToken>", "p.Token", `new p.Token[]{null,new p.Token(0),new p.Token(-1),new p.Token(Integer.MAX_VALUE)}`, "Object", 4},
		{"array_cloneable", `package p;public interface Supply<R>{R produce();}`, "p.Supply<Cloneable>", "p.Token[]", `new p.Token[][]{null,new p.Token[0],new p.Token[]{null,new p.Token(-1)}}`, "Object", 3},
		{"primitive_array_serializable", `package p;public interface Supply<R>{R produce();}`, "p.Supply<java.io.Serializable>", "int[]", `new int[][]{null,new int[0],new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}}`, "Object", 3},
		{"string_interface", `package p;public interface Supply<R>{R produce();}`, "p.Supply<CharSequence>", "String", `new String[]{null,new String(""),new String("中文"),new String("identity")}`, "Object", 4},
	} {
		t.Run(row.name, func(t *testing.T) {
			sources := map[string]string{
				"p/Supply.java":      row.iface,
				"p/ParentToken.java": `package p;public class ParentToken{}`,
				"p/Token.java":       `package p;public final class Token extends ParentToken{public final int value;public Token(int value){this.value=value;}}`,
				"p/Effects.java":     `package p;public class Effects{public static Object scope;public static int calls;public static boolean fail;public static final RuntimeException failure=new IllegalArgumentException("original");public static Object next(Object value){calls++;if(fail)throw failure;return value;}}`,
				"q/SourceOwner.java": "package q;public class SourceOwner{public static " + row.typ + " read(final " + row.typ + " value){return (" + row.typ + ")new p.Entry(){public Object read(){return new " + row.implementation + "(){public " + row.typ + " produce(){p.Effects.scope=this;return (" + row.typ + ")p.Effects.next(value);}}.produce();}}.read();}}",
				"p/Entry.java":       `package p;public abstract class Entry{public abstract Object read();}`,
			}
			want := fmt.Sprintf("%d:covariant:source-and-bridge:identity:effects:scope\n", row.rows*2)
			sources["q/SourceDriver.java"] = fmt.Sprintf(`package q;public class SourceDriver{public static void main(String[]args)throws Exception{int rows=0;for(%s value:%s)for(boolean fail:new boolean[]{false,true}){p.Effects.calls=0;p.Effects.fail=fail;p.Effects.scope=null;boolean caught=false;try{if(SourceOwner.read(value)!=value)throw new AssertionError("typed identity");}catch(RuntimeException ex){if(ex!=p.Effects.failure)throw ex;caught=true;}if(caught!=fail||p.Effects.calls!=1)throw new AssertionError("typed effects or original failure identity");Object instance=p.Effects.scope;if(instance==null||!instance.getClass().isAnonymousClass()||!instance.getClass().getEnclosingMethod().getDeclaringClass().isAnonymousClass()||instance.getClass().getEnclosingMethod().getDeclaringClass().getEnclosingMethod().getDeclaringClass()!=SourceOwner.class||!instance.getClass().getEnclosingMethod().getName().equals("read"))throw new AssertionError("original allocation source scope");java.lang.reflect.Method bridge=null;for(java.lang.reflect.Method m:instance.getClass().getDeclaredMethods())if(m.isBridge()){if(bridge!=null||!m.isSynthetic()||m.getReturnType()!=%s.class||m.getParameterTypes().length!=0)throw new AssertionError("original erased bridge ABI");bridge=m;}if(bridge==null)throw new AssertionError("bridge missing");bridge.setAccessible(true);p.Effects.calls=0;caught=false;try{if(bridge.invoke(instance)!=value)throw new AssertionError("bridge identity");}catch(java.lang.reflect.InvocationTargetException ex){if(ex.getCause()!=p.Effects.failure)throw ex;caught=true;}if(caught!=fail||p.Effects.calls!=1||p.Effects.scope!=instance)throw new AssertionError("bridge changed dispatch or effects");rows++;}System.out.println(rows+":covariant:source-and-bridge:identity:effects:scope");}}`, row.typ, row.values, row.bridgeType)
			if native {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for independent compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return fieldOwnerCompileJava8Sources(t, sources, debug) }, NativeJavac8, compiler, []string{"q/SourceOwner"}, "q.SourceDriver", want, nil, nativeLexicalExactSignatures, nativeCovariantBridgeABI, func(t *testing.T, n string, a, b []byte) {
					if nativeAnonymousAccessorShape(t, a) != nativeAnonymousAccessorShape(t, b) {
						t.Fatalf("accessor ABI changed %s", n)
					}
				})
				return
			}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/SourceOwner", "q.SourceDriver", want, func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
		})
	}
}

func TestAdversarialAnonymousCovariantSubstitutedSuperReturn(t *testing.T) {
	testNativeAnonymousCovariantSuperReturnCalls(t, false)
}
func TestAdversarialAnonymousCovariantSubstitutedSuperReturnNativeCompiler(t *testing.T) {
	testNativeAnonymousCovariantSuperReturnCalls(t, true)
}
