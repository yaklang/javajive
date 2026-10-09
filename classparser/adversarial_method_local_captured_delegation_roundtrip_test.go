package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialMethodLocalCapturedDelegationNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 original/rebuild oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("native compiler identity", err, string(version))
	}
	for _, layout := range []string{"instance", "static", "reference object", "reference matrix"} {
		t.Run(layout, func(t *testing.T) {
			fixture := localCapturedDelegationFixture
			if layout == "static" {
				fixture = strings.ReplaceAll(fixture, "Object make(final int n,", "static Object make(final int n,")
				fixture = strings.ReplaceAll(fixture, "return DelegationOwner.this;", "return null;")
				fixture = strings.ReplaceAll(fixture, ".invoke(b)!=owner", ".invoke(b)!=null")
			}
			if layout == "reference object" || layout == "reference matrix" {
				actual, formal, token, candidates := "String", "Object", `new String("same")`, `new String[]{null,token}`
				if layout == "reference matrix" {
					actual, formal, token, candidates = "String[][]", "Object[]", `new String[][]{{"row"}}`, `new String[][][]{null,token}`
					fixture = strings.ReplaceAll(fixture, "Object t)", "Object[] t)")
				}
				fixture = strings.ReplaceAll(fixture, "final Object token", "final "+actual+" token")
				fixture = strings.ReplaceAll(fixture, "super(n,word,token)", "super(n,word,("+formal+")token)")
				fixture = strings.ReplaceAll(fixture, "Object token=new Object()", actual+" token="+token)
				fixture = strings.ReplaceAll(fixture, "Object value:new Object[]{null,token}", actual+" value:"+candidates)
			}
			compile := func(debug string) map[string][]byte {
				root := t.TempDir()
				path := filepath.Join(root, "DelegationOwner.java")
				if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); err != nil {
					t.Fatal("authored original compile", err, string(log))
				}
				entries, err := os.ReadDir(root)
				if err != nil {
					t.Fatal(err)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						b, err := os.ReadFile(filepath.Join(root, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						files[entry.Name()] = b
					}
				}
				return files
			}
			testNativeIndependentCompilerFamilyFixture(t, compile, NativeJavac8, javac, []string{"DelegationOwner"}, "DelegationDriver", "18:local:delegation\n", nil, nativeLexicalExactSignatures)
		})
	}
}

const localCapturedDelegationFixture = `class DelegationBase{final int number;final long seen;final Object seenToken;DelegationBase(int a,long b,Object t){number=a;seen=b;seenToken=capture();if(t!=seenToken)throw new AssertionError("pre-super capture");}Object capture(){return null;}}
class DelegationOwner{Object make(final int n,final long word,final Object token){class Entry extends DelegationBase{Entry(){super(n,word,token);}Object capture(){return token;}long get(){return word;}int n(){return n;}Object owner(){return DelegationOwner.this;}}return new Entry();}}
class DelegationDriver{public static void main(String[]args)throws Exception{DelegationOwner owner=new DelegationOwner();Object token=new Object();int rows=0;for(int n:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE})for(long word:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(Object value:new Object[]{null,token}){DelegationBase b=(DelegationBase)owner.make(n,word,value);if(b.number!=n||b.seen!=word||b.seenToken!=value||b.capture()!=value)throw new AssertionError("bound super args and capture");Object get=b.getClass().getDeclaredMethod("get").invoke(b);if(((Long)get).longValue()!=word||b.getClass().getDeclaredMethod("owner").invoke(b)!=owner)throw new AssertionError("wide and enclosing");rows++;}System.out.println(rows+":local:delegation");}}`

