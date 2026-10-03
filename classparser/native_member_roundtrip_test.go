package javaclassparser

import (
	"archive/zip"
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberFixture = `
class MemberEffects {static Object published;static String trace="";static java.io.IOException failure=new java.io.IOException("original");}
class MemberParent {final Object observed;final long number;MemberParent(long n)throws java.io.IOException {MemberEffects.trace+="P";observed=owner();number=n;MemberEffects.published=this;if(n<0)throw MemberEffects.failure;}Object owner(){return null;}}
class MemberCapture {
 final Object token;MemberCapture(Object x){token=x;}
 class Child extends MemberParent {Child()throws java.io.IOException {this(7);}Child(long n)throws java.io.IOException {super(n);}Object owner(){return MemberCapture.this;}Object token(){return MemberCapture.this.token;}}
 MemberParent make(long n)throws java.io.IOException {return new Child(n);}
}
class MemberExternal {MemberCapture.Child declaredOnly;MemberCapture.Child[] arrayOnly;static MemberCapture.Child qualified(MemberCapture outer,long n)throws java.io.IOException {return outer.new Child(n);}static MemberCapture.Child zero(MemberCapture outer)throws java.io.IOException{return outer.new Child();}}
public class MemberDriver {
 public static void main(String[]args)throws Exception {int rows=0;Object token=new Object();for(Object x:new Object[]{null,token}){MemberCapture outer=new MemberCapture(x);for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean qualified:new boolean[]{false,true}){MemberEffects.published=null;try{MemberParent p=qualified?MemberExternal.qualified(outer,n):outer.make(n);if(n<0||p.observed!=outer||p.number!=n||((MemberCapture.Child)p).token()!=x)throw new AssertionError("capture");}catch(java.io.IOException e){MemberCapture.Child p=(MemberCapture.Child)MemberEffects.published;if(n>=0||e!=MemberEffects.failure||p==null||p.observed!=outer||p.number!=n||p.token()!=x)throw new AssertionError("publication",e);}rows++;}MemberCapture.Child c=MemberExternal.zero(outer);if(c.number!=7||c.observed!=outer)throw new AssertionError("this chain");rows++;}try{MemberExternal.qualified(null,3);throw new AssertionError("null outer");}catch(NullPointerException expected){rows++;}System.out.println(rows+":"+MemberEffects.trace);}
}
`

func TestNativeMemberCaptureRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	for _, fixture := range []struct{ owner, external, driver, source string }{{"MemberCapture", "MemberExternal", "MemberDriver", nativeMemberFixture}, {"GenericMember", "GenericExternal", "GenericDriver", nativeGenericMemberFixture}, {"OuterArgument", "ArgumentExternal", "ArgumentDriver", nativeMemberArgumentFixture}, {"InitMember", "InitExternal", "InitDriver", nativeMemberInitializationFixture}, {"FailInitMember", "FailInitExternal", "FailInitDriver", nativeMemberFailInitializationFixture}} {
		t.Run(fixture.owner, func(t *testing.T) {
			for _, debug := range []string{"-g", "-g:none"} {
				t.Run(debug, func(t *testing.T) {
					original := t.TempDir()
					file := filepath.Join(original, fixture.driver+".java")
					if e := os.WriteFile(file, []byte(fixture.source), 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); e != nil {
						t.Fatalf("%v %s", e, out)
					}
					oracle := t04RunJava(t, java, original, fixture.driver)
					files := map[string][]byte{}
					entries, _ := os.ReadDir(original)
					var data bytes.Buffer
					writer := zip.NewWriter(&data)
					for _, entry := range entries {
						if !strings.HasSuffix(entry.Name(), ".class") {
							continue
						}
						raw, e := os.ReadFile(filepath.Join(original, entry.Name()))
						if e != nil {
							t.Fatal(e)
						}
						files[strings.TrimSuffix(entry.Name(), ".class")] = raw
						if strings.HasPrefix(entry.Name(), fixture.owner) || entry.Name() == fixture.external+".class" {
							w, e := writer.Create(entry.Name())
							if e != nil {
								t.Fatal(e)
							}
							if _, e = w.Write(raw); e != nil {
								t.Fatal(e)
							}
						}
					}
					if e := writer.Close(); e != nil {
						t.Fatal(e)
					}
					jar := filepath.Join(t.TempDir(), "input.jar")
					if e := os.WriteFile(jar, data.Bytes(), 0600); e != nil {
						t.Fatal(e)
					}
					for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(mode, func(t *testing.T) {
							if mode == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if mode == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							z, e := NewJarFSFromLocalWithResolver(jar, func(n string) ([]byte, bool) { r, ok := files[n]; return r, ok })
							if e != nil {
								t.Fatal(e)
							}
							child, e := z.ReadFile(fixture.owner + "$Child.class")
							if e != nil || !strings.Contains(string(child), "original member body owned by") {
								t.Fatalf("child %v %s", e, child)
							}
							output := t.TempDir()
							for n, raw := range files {
								if n == fixture.owner || n == fixture.owner+"$Child" || n == fixture.external {
									continue
								}
								if e := os.WriteFile(filepath.Join(output, n+".class"), raw, 0600); e != nil {
									t.Fatal(e)
								}
							}
							sources := []string{}
							for _, n := range []string{fixture.owner, fixture.external} {
								src, e := z.ReadFile(n + ".class")
								if e != nil || strings.Contains(string(src), DecompileStubMarker) {
									t.Fatalf("source %s: %v %s", n, e, src)
								}

								path := filepath.Join(output, n+".java")
								os.WriteFile(path, src, 0600)
								sources = append(sources, path)
							}
							argv := append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, sources...)
							if out, e := exec.Command(javac, argv...).CombinedOutput(); e != nil {
								t.Fatalf("rebuilt %v %s", e, out)
							}
							for _, n := range []string{fixture.owner + "$Child", fixture.owner} {
								raw, e := os.ReadFile(filepath.Join(output, n+".class"))
								if e != nil {
									t.Fatal(e)
								}
								if want, got := nativeBinaryShape(t, files[n]), nativeBinaryShape(t, raw); want != got {
									t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, got)
								}
							}
							if got := t04RunJava(t, java, output, fixture.driver); got != oracle {
								t.Fatalf("%q != %q", got, oracle)
							}
						})
					}
				})
			}
		})
	}
}

