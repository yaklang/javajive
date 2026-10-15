package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The original driver observes each physical field independently. Rebuilding
// the driver would let the same lost-owner bug rewrite both sides of an oracle.
// Only the accessor class is replaced; the hierarchy and effect producers stay
// original bytecode, including distinct same-name fields with equal types.
func TestAdversarialInstanceFieldSelectionKeepsOriginalSymbolicOwner(t *testing.T) {
	const fixture = `class BindingBase {public int word;public long wide;public Object value;}
class BindingMiddle extends BindingBase {public int word;public long wide;public Object value;}
class BindingChild extends BindingMiddle {public int word;public long wide;public Object value;}
class BindingEffects {static int trace;static BindingChild receiver(BindingChild c){trace=trace*7+1;return c;}static int rhs(int n){trace=trace*7+2;return n;}}
class BindingAccess {
 static int read(BindingChild c){return ((BindingBase)c).word;}
 static int middle(BindingChild c){return ((BindingMiddle)c).word;}
 static int own(BindingChild c){return c.word;}
 static void write(BindingChild c,int n){((BindingBase)c).word=n;}
 static long readWide(BindingChild c){return ((BindingBase)c).wide;}
 static void writeWide(BindingChild c,long n){((BindingBase)c).wide=n;}
 static Object readReference(BindingChild c){return ((BindingBase)c).value;}
 static void writeReference(BindingChild c,Object value){((BindingBase)c).value=value;}
 static int selected(BindingChild a,BindingChild b,boolean choose){return ((BindingBase)(choose?a:b)).word;}
 static void ordered(BindingChild c,int n){((BindingBase)BindingEffects.receiver(c)).word=BindingEffects.rhs(n);}
 static int assigned(BindingChild c,int n){return ((BindingBase)c).word=n;}
 static int post(BindingChild c){return ((BindingBase)c).word++;}
 static int pre(BindingChild c){return ++((BindingBase)c).word;}
 static long compound(BindingChild c,long n){return ((BindingBase)c).wide+=n;}
}
class BindingDriver {public static void main(String[]args){int rows=0;Object base=new Object(),middle=new Object(),child=new Object();
 for(int repeat=0;repeat<3;repeat++)for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  BindingChild c=new BindingChild();((BindingBase)c).word=17;((BindingMiddle)c).word=29;c.word=43;
  ((BindingBase)c).wide=Long.MIN_VALUE+n;((BindingMiddle)c).wide=Long.MAX_VALUE;c.wide=31;
  ((BindingBase)c).value=base;((BindingMiddle)c).value=middle;c.value=child;
  if(BindingAccess.read(c)!=17||BindingAccess.middle(c)!=29||BindingAccess.own(c)!=43)throw new AssertionError("original field read owner");
  BindingAccess.write(c,n);if(((BindingBase)c).word!=n||((BindingMiddle)c).word!=29||c.word!=43)throw new AssertionError("original field store owner");
  if(BindingAccess.readWide(c)!=Long.MIN_VALUE+n)throw new AssertionError("wide read owner");
  BindingAccess.writeWide(c,Long.MAX_VALUE+n);if(((BindingBase)c).wide!=Long.MAX_VALUE+n||((BindingMiddle)c).wide!=Long.MAX_VALUE||c.wide!=31)throw new AssertionError("wide store owner");
  if(BindingAccess.readReference(c)!=base)throw new AssertionError("reference read identity");
  BindingAccess.writeReference(c,child);if(((BindingBase)c).value!=child||((BindingMiddle)c).value!=middle||c.value!=child)throw new AssertionError("reference store identity");
  BindingChild other=new BindingChild();((BindingBase)other).word=71;other.word=83;
  if(BindingAccess.selected(c,other,true)!=n||BindingAccess.selected(c,other,false)!=71)throw new AssertionError("conditional receiver binding");
  BindingEffects.trace=0;BindingAccess.ordered(c,n+1);if(BindingEffects.trace!=9||((BindingBase)c).word!=n+1||c.word!=43)throw new AssertionError("effectful store order/owner");
  if(BindingAccess.assigned(c,n-1)!=n-1||((BindingBase)c).word!=n-1||c.word!=43)throw new AssertionError("assignment result owner");
  if(BindingAccess.post(c)!=n-1||((BindingBase)c).word!=n||BindingAccess.pre(c)!=n+1||((BindingBase)c).word!=n+1||c.word!=43)throw new AssertionError("pre/post update identity");
  long expected=(Long.MAX_VALUE+n)+Long.MIN_VALUE;if(BindingAccess.compound(c,Long.MIN_VALUE)!=expected||((BindingBase)c).wide!=expected||c.wide!=31)throw new AssertionError("compound wide store/result");rows++;
 }
 BindingEffects.trace=0;try{BindingAccess.ordered(null,1);throw new AssertionError("null store");}catch(NullPointerException expected){}
 if(BindingEffects.trace!=9)throw new AssertionError("RHS must run before store null check");
 System.out.println(rows+":field-owner:wide:identity:null:order");}}
`
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "15:field-owner:wide:identity:null:order\n")
}

