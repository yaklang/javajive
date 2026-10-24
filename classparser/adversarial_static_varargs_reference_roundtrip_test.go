package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The unchanged binary driver checks independent arithmetic/formatting models,
// null adaptation and the actual synthetic ABI. In particular, an (int)->long
// arrow has a different implementation port from an int SAM referencing a
// long... declaration, even when both return the same result for every int.
const staticVarargsReferenceFixture = `
import java.util.*;import java.util.function.*;import java.math.BigInteger;
class DefaultCompilerCases {
 static long last;
 static long checksum(long... values){long result=0;for(long value:values)result=result*65537+value;return result;}
 static String show(Object... values){return Arrays.deepToString(values);}
 static String prefix(long seed,Object... values){return String.valueOf(seed)+Arrays.deepToString(values);}
 static void remember(long... values){last=checksum(values);}
 static IntToLongFunction wide(){return DefaultCompilerCases::checksum;}
 static IntFunction<String> boxed(){return DefaultCompilerCases::show;}
 static Function<Integer,Long> unboxed(){return DefaultCompilerCases::checksum;}
 static IntConsumer discarded(){return DefaultCompilerCases::remember;}
 static Supplier<String> empty(){return DefaultCompilerCases::show;}
 static BiFunction<Long,Integer,String> mixed(){return DefaultCompilerCases::prefix;}
}
class DefaultCompilerDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int value:new int[]{Integer.MIN_VALUE,-33,-1,0,1,31,32,Integer.MAX_VALUE}){
   long expected=BigInteger.valueOf(value).longValueExact();
   if(DefaultCompilerCases.wide().applyAsLong(value)!=expected)throw new AssertionError("primitive widening");rows++;
   if(DefaultCompilerCases.unboxed().apply(value).longValue()!=expected)throw new AssertionError("unboxing and result boxing");rows++;
   if(!DefaultCompilerCases.boxed().apply(value).equals("["+Integer.toString(value)+"]"))throw new AssertionError("input boxing");rows++;
   DefaultCompilerCases.discarded().accept(value);if(DefaultCompilerCases.last!=expected)throw new AssertionError("void effect");rows++;
  }
  for(Long seed:new Long[]{Long.MIN_VALUE,0L,Long.MAX_VALUE})for(Integer value:new Integer[]{null,-1,7}){
   String expected=Long.toString(seed.longValue())+"["+String.valueOf(value)+"]";
   if(!DefaultCompilerCases.mixed().apply(seed,value).equals(expected))throw new AssertionError("wide fixed and packed ports");rows++;
  }
  if(!DefaultCompilerCases.empty().get().equals("[]"))throw new AssertionError("empty pack");rows++;
  try{DefaultCompilerCases.unboxed().apply(null);throw new AssertionError("missing null unboxing");}catch(NullPointerException expected){rows++;}
  List<String> methods=new ArrayList<String>();
  for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic()){
   String signature=method.getName()+":"+method.getModifiers()+":"+method.getReturnType().getName();
   for(Class<?> parameter:method.getParameterTypes())signature+=":"+parameter.getName();methods.add(signature);
  }
  Collections.sort(methods);System.out.println(rows+":static-varargs:"+methods);
 }
}`

func TestAdversarialStaticVarargsReferenceRetainsAdaptationAndImplementationABI(t *testing.T) {
	assertDefaultCompilerRegistrationOracle(t, staticVarargsReferenceFixture, "43:static-varargs:")
}

func TestAdversarialStaticReferenceRefusalCannotPublishChangedArrowABI(t *testing.T) {
	compiler, java := t04Tools(t)
	const fixture = `import java.util.function.IntToLongFunction;
class StaticPortShadow {
 static long sum(long... values){long total=0;for(long value:values)total+=value;return total;}
 static IntToLongFunction factory(int ignored){return StaticPortShadow::sum;}
}
class StaticPortDriver {
 public static void main(String[]args)throws Exception{
  if(!StaticPortShadow.class.getDeclaredMethod("factory",int.class).getParameters()[0].getName().equals("StaticPortShadow"))throw new AssertionError("original parameter metadata");
  for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(StaticPortShadow.factory(3).applyAsLong(value)!=(long)value)throw new AssertionError("original arithmetic");
  System.out.println("shadowed:ok");
 }
}`
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, fixture, "StaticPortShadow", "none")
	object, err := Parse(bytes.Clone(files["StaticPortShadow.class"]))
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name != "factory" {
			continue
		}
		for _, attribute := range method.Attributes {
			if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
				if changed || len(table.Info) != 5 || table.Info[0] != 1 {
					t.Fatal("authored one-parameter declaration table")
				}
				// This legal class-file metadata does not alter any executable
				// instruction. Its preserved value name obscures the only TypeName
				// spelling in the unnamed package, so this projection must refuse.
				binary.BigEndian.PutUint16(table.Info[1:3], sourceBridgePoolString(t, object, "StaticPortShadow"))
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("authored declaration metadata absent")
	}
	files["StaticPortShadow.class"] = object.Bytes()
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "StaticPortDriver"); got != "shadowed:ok\n" {
		t.Fatalf("mutated metadata is not an executable independent original: %q", got)
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			result, err := DecompileWithOptions(files["StaticPortShadow.class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, SourceCompiler: ModernJavac, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return raw, known }})
			if err != nil || result.Status == "complete" || !strings.Contains(fmt.Sprint(result.Diagnostics), "ordinary arrow changes original static implementation input types") {
				t.Fatalf("unproved reference fell back to a false complete arrow: status=%s err=%v diagnostics=%v\n%s", result.Status, err, result.Diagnostics, result.Source)
			}
		})
	}
}
