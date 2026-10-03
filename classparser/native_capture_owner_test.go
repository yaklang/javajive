package javaclassparser

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const nativeOwnerFixture = `
class NativeEffects {static String trace="";static RuntimeException failure=new IllegalArgumentException("parent");static Object published;}
class NativeParent {static {NativeEffects.trace+="I";}final Object seen,owner;final long number;NativeParent(long n,Object value){NativeEffects.trace+="P";seen=capture();owner=owner();number=n;NativeEffects.published=this;if(n<0)throw NativeEffects.failure;}Object capture(){return null;}Object owner(){return null;}long wide(){return 0;}}
class NativeOwner {NativeParent make(final Object var1,final long var2,long n){return new NativeParent(n,var1){Object capture(){return var1;}Object owner(){return NativeOwner.this;}long wide(){return var2;}int shadow(int var1){return var1+1;}protected void finalize(){if(capture()!=var1)throw new AssertionError("capture finalizer");}};}}
public class NativeDriver {public static void main(String[]x){NativeOwner owner=new NativeOwner();Object token=new Object();int rows=0;for(Object v:new Object[]{null,token})for(long wide:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(long n:new long[]{-1,0,1}){NativeEffects.published=null;try{NativeParent p=owner.make(v,wide,n);if(n<0||p.seen!=v||p.owner!=owner||p.capture()!=v||p.wide()!=wide||p.number!=n)throw new AssertionError("capture");}catch(RuntimeException e){if(n>=0||e!=NativeEffects.failure)throw new AssertionError("failure",e);NativeParent p=(NativeParent)NativeEffects.published;if(p.seen!=v||p.owner!=owner||p.capture()!=v||p.wide()!=wide)throw new AssertionError("failed captured receiver");}rows++;}System.out.println(rows+":"+NativeEffects.trace);}}
`

const nativeGenericFixture = `
interface NativeValue<T>{T get();}
class NativeGenericParent<T>{final int selected;NativeGenericParent(T t){selected=1;}NativeGenericParent(String s){selected=2;}T capture(){return null;}T parameter(T t){return t;}}
class NativeGenericOwner<T>{NativeGenericParent<T> make(final T var1){return new NativeGenericParent<T>(var1){T capture(){return var1;}T parameter(T var1){return var1;}};}static <E> NativeValue<E> local(E input){final E var1=input;return new NativeValue<E>(){public E get(){return var1;}};}static NativeValue<Long> wide(long input){final long var2=input;return new NativeValue<Long>(){public Long get(){return var2;}};}}
public class NativeGenericDriver{public static void main(String[] args){NativeGenericOwner<Object> owner=new NativeGenericOwner<>();Object[] vals={null,"text",new Object()};int rows=0;for(Object v:vals){NativeGenericParent<Object> p=owner.make(v);if(p.selected!=1||p.capture()!=v||p.parameter(v)!=v||NativeGenericOwner.local(v).get()!=v)throw new AssertionError("generic binding");rows++;}for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){if(NativeGenericOwner.wide(n).get().longValue()!=n)throw new AssertionError("wide");rows++;}System.out.println(rows);}}
`

const nativeInitFixture = `
class NativeOrder {static String trace="";static RuntimeException boom=new IllegalArgumentException("argument");static Object token=new Object();static Object produce(boolean fail){trace+="A";if(fail)throw boom;return token;}}
class NativeInitParent {static {NativeOrder.trace+="I";}Object seen;NativeInitParent(Object v){NativeOrder.trace+="P";seen=capture();if(seen!=v)throw new AssertionError("pre-super capture");}Object capture(){return null;}}
class NativeInitOwner {static NativeInitParent make(final Object var1,boolean fail){return new NativeInitParent(NativeOrder.produce(fail)){Object capture(){return var1;}};}}
public class NativeInitDriver {public static void main(String[] args){try{NativeInitOwner.make(NativeOrder.token,true);throw new AssertionError("missing throw");}catch(RuntimeException e){if(e!=NativeOrder.boom)throw new AssertionError("identity",e);}if(!NativeOrder.trace.equals("IA"))throw new AssertionError(NativeOrder.trace);NativeInitParent p=NativeInitOwner.make(NativeOrder.token,false);if(p.seen!=NativeOrder.token||!NativeOrder.trace.equals("IAAP"))throw new AssertionError(NativeOrder.trace);System.out.println(NativeOrder.trace);}}
`