func TestAdversarialGenericFieldSelectionKeepsOriginalErasedStore(t *testing.T) {
	const fixture = `class BindingBase<T>{public T value;}
class BindingChild<T> extends BindingBase<T>{public T value;}
class BindingAccess{
 static String read(BindingChild<String> c){return ((BindingBase<String>)c).value;}
 static Object erased(BindingChild<String> c){return ((BindingBase)c).value;}
 static void rawStore(BindingChild<String> c,Object value){((BindingBase)c).value=value;}
 static void typedStore(BindingChild<String> c,String value){((BindingBase<String>)c).value=value;}
}
class BindingDriver{public static void main(String[]args){int rows=0;
 for(int repeat=0;repeat<3;repeat++)for(Object value:new Object[]{"base",new Object(),null}){
  BindingChild<String> c=new BindingChild<String>();c.value="child";
  BindingAccess.rawStore(c,value);
  if(((BindingBase)c).value!=value||c.value!="child"||BindingAccess.erased(c)!=value)throw new AssertionError("erased store must not narrow or change owner");
  if(value==null||value instanceof String){if(BindingAccess.read(c)!=value)throw new AssertionError("checked source result");}
  else{try{BindingAccess.read(c);throw new AssertionError("original result check");}catch(ClassCastException expected){}}
  BindingAccess.typedStore(c,"typed");if(((BindingBase)c).value!="typed"||c.value!="child")throw new AssertionError("typed store owner");rows++;
 }System.out.println(rows+":generic:erased-store:identity:result-check");}}
`
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "9:generic:erased-store:identity:result-check\n")
}

