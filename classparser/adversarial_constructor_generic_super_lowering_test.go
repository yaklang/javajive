package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// JVM invocation descriptors erase T to Object, whereas the superclass use
// fixes T=String. Keeping an Object cast preserves the erased descriptor but
// makes the reconstructed Java illegal. The unchanged original driver still
// observes every pre-SUPER capture and abrupt constructor exit.
func TestAdversarialConstructorLoweringKeepsInstantiatedGenericSuperBinding(t *testing.T) {
	modern, java := t04Tools(t)
	source := strings.Replace(constructorLoweringFixture, "abstract class LoweringParent {", "abstract class LoweringParent<T> {", 1)
	source = strings.Replace(source, "LoweringParent(Object input,int mode)", "LoweringParent(T input,int mode)", 1)
	source = strings.Replace(source, "extends LoweringParent {", "extends LoweringParent<String> {", 1)
	source = strings.ReplaceAll(source, "Object input", "String input")
	source = strings.Replace(source, "String input=mode==0?new Object():null;", "String input=mode==0?\"input\":null;", 1)
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(string(profile)+"/"+debug, func(t *testing.T) {
				compiler := modern
				if profile == NativeJavac8 {
					compiler = os.Getenv("JAVA8_JAVAC")
					if compiler == "" {
						t.Fatal("native original compiler required")
					}
				}
				testConstructorLoweringSourceRoundTrip(t, compiler, java, profile, debug, source)
			})
		}
	}
}
