package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// These are independent additions to the monitor-boundary MVP: numeric
// negation cannot throw, whereas factory creation is eager and may throw even
// though invoking the callback is deferred. Both must retain their own domain.
func TestAdversarialMonitorFactoryAndNegationKeepOriginalDomains(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"BoundaryOwner", "OtherFactoryOwner"} {
			for _, shape := range []string{"negation", "bound reference", "lambda"} {
				t.Run(compiler.name+"/"+owner+"/"+shape, func(t *testing.T) {
					source := strings.ReplaceAll(monitorCleanupBoundaryFixture, "BoundaryOwner", owner)
					if shape == "negation" {
						source = strings.Replace(source, "synchronized(this){answer=step(1,answer,faults);}", "if(answer<0)answer=-answer;synchronized(this){answer=step(1,answer,faults);}", 1)
						source = strings.Replace(source, "word=first^0x5a5a5a5aL", "word=((((first&0x80000000L)!=0?-first:first)&0xffffffffL)^0x5a5a5a5aL)", 1)
					} else {
						source = strings.Replace(source, "final Object outer=", `java.util.function.IntUnaryOperator callback;
 int captureCount;
 `+owner+` capture(){if(!Thread.holdsLock(outer)||!Thread.holdsLock(inner)||!Thread.holdsLock(this))throw new AssertionError("eager capture escaped held domain");captureCount++;return this;}
 int bounce(int value){if(!Thread.holdsLock(outer)||Thread.holdsLock(inner)||Thread.holdsLock(this)||captureCount!=1||callback==null)throw new AssertionError("deferred callback domain");callback=null;return value;}
 final Object outer=`, 1)
						factory := "capture()::bounce"
						if shape == "lambda" {
							factory = "(value)->bounce(value)"
						}
						assignment := "callback=" + factory + ";"
						if shape == "lambda" {
							assignment = "capture();" + assignment
						}
						source = strings.Replace(source, "answer=step(1,answer,faults);}", "answer=step(1,answer,faults);"+assignment+"}", 1)
						source = strings.Replace(source, "return step(4,answer,faults);", "return step(4,callback.applyAsInt(answer),faults);", 1)
					}
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{owner}, "BoundaryDriver", "640:monitor-domains:nested-finally:failure-overrides:values\n", nil, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures)
				})
			}
		}
	}
}