const nativeGenericMemberFixture = `
class GenericEffects {static Object published;static java.io.IOException failure=new java.io.IOException("same");}
class GenericParent {final Object owner;final Number number;final int selected;GenericParent(Number n)throws java.io.IOException {owner=owner();number=n;selected=1;GenericEffects.published=this;if(n!=null&&n.longValue()<0)throw GenericEffects.failure;}GenericParent(Object o){owner=owner();number=null;selected=2;}Object owner(){return null;}}
class GenericMember<T extends Number> {final T token;GenericMember(T x){token=x;}class Child extends GenericParent {Child(T n)throws java.io.IOException {super(n);}Child(Object n){super(n);}Object owner(){return GenericMember.this;}T token(){return GenericMember.this.token;}}Child make(T n)throws java.io.IOException{return new Child(n);}}
class GenericExternal {static <T extends Number> GenericMember<T>.Child pick(GenericMember<T> outer,T n)throws java.io.IOException{return outer.new Child(n);}}
public class GenericDriver {public static void main(String[]args)throws Exception {int rows=0;for(Number n:new Number[]{null,Integer.valueOf(-1),Integer.valueOf(0),Long.valueOf(Long.MIN_VALUE),Long.valueOf(Long.MAX_VALUE),Double.valueOf(-0.0),Double.valueOf(Double.NaN)}){GenericMember<Number> outer=new GenericMember<>(n);for(boolean external:new boolean[]{false,true}){GenericEffects.published=null;try{GenericMember<Number>.Child c=external?GenericExternal.pick(outer,n):outer.make(n);if(c.owner!=outer||c.number!=n||c.token()!=n||c.selected!=1)throw new AssertionError("generic member overload");}catch(java.io.IOException e){GenericMember<Number>.Child c=(GenericMember<Number>.Child)GenericEffects.published;if(n==null||n.longValue()>=0||e!=GenericEffects.failure||c.owner!=outer||c.number!=n||c.token()!=n||c.selected!=1)throw new AssertionError("generic publication",e);}rows++;}}System.out.println(rows);}}
`

const nativeMemberArgumentFixture = `
class ArgumentEffects {static String trace="";static Object published;static java.io.IOException failure=new java.io.IOException("argument");}
class ArgumentParent {final Object owner,token;final long number;final int before;ArgumentParent(long n,Object x){ArgumentEffects.trace+="P";owner=owner();token=x;number=n;before=state();ArgumentEffects.published=this;}Object owner(){return null;}int state(){return -1;}}
class OuterArgument {final Object token;OuterArgument(Object x){token=x;}long argument(long n)throws java.io.IOException{ArgumentEffects.trace+="A";if(n<0)throw ArgumentEffects.failure;return n;}class Child extends ArgumentParent {final int after;Child(long n)throws java.io.IOException{super(OuterArgument.this.argument(n),OuterArgument.this.token);after=7;}Object owner(){return OuterArgument.this;}int state(){return after;}}ArgumentParent make(long n)throws java.io.IOException{return new Child(n);}}
class ArgumentExternal {static OuterArgument.Child pick(OuterArgument outer,long n)throws java.io.IOException{return outer.new Child(n);}}
public class ArgumentDriver {public static void main(String[]args)throws Exception {int rows=0;Object x=new Object();for(Object token:new Object[]{null,x})for(long n:new long[]{-1,0,Long.MAX_VALUE})for(boolean external:new boolean[]{false,true}){OuterArgument outer=new OuterArgument(token);ArgumentEffects.trace="";ArgumentEffects.published=null;try{OuterArgument.Child c=external?ArgumentExternal.pick(outer,n):(OuterArgument.Child)outer.make(n);if(n<0||c.owner!=outer||c.token!=token||c.number!=n||c.before!=0||c.after!=7||!ArgumentEffects.trace.equals("AP"))throw new AssertionError("enclosing argument order");}catch(java.io.IOException e){if(n>=0||e!=ArgumentEffects.failure||ArgumentEffects.published!=null||!ArgumentEffects.trace.equals("A"))throw new AssertionError("argument exception",e);}rows++;}System.out.println(rows);}}
`

