package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The initializer occurs between original method declarations. Reordering
// declarations may restore compiler ordinals; moving an initialization effect,
// weakening a source receipt, or silently accepting another compiler may not.
const defaultCompilerRegistrationFixture = `
import java.util.*;import java.util.function.*;import java.math.BigInteger;
class DefaultCompilerCases {
 static String join(Object... values){return Arrays.deepToString(values);}
 static BiFunction<String,Integer,String> zReference(){return DefaultCompilerCases::join;}
 static IntUnaryOperator aOrdinary(int n){return value->Integer.rotateLeft(value,n)^n;}
 static final IntUnaryOperator FIELD=value->value^17;
 static IntUnaryOperator middleNested(int n){return value->{IntUnaryOperator inner=argument->(argument^value)+n;return inner.applyAsInt(value*3);};}
}
class DefaultCompilerDriver {
 static final BigInteger MASK=BigInteger.ONE.shiftLeft(32).subtract(BigInteger.ONE);
 static BigInteger word(int n){return BigInteger.valueOf(n).and(MASK);}
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int n:new int[]{Integer.MIN_VALUE,-33,-1,0,1,31,32,Integer.MAX_VALUE})for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   BigInteger bits=word(value);int shift=n&31;
   int rotated=bits.shiftLeft(shift).or(bits.shiftRight((32-shift)&31)).and(MASK).xor(word(n)).intValue();
   int nested=word(word(value).multiply(BigInteger.valueOf(3)).intValue()).xor(word(value)).add(BigInteger.valueOf(n)).intValue();
   if(DefaultCompilerCases.aOrdinary(n).applyAsInt(value)!=rotated||DefaultCompilerCases.middleNested(n).applyAsInt(value)!=nested||DefaultCompilerCases.FIELD.applyAsInt(value)!=bits.xor(BigInteger.valueOf(17)).intValue())throw new AssertionError("word model "+rows);
   rows++;
  }
  for(String text:new String[]{null,"x","λ"})for(Integer count:new Integer[]{null,7}){
   String expected="["+String.valueOf(text)+", "+String.valueOf(count)+"]";
   if(!DefaultCompilerCases.zReference().apply(text,count).equals(expected))throw new AssertionError("varargs binding");rows++;
  }
  List<String> methods=new ArrayList<String>();
  for(java.lang.reflect.Method method:DefaultCompilerCases.class.getDeclaredMethods())if(method.isSynthetic()){
   String signature=method.getName()+":"+method.getModifiers()+":"+method.getReturnType().getName();
   for(Class<?> parameter:method.getParameterTypes())signature+=":"+parameter.getName();methods.add(signature);
  }
  Collections.sort(methods);
  System.out.println(rows+":word:varargs:initializer:nested:"+methods);
 }
}`

func TestAdversarialDefaultCompilerRegistrationMatchesExplicitModern(t *testing.T) {
	assertDefaultCompilerRegistration(t, defaultCompilerRegistrationFixture)
}

func TestAdversarialDefaultCompilerOrdinaryNestedRegistration(t *testing.T) {
	// An independently compiled arrow control isolates class-wide registration
	// from the static varargs-reference implementation-parameter protocol.
	fixture := strings.Replace(defaultCompilerRegistrationFixture, "return DefaultCompilerCases::join;", "return (text,count)->join(text,count);", 1)
	assertDefaultCompilerRegistration(t, fixture)
}

func assertDefaultCompilerRegistration(t *testing.T, fixture string) {
	t.Helper()
	assertDefaultCompilerRegistrationOracle(t, fixture, "46:word:varargs:initializer:nested:")
}

func assertDefaultCompilerRegistrationOracle(t *testing.T, fixture, oraclePrefix string) {
	t.Helper()
	assertDefaultCompilerRegistrationOracleConfigured(t, fixture, oraclePrefix, true)
}

func assertDefaultCompilerRegistrationOracleConfigured(t *testing.T, fixture, oraclePrefix string, parameters bool) {
	t.Helper()
	assertDefaultCompilerRegistrationOracleAtReleases(t, fixture, oraclePrefix, parameters, 8, 8)
}

func assertDefaultCompilerRegistrationOracleAtReleases(t *testing.T, fixture, oraclePrefix string, parameters bool, originalRelease, sourceRelease int) {
	t.Helper()
	compiler, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, compiler, ModernJavac, fixture, "DefaultCompilerCases", debug, nil, originalRelease, parameters)
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "DefaultCompilerDriver")
			if !strings.HasPrefix(oracle, oraclePrefix) {
				t.Fatalf("original independent word/binding oracle: %q", oracle)
			}
			for _, profile := range []SourceCompilerProfile{"", ModernJavac} {
				name := string(profile)
				if name == "" {
					name = "default"
				}
				for _, mode := range []DecompileMode{Precision, Compatibility} {
					t.Run(name+"/"+string(mode), func(t *testing.T) {
						result, err := DecompileWithOptions(files["DefaultCompilerCases.class"], DecompileOptions{Mode: mode, TargetSourceVersion: sourceRelease, SourceCompiler: profile, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, found := files[name+".class"]; return raw, found }})
						if err != nil || result.Status != "complete" || len(result.StubMethods) != 0 {
							t.Fatalf("original source scope did not close: %s %v %v", result.Status, err, result.Diagnostics)
						}
						if result.EffectiveConfig == nil || result.EffectiveConfig.SourceCompiler != ModernJavac {
							t.Fatal("default profile did not report its actual compiler domain", result.EffectiveConfig)
						}
						out := t.TempDir()
						path := filepath.Join(out, "DefaultCompilerCases.java")
						if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
							t.Fatal(err)
						}
						compileRelease := sourceRelease
						if compileRelease == 0 {
							compileRelease = originalRelease
						}
						args := []string{"-proc:none", "--release", strconv.Itoa(compileRelease), "-g:" + debug, "-cp", original, "-d", out, path}
						if parameters {
							args = append(args, "-parameters")
						}
						if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
							t.Fatalf("candidate compile: %v\n%s\n%s", err, log, result.Source)
						}
						// Execute the unchanged binary driver and its original top-level
						// SAM declarations, with only the consumer replaced. This harness
						// does not accept an original owned class as a missing candidate.
						for name, raw := range files {
							if name == "DefaultCompilerCases.class" {
								continue
							}
							if strings.HasPrefix(name, "DefaultCompilerCases$") {
								t.Fatal("owned class needs a family round-trip harness", name)
							}
							if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						if got := t04RunJava(t, java, out, "DefaultCompilerDriver"); got != oracle {
							t.Fatalf("unchanged binary driver binding/implementation identities:\noriginal %s\ncandidate %s\n%s", oracle, got, result.Source)
						}
					})
				}
			}
		})
	}
}