const nativePrimitiveFixture = `
class NativePrimitiveParent {final boolean selected;NativePrimitiveParent(boolean b){selected=b;}int capture(){return 0;}}
class NativePrimitiveOwner {static NativePrimitiveParent make(final boolean var1,final byte var2,final char var3,final short var4){return new NativePrimitiveParent(false){int capture(){return (var1?1:0)+var2+var3+var4;}};}}
public class NativePrimitiveDriver {public static void main(String[]args){int rows=0;for(boolean b:new boolean[]{false,true})for(byte n:new byte[]{Byte.MIN_VALUE,0,Byte.MAX_VALUE})for(char c:new char[]{0,65535})for(short d:new short[]{Short.MIN_VALUE,Short.MAX_VALUE}){NativePrimitiveParent p=NativePrimitiveOwner.make(b,n,c,d);if(p.selected||p.capture()!=(b?1:0)+n+c+d)throw new AssertionError("primitive capture");rows++;}System.out.println(rows);}}
`
const nativeComparatorFixture = `
interface NativeComparableValue<T>{T get();}
class NativeComparatorOwner {static <E> NativeComparableValue<E> pick(final NativeComparableValue<E> first,final NativeComparableValue<E> second,final java.util.Comparator<E> comparator){return new NativeComparableValue<E>(){public E get(){E a=first.get();E b=second.get();return comparator.compare(a,b)>=0?a:b;}};}}
public class NativeComparatorDriver {public static void main(String[]args){int rows=0;for(final String a:new String[]{"","a","zz"})for(final String b:new String[]{"","a","zz"}){NativeComparableValue<String> first=()->a,second=()->b;java.util.Comparator<String> cmp=(x,y)->x.compareTo(y);if(NativeComparatorOwner.pick(first,second,cmp).get()!=(a.compareTo(b)>=0?a:b))throw new AssertionError("captured generic receiver");rows++;}System.out.println(rows);}}
`
const nativeImportFixture = `
interface NativeTime<T>{T deserialize(java.util.Date value);}
class NativeImportOwner {static NativeTime<java.sql.Date> make(){return new NativeTime<java.sql.Date>(){public java.sql.Date deserialize(java.util.Date value){return new java.sql.Date(value.getTime());}};}}
public class NativeImportDriver {public static void main(String[]args){for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){Object v=NativeImportOwner.make().deserialize(new java.util.Date(n));if(v.getClass()!=java.sql.Date.class||((java.sql.Date)v).getTime()!=n)throw new AssertionError("import capture");}System.out.println(3);}}
`

const nativeScopeFixture = `
class NativeScopeParent {final float number;NativeScopeParent(float n){number=n;}long read(Object x,int y){return 0;}}
class NativeScopeOwner {NativeScopeParent make(Object var1,int var2,float var3){return new NativeScopeParent(var3){long read(Object var1,int var2){long var3=var1==null?7:11;return var3+var2;}};}}
public class NativeScopeDriver {public static void main(String[] args){int rows=0;for(float f:new float[]{-0.0f,0.0f,Float.NEGATIVE_INFINITY,Float.POSITIVE_INFINITY}){NativeScopeParent p=new NativeScopeOwner().make(null,0,f);if(Float.floatToRawIntBits(p.number)!=Float.floatToRawIntBits(f)||p.read(null,3)!=10||p.read(new Object(),-2)!=9)throw new AssertionError("lexical variable scope");rows++;}System.out.println(rows);}}
`
const nativeStringFixture = `
class NativeStringParent {final String text;NativeStringParent(String s){text=s;}Object get(){return null;}}
class NativeStringOwner {static NativeStringParent make(final Object x){return new NativeStringParent("\uD800x\uDC00\000\\\"}"){Object get(){return x;}};}}
public class NativeStringDriver {public static void main(String[] args){Object x=new Object();NativeStringParent p=NativeStringOwner.make(x);char[] expected={0xD800,'x',0xDC00,0,'\\','"','}'};if(p.get()!=x||p.text.length()!=expected.length)throw new AssertionError("literal capture");for(int i=0;i<expected.length;i++)if(p.text.charAt(i)!=expected[i])throw new AssertionError(i);System.out.println(expected.length);}}
`

