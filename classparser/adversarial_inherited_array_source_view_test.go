package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The field is physically Object, but its original owner path substitutes an
// array of the current class formal. Keeping only the physical Object[] cast
// loses the type argument before the static generic call and its fluent chain.
// A source view may recover that argument only while retaining the same JVM
// CHECKCAST, including polluted reference/primitive arrays and exception order.
const inheritedArraySourceViewFixture = `
class ArrayViewState {static String trace="";static final RuntimeException failure=new IllegalStateException("same");}
class ArrayViewBase<T> {protected T payload;ArrayViewBase(T value){payload=value;}}
interface ArrayViewCondition<T> {boolean test(T value);}
class ArrayViewCell<T> {
 final T[] values;ArrayViewCell(T[] values){this.values=values;}
 ArrayViewCell<T> being(ArrayViewCondition<? super T> predicate){
  ArrayViewState.trace+="P";java.util.List<T> kept=new java.util.ArrayList<T>();
  for(T value:values){ArrayViewState.trace+="V";if(predicate.test(value))kept.add(value);}
  @SuppressWarnings("unchecked")T[] result=(T[])kept.toArray();return new ArrayViewCell<T>(result);
 }
 java.util.List<T> get(){return java.util.Arrays.asList(values);}
}
class ArrayViewFilters {
 static <T>ArrayViewCell<T> filter(T[] values){ArrayViewState.trace+="F";if(values==null)throw ArrayViewState.failure;return new ArrayViewCell<T>(values);}
 static <T>ArrayViewCell<T> filter(Iterable<T> values){throw new AssertionError("wrong source overload");}
}
class ArrayViewCarrier<E> extends ArrayViewBase<E[]> {
 ArrayViewCarrier(E[] values){super(values);}
 java.util.List<E> all(){return ArrayViewFilters.filter(payload).get();}
 java.util.List<E> selected(ArrayViewCondition<? super E> predicate){return consume(ArrayViewFilters.filter(payload).being(predicate).get());}
 private java.util.List<E> consume(java.util.List<E> values){return values;}
 private java.util.List<E> consume(E[] values){throw new AssertionError("wrong array overload");}
 String selectedOverload(){return choose((Object[])payload,"literal");}
 private String choose(E[] values,Object token){return "wrong";}
 private String choose(Object[] values,String token){return "expected";}
}
class ArrayViewDriver {
 @SuppressWarnings({"rawtypes","unchecked"}) public static void main(String[]args){
  Object token=new Object();int rows=0;ArrayViewCarrier<Object> owner=new ArrayViewCarrier<Object>(new Object[0]);
  for(Object[] values:new Object[][]{new Object[]{token,null,"text"},new String[]{"a",null,"b"},new Object[][]{new Object[]{token},null,new Object[]{"other"}}}){
   ((ArrayViewBase)owner).payload=values;ArrayViewState.trace="";
   java.util.List<Object> all=owner.all();if(all.size()!=values.length||!ArrayViewState.trace.equals("F"))throw new AssertionError("generic result/order");
   for(int i=0;i<values.length;i++)if(all.get(i)!=values[i])throw new AssertionError("array element identity");rows++;
   ArrayViewState.trace="";java.util.List<Object> selected=owner.selected(value->value!=null);
   int kept=0;for(Object value:values)if(value!=null){if(selected.get(kept++)!=value)throw new AssertionError("selected identity");}
   if(selected.size()!=kept||!ArrayViewState.trace.equals("FPVVV"))throw new AssertionError("predicate effects");rows++;
   ArrayViewState.trace="";Throwable actual=null;try{owner.selected(value->{throw ArrayViewState.failure;});}catch(Throwable e){actual=e;}
   if(actual!=ArrayViewState.failure||!ArrayViewState.trace.equals("FPV"))throw new AssertionError("callback identity/order",actual);rows++;
   if(!owner.selectedOverload().equals("expected"))throw new AssertionError("changing an array view stole a source overload");rows++;
  }
  for(Object value:new Object[]{null,new Object(),new int[]{1,2}})for(boolean filter:new boolean[]{false,true}){
   ((ArrayViewBase)owner).payload=value;ArrayViewState.trace="";Throwable actual=null;
   try{if(filter)owner.selected(item->{throw new AssertionError("callback before original cast");});else owner.all();}catch(Throwable e){actual=e;}
   if(value==null?(actual!=ArrayViewState.failure||!ArrayViewState.trace.equals("F")):(!(actual instanceof ClassCastException)||!ArrayViewState.trace.equals("")))throw new AssertionError("original CHECKCAST/effect priority",actual);rows++;
  }
  System.out.println(rows+":field:generic:identity:cast:methods="+ArrayViewCarrier.class.getDeclaredMethods().length);
 }
}`

