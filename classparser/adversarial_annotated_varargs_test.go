package javaclassparser

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// The unchanged caller checks the actual declaration tree and calling convention,
// as well as array identity, writes, exceptions and independently computed sums.
// In particular, a variable-arity declaration is not an ordinary array formal.
const annotatedVarargsFixture = `import java.lang.annotation.*;
import java.lang.reflect.*;
import java.util.*;
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.TYPE_USE) @interface ElementMark {String value() default "[]";}
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.TYPE_USE) @interface OuterMark {}
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.TYPE_USE) @interface InnerMark {}
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.PARAMETER) @interface ParameterMark {}
@Retention(RetentionPolicy.CLASS) @Target(ElementType.TYPE_USE) @interface InvisibleMark {}
public class ArtifactLambdaOwner {
 public final String[] stored;
 public ArtifactLambdaOwner(@ParameterMark @ElementMark String @OuterMark ... values){stored=values;}
 public static Object[] reference(@ParameterMark @ElementMark Object... values){return values;}
 public static Object[] distinct(@ParameterMark @ElementMark("parameter") Object @ElementMark("dimension") ... values){return values;}
 public static long primitive(long seed,@ParameterMark @InvisibleMark long @OuterMark ... values){return seed+values[0]*31L+values[1];}
 public static String[][] matrix(@ParameterMark @ElementMark String @OuterMark [] @InnerMark ... values){return values;}
 public static <T> T[] generic(@ParameterMark @ElementMark T @OuterMark ... values){return values;}
 public static List<String>[] parameterized(@ParameterMark List<@ElementMark String> @OuterMark ... values){return values;}
 public static List<String[]>[] nested(@ParameterMark List<@ElementMark("[]") String @InnerMark []> @OuterMark ... values){return values;}
 public static Object[] fixed(@ParameterMark @ElementMark Object @OuterMark [] values){return values;}
 public Object[] virtual(@ParameterMark @ElementMark Object @OuterMark ... values){return values;}
}
class TypedVarargImplementation extends ArtifactLambdaOwner {
 TypedVarargImplementation(String[] seed){super(seed);}
 public Object[] virtual(Object... values){return values;}
}
class ArtifactLambdaDriver {
 static void require(boolean p,String why){if(!p)throw new AssertionError(why);}
 static String marks(Annotation[] annotations){TreeSet<String> names=new TreeSet<String>();for(Annotation a:annotations)names.add(a.annotationType().getSimpleName());return String.join(",",names);}
 static String tree(AnnotatedType t){String a=marks(t.getDeclaredAnnotations());if(t instanceof AnnotatedArrayType)return "A{"+a+"}("+tree(((AnnotatedArrayType)t).getAnnotatedGenericComponentType())+")";if(t instanceof AnnotatedParameterizedType){String s="P{"+a+"}<";for(AnnotatedType x:((AnnotatedParameterizedType)t).getAnnotatedActualTypeArguments())s+=tree(x)+";";return s+">";}return "T{"+a+"}"+t.getType().getTypeName();}
 static void formal(String name,boolean variable,String expected,Class<?>... types)throws Exception{
  Method m=ArtifactLambdaOwner.class.getDeclaredMethod(name,types);require(m.isVarArgs()==variable,"variable arity "+name);
  int i=types.length-1;require(tree(m.getAnnotatedParameterTypes()[i]).equals(expected),"type path "+name+": "+tree(m.getAnnotatedParameterTypes()[i]));
  require(m.getParameterAnnotations()[i].length==1&&m.getParameterAnnotations()[i][0].annotationType()==ParameterMark.class,"declaration annotation "+name);
 }
 public static void main(String[] args)throws Exception{
  formal("reference",true,"A{}(T{ElementMark}java.lang.Object)",Object[].class);
  formal("distinct",true,"A{ElementMark}(T{ElementMark}java.lang.Object)",Object[].class);
  formal("primitive",true,"A{OuterMark}(T{}long)",long.class,long[].class);
  formal("matrix",true,"A{OuterMark}(A{InnerMark}(T{ElementMark}java.lang.String))",String[][].class);
  formal("generic",true,"A{OuterMark}(T{ElementMark}T)",Object[].class);
  formal("parameterized",true,"A{OuterMark}(P{}<T{ElementMark}java.lang.String;>)",List[].class);
  formal("nested",true,"A{OuterMark}(P{}<A{InnerMark}(T{ElementMark}java.lang.String);>)",List[].class);
  formal("fixed",false,"A{OuterMark}(T{ElementMark}java.lang.Object)",Object[].class);
  formal("virtual",true,"A{OuterMark}(T{ElementMark}java.lang.Object)",Object[].class);
  Constructor<?> constructor=ArtifactLambdaOwner.class.getDeclaredConstructor(String[].class);
  require(constructor.isVarArgs()&&tree(constructor.getAnnotatedParameterTypes()[0]).equals("A{OuterMark}(T{ElementMark}java.lang.String)")&&constructor.getParameterAnnotations()[0].length==1,"constructor formal tree");
  String[] seed={"seed"};ArtifactLambdaOwner target=new TypedVarargImplementation(seed);require(target.stored==seed,"constructor array identity");
  int rows=0;long[] edges={Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE};
  for(long a:edges)for(long b:edges)for(long c:edges){
   long[] values={b,c};long actual=ArtifactLambdaOwner.primitive(a,values);
   long expected=java.math.BigInteger.valueOf(a).add(java.math.BigInteger.valueOf(b).multiply(java.math.BigInteger.valueOf(31L))).add(java.math.BigInteger.valueOf(c)).longValue();
   require(actual==expected&&values[0]==b&&values[1]==c,"modular primitive oracle");
   Object identity=new Object();Object[] array={identity};require(ArtifactLambdaOwner.reference(array)==array&&ArtifactLambdaOwner.distinct(array)==array&&ArtifactLambdaOwner.fixed(array)==array&&target.virtual(array)==array,"fixed array ports");
   require(ArtifactLambdaOwner.reference(identity)[0]==identity,"expanded variable call");
   String[][] matrix={{"a"},null};require(ArtifactLambdaOwner.matrix(matrix)==matrix&&ArtifactLambdaOwner.matrix(matrix)[0]==matrix[0],"matrix identity");
   require(ArtifactLambdaOwner.generic(seed)==seed,"generic component and identity");
   List<String>[] lists=new List[]{Arrays.asList("a")};require(ArtifactLambdaOwner.parameterized(lists)==lists,"parameterized array identity");rows++;
   List<String[]>[] nested=new List[]{Collections.singletonList(seed)};require(ArtifactLambdaOwner.nested(nested)==nested&&ArtifactLambdaOwner.nested(nested)[0].get(0)==seed,"nested type argument array identity");
  }
  require(ArtifactLambdaOwner.reference((Object[])null)==null&&ArtifactLambdaOwner.generic((String[])null)==null&&ArtifactLambdaOwner.matrix((String[][])null)==null,"null fixed arity");
  for(int route=0;route<2;route++){Throwable caught=null;try{ArtifactLambdaOwner.primitive(0L,route==0?(long[])null:new long[0]);}catch(Throwable error){caught=error;}require(route==0?caught instanceof NullPointerException:caught instanceof ArrayIndexOutOfBoundsException,"primitive failure boundary");}
  System.out.println(rows+":annotated:varargs:declaration:identity:overflow:null");
 }
}`