// NEW initializes the member and its superclass before the qualifier's null
// check. Neither a moved requireNonNull nor a factory call has this order.
const nativeMemberInitializationFixture = `
class InitEffects {static String trace="";static long argument(){trace+="A";return 3;}}
class InitParent {static {InitEffects.trace+="C";}InitParent(long n){InitEffects.trace+="P";}}
class InitMember {class Child extends InitParent{Child(long n){super(n);}}}
class InitExternal {static InitMember.Child make(InitMember outer){return outer.new Child(InitEffects.argument());}}
public class InitDriver {public static void main(String[]args){try{InitExternal.make(null);throw new AssertionError("null enclosing");}catch(NullPointerException expected){if(!InitEffects.trace.equals("C"))throw new AssertionError("NEW before qualifier and argument:"+InitEffects.trace);}InitExternal.make(new InitMember());if(!InitEffects.trace.equals("CAP"))throw new AssertionError("argument then parent:"+InitEffects.trace);System.out.println(InitEffects.trace);}}
`
const nativeMemberFailInitializationFixture = `
class FailInitEffects {static String trace="";static void fail(){trace+="C";throw new IllegalStateException("initialization");}static long argument(){trace+="A";return 3;}}
class FailInitParent {static {FailInitEffects.fail();}FailInitParent(long n){FailInitEffects.trace+="P";}}
class FailInitMember {class Child extends FailInitParent{Child(long n){super(n);}}}
class FailInitExternal {static FailInitMember.Child make(FailInitMember outer){return outer.new Child(FailInitEffects.argument());}}
public class FailInitDriver {public static void main(String[]args){try{FailInitExternal.make(null);throw new AssertionError("initialization");}catch(ExceptionInInitializerError expected){if(!(expected.getCause() instanceof IllegalStateException)||!FailInitEffects.trace.equals("C"))throw new AssertionError("initialization priority");}try{FailInitExternal.make(null);throw new AssertionError("erroneous class");}catch(NoClassDefFoundError expected){if(!FailInitEffects.trace.equals("C"))throw new AssertionError("repeated initialization");}System.out.println(FailInitEffects.trace);}}
`

// Inspect and execute a reviewed original fixture with the old javac null-check
// bytecode. The two constructors' descriptors and all NEW/POP sites stay original.
func TestNativeMemberLegacyNullCheckKeepsInitializationPriority(t *testing.T) {
	javac, java := t04Tools(t)
	for _, row := range []struct{ owner, external, driver, source string }{{"InitMember", "InitExternal", "InitDriver", nativeMemberInitializationFixture}, {"FailInitMember", "FailInitExternal", "FailInitDriver", nativeMemberFailInitializationFixture}} {
		t.Run(row.owner, func(t *testing.T) {
			original := t.TempDir()
			file := filepath.Join(original, row.driver+".java")
			if e := os.WriteFile(file, []byte(row.source), 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", original, file).CombinedOutput(); e != nil {
				t.Fatalf("original %v %s", e, out)
			}
			files := map[string][]byte{}
			entries, e := os.ReadDir(original)
			if e != nil {
				t.Fatal(e)
			}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".class") {
					raw, e := os.ReadFile(filepath.Join(original, entry.Name()))
					if e != nil {
						t.Fatal(e)
					}
					files[entry.Name()] = raw
				}
			}
			raw := files[row.external+".class"]
			obj, e := Parse(append([]byte(nil), raw...))
			if e != nil {
				t.Fatal(e)
			}
			cp := NewConstantPoolWithConstant(&obj.ConstantPool)
			index := cp.AddNewMethodInfo("java/lang/Object", "getClass", "()Ljava/lang/Class;")
			changed := 0
			for _, m := range obj.Methods {
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						d := core.NewDecompiler(code.Code, func(int) values.JavaValue { return nil })
						if e := d.ParseOpcode(); e != nil {
							t.Fatal(e)
						}
						for _, op := range d.Opcodes() {
							if call := constructorMotionMember(obj, op, core.OP_INVOKESTATIC); call != nil && call.Name == "java/util/Objects" && call.Member == "requireNonNull" {
								pc := int(op.CurrentOffset)
								code.Code[pc] = byte(core.OP_INVOKEVIRTUAL)
								code.Code[pc+1] = byte(index >> 8)
								code.Code[pc+2] = byte(index)
								changed++
							}
						}
					}
				}
			}
			if changed != 1 {
				t.Fatalf("changed %d check sites", changed)
			}
			files[row.external+".class"] = obj.Bytes()
			if e := os.WriteFile(filepath.Join(original, row.external+".class"), obj.Bytes(), 0600); e != nil {
				t.Fatal(e)
			}
			oracle := t04RunJava(t, java, original, row.driver)
			z := nativeArchive(t, files)
			output := t.TempDir()
			for n, b := range files {
				if n == row.owner+".class" || n == row.owner+"$Child.class" || n == row.external+".class" {
					continue
				}
				if e := os.WriteFile(filepath.Join(output, n), b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			paths := []string{}
			for _, n := range []string{row.owner, row.external} {
				src, e := z.ReadFile(n + ".class")
				if e != nil || strings.Contains(string(src), DecompileStubMarker) {
					t.Fatalf("source %s: %v %s", n, e, src)
				}
				p := filepath.Join(output, n+".java")
				if e := os.WriteFile(p, src, 0600); e != nil {
					t.Fatal(e)
				}
				paths = append(paths, p)
			}
			child, e := z.ReadFile(row.owner + "$Child.class")
			if e != nil || !strings.Contains(string(child), "original member body owned by") {
				t.Fatalf("refused old null lowering %v %s", e, child)
			}
			if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, paths...)...).CombinedOutput(); e != nil {
				t.Fatalf("rebuilt %v %s", e, out)
			}
			if got := t04RunJava(t, java, output, row.driver); got != oracle {
				t.Fatalf("%q != %q", got, oracle)
			}
		})
	}
}

