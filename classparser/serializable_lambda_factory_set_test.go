package javaclassparser

import (
	"bytes"
	"strings"
	"testing"
)

func TestNativeSerializableHelperRequiresAllActualFactoryTargetAncestries(t *testing.T) {
	fixture := strings.Replace(serializableStaticReferenceFixture, "class SerializableReferences {", `interface OtherPort extends java.util.function.Function<String,String>{}
class SerializableReferences {
 static OtherPort ordinary(){return ExternalOperations::BB;}
`, 1) + `class FactoryGraphDriver{public static void main(String[]args)throws Exception{OtherPort f=SerializableReferences.ordinary();if(!"x:B".equals(f.apply("x"))||f instanceof java.io.Serializable)throw new AssertionError("ordinary original factory");Object explicit=SerializableReferences.first();SerializationDriver.form(explicit);System.out.println("original:ordinary-and-serializable");}}`
	files := nativeCompileDebugClasses(t, fixture, "none")
	_, java := t04Tools(t)
	if out := t04RunJava(t, java, adversarialAssertionClasses(t, files), "FactoryGraphDriver"); out != "original:ordinary-and-serializable\n" {
		t.Fatal("valid original factory-set oracle", out)
	}
	for _, visibility := range []string{"complete", "unknown ordinary interface", "different ordinary declaration"} {
		t.Run(visibility, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["SerializableReferences.class"]))
			if err != nil {
				t.Fatal(err)
			}
			plan := buildSerializableReferenceProtocol(object, nil)
			if plan == nil {
				t.Fatal("valid original explicit serialization automaton")
			}
			dumper := NewClassObjectDumper(object)
			dumper.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "OtherPort" {
					if visibility == "unknown ordinary interface" {
						return nil, false
					}
					if visibility == "different ordinary declaration" {
						return bytes.Clone(files["ExternalOperations.class"]), true
					}
				}
				data, known := files[name+".class"]
				return bytes.Clone(data), known
			}
			if closed := dumper.serializableOtherFactoriesClosed(plan); closed != (visibility == "complete") {
				t.Fatal("helper borrowed an unknown/unrelated factory ancestry", visibility, closed)
			}
		})
	}
}

func TestNativeSerializableStaticTargetRequiresActualExternalDeclaringScope(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class Plain$Operations{static String accept(String x){return x+"!";}} class InheritedOperations extends Plain$Operations{} class ScopeContainer{public static class NestedOperations{public static String accept(String x){return x+"!";}}} class CapturingScope{} class ScopeDriver{public static void main(String[]args){if(!"x!".equals(Plain$Operations.accept("x"))||!"x!".equals(InheritedOperations.accept("x"))||!"x!".equals(ScopeContainer.NestedOperations.accept("x")))throw new AssertionError("original declaration identities");System.out.println("original:declaring-scopes");}}`, "none")
	_, java := t04Tools(t)
	if out := t04RunJava(t, java, adversarialAssertionClasses(t, files), "ScopeDriver"); out != "original:declaring-scopes\n" {
		t.Fatal("original access/scope oracle", out)
	}
	for _, owner := range []string{"Plain$Operations", "InheritedOperations", "ScopeContainer$NestedOperations"} {
		t.Run(owner, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["CapturingScope.class"]))
			if err != nil {
				t.Fatal(err)
			}
			dumper := NewClassObjectDumper(object)
			dumper.foldSiblingResolver = func(name string) ([]byte, bool) { data, known := files[name+".class"]; return bytes.Clone(data), known }
			target := nativeLambdaHandleTarget{owner: owner, name: "accept", descriptor: "(Ljava/lang/String;)Ljava/lang/String;"}
			if known := dumper.serializableReferenceOriginalTarget(target); known != (owner == "Plain$Operations") {
				t.Fatal("member/scope source permission was inferred from a name", owner, known)
			}
		})
	}
}
