package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func ownedInterfaceLambdaFixture() string {
	source := instanceLambdaParameterFixture
	start := strings.Index(source, "public abstract class UncapturedLambdaNames")
	end := strings.Index(source, "class InstanceNode")
	source = source[:start] + `public class UncapturedLambdaNames{
 public interface Gate{
  boolean matches(int sample);
  default Gate and(Gate next){return(int sample)->this.matches(sample)&&next.matches(sample);}
 }
 public static Gate mix(Gate left,Gate right){return left.and(right);}
}
` + source[end:]
	source = strings.ReplaceAll(source, "extends UncapturedLambdaNames", "implements UncapturedLambdaNames.Gate")
	source = strings.ReplaceAll(source, "UncapturedLambdaNames.class", "UncapturedLambdaNames.Gate.class")
	source = strings.ReplaceAll(source, "IntPredicate operation", "UncapturedLambdaNames.Gate operation")
	source = strings.ReplaceAll(source, "operation.test(sample)", "operation.matches(sample)")
	source = strings.ReplaceAll(source, "operation=receiver.and(next)", "operation=UncapturedLambdaNames.mix(receiver,next)")
	source = strings.Replace(source, "  Method implementation=null;", `  Class<?> gate=UncapturedLambdaNames.Gate.class;
  if(!gate.isInterface()||!gate.isMemberClass()||gate.isLocalClass()||gate.isAnonymousClass()||gate.isSynthetic()||gate.getModifiers()!=1545||gate.getDeclaringClass()!=UncapturedLambdaNames.class||gate.getEnclosingClass()!=UncapturedLambdaNames.class||!gate.getSimpleName().equals("Gate")||!gate.getName().equals(UncapturedLambdaNames.class.getName()+"$Gate")||gate.getClassLoader()!=UncapturedLambdaNames.class.getClassLoader()||!Arrays.equals(UncapturedLambdaNames.class.getDeclaredClasses(),new Class<?>[]{gate}))throw new AssertionError("original owning member declaration");
  Method implementation=null;`, 1)
	// The Java caller keeps its independent behavior, identity, short-circuit
	// and direct-helper oracle. Go compares every owned declaration/MP table,
	// including private implementations, so no compiler-dependent hash is a
	// fixed expected-output string for this family fixture.
	at := strings.Index(source, "  List<String> declarations=")
	return source[:at] + "  if(rows!=300)throw new AssertionError(\"world conservation\");System.out.println(\"300:owned-interface:receiver:capture:order\");\n }\n}"
}

func TestAdversarialOwnedInterfaceLambdaKeepsReceiverMetadataAndSourceOwnership(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for the original member-interface oracle")
				}
			}
			for _, owner := range []string{"UncapturedLambdaNames", "DifferentGateContainer"} {
				t.Run(owner, func(t *testing.T) {
					fixture := strings.ReplaceAll(ownedInterfaceLambdaFixture(), "UncapturedLambdaNames", owner)
					for _, named := range []bool{false, true} {
						name := "without-parameters"
						if named {
							name = "with-parameters"
						}
						t.Run(name, func(t *testing.T) {
							compile := func(debug string) map[string][]byte {
								return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, owner, debug, named)
							}
							verify := func(t *testing.T, name string, original, rebuilt []byte) {
								t.Helper()
								nativeLexicalExactSignatures(t, name, original, rebuilt)
								if a, b := lambdaParameterMetadataTables(t, original), lambdaParameterMetadataTables(t, rebuilt); a != b {
									t.Fatalf("owned %s complete physical parameter metadata changed:\n%s\n%s", name, a, b)
								}
								if a, b := constructorInvocationThrowsDeclarations(t, original), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
									t.Fatalf("owned %s complete declaration inventory changed:\n%s\n%s", name, a, b)
								}
								if a, b := arrayProducerCheckedInvocations(t, original), arrayProducerCheckedInvocations(t, rebuilt); a != b {
									t.Fatalf("owned %s original checked invocations changed:\n%s\n%s", name, a, b)
								}
							}
							testNativeIndependentCompilerFamilyWithParameterFlag(t, compile, profile, compiler, []string{owner}, "LambdaParameterDriver", "300:owned-interface:receiver:capture:order\n", nil, nil, []string{"normal", "no-source-rewrites"}, named, verify)
						})
					}
				})
			}
		})
	}
}
