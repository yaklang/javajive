package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Only the owner family is rebuilt; the driver, fields and effect producers
// remain original bytecode. Generated-looking field names stress lexical lookup;
// own private fields hide equal-named inherited storage. Null enclosing words
// distinguish an intermediate dereference from a final store after RHS effects.
// Concrete interfaces exercise every storage width without borrowing a separate
// covariant-bridge proof; the reference case also uses the erased JDK Supplier.
func testAnonymousSetterTypedCase(t *testing.T, selected string) {
	cases := []struct{ name, typ, values, equal, zero string }{
		{"integer", "int", "new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}", "%s==%s", "0"},
		{"wide", "long", "new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}", "%s==%s", "0L"},
		{"floating", "double", "new double[]{Double.longBitsToDouble(0x7ff8000000001234L),Double.NEGATIVE_INFINITY,-0.0d,0.0d,Double.POSITIVE_INFINITY}", "Double.doubleToRawLongBits(%s)==Double.doubleToRawLongBits(%s)", "0.0d"},
		{"single", "float", "new float[]{Float.intBitsToFloat(0x7fc01234),Float.NEGATIVE_INFINITY,-0.0f,0.0f,Float.POSITIVE_INFINITY}", "Float.floatToRawIntBits(%s)==Float.floatToRawIntBits(%s)", "0.0f"},
		{"byte", "byte", "new byte[]{Byte.MIN_VALUE,-1,0,1,Byte.MAX_VALUE}", "%s==%s", "0"},
		{"short", "short", "new short[]{Short.MIN_VALUE,-1,0,1,Short.MAX_VALUE}", "%s==%s", "0"},
		{"character", "char", "new char[]{0,1,0x7fff,0x8000,0xffff}", "%s==%s", "0"},
		{"boolean", "boolean", "new boolean[]{false,true}", "%s==%s", "false"},
		{"reference", "Object", "new Object[]{null,new Object(),new StringBuilder(\"identity\")}", "%s==%s", "null"},
		{"array", "Object[]", "new Object[][]{null,new Object[0],new Object[]{new Object()}}", "%s==%s", "null"},
	}
	for _, row := range cases {
		if row.name != selected {
			continue
		}
		shapes := []string{"named", "deep named", "anonymous"}
		if row.name == "reference" {
			shapes = append(shapes, "generic Supplier")
		}
		for _, shape := range shapes {
			for _, storage := range []string{"inherited", "own private"} {
				for _, prefix := range []string{"Holder", "Cove"} {
					t.Run(row.name+"/"+shape+"/"+storage+"/"+prefix, func(t *testing.T) {
						T := row.typ
						consumer := fmt.Sprintf("class Writer{%s put(%s next){return var0=p.Effects.next(next);}}public %s put(%s next){return new Writer().put(next);}", T, T, T, T)
						if shape == "deep named" {
							consumer = fmt.Sprintf("class Writer{class Leaf{%s put(%s next){return var0=p.Effects.next(next);}}%s put(%s next){return new Leaf().put(next);}}public %s put(%s next){return new Writer().put(next);}", T, T, T, T, T, T)
						}
						if shape == "anonymous" {
							consumer = fmt.Sprintf("public %s put(final %s next){return new p.Fetch(){public %s get(){return var0=p.Effects.next(next);}}.get();}", T, T, T)
						}
						if shape == "generic Supplier" {
							consumer = "public Object put(final Object next){return new java.util.function.Supplier<Object>(){public Object get(){return var0=p.Effects.next(next);}}.get();}"
						}

						own := ""
						if storage == "own private" {
							own = "private " + T + " var0;"
						}
						equal := func(a, b string) string { return fmt.Sprintf(row.equal, a, b) }
						base := fmt.Sprintf("package p;public abstract class Base{protected %s var0;public abstract %s put(%s next);public abstract %s get();public %s inherited(){return var0;}}", T, T, T, T, T)
						effects := fmt.Sprintf("package p;public class Effects{public static int calls;public static boolean fail;public static final RuntimeException failure=new IllegalArgumentException(\"identity\");public static %s next(%s value){calls++;if(fail)throw failure;return value;}}", T, T)
						owner := "package q;public class HolderOwner{public static p.Base build(){return new p.Base(){" + own + consumer + "public " + T + " get(){return var0;}};}}"
						inherited := "value"
						if storage == "own private" {
							inherited = row.zero
						}
						driver := fmt.Sprintf(`package q;public class HolderDriver{public static void main(String[]args)throws Exception{p.Base owner=HolderOwner.build();if(!owner.getClass().isAnonymousClass()||owner.getClass().getEnclosingMethod().getDeclaringClass()!=HolderOwner.class)throw new AssertionError("scope");int rows=0;for(%s value:%s){p.Effects.fail=false;p.Effects.calls=0;if(!(%s)||!(%s)||!(%s)||p.Effects.calls!=1)throw new AssertionError("typed storage/result/owner/order");p.Effects.fail=true;p.Effects.calls=0;try{owner.put(value);throw new AssertionError("RHS exception lost");}catch(RuntimeException e){if(e!=p.Effects.failure)throw new AssertionError("exception identity");}if(p.Effects.calls!=1||!(%s))throw new AssertionError("abrupt write/effect");rows++;}System.out.println(rows+":typed:anonymous:storage:identity:order");}}`, T, row.values, equal("owner.put(value)", "value"), equal("owner.get()", "value"), equal("owner.inherited()", inherited), equal("owner.get()", "value"))
						if shape == "named" || shape == "deep named" {
							reflection := `Class<?> writer=null;for(Class<?> c:owner.getClass().getDeclaredClasses())if(c.getSimpleName().equals("Writer"))writer=c;if(writer==null)throw new AssertionError("original named scope");java.lang.reflect.Constructor<?> wc=writer.getDeclaredConstructor(owner.getClass());wc.setAccessible(true);Object nullOuter=wc.newInstance((Object)null);java.lang.reflect.Method write=writer.getDeclaredMethod("put",TYPE_TOKEN.class);write.setAccessible(true);for(boolean fail:new boolean[]{false,true}){p.Effects.fail=fail;p.Effects.calls=0;try{write.invoke(nullOuter,(Object)value);throw new AssertionError("null enclosing receiver");}catch(java.lang.reflect.InvocationTargetException e){Throwable cause=e.getCause();if(fail?cause!=p.Effects.failure:!(cause instanceof NullPointerException))throw new AssertionError("receiver/RHS throw order");}if(p.Effects.calls!=1)throw new AssertionError("RHS evaluated before final null store");}`
							if shape == "deep named" {
								reflection += `Class<?> leaf=null;for(Class<?> c:writer.getDeclaredClasses())if(c.getSimpleName().equals("Leaf"))leaf=c;if(leaf==null)throw new AssertionError("original deep scope");java.lang.reflect.Constructor<?> lc=leaf.getDeclaredConstructor(writer);lc.setAccessible(true);Object[] receivers={lc.newInstance(nullOuter),lc.newInstance((Object)null)};java.lang.reflect.Method nested=leaf.getDeclaredMethod("put",TYPE_TOKEN.class);nested.setAccessible(true);for(int depth=0;depth<2;depth++)for(boolean fail:new boolean[]{false,true}){p.Effects.fail=fail;p.Effects.calls=0;try{nested.invoke(receivers[depth],(Object)value);throw new AssertionError("deep null enclosing receiver");}catch(java.lang.reflect.InvocationTargetException e){Throwable cause=e.getCause();if(fail&&depth==0?cause!=p.Effects.failure:!(cause instanceof NullPointerException))throw new AssertionError("original dereference/RHS failure point");}if(p.Effects.calls!=(depth==0?1:0))throw new AssertionError("original dereference/RHS effect order");}`
							}
							reflection = strings.ReplaceAll(reflection, "TYPE_TOKEN", T)
							driver = strings.Replace(driver, "rows++;", reflection+"rows++;", 1)
						}
						sources := map[string]string{"p/Fetch.java": "package p;public interface Fetch{" + T + " get();}", "p/Base.java": base, "p/Effects.java": effects, "q/HolderOwner.java": owner, "q/HolderDriver.java": driver}
						ss := map[string]string{}
						for n, s := range sources {
							ss[strings.ReplaceAll(n, "Holder", prefix)] = strings.ReplaceAll(s, "Holder", prefix)
						}
						count := 5
						if row.name == "boolean" {
							count = 2
						}
						if row.name == "reference" || row.name == "array" {
							count = 3
						}
						testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/"+prefix+"Owner", "q."+prefix+"Driver", fmt.Sprintf("%d:typed:anonymous:storage:identity:order\n", count), func(t *testing.T, debug string) map[string][]byte {
							return nativeCompileSourceReleaseClasses(t, ss, debug, "8")
						}, nil, []string{"normal"})
					})
				}
			}
		}
	}
}

func TestAdversarialAnonymousIntegerSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "integer")
}
func TestAdversarialAnonymousWideSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "wide")
}
func TestAdversarialAnonymousFloatingSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "floating")
}
func TestAdversarialAnonymousBooleanSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "boolean")
}
func TestAdversarialAnonymousReferenceSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "reference")
}
func TestAdversarialAnonymousArraySetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "array")
}

func TestAdversarialAnonymousSinglePrecisionSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "single")
}
func TestAdversarialAnonymousByteSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "byte")
}
func TestAdversarialAnonymousShortSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "short")
}
func TestAdversarialAnonymousCharacterSetterKeepsLexicalStorage(t *testing.T) {
	testAnonymousSetterTypedCase(t, "character")
}