func TestNativeMemberPackagedScopeBindsShadowedTypes(t *testing.T) {
	javac, java := t04Tools(t)
	const fixture = `package scope.member;
class ScopeParent{final java.lang.String seen;ScopeParent(java.lang.String s){seen=s;}}
class ScopeOwner{final java.lang.String text;final java.lang.Number value;ScopeOwner(java.lang.String s,java.lang.Number n){text=s;value=n;}
 class Iterator implements java.util.Iterator<Integer>{int cursor;Iterator(){}private boolean remaining(){return cursor<3;}private Integer pull(){return cursor++;}public boolean hasNext(){return remaining();}public Integer next(){return pull();}public void remove(){throw new UnsupportedOperationException();}}
 class String extends ScopeParent{String(){super(ScopeOwner.this.text);}java.lang.String read(){return ScopeOwner.this.text;}}
 class Number{Number(){}java.lang.Number read(){return ScopeOwner.this.value;}}
 java.util.Iterator<Integer> iterator(){return new Iterator();}java.lang.String read(){return new String().read();}java.lang.Number number(){return new Number().read();}
}
class ScopeExternal{static ScopeOwner.String make(ScopeOwner o){return o.new String();}}
public class ScopeDriver{public static void main(java.lang.String[]args){for(java.lang.String s:new java.lang.String[]{null,"text"})for(java.lang.Number n:new java.lang.Number[]{null,Integer.valueOf(-1),Long.valueOf(Long.MAX_VALUE)}){ScopeOwner o=new ScopeOwner(s,n);if(o.read()!=s||o.number()!=n||ScopeExternal.make(o).seen!=s)throw new AssertionError("packaged enclosing binding");java.util.Iterator<Integer> it=o.iterator();int total=0;while(it.hasNext())total+=it.next();if(total!=3)throw new AssertionError("iterator namespace");try{it.remove();throw new AssertionError("remove");}catch(UnsupportedOperationException expected){}}System.out.println(6);}}
`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			file := filepath.Join(original, "ScopeDriver.java")
			if e := os.WriteFile(file, []byte(fixture), 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); e != nil {
				t.Fatalf("original %v %s", e, out)
			}
			oracle := t04RunJava(t, java, original, "scope.member.ScopeDriver")
			files := map[string][]byte{}
			if e := filepath.Walk(original, func(p string, info os.FileInfo, e error) error {
				if e != nil {
					return e
				}
				if !info.IsDir() && strings.HasSuffix(p, ".class") {
					r, e := os.ReadFile(p)
					if e != nil {
						return e
					}
					rel, e := filepath.Rel(original, p)
					if e != nil {
						return e
					}
					files[filepath.ToSlash(rel)] = r
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(mode, func(t *testing.T) {
					if mode == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if mode == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					child, e := z.ReadFile("scope/member/ScopeOwner$Iterator.class")
					if e != nil || !strings.Contains(string(child), "original member body owned by") {
						t.Fatalf("packaged member refused %v %s", e, child)
					}
					output := t.TempDir()
					for n, b := range files {
						if strings.HasPrefix(n, "scope/member/ScopeOwner") || n == "scope/member/ScopeExternal.class" {
							continue
						}
						p := filepath.Join(output, filepath.FromSlash(n))
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, b, 0600); e != nil {
							t.Fatal(e)
						}
					}
					paths := []string{}
					for _, n := range []string{"scope/member/ScopeOwner", "scope/member/ScopeExternal"} {
						src, e := z.ReadFile(n + ".class")
						if e != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v %s", e, src)
						}
						p := filepath.Join(output, filepath.FromSlash(n)+".java")
						if e := os.WriteFile(p, src, 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, p)
					}
					if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s", e, out)
					}
					for _, n := range []string{"scope/member/ScopeOwner", "scope/member/ScopeOwner$Iterator", "scope/member/ScopeOwner$String", "scope/member/ScopeOwner$Number"} {
						rebuilt, e := os.ReadFile(filepath.Join(output, filepath.FromSlash(n)+".class"))
						if e != nil {
							t.Fatal(e)
						}
						if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, rebuilt); want != got {
							t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, got)
						}
					}
					if got := t04RunJava(t, java, output, "scope.member.ScopeDriver"); got != oracle {
						t.Fatalf("%q != %q", got, oracle)
					}
				})
			}
		})
	}
}