func nativeBinaryShape(t *testing.T, raw []byte) string {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	rows := []string{fmt.Sprintf("C %x %s", obj.AccessFlags, obj.GetSupperClassName())}
	for _, field := range obj.Fields {
		n, _ := obj.getUtf8(field.NameIndex)
		d, _ := obj.getUtf8(field.DescriptorIndex)
		rows = append(rows, fmt.Sprintf("F %s %s %x", n, d, field.AccessFlags))
	}
	for _, method := range obj.Methods {
		n, _ := obj.getUtf8(method.NameIndex)
		d, _ := obj.getUtf8(method.DescriptorIndex)
		if method.AccessFlags&0x1000 == 0 {
			rows = append(rows, fmt.Sprintf("M %s %s %x", n, d, method.AccessFlags))
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

func TestNativeAnonymousCaptureRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, fixture := range []struct{ name, owner, driver, source string }{
		{"published-receiver-finalizer-wide-shadow", "NativeOwner", "NativeDriver", nativeOwnerFixture},
		{"generic-overload-and-local-identities", "NativeGenericOwner", "NativeGenericDriver", nativeGenericFixture},
		{"new-before-throwing-argument", "NativeInitOwner", "NativeInitDriver", nativeInitFixture},
		{"primitive-domain-and-super-literal", "NativePrimitiveOwner", "NativePrimitiveDriver", nativePrimitiveFixture},
		{"parameterized-capture-result", "NativeComparatorOwner", "NativeComparatorDriver", nativeComparatorFixture},
		{"one-lexical-import-namespace", "NativeImportOwner", "NativeImportDriver", nativeImportFixture},
		{"nested-member-local-scope", "NativeScopeOwner", "NativeScopeDriver", nativeScopeFixture},
		{"lossless-super-string-operand", "NativeStringOwner", "NativeStringDriver", nativeStringFixture},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			for _, debug := range []string{"-g", "-g:none"} {
				t.Run(debug, func(t *testing.T) {
					original := t.TempDir()
					p := filepath.Join(original, fixture.driver+".java")
					if err := os.WriteFile(p, []byte(fixture.source), 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, p).CombinedOutput(); err != nil {
						t.Fatalf("original: %v\n%s", err, out)
					}
					oracle := t04RunJava(t, java, original, fixture.driver)
					files := map[string][]byte{}
					entries, err := os.ReadDir(original)
					if err != nil {
						t.Fatal(err)
					}
					var buffer bytes.Buffer
					writer := zip.NewWriter(&buffer)
					children := []string{}
					for _, entry := range entries {
						if !strings.HasSuffix(entry.Name(), ".class") {
							continue
						}
						raw, err := os.ReadFile(filepath.Join(original, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						files[strings.TrimSuffix(entry.Name(), ".class")] = raw
						if strings.TrimSuffix(entry.Name(), ".class") == fixture.owner || strings.HasPrefix(entry.Name(), fixture.owner+"$") {
							w, err := writer.Create(entry.Name())
							if err != nil {
								t.Fatal(err)
							}
							if _, err = w.Write(raw); err != nil {
								t.Fatal(err)
							}
							if strings.HasPrefix(entry.Name(), fixture.owner+"$") {
								children = append(children, strings.TrimSuffix(entry.Name(), ".class"))
							}
						}
					}
					if err := writer.Close(); err != nil {
						t.Fatal(err)
					}
					jar := filepath.Join(t.TempDir(), "input.jar")
					if err := os.WriteFile(jar, buffer.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
					for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(mode, func(t *testing.T) {
							if mode == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if mode == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							z, err := NewJarFSFromLocalWithResolver(jar, func(name string) ([]byte, bool) { raw, known := files[name]; return raw, known })
							if err != nil {
								t.Fatal(err)
							}
							// Request the child before its owner: suppression is valid only after the
							// exact owner source and numbering proof have completed for this policy.
							for _, child := range children {
								raw, err := z.ReadFile(child + ".class")
								if err != nil || !strings.Contains(string(raw), "original anonymous body owned by") {
									t.Fatalf("child ownership: %v\n%s", err, raw)
								}
							}
							src, err := z.ReadFile(fixture.owner + ".class")
							if err != nil {
								t.Fatal(err)
							}
							if strings.Contains(string(src), DecompileStubMarker) {
								t.Fatalf("stub:\n%s", src)
							}
							output := t.TempDir()
							for name, raw := range files {
								if name == fixture.owner || strings.HasPrefix(name, fixture.owner+"$") {
									continue
								}
								if err := os.WriteFile(filepath.Join(output, name+".class"), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							sourcePath := filepath.Join(output, fixture.owner+".java")
							if err := os.WriteFile(sourcePath, src, 0600); err != nil {
								t.Fatal(err)
							}
							if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, sourcePath).CombinedOutput(); err != nil {
								t.Fatalf("rebuilt: %v\n%s\n%s", err, out, src)
							}
							for _, child := range children {
								rebuilt, err := os.ReadFile(filepath.Join(output, child+".class"))
								if err != nil {
									t.Fatal(err)
								}
								want, got := nativeBinaryShape(t, files[child]), nativeBinaryShape(t, rebuilt)
								if want != got {
									t.Fatalf("anonymous ABI %s:\noriginal:\n%s\nrebuilt:\n%s", child, want, got)
								}
							}
							if actual := t04RunJava(t, java, output, fixture.driver); actual != oracle {
								t.Fatalf("JVM differs: %q != %q", actual, oracle)
							}
						})
					}
				})
			}
		})
	}
}
