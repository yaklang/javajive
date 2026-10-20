package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

func TestAdversarialVarargsReferenceSelectionMatchesIndependentCompilers(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, variableReferenceBindingFixture, "VariableBindingDriver", "none", true)
			dir := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := "reference=pack\nfixed=fixed\nboxing=boxed\nwidening=widened\nzero=narrow\ntwo=two\ncovariance=fixed-array\narray-pack=array-pack\nprimitive-array=primitive-array\nunboxing=unboxed\nnull=null-item\nalias=alias\nvoid=pack\nthrow=same\n"
			if got := t04RunJava(t, java, dir, "VariableBindingDriver"); got != want {
				t.Fatalf("independent compiler selection/identity oracle %q want %q", got, want)
			}
			const prefix = "LVariableBindingDriver$"
			const result = "Ljava/lang/String;"
			for _, fixture := range []struct {
				name, owner, target, sam string
				want                     bool
			}{
				{"reference pack", "Pack", "([" + prefix + "A;)" + result, "(" + prefix + "A;)" + result, true},
				{"fixed reference phase", "Fixed", "([" + prefix + "A;)" + result, "(" + prefix + "A;)" + result, false},
				{"boxing phase", "Boxing", "([I)" + result, "(I)" + result, false},
				{"primitive widening phase", "Widening", "([I)" + result, "(I)" + result, false},
				{"zero pack specificity", "Zero", "([" + prefix + "A;)" + result, "()" + result, true},
				{"different declared arities", "Two", "(" + prefix + "A;[" + prefix + "A;)" + result, "(" + prefix + "A;" + prefix + "A;)" + result, true},
				{"array covariance earlier phase", "Covariance", "([[" + prefix + "A;)" + result, "([" + prefix + "B;)" + result, false},
				{"array covariance within pack", "ArrayPack", "([[" + prefix + "A;)" + result, "([" + prefix + "B;)" + result, true},
				{"primitive array one reference", "PrimitiveArray", "([Ljava/lang/Object;)" + result, "([I)" + result, true},
				{"unbox pack", "Unboxing", "([I)" + result, "(Ljava/lang/Integer;)" + result, true},
				{"null retains SAM static type", "Pack", "([" + prefix + "A;)" + result, "(" + prefix + "A;)" + result, true},
				{"array alias uses fixed invocation", "Alias", "([" + prefix + "A;)" + result, "([" + prefix + "A;)" + result, false},
				{"void return discards independently", "Pack", "([" + prefix + "A;)" + result, "(" + prefix + "A;)V", true},
				{"target runtime exception identity", "Throwing", "([" + prefix + "A;)" + result, "(" + prefix + "A;)" + result, true},
			} {
				t.Run(fixture.name, func(t *testing.T) {
					owner := "VariableBindingDriver$" + fixture.owner
					object, err := Parse(bytes.Clone(files[owner+".class"]))
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(object)
					d.options.SourceCompiler, d.options.TargetSourceVersion = profile, 8
					d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, found := files[name+".class"]; return bytes.Clone(raw), found }
					plan := callbinding.SelectVariableArityReference(callbinding.Witness{Owner: owner, Name: "write", Desc: fixture.target, Kind: callbinding.Virtual}, fixture.sam, d.buildInvocationMetadata(), nil)
					if plan.Supported != fixture.want || plan.Supported && plan.Selected.Desc != fixture.target {
						t.Fatalf("original declared overload binding=%+v want packing=%v", plan, fixture.want)
					}
				})
			}
			t.Run("zero tail ambiguity", func(t *testing.T) {
				const control = `import java.util.function.Function;class AmbiguousBinding{static class A{};String write(A first,Object...rest){return "first";}String write(Object first,A...rest){return "second";}Function<A,String> choose(){return value->write(value,new Object[0]);}}`
				// The surrounding declarations are checked with a valid exact
				// invocation before testing only the ambiguous method reference.
				_ = compileLambdaParameterMetadataFixture(t, compiler, profile, control, "AmbiguousBinding", "none", false)
				source := strings.Replace(control, "value->write(value,new Object[0])", "this::write", 1)
				out := t.TempDir()
				file := filepath.Join(out, "AmbiguousBinding.java")
				if err := os.WriteFile(file, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-proc:none", "--release", "8"}
				if profile == NativeJavac8 {
					args = []string{"-proc:none", "-source", "8", "-target", "8"}
				}
				args = append(args, "-d", out, file)
				if log, err := exec.Command(compiler, args...).CombinedOutput(); err == nil {
					t.Fatalf("compiler unexpectedly accepted an ambiguous zero-tail reference\n%s", log)
				}
			})
		})
	}
}