func TestNativeMemberMixedStaticGenericScopeRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `package mixed.scope;
class MixedEffects{static java.lang.String trace="";static Object seen;static java.io.IOException failure=new java.io.IOException("parent");}
class MixedParent{final Object owner;final java.lang.Number value;MixedParent(java.lang.Number n)throws java.io.IOException{MixedEffects.trace+="P";owner=owner();value=n;MixedEffects.seen=this;if(n!=null&&n.longValue()<0)throw MixedEffects.failure;}Object owner(){return null;}}
class MixedOwner<T extends java.lang.Number>{final T token;MixedOwner(T n){token=n;}
 static class Box<U extends java.lang.Number>{static final int runtime;static{MixedEffects.trace+="B";runtime=7;}final U value;final int selected;Box(U n){value=n;selected=1;}Box(Object rival){value=null;selected=2;}U get(){return value;}<V extends U> V echo(V v){return v;}static <T extends java.lang.Number> T own(T v){return v;}static int status(){return runtime;}}
 class Child extends MixedParent{Child(T n)throws java.io.IOException{super(n);}Object owner(){return MixedOwner.this;}T token(){return MixedOwner.this.token;}}
 Box<T> box(T n){return new Box<T>(n);}Child child(T n)throws java.io.IOException{return new Child(n);}
}
class MixedExternal{static <T extends java.lang.Number> MixedOwner.Box<T> box(T n){return new MixedOwner.Box<T>(n);}static <T extends java.lang.Number> MixedOwner<T>.Child child(MixedOwner<T> o,T n)throws java.io.IOException{return o.new Child(n);}static int status(){return MixedOwner.Box.runtime;}}
public class MixedDriver{public static void main(java.lang.String[]args)throws Exception{if(MixedExternal.status()!=7||!MixedEffects.trace.equals("B"))throw new AssertionError("blank static read");int rows=0;for(java.lang.Number n:new java.lang.Number[]{null,Integer.valueOf(-1),Integer.valueOf(0),Long.valueOf(Long.MIN_VALUE),Long.valueOf(Long.MAX_VALUE),Double.valueOf(-0.0),Double.valueOf(Double.NaN)}){MixedOwner<java.lang.Number> o=new MixedOwner<>(n);for(boolean external:new boolean[]{false,true}){MixedOwner.Box<java.lang.Number> b=external?MixedExternal.box(n):o.box(n);if(b.value!=n||b.get()!=n||b.echo(n)!=n||MixedOwner.Box.own(n)!=n||b.selected!=1)throw new AssertionError("static generic rival");MixedEffects.seen=null;try{MixedOwner<java.lang.Number>.Child c=external?MixedExternal.child(o,n):o.child(n);if(c.owner!=o||c.value!=n||c.token()!=n)throw new AssertionError("member identity");}catch(java.io.IOException e){MixedOwner<java.lang.Number>.Child c=(MixedOwner<java.lang.Number>.Child)MixedEffects.seen;if(e!=MixedEffects.failure||n==null||n.longValue()>=0||c.owner!=o||c.value!=n||c.token()!=n)throw new AssertionError("parent publication",e);}rows++;}}System.out.println(rows+":"+MixedEffects.trace);}}
`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			file := filepath.Join(original, "MixedDriver.java")
			if e := os.WriteFile(file, []byte(source), 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); e != nil {
				t.Fatalf("original %v %s", e, out)
			}
			oracle := t04RunJava(t, java, original, "mixed.scope.MixedDriver")
			files := map[string][]byte{}
			if e := filepath.Walk(original, func(p string, info os.FileInfo, e error) error {
				if e != nil {
					return e
				}
				if !info.IsDir() && strings.HasSuffix(p, ".class") {
					r, e := os.ReadFile(p)
					if e != nil {
						return e
					}
					rel, e := filepath.Rel(original, p)
					if e != nil {
						return e
					}
					files[filepath.ToSlash(rel)] = r
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(mode, func(t *testing.T) {
					if mode == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if mode == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					for _, n := range []string{"mixed/scope/MixedOwner$Box.class", "mixed/scope/MixedOwner$Child.class"} {
						s, e := z.ReadFile(n)
						if e != nil || !strings.Contains(string(s), "original member body owned by") {
							t.Fatalf("unowned %s %v %s", n, e, s)
						}
					}
					output := t.TempDir()
					for n, b := range files {
						if strings.HasPrefix(n, "mixed/scope/MixedOwner") || n == "mixed/scope/MixedExternal.class" {
							continue
						}
						p := filepath.Join(output, filepath.FromSlash(n))
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, b, 0600); e != nil {
							t.Fatal(e)
						}
					}
					paths := []string{}
					for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedExternal"} {
						src, e := z.ReadFile(n + ".class")
						if e != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v %s", e, src)
						}
						p := filepath.Join(output, filepath.FromSlash(n)+".java")
						if e := os.WriteFile(p, src, 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, p)
					}
					if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s", e, out)
					}
					for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedOwner$Box", "mixed/scope/MixedOwner$Child"} {
						b, e := os.ReadFile(filepath.Join(output, filepath.FromSlash(n)+".class"))
						if e != nil {
							t.Fatal(e)
						}
						if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, b); want != got {
							t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, got)
						}
					}
					if got := t04RunJava(t, java, output, "mixed.scope.MixedDriver"); got != oracle {
						t.Fatalf("%q != %q", got, oracle)
					}
				})
			}
		})
	}
}