func fieldOwnerOriginalDriverRoundTrip(t *testing.T, fixture, expected string) {
	t.Helper()
	fieldOwnerOriginalSourcesDriverRoundTrip(t, map[string]string{"BindingAccess.java": fixture}, "BindingAccess", "BindingDriver", expected)
}
func fieldOwnerOriginalSourcesDriverRoundTrip(t *testing.T, fixtures map[string]string, owner, driver, expected string) {
	t.Helper()
	javac, java := t04Tools(t)
	for _, prefix := range []string{"Binding", "Renamed"} {
		t.Run(prefix, func(t *testing.T) {
			for _, compiler := range []string{"modern", "javac8"} {
				t.Run(compiler, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							sources := map[string]string{}
							for name, source := range fixtures {
								sources[strings.ReplaceAll(name, "Binding", prefix)] = strings.ReplaceAll(source, "Binding", prefix)
							}
							selectedOwner := strings.ReplaceAll(owner, "Binding", prefix)
							selectedDriver := strings.ReplaceAll(driver, "Binding", prefix)
							var files map[string][]byte
							if compiler == "javac8" {
								files = fieldOwnerCompileJava8Sources(t, sources, debug)
							} else {
								files = nativeCompileSourceReleaseClasses(t, sources, debug, "8")
							}
							original := t.TempDir()
							for name, raw := range files {
								if err := os.MkdirAll(filepath.Dir(filepath.Join(original, name)), 0700); err != nil {
									t.Fatal(err)
								}
								if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							want := t04RunJava(t, java, original, selectedDriver)
							if want != expected {
								t.Fatal("original independent oracle", want)
							}
							resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
							for _, mode := range []DecompileMode{Precision, Compatibility, "legacy", "archive"} {
								t.Run(string(mode), func(t *testing.T) {
									var generated string
									if mode == "archive" {
										z := nativeArchive(t, files)
										defer z.Close()
										z.targetSourceVersion = 8
										raw, err := z.ReadFile(selectedOwner + ".class")
										if err != nil {
											t.Fatal(err)
										}
										generated = string(raw)
									} else if mode == "legacy" {
										var err error
										generated, err = DecompileWithResolver(files[selectedOwner+".class"], resolve)
										if err != nil {
											t.Fatal(err)
										}
									} else {
										result, err := DecompileWithOptions(files[selectedOwner+".class"], DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
										if err != nil {
											t.Fatal(err)
										}
										generated = result.Source
									}
									if strings.Contains(generated, DecompileStubMarker) {
										t.Fatal("stub", generated)
									}
									out := t.TempDir()
									for name, raw := range files {
										if name == selectedOwner+".class" {
											continue
										}
										if err := os.MkdirAll(filepath.Dir(filepath.Join(out, name)), 0700); err != nil {
											t.Fatal(err)
										}
										if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
											t.Fatal(err)
										}
									}
									path := filepath.Join(out, selectedOwner+".java")
									if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
										t.Fatal(err)
									}
									if err := os.WriteFile(path, []byte(generated), 0600); err != nil {
										t.Fatal(err)
									}
									if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput(); err != nil {
										t.Fatalf("rebuild accessor without original fallback: %v\n%s\n%s", err, raw, generated)
									}
									if got := t04RunJava(t, java, out, selectedDriver); got != want {
										t.Fatalf("candidate=%q want=%q\n%s", got, want, generated)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

// Conditional stores can bypass standalone AssignStatement lowering. The driver
// remains original bytecode, so a lost symbolic target cannot corrupt the oracle.
func TestAdversarialConditionalFieldStoreKeepsOriginalSymbolicOwner(t *testing.T) {
	const fixture = `class BindingBase {public int word;public long wide;}
class BindingChild extends BindingBase {public int word;public long wide;}
class BindingAccess {
 static int choose(BindingChild c,boolean first,int n){return first?(((BindingBase)c).word=n):(((BindingBase)c).word=n+1);}
 static long wide(BindingChild c,boolean first,long n){return first?(((BindingBase)c).wide=n):(((BindingBase)c).wide=n-1);}
}
class BindingDriver {public static void main(String[]args){int rows=0;for(boolean first:new boolean[]{false,true})for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){BindingChild c=new BindingChild();c.word=43;c.wide=71;int expected=first?n:n+1;if(BindingAccess.choose(c,first,n)!=expected||((BindingBase)c).word!=expected||c.word!=43)throw new AssertionError("conditional physical field target/result");long value=(long)n*Integer.MAX_VALUE;long wanted=first?value:value-1;if(BindingAccess.wide(c,first,value)!=wanted||((BindingBase)c).wide!=wanted||c.wide!=71)throw new AssertionError("conditional wide target/result");rows++;}System.out.println(rows+":conditional-store:owner:result");}}
`
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "10:conditional-store:owner:result\n")
}

func fieldOwnerCompileJava8Sources(t *testing.T, sources map[string]string, debug string) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for name, source := range sources {
		p := filepath.Join(dir, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte(source), 0600); e != nil {
			t.Fatal(e)
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	args := append([]string{"-proc:none", "-g:" + debug, "-d", dir}, paths...)
	if out, e := exec.Command(os.Getenv("JAVA8_JAVAC"), args...).CombinedOutput(); e != nil {
		t.Fatalf("original javac8: %v\n%s", e, out)
	}
	files := map[string][]byte{}
	if e := filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() || !strings.HasSuffix(p, ".class") {
			return nil
		}
		name, e := filepath.Rel(dir, p)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		files[filepath.ToSlash(name)] = b
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	return files
}
func TestAdversarialProtectedFieldSelectionKeepsAccessingSubclassQualifier(t *testing.T) {
	sources := map[string]string{
		"p/BindingBase.java":   `package p;public class BindingBase{protected int word;protected long wide;public int baseWord(){return word;}public long baseWide(){return wide;}}`,
		"q/BindingMiddle.java": `package q;public class BindingMiddle extends p.BindingBase{}`,
		"q/BindingChild.java":  `package q;public class BindingChild extends BindingAccess{public int word=43;public long wide=71;}`,
		"q/BindingAccess.java": `package q;public class BindingAccess extends BindingMiddle{static int read(BindingChild c){return ((BindingAccess)c).word;}static void write(BindingChild c,int n){((BindingAccess)c).word=n;}static long wide(BindingChild c){return ((BindingAccess)c).wide;}static long assigned(BindingChild c,long n){return ((BindingAccess)c).wide=n;}static int selected(BindingChild c,boolean first,int n){return first?(((BindingAccess)c).word=n):(((BindingAccess)c).word=n+1);}}`,
		"q/BindingDriver.java": `package q;class BindingDriver{public static void main(String[]args){int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){BindingChild c=new BindingChild();BindingAccess.write(c,n);if(BindingAccess.read(c)!=n||c.baseWord()!=n||c.word!=43)throw new AssertionError("protected owner/qualifier");long value=(long)n*Integer.MAX_VALUE;if(BindingAccess.assigned(c,value)!=value||BindingAccess.wide(c)!=value||c.baseWide()!=value||c.wide!=71)throw new AssertionError("protected wide owner");for(boolean first:new boolean[]{false,true}){int expected=first?n:n+1;if(BindingAccess.selected(c,first,n)!=expected||c.baseWord()!=expected||c.word!=43)throw new AssertionError("protected conditional owner");rows++;}}try{BindingAccess.read(null);throw new AssertionError("null read");}catch(NullPointerException expected){}System.out.println(rows+":protected:qualifier:owner:null");}}`,
	}
	fieldOwnerOriginalSourcesDriverRoundTrip(t, sources, "q/BindingAccess", "q.BindingDriver", "10:protected:qualifier:owner:null\n")
}