const variableReferenceBindingFixture = `import java.util.function.*;
public class VariableBindingDriver{
 public static class A{}public static final class B extends A{}public static final class C{}
 public static String trace="";
 public static final class Pack{public String write(A...values){if(values.length!=1)throw new AssertionError("pack count");trace="pack";return values[0]==null?"null-item":"pack";}public String write(C...values){return "other";}}
 public static final class Fixed{public String write(A...values){return "pack";}public String write(Object value){return "fixed";}}
 public static final class Boxing{public String write(int...values){return "pack";}public String write(Integer value){return "boxed";}}
 public static final class Widening{public String write(int...values){return "pack";}public String write(long value){return "widened";}}
 public static final class Zero{public String write(A...values){if(values.length!=0)throw new AssertionError("zero");return "narrow";}public String write(Object...values){return "other";}}
 public static final class Two{public String write(A first,A...rest){if(first==null||rest.length!=1||rest[0]==null)throw new AssertionError("fixed and packed ports");return "two";}public String write(Object...values){return "other";}}
 public static final class Covariance{public String write(A[]...values){return "array-pack";}public String write(Object...values){return "fixed-array";}}
 public static final class ArrayPack{public String write(A[]...values){if(values.length!=1||values[0].length!=2)throw new AssertionError("array nested pack");return "array-pack";}}
 public static final class PrimitiveArray{public String write(Object...values){if(values.length!=1||!(values[0]instanceof int[])||((int[])values[0])[0]!=7)throw new AssertionError("primitive array identity");return "primitive-array";}}
 public static final class Unboxing{public String write(int...values){if(values.length!=1||values[0]!=7)throw new AssertionError("unboxed item");return "unboxed";}}
 public static final class Alias{static A[] expected;public String write(A...values){if(values!=expected)throw new AssertionError("fixed array alias replaced");return "alias";}}
 public static final class Throwing{static final RuntimeException failure=new IllegalStateException("original payload");public String write(A...values){throw failure;}}
 static void row(String name,String got){System.out.println(name+"="+got);}
 public static void main(String[]args){A a=new A();B[] array={new B(),new B()};
  Function<A,String> reference=new Pack()::write;row("reference",reference.apply(a));
  Function<A,String> fixed=new Fixed()::write;row("fixed",fixed.apply(a));
  IntFunction<String> boxing=new Boxing()::write;row("boxing",boxing.apply(7));
  IntFunction<String> widening=new Widening()::write;row("widening",widening.apply(7));
  Supplier<String> zero=new Zero()::write;row("zero",zero.get());
  BiFunction<A,A,String> two=new Two()::write;row("two",two.apply(a,new A()));
  Function<B[],String> covariance=new Covariance()::write;row("covariance",covariance.apply(array));
  Function<B[],String> arrayPack=new ArrayPack()::write;row("array-pack",arrayPack.apply(array));
  Function<int[],String> primitiveArray=new PrimitiveArray()::write;row("primitive-array",primitiveArray.apply(new int[]{7}));
  Function<Integer,String> unboxing=new Unboxing()::write;row("unboxing",unboxing.apply(7));
  row("null",reference.apply(null));
  Function<A[],String> alias=new Alias()::write;Alias.expected=new A[]{a};row("alias",alias.apply(Alias.expected));
  Consumer<A> discard=new Pack()::write;trace="";discard.accept(a);row("void",trace);
  Function<A,String> throwing=new Throwing()::write;try{throwing.apply(a);throw new AssertionError("no throw");}catch(RuntimeException failure){if(failure!=Throwing.failure)throw new AssertionError("wrapped target exception");row("throw","same");}
 }
}`