func TestAdversarialAnnotatedVariableArityKeepsTypeAndDeclarationAnnotations(t *testing.T) {
	testArtifactLambdaMetadata(t, annotatedVarargsFixture, []bool{false, true}, "125:annotated:varargs:declaration:identity:overflow:null\n", annotatedVariableArityDeclarationsEqual)
}

func TestAdversarialAnnotatedAbstractVariableArityKeepsPhysicalArrayTree(t *testing.T) {
	fixture := strings.NewReplacer(
		"public class ArtifactLambdaOwner {", "public abstract class ArtifactLambdaOwner {",
		"public Object[] virtual(@ParameterMark @ElementMark Object @OuterMark ... values){return values;}", "public abstract Object[] virtual(@ParameterMark @ElementMark Object @OuterMark ... values);",
	).Replace(annotatedVarargsFixture)
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "125:annotated:varargs:declaration:identity:overflow:null\n", annotatedVariableArityDeclarationsEqual)
}

func TestAdversarialAnnotatedVariableArityKeepsOneSharedDeclarationOccurrence(t *testing.T) {
	fixture := strings.NewReplacer(
		"@Target(ElementType.TYPE_USE) @interface ElementMark", "@Target({ElementType.TYPE_USE,ElementType.PARAMETER}) @interface ElementMark",
		"@Target(ElementType.TYPE_USE) @interface InvisibleMark", "@Target({ElementType.TYPE_USE,ElementType.PARAMETER}) @interface InvisibleMark",
		`require(m.getParameterAnnotations()[i].length==1&&m.getParameterAnnotations()[i][0].annotationType()==ParameterMark.class,"declaration annotation "+name);`,
		`String declaration=(name.equals("reference")||name.equals("distinct")||name.equals("matrix")||name.equals("generic")||name.equals("fixed")||name.equals("virtual"))?"ElementMark,ParameterMark":"ParameterMark";require(marks(m.getParameterAnnotations()[i]).equals(declaration),"shared declaration annotation "+name);`,
		"constructor.getParameterAnnotations()[0].length==1", "constructor.getParameterAnnotations()[0].length==2",
	).Replace(annotatedVarargsFixture)
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "125:annotated:varargs:declaration:identity:overflow:null\n", annotatedVariableArityDeclarationsEqual)
}