func TestAdversarialMethodLocalCapturedDelegationAbstractParent(t *testing.T) {
	fixture := strings.ReplaceAll(localCapturedDelegationFixture, "class DelegationBase", "abstract class DelegationBase")
	fixture = strings.ReplaceAll(fixture, "Object capture(){return null;}", "abstract Object capture();")
	for _, prefix := range []string{"Delegation", "AbstractLocal"} {
		t.Run(prefix, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, strings.ReplaceAll(fixture, "Delegation", prefix), []string{prefix + "Owner"}, prefix+"Driver", "18:local:delegation\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialMethodLocalCapturedDelegationKeepsFloatingWords(t *testing.T) {
	const fixture = `class FloatingWordsBase{final int floatWord;final long doubleWord;FloatingWordsBase(float f,double d){floatWord=Float.floatToRawIntBits(readFloat());doubleWord=Double.doubleToRawLongBits(readDouble());if(floatWord!=Float.floatToRawIntBits(f)||doubleWord!=Double.doubleToRawLongBits(d))throw new AssertionError("pre-super floating words");}float readFloat(){return 0;}double readDouble(){return 0;}}
class FloatingWordsOwner{Object make(final float f,final double d){class Entry extends FloatingWordsBase{Entry(){super(f,d);}float readFloat(){return f;}double readDouble(){return d;}}return new Entry();}}
class FloatingWordsDriver{public static void main(String[]args){FloatingWordsOwner owner=new FloatingWordsOwner();int rows=0;for(int f:new int[]{0,0x80000000,0x3f800000,0x7f800000,0xff800000,0x7fc00001,0xffc54321})for(long d:new long[]{0L,0x8000000000000000L,0x3ff0000000000000L,0x7ff0000000000000L,0xfff0000000000000L,0x7ff8000000000001L,0xfff8123456789abcL}){FloatingWordsBase value=(FloatingWordsBase)owner.make(Float.intBitsToFloat(f),Double.longBitsToDouble(d));if(value.floatWord!=f||value.doubleWord!=d||Float.floatToRawIntBits(value.readFloat())!=f||Double.doubleToRawLongBits(value.readDouble())!=d)throw new AssertionError("raw floating word transfer");rows++;}System.out.println(rows+":local:floating:words");}}`
	for _, prefix := range []string{"FloatingWords", "SeparateIEEE"} {
		t.Run(prefix, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, strings.ReplaceAll(fixture, "FloatingWords", prefix), []string{prefix + "Owner"}, prefix+"Driver", "49:local:floating:words\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialMethodLocalCapturedDelegationOriginalIdentity(t *testing.T) {
	for _, prefix := range []string{"Delegation", "SeparateLocal"} {
		for _, layout := range []string{"instance", "static", "repeated word", "enclosing word"} {
			t.Run(prefix+"/"+layout, func(t *testing.T) {
				f := localCapturedDelegationFixture
				if layout == "static" {
					f = strings.ReplaceAll(f, "Object make(final int n,", "static Object make(final int n,")
					f = strings.ReplaceAll(f, "return DelegationOwner.this;", "return null;")
					f = strings.ReplaceAll(f, ".invoke(b)!=owner", ".invoke(b)!=null")
				}
				if layout == "repeated word" {
					f = strings.ReplaceAll(f, "DelegationBase(int a,long b,Object t)", "DelegationBase(int a,long b,Object t,int again)")
					f = strings.ReplaceAll(f, "seen=b;seenToken", "if(again!=a)throw new AssertionError(\"same captured word\");seen=b;seenToken")
					f = strings.ReplaceAll(f, "super(n,word,token)", "super(n,word,token,n)")
				}
				if layout == "enclosing word" {
					f = strings.ReplaceAll(f, "DelegationBase(int a,long b,Object t)", "DelegationBase(DelegationOwner expected,int a,long b,Object t)")
					f = strings.ReplaceAll(f, "number=a;seen=b;", "if(enclosing()!=expected)throw new AssertionError(\"pre-super enclosing word\");number=a;seen=b;")
					f = strings.ReplaceAll(f, "Object capture(){return null;}", "Object enclosing(){return null;}Object capture(){return null;}")
					f = strings.ReplaceAll(f, "super(n,word,token)", "super(DelegationOwner.this,n,word,token)")
					f = strings.ReplaceAll(f, "Object owner(){return DelegationOwner.this;}", "Object enclosing(){return DelegationOwner.this;}Object owner(){return DelegationOwner.this;}")
				}
				f = strings.ReplaceAll(f, "Delegation", prefix)
				testNativeIndependentFamilyFixture(t, f, []string{prefix + "Owner"}, prefix+"Driver", "18:local:delegation\n", nativeLexicalExactSignatures)
			})
		}
	}
}

// The parent, effect recorder and caller stay original. Its virtual callbacks
// observe captures before either successful initialization or an abrupt exit.
func TestAdversarialMethodLocalCapturedDelegationKeepsOriginalAbruptEffects(t *testing.T) {
	const fixture = `class LocalAbruptEffects{static final RuntimeException failure=new RuntimeException("original");static Object seen;static int calls;}
class LocalAbruptBase{LocalAbruptBase(int n,Object expected){LocalAbruptEffects.calls++;LocalAbruptEffects.seen=capture();if(LocalAbruptEffects.seen!=expected)throw new AssertionError("early capture");if(n<0)throw LocalAbruptEffects.failure;}Object capture(){return null;}}
class LocalAbruptOwner{Object make(final int n,final Object token){class Entry extends LocalAbruptBase{Entry(){super(n,token);}Object capture(){return token;}}return new Entry();}}
class LocalAbruptDriver{public static void main(String[]args){LocalAbruptOwner owner=new LocalAbruptOwner();Object marker=new Object();int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object value:new Object[]{null,marker}){int before=LocalAbruptEffects.calls;try{LocalAbruptBase b=(LocalAbruptBase)owner.make(n,value);if(n<0||b.capture()!=value)throw new AssertionError("normal exit");}catch(RuntimeException failure){if(n>=0||failure!=LocalAbruptEffects.failure)throw new AssertionError("abrupt identity");}if(LocalAbruptEffects.seen!=value||LocalAbruptEffects.calls!=before+1)throw new AssertionError("partial effects");rows++;}System.out.println(rows+":local:abrupt");}}`
	for _, prefix := range []string{"LocalAbrupt", "SeparateAbrupt"} {
		t.Run(prefix, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, strings.ReplaceAll(fixture, "LocalAbrupt", prefix), []string{prefix + "Owner"}, prefix+"Driver", "10:local:abrupt\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialMethodLocalCapturedDelegationSeparatesEqualTypedWords(t *testing.T) {
	const fixture = `class WordBindingBase{final Object left,right;final long wide;WordBindingBase(Object l,Object r,long w){left=left();right=right();wide=w;if(left!=l||right!=r)throw new AssertionError("independent super words");}Object left(){return null;}Object right(){return null;}}
class WordBindingOwner{Object make(final Object first,final Object second,final long word){class Entry extends WordBindingBase{Entry(){super(second,first,word);}Object left(){return second;}Object right(){return first;}}return new Entry();}}
class WordBindingDriver{public static void main(String[]args){WordBindingOwner owner=new WordBindingOwner();Object a=new Object(),b=new Object();int rows=0;for(Object first:new Object[]{null,a,b})for(Object second:new Object[]{null,a,b})for(long word:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){WordBindingBase value=(WordBindingBase)owner.make(first,second,word);if(value.left!=second||value.right!=first||value.wide!=word)throw new AssertionError("source capture binding");rows++;}System.out.println(rows+":local:separate-words");}}`
	for _, prefix := range []string{"WordBinding", "IndependentWords"} {
		t.Run(prefix, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, strings.ReplaceAll(fixture, "WordBinding", prefix), []string{prefix + "Owner"}, prefix+"Driver", "27:local:separate-words\n", nativeLexicalExactSignatures)
		})
	}
}

// Parent class formals do not participate in a constructor whose source
// parameters have no Signature. The original callback must still see captures.
func TestAdversarialMethodLocalCapturedDelegationIndependentGenericParent(t *testing.T) {
	for _, prefix := range []string{"Delegation", "IndependentParent"} {
		for _, layout := range []string{"raw", "concrete", "method formal", "bounded class formal", "generic hierarchy"} {
			t.Run(prefix+"/"+layout, func(t *testing.T) {
				fixture := strings.ReplaceAll(localCapturedDelegationFixture, "class DelegationBase{", "class DelegationBase<T>{")
				switch layout {
				case "concrete":
					fixture = strings.ReplaceAll(fixture, "class Entry extends DelegationBase{", "class Entry extends DelegationBase<String>{")
				case "method formal":
					fixture = strings.ReplaceAll(fixture, "Object make(final int n,", "<U> Object make(final int n,")
					fixture = strings.ReplaceAll(fixture, "class Entry extends DelegationBase{", "class Entry extends DelegationBase<U>{")
				case "generic hierarchy":
					fixture = "class DelegationAncestor<U>{}" + strings.ReplaceAll(fixture, "class DelegationBase<T>{", "class DelegationBase<T> extends DelegationAncestor<T> implements java.io.Serializable,java.lang.Cloneable{")
					fixture = strings.ReplaceAll(fixture, "class Entry extends DelegationBase{", "class Entry extends DelegationBase<String>{")
				case "bounded class formal":
					fixture = strings.ReplaceAll(fixture, "class DelegationBase<T>{", "class DelegationBase<T extends Number & Comparable<T>>{")
					fixture = strings.ReplaceAll(fixture, "class Entry extends DelegationBase{", "class Entry extends DelegationBase<Integer>{")
				}
				testNativeIndependentFamilyFixture(t, strings.ReplaceAll(fixture, "Delegation", prefix), []string{prefix + "Owner"}, prefix+"Driver", "18:local:delegation\n", nativeLexicalExactSignatures)
			})
		}
	}
}

func TestAdversarialMethodLocalCapturedDelegationReferenceWidening(t *testing.T) {
	for _, prefix := range []string{"Widen", "DifferentReference"} {
		for _, kind := range []string{"string object", "string interface", "reference array object", "reference matrix object array", "primitive matrix object array", "primitive array cloneable", "reference array serializable", "leaf superclass", "leaf interface"} {
			t.Run(prefix+"/"+kind, func(t *testing.T) {
				actual, formal, other, values := "String", "Object", "CharSequence", `new String[]{null,new String("same"),new String("same"),""}`
				switch kind {
				case "string interface":
					formal, other = "CharSequence", "String"
				case "reference array object":
					actual, values = "String[]", `new String[][]{null,new String[0],new String[]{null,"value"},new String[]{new String("value")}}`
				case "reference matrix object array":
					actual, formal, other, values = "String[][]", "Object[]", "String[][]", `new String[][][]{null,new String[0][],new String[][]{{null,"value"}},new String[][]{null}}`
				case "primitive matrix object array":
					actual, formal, other, values = "int[][]", "Object[]", "int[][]", `new int[][][]{null,new int[0][],new int[][]{{Integer.MIN_VALUE,Integer.MAX_VALUE}},new int[][]{null}}`
				case "primitive array cloneable":
					actual, formal, other, values = "int[]", "Cloneable", "int[]", `new int[][]{null,new int[0],new int[]{Integer.MIN_VALUE,Integer.MAX_VALUE},new int[]{0}}`
				case "reference array serializable":
					actual, formal, other, values = "String[]", "java.io.Serializable", "String[]", `new String[][]{null,new String[0],new String[]{null,"value"},new String[]{new String("value")}}`
				case "leaf superclass":
					actual, formal, other, values = "WidenLeaf", "WidenRoot", "WidenLeaf", `new WidenLeaf[]{null,new WidenLeaf(),new WidenLeaf(),null}`
				case "leaf interface":
					actual, formal, other, values = "WidenLeaf", "WidenMarker", "WidenLeaf", `new WidenLeaf[]{null,new WidenLeaf(),new WidenLeaf(),null}`
				}
				fixture := `interface WidenMarker{}class WidenRoot implements WidenMarker{}class WidenMiddle extends WidenRoot{}class WidenLeaf extends WidenMiddle{}
class WidenParent{final Object seen;WidenParent(FORMAL x){seen=capture();if(seen!=x)throw new AssertionError("original widened capture before SUPER");}WidenParent(OTHER x){throw new AssertionError("wrong overload");}Object capture(){return null;}}
class WidenOwner{Object make(final ACTUAL x){class Entry extends WidenParent{Entry(){super((FORMAL)x);}Object capture(){return x;}}return new Entry();}}
class WidenDriver{public static void main(String[]args){WidenOwner owner=new WidenOwner();int n=0;for(ACTUAL s:VALUES){WidenParent v=(WidenParent)owner.make(s);if(v.seen!=s||v.capture()!=s)throw new AssertionError("capture reference identity");n++;}System.out.println(n+":widen:reference");}}`
				fixture = strings.NewReplacer("FORMAL", formal, "OTHER", other, "ACTUAL", actual, "VALUES", values).Replace(fixture)
				testNativeIndependentFamilyFixture(t, strings.ReplaceAll(fixture, "Widen", prefix), []string{prefix + "Owner"}, prefix+"Driver", "4:widen:reference\n", nativeLexicalExactSignatures)
			})
		}
	}
}