func TestNativeMemberAndAnonymousCrossFamilySourceBinding(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `package mixed.scope;
class MixedOwner{public static abstract class Box{final int tag;Box(int t){tag=t;}abstract long read();}}
class MixedExternal{MixedOwner.Box make(final long n){return new MixedOwner.Box(7){long read(){return n;}};}}
public class MixedDriver{public static void main(String[]args){for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){MixedOwner.Box b=new MixedExternal().make(n);if(b.tag!=7||b.read()!=n)throw new AssertionError("cross family capture");}System.out.println(5);}}
`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			file := filepath.Join(original, "MixedDriver.java")
			if e := os.WriteFile(file, []byte(source), 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); e != nil {
				t.Fatalf("original %v %s", e, out)
			}
			oracle := t04RunJava(t, java, original, "mixed.scope.MixedDriver")
			files := map[string][]byte{}
			if e := filepath.Walk(original, func(p string, info os.FileInfo, e error) error {
				if e != nil {
					return e
				}
				if !info.IsDir() && strings.HasSuffix(p, ".class") {
					r, e := os.ReadFile(p)
					if e != nil {
						return e
					}
					rel, e := filepath.Rel(original, p)
					if e != nil {
						return e
					}
					files[filepath.ToSlash(rel)] = r
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(mode, func(t *testing.T) {
					if mode == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if mode == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					for _, n := range []string{"mixed/scope/MixedOwner$Box.class", "mixed/scope/MixedExternal$1.class"} {
						s, e := z.ReadFile(n)
						if e != nil || !strings.Contains(string(s), "body owned by") {
							t.Fatalf("unowned %s %v %s", n, e, s)
						}
					}
					output := t.TempDir()
					for n, b := range files {
						if strings.HasPrefix(n, "mixed/scope/MixedOwner") || strings.HasPrefix(n, "mixed/scope/MixedExternal") {
							continue
						}
						p := filepath.Join(output, filepath.FromSlash(n))
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, b, 0600); e != nil {
							t.Fatal(e)
						}
					}
					paths := []string{}
					for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedExternal"} {
						src, e := z.ReadFile(n + ".class")
						if e != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v %s", e, src)
						}
						p := filepath.Join(output, filepath.FromSlash(n)+".java")
						if e := os.WriteFile(p, src, 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, p)
					}
					if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s", e, out)
					}
					for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedOwner$Box", "mixed/scope/MixedExternal$1"} {
						b, e := os.ReadFile(filepath.Join(output, filepath.FromSlash(n)+".class"))
						if e != nil {
							t.Fatal(e)
						}
						if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, b); want != got {
							t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, got)
						}
					}
					if got := t04RunJava(t, java, output, "mixed.scope.MixedDriver"); got != oracle {
						t.Fatalf("%q != %q", got, oracle)
					}
				})
			}
		})
	}
}