// Check fresh compiler output before metadata restoration too. Reflection cannot
// see CLASS-retained type annotations, and annotation order differs by compiler.
func annotatedVariableArityDeclarationsEqual(t *testing.T, original, rebuilt []byte) {
	t.Helper()
	shape := func(raw []byte) string {
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var rows []string
		invisible := false
		for _, member := range obj.Methods {
			name, _ := obj.getUtf8(member.NameIndex)
			descriptor, _ := obj.getUtf8(member.DescriptorIndex)
			for _, attribute := range member.Attributes {
				table, ok := attribute.(*TypeAnnotationsAttribute)
				if !ok {
					continue
				}
				for _, annotation := range table.Annotations {
					row, err := json.Marshal(struct {
						Method, Descriptor string
						Invisible          bool
						Annotation         *TypeAnnotation
					}{name, descriptor, table.IsInvisible, annotation})
					if err != nil {
						t.Fatal(err)
					}
					rows = append(rows, string(row))
					invisible = invisible || table.IsInvisible
				}
			}
		}
		if !invisible || len(rows) < 15 {
			t.Fatal("missing original visible/invisible type annotation coverage", len(rows))
		}
		sort.Strings(rows)
		return strings.Join(rows, "\n")
	}
	if want, got := shape(original), shape(rebuilt); want != got {
		t.Fatalf("original visible/invisible formal type paths changed\nwant %s\ngot %s", want, got)
	}
	if nativeParameterAnnotationShape(t, original) != nativeParameterAnnotationShape(t, rebuilt) {
		t.Fatal("original visible/invisible parameter declarations changed")
	}
}

func TestAdversarialAnnotatedVariableArityOracleRejectsDeclarationAndBehaviorMutants(t *testing.T) {
	compiler, java := t04Tools(t)
	for _, mutation := range []struct{ name, before, after, failure string }{
		{"lost arity", "long @OuterMark ... values", "long @OuterMark [] values", "variable arity primitive"},
		{"swapped dimensions", "String @OuterMark [] @InnerMark ...", "String @InnerMark [] @OuterMark ...", "type path matrix"},
		{"lost declaration annotation", "reference(@ParameterMark @ElementMark", "reference(@ElementMark", "declaration annotation reference"},
		{"copied array", "reference(@ParameterMark @ElementMark Object... values){return values;}", "reference(@ParameterMark @ElementMark Object... values){return values==null?null:values.clone();}", "fixed array ports"},
		{"wrong arithmetic", "return seed+values[0]*31L+values[1];", "return seed+values[0]*30L+values[1];", "modular primitive oracle"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			fixture := strings.Replace(annotatedVarargsFixture, mutation.before, mutation.after, 1)
			if fixture == annotatedVarargsFixture {
				t.Fatal("mutant did not change the authored program")
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, fixture, "ArtifactLambdaOwner", "none", true)
			out, err := exec.Command(java, "-Xverify:all", "-cp", sourceArtifactWriteFiles(t, files), "ArtifactLambdaDriver").CombinedOutput()
			if err == nil || !strings.Contains(string(out), "java.lang.AssertionError: "+mutation.failure) || strings.Contains(string(out), "VerifyError") || strings.Contains(string(out), "LinkageError") {
				t.Fatalf("unchanged independent oracle did not catch legal mutant: %v\n%s", err, out)
			}
		})
	}
}
