package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The driver is compiled only against the original declarations. Wide slots,
// reassigned entry words, field shadowing and deferred Function failures are
// independent of constructor names, but must keep their binding after naming.
const constructorParameterNamesFixture = `
import java.util.function.Function;
class CtorNamedPlain {
 final long amount;final double ratio;final Object token;final int first,second;
 CtorNamedPlain(final long amount,double ratio,Object token,int var1,int var0){this.amount=amount;this.ratio=ratio;this.token=token;var1+=3;this.first=var1;this.second=var0;}
}
class CtorNamedOwner {
 static final class Box<T> {
  final Function<T,T> valueExtractor;
  Box(Function<T,T> valueExtractor){this.valueExtractor=valueExtractor;}
  T read(T value){return valueExtractor.apply(value);}
 }
}
class Ctor$Literal {final Object token;Ctor$Literal(Object token){this.token=token;}}
class CtorNameDriver {
 static final RuntimeException fault=new RuntimeException("original");static String trace="";
 static final class Operation implements Function<Object,Object>{final int mode;Operation(int mode){this.mode=mode;}public Object apply(Object value){trace+="F";if(mode==1)throw fault;return value;}}
 static void names(Class<?> owner,Class<?>[]types,String[]names,int[]flags)throws Exception{
  java.lang.reflect.Parameter[] parameters=owner.getDeclaredConstructor(types).getParameters();
  if(parameters.length!=names.length)throw new AssertionError("constructor arity");
  for(int i=0;i<names.length;i++)if(!parameters[i].isNamePresent()||!parameters[i].getName().equals(names[i])||parameters[i].getModifiers()!=flags[i]||parameters[i].getType()!=types[i])throw new AssertionError(owner.getName()+" constructor parameter "+i+"="+parameters[i].getName());
 }
 public static void main(String[]args)throws Exception{
  names(CtorNamedPlain.class,new Class<?>[]{long.class,double.class,Object.class,int.class,int.class},new String[]{"amount","ratio","token","var1","var0"},new int[]{16,0,0,0,0});
  names(CtorNamedOwner.Box.class,new Class<?>[]{Function.class},new String[]{"valueExtractor"},new int[]{0});
  names(Ctor$Literal.class,new Class<?>[]{Object.class},new String[]{"token"},new int[]{0});
  java.lang.reflect.Type generic=CtorNamedOwner.Box.class.getDeclaredConstructor(Function.class).getGenericParameterTypes()[0];
  if(!generic.getTypeName().equals("java.util.function.Function<T, T>"))throw new AssertionError("constructor formal signature "+generic);
  int worlds=0;Object token=new Object();
  for(long wide:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(double ratio:new double[]{Double.NEGATIVE_INFINITY,-0D,Double.NaN,Double.POSITIVE_INFINITY})for(Object captured:new Object[]{null,token})for(int entry:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   CtorNamedPlain value=new CtorNamedPlain(wide,ratio,captured,entry,~entry);
   if(value.amount!=wide||Double.doubleToLongBits(value.ratio)!=Double.doubleToLongBits(ratio)||value.token!=captured||value.first!=entry+3||value.second!=~entry||new Ctor$Literal(captured).token!=captured)throw new AssertionError("constructor entry/storage binding "+worlds);worlds++;
  }
  for(int mode:new int[]{-1,0,1})for(Object captured:new Object[]{null,token}){
   trace="";Function<Object,Object> operation=mode<0?null:new Operation(mode);CtorNamedOwner.Box<Object> box=new CtorNamedOwner.Box<Object>(operation);
   if(box.valueExtractor!=operation||!trace.equals(""))throw new AssertionError("eager constructor extraction");
   Object actual=null;Throwable caught=null;try{actual=box.read(captured);}catch(Throwable error){caught=error;}
   if(mode<0?(caught==null||caught.getClass()!=NullPointerException.class||!trace.equals("")):mode==1?(caught!=fault||!trace.equals("F")):(caught!=null||actual!=captured||!trace.equals("F")))throw new AssertionError("deferred extractor/fault identity "+worlds,caught);worlds++;
  }
  System.out.println(worlds+":constructor:entry:metadata:wide:shadow:deferred");
 }
}`

func TestAdversarialConstructorParameterNamesKeepPhysicalEntryAndDeferredBinding(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required for independent constructor compiler oracle")
	}
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = native
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, constructorParameterNamesFixture, "CtorNamedOwner", debug)
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					oracle := t04RunJava(t, java, original, "CtorNameDriver")
					if oracle != "206:constructor:entry:metadata:wide:shadow:deferred\n" {
						t.Fatalf("original independent constructor model: %q", oracle)
					}
					for _, policy := range []string{"precision", "compatibility"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "compatibility" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							jar := filepath.Join(t.TempDir(), "original.jar")
							if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
								t.Fatal(err)
							}
							z, err := NewJarFSFromLocalWithCompilerProfile(jar, 8, profile, nil)
							if err != nil {
								t.Fatal(err)
							}
							defer z.Close()
							out := t.TempDir()
							var paths, owned []string
							for name, raw := range files {
								if name == "CtorNamedPlain.class" || name == "Ctor$Literal.class" || name == "CtorNamedOwner.class" || strings.HasPrefix(name, "CtorNamedOwner$") {
									owned = append(owned, name)
									source, e := z.ReadFile(name)
									if e != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "// decompile dump failed") {
										t.Fatalf("source %s: %v\n%s", name, e, source)
									}
									path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
									if e := os.WriteFile(path, source, 0600); e != nil {
										t.Fatal(e)
									}
									paths = append(paths, path)
								} else if e := os.WriteFile(filepath.Join(out, name), raw, 0600); e != nil {
									t.Fatal(e)
								}
							}
							sort.Strings(paths)
							args := []string{"-proc:none", "-parameters", "--release", "8"}
							if profile == NativeJavac8 {
								args = []string{"-proc:none", "-parameters", "-source", "8", "-target", "8"}
							}
							args = append(args, "-g:"+debug, "-cp", out, "-d", out)
							if log, err := exec.Command(compiler, append(args, paths...)...).CombinedOutput(); err != nil {
								t.Fatalf("rebuilt constructor compilation: %v\n%s", err, log)
							}
							if got := t04RunJava(t, java, out, "CtorNameDriver"); got != oracle {
								t.Fatalf("rebuilt constructor independent model: %q want %q", got, oracle)
							}
							for _, name := range owned {
								rebuilt, err := os.ReadFile(filepath.Join(out, name))
								if err != nil {
									t.Fatal(err)
								}
								nativeHiddenConstructorParameterMetadata(t, name, files[name], rebuilt)
								nativeLexicalExactSignatures(t, name, files[name], rebuilt)
								if a, b := nativeBinaryShape(t, files[name]), nativeBinaryShape(t, rebuilt); a != b {
									t.Fatalf("constructor ABI %s:\n%s\n%s", name, a, b)
								}
								if a, b := methodParameterNamesAllMethodDeclarations(t, files[name]), methodParameterNamesAllMethodDeclarations(t, rebuilt); !reflect.DeepEqual(a, b) {
									t.Fatalf("complete constructor method inventory %s: %#v -> %#v", name, a, b)
								}
							}
						})
					}
				})
			}
		})
	}
}