func TestAdversarialInheritedArrayReadPreservesSourceInference(t *testing.T) {
	t.Run("ordinary result", func(t *testing.T) {
		inheritedArraySourceViewRoundTrip(t, inheritedArraySourceViewFixture, "18:field:generic:identity:cast:methods=7\n")
	})
	t.Run("self-bound fluent result", func(t *testing.T) {
		fixture := strings.Replace(inheritedArraySourceViewFixture, "class ArrayViewCarrier<E>", "class ArrayViewCarrier<SELF extends ArrayViewCarrier<SELF,E>,E>", 1)
		fixture = strings.Replace(fixture, "class ArrayViewDriver", "class ArrayViewConcrete extends ArrayViewCarrier<ArrayViewConcrete,Object>{ArrayViewConcrete(Object[] values){super(values);}}\nclass ArrayViewDriver", 1)
		fixture = strings.Replace(fixture, "ArrayViewCarrier<Object> owner=new ArrayViewCarrier<Object>(new Object[0]);", "ArrayViewConcrete owner=new ArrayViewConcrete(new Object[0]);", 1)
		fixture = strings.Replace(fixture, "private String choose(Object[] values,String token){return \"expected\";}", `private String choose(Object[] values,String token){return "expected";}
 SELF fluent(ArrayViewCondition<? super E> condition){return withList(ArrayViewFilters.filter(payload).being(condition).get());}
 @SuppressWarnings("unchecked") private SELF withList(java.util.List<E> values){return(SELF)this;}
 private SELF withList(E[] values){throw new AssertionError("wrong fluent array overload");}`, 1)
		fixture = strings.Replace(fixture, `if(!owner.selectedOverload().equals("expected"))throw new AssertionError("changing an array view stole a source overload");rows++;`, `if(!owner.selectedOverload().equals("expected"))throw new AssertionError("changing an array view stole a source overload");rows++;
   ArrayViewState.trace="";if(owner.fluent(value->value!=null)!=owner||!ArrayViewState.trace.equals("FPVVV"))throw new AssertionError("fluent receiver identity and callback order");rows++;`, 1)
		inheritedArraySourceViewRoundTrip(t, fixture, "21:field:generic:identity:cast:methods=10\n")
	})
}

func inheritedArraySourceViewRoundTrip(t *testing.T, originalFixture, want string) {
	t.Helper()
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			javac := modern
			if profile == NativeJavac8 {
				javac = os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for the independent compiler")
				}
				if version, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(version), "javac 1.8.") {
					t.Fatalf("native8 needs actual javac8: %v / %s", err, version)
				}
			}
			for _, owner := range []string{"ArrayViewCarrier", "AlternateArrayView"} {
				t.Run(owner, func(t *testing.T) {
					fixture := strings.ReplaceAll(originalFixture, "ArrayViewCarrier", owner)
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							var files map[string][]byte
							if profile == NativeJavac8 {
								files = nativePrivateEnumCompile(t, fixture, owner, debug)
							} else {
								files = nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
							}
							write := func(dir, name string, data []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
									t.Fatal(err)
								}
							}
							original := t.TempDir()
							before := map[string][]byte{}
							for name, raw := range files {
								before[name] = bytes.Clone(raw)
								write(original, name, raw)
							}
							if got := t04RunJava(t, java, original, "ArrayViewDriver"); got != want {
								t.Fatalf("independent original: %q want %q", got, want)
							}
							t.Log("independent original inherited-array JVM: " + strings.TrimSpace(want))
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { data, known := files[name+".class"]; return bytes.Clone(data), known }})
									if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
										t.Fatalf("source status=%s: %v\n%s", result.Status, err, result.Source)
									}
									out := t.TempDir()
									for name, raw := range files {
										if name != owner+".class" {
											write(out, name, raw)
										}
									}
									write(out, owner+".java", []byte(result.Source))
									args := []string{"-proc:none", "--release", "8"}
									if profile == NativeJavac8 {
										args = []string{"-proc:none", "-source", "8", "-target", "8"}
									}
									args = append(args, "-g:"+debug, "-cp", out, "-d", out, filepath.Join(out, owner+".java"))
									if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
										t.Fatalf("rebuild without original target: %v\n%s\n%s", err, log, result.Source)
									}
									if got := t04RunJava(t, java, out, "ArrayViewDriver"); got != want {
										t.Fatalf("unchanged binary caller: %q want %q\n%s", got, want, result.Source)
									}
									rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
									if err != nil {
										t.Fatal(err)
									}
									if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
										t.Fatalf("declaration inventory changed:\n%s\n%s", a, b)
									}
									for name, raw := range files {
										if !bytes.Equal(raw, before[name]) {
											t.Fatal("original input changed", name)
										}
									}
									t.Log("fully rebuilt inherited-array JVM: " + strings.TrimSpace(want))
								})
							}
						})
					}
				})
			}
		})
	}
}