func TestNativeMemberPeerGenericFieldsKeepSeparateBindings(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `package mixed.scope;
interface ValueObserver<T>{void onNext(T n);}
class MixedOwner{public static abstract class Box<T> implements ValueObserver<T>{Object seen;}public static final class Source<T,R> implements ValueObserver<T>{final Box<T> box;Source(Box<T> b){box=b;}public void onNext(T n){box.onNext(n);}}public static final class Target<T,R> implements ValueObserver<R>{final ValueObserver<? super R> sink;Target(ValueObserver<? super R> b){sink=b;}public void onNext(R n){sink.onNext(n);}}}
class MixedBox<A> extends MixedOwner.Box<A>{public void onNext(A n){seen=n;}}
class MixedExternal{static <T,R> MixedOwner.Source<T,R> source(MixedOwner.Box<T> b){return new MixedOwner.Source<T,R>(b);}static <T,R> MixedOwner.Target<T,R> target(ValueObserver<R> s){return new MixedOwner.Target<T,R>(s);}}
public class MixedDriver{public static void main(String[]args){for(Object n:new Object[]{null,"text",Long.valueOf(Long.MIN_VALUE),Long.valueOf(Long.MAX_VALUE)}){MixedOwner.Box<Object> a=new MixedBox<>();MixedOwner.Box<String> b=new MixedBox<>();MixedOwner.Source<Object,String> source=MixedExternal.source(a);MixedOwner.Target<Object,String> target=MixedExternal.target(b);source.onNext(n);target.onNext("marker");if(a.seen!=n||b.seen!="marker")throw new AssertionError("peer generic binding");}System.out.println(4);}}
`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			file := filepath.Join(original, "MixedDriver.java")
			if e := os.WriteFile(file, []byte(source), 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); e != nil {
				t.Fatalf("original %v %s", e, out)
			}
			oracle := t04RunJava(t, java, original, "mixed.scope.MixedDriver")
			files := map[string][]byte{}
			if e := filepath.Walk(original, func(p string, info os.FileInfo, e error) error {
				if e != nil {
					return e
				}
				if !info.IsDir() && strings.HasSuffix(p, ".class") {
					r, e := os.ReadFile(p)
					if e != nil {
						return e
					}
					rel, e := filepath.Rel(original, p)
					if e != nil {
						return e
					}
					files[filepath.ToSlash(rel)] = r
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(mode, func(t *testing.T) {
					if mode == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if mode == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					for _, n := range []string{"mixed/scope/MixedOwner$Box.class", "mixed/scope/MixedOwner$Source.class", "mixed/scope/MixedOwner$Target.class"} {
						s, e := z.ReadFile(n)
						if e != nil || !strings.Contains(string(s), "body owned by") {
							t.Fatalf("unowned %s %v %s", n, e, s)
						}
					}
					output := t.TempDir()
					for n, b := range files {
						if strings.HasPrefix(n, "mixed/scope/MixedOwner") || strings.HasPrefix(n, "mixed/scope/MixedExternal") {
							continue
						}
						p := filepath.Join(output, filepath.FromSlash(n))
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, b, 0600); e != nil {
							t.Fatal(e)
						}
					}
					paths := []string{}
					for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedExternal"} {
						src, e := z.ReadFile(n + ".class")
						if e != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v %s", e, src)
						}
						p := filepath.Join(output, filepath.FromSlash(n)+".java")
						if e := os.WriteFile(p, src, 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, p)
					}
					if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s", e, out)
					}
					for _, n := range []string{"mixed/scope/MixedOwner", "mixed/scope/MixedOwner$Box", "mixed/scope/MixedOwner$Source", "mixed/scope/MixedOwner$Target"} {
						b, e := os.ReadFile(filepath.Join(output, filepath.FromSlash(n)+".class"))
						if e != nil {
							t.Fatal(e)
						}
						if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, b); want != got {
							t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, got)
						}
					}
					if got := t04RunJava(t, java, output, "mixed.scope.MixedDriver"); got != oracle {
						t.Fatalf("%q != %q", got, oracle)
					}
				})
			}
		})
	}
}

