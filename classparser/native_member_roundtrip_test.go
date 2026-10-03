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