func TestNativeMemberOwnAndOuterGenericScopeRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `package generic.member;
class GenericEffects{static java.lang.String trace="";static Object seen;static java.io.IOException failure=new java.io.IOException("parent");}
class GenericParent{final Object owner;final java.lang.Number value;GenericParent(java.lang.Number n)throws java.io.IOException{GenericEffects.trace+="P";owner=owner();value=n;GenericEffects.seen=this;if(n!=null&&n.longValue()<0)throw GenericEffects.failure;}Object owner(){return null;}}
class GenericOwner<T extends java.lang.Number>{final T token;GenericOwner(T n){token=n;}
 static class Box<U extends java.lang.Number>{static final int runtime;static{GenericEffects.trace+="B";runtime=7;}final U value;final int selected;Box(U n){value=n;selected=1;}Box(Object rival){value=null;selected=2;}U get(){return value;}<V extends U> V echo(V v){return v;}static <T extends java.lang.Number> T own(T v){return v;}static int status(){return runtime;}}
 class Child<U extends T> extends GenericParent{final U argument;final int selected;Child(U n)throws java.io.IOException{super(n);argument=n;selected=1;}Child(Object rival)throws java.io.IOException{super(null);argument=null;selected=2;}Child(CharSequence rival)throws java.io.IOException{super(null);argument=null;selected=3;}Object owner(){return GenericOwner.this;}T token(){return GenericOwner.this.token;}U argument(){return argument;}Child<U> self(){return this;}<V extends U> V echo(V n){return n;}}class Shadow<T extends CharSequence>{final T text;final int selected;Shadow(T t){text=t;selected=1;}Shadow(Object rival){text=null;selected=2;}T text(){return text;}Shadow<T> self(){return this;}Object owner(){return GenericOwner.this;}}
 Box<T> box(T n){return new Box<T>(n);}Child<T> child(T n)throws java.io.IOException{return new Child<T>(n);}Shadow<java.lang.String> shadow(java.lang.String s){return new Shadow<java.lang.String>(s);}
}
class GenericExternal{static <T extends java.lang.Number> GenericOwner.Box<T> box(T n){return new GenericOwner.Box<T>(n);}static <T extends java.lang.Number> GenericOwner<T>.Child<T> child(GenericOwner<T> o,T n)throws java.io.IOException{return o.new Child<T>(n);}static GenericOwner<java.lang.Number>.Child<java.lang.Long> specific(GenericOwner<java.lang.Number> o,java.lang.Long n)throws java.io.IOException{return o.new Child<java.lang.Long>(n);}static Object raw(GenericOwner o,java.lang.Number n)throws java.io.IOException{return o.new Child(n);}static Object rival(GenericOwner o,CharSequence n)throws java.io.IOException{return o.new Child((Object)n);}static int status(){return GenericOwner.Box.runtime;}}
public class GenericDriver{public static void main(java.lang.String[]args)throws Exception{if(GenericExternal.status()!=7||!GenericEffects.trace.equals("B"))throw new AssertionError("blank static read");int rows=0;for(java.lang.Number n:new java.lang.Number[]{null,Integer.valueOf(-1),Integer.valueOf(0),Long.valueOf(Long.MIN_VALUE),Long.valueOf(Long.MAX_VALUE),Double.valueOf(-0.0),Double.valueOf(Double.NaN)}){GenericOwner<java.lang.Number> o=new GenericOwner<>(n);for(boolean external:new boolean[]{false,true}){GenericOwner.Box<java.lang.Number> b=external?GenericExternal.box(n):o.box(n);if(b.value!=n||b.get()!=n||b.echo(n)!=n||GenericOwner.Box.own(n)!=n||b.selected!=1)throw new AssertionError("static generic rival");GenericEffects.seen=null;try{GenericOwner<java.lang.Number>.Child<java.lang.Number> c=external?GenericExternal.child(o,n):o.child(n);if(c.owner!=o||c.value!=n||c.token()!=n||c.argument()!=n||c.echo(n)!=n)throw new AssertionError("member identity");}catch(java.io.IOException e){GenericOwner<java.lang.Number>.Child<java.lang.Number> c=(GenericOwner<java.lang.Number>.Child<java.lang.Number>)GenericEffects.seen;if(e!=GenericEffects.failure||n==null||n.longValue()>=0||c.owner!=o||c.value!=n||c.token()!=n)throw new AssertionError("parent publication",e);}GenericOwner<java.lang.Number>.Shadow<java.lang.String> shadow=o.shadow("value");if(shadow.owner()!=o||shadow.text()!= "value"||shadow.selected!=1||shadow.self()!=shadow)throw new AssertionError("shadowed class variable");rows++;}}GenericOwner<java.lang.Number> rawOwner=new GenericOwner<>(Long.valueOf(31));java.lang.Long rawValue=Long.valueOf(41);GenericOwner.Child rawChild=(GenericOwner.Child)GenericExternal.raw(rawOwner,rawValue);if(rawChild.owner!=rawOwner||rawChild.argument()!=rawValue)throw new AssertionError("raw member pair");GenericOwner<java.lang.Number>.Child<java.lang.Long> precise=GenericExternal.specific(rawOwner,rawValue);if(precise.owner!=rawOwner||precise.argument()!=rawValue||precise.selected!=1||precise.self()!=precise)throw new AssertionError("separate owner and member arguments");GenericOwner.Child pinned=(GenericOwner.Child)GenericExternal.rival(rawOwner,"text");if(pinned.selected!=2||pinned.owner!=rawOwner)throw new AssertionError("original Object constructor");System.out.println(rows+":"+GenericEffects.trace);}}
`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			file := filepath.Join(original, "GenericDriver.java")
			if e := os.WriteFile(file, []byte(source), 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); e != nil {
				t.Fatalf("original %v %s", e, out)
			}
			oracle := t04RunJava(t, java, original, "generic.member.GenericDriver")
			files := map[string][]byte{}
			if e := filepath.Walk(original, func(p string, info os.FileInfo, e error) error {
				if e != nil {
					return e
				}
				if !info.IsDir() && strings.HasSuffix(p, ".class") {
					r, e := os.ReadFile(p)
					if e != nil {
						return e
					}
					rel, e := filepath.Rel(original, p)
					if e != nil {
						return e
					}
					files[filepath.ToSlash(rel)] = r
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(mode, func(t *testing.T) {
					if mode == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if mode == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					for _, n := range []string{"generic/member/GenericOwner$Box.class", "generic/member/GenericOwner$Child.class", "generic/member/GenericOwner$Shadow.class"} {
						s, e := z.ReadFile(n)
						if e != nil || !strings.Contains(string(s), "original member body owned by") {
							t.Fatalf("unowned %s %v %s", n, e, s)
						}
					}
					output := t.TempDir()
					for n, b := range files {
						if strings.HasPrefix(n, "generic/member/GenericOwner") || n == "generic/member/GenericExternal.class" {
							continue
						}
						p := filepath.Join(output, filepath.FromSlash(n))
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, b, 0600); e != nil {
							t.Fatal(e)
						}
					}
					paths := []string{}
					for _, n := range []string{"generic/member/GenericOwner", "generic/member/GenericExternal"} {
						src, e := z.ReadFile(n + ".class")
						if e != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %v %s", e, src)
						}
						p := filepath.Join(output, filepath.FromSlash(n)+".java")
						if e := os.WriteFile(p, src, 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, p)
					}
					if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", output, "-d", output}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s", e, out)
					}
					for _, n := range []string{"generic/member/GenericOwner", "generic/member/GenericOwner$Box", "generic/member/GenericOwner$Child", "generic/member/GenericOwner$Shadow"} {
						b, e := os.ReadFile(filepath.Join(output, filepath.FromSlash(n)+".class"))
						if e != nil {
							t.Fatal(e)
						}
						if want, got := nativeBinaryShape(t, files[n+".class"]), nativeBinaryShape(t, b); want != got {
							t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, got)
						}
					}
					if got := t04RunJava(t, java, output, "generic.member.GenericDriver"); got != oracle {
						t.Fatalf("%q != %q", got, oracle)
					}
				})
			}
		})
	}
}
