package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Private constructor bridges and ordinary private method handles are separate
// lookup obligations. An unchanged own-class private method remains callable
// from that same lookup; it grants no foreign or constructor-handle privilege.
func TestAdversarialRootBridgeOwnPrivateReferencesPreserveLookupAndDeferredReceiver(t *testing.T) {
	for _, variant := range []string{"static", "bound instance", "unbound instance", "renamed static"} {
		t.Run(variant, func(t *testing.T) {
			source := nativeRootBridgeHandleSource("both")
			root, driver := "BridgeHandleRoot", "BridgeHandleDriver"
			if variant == "bound instance" || variant == "unbound instance" {
				source = strings.Replace(source, "public static Object echo(Object value)", "private Object echo(Object value)", 1)
				if variant == "bound instance" {
					source = strings.Replace(source, "static java.util.function.Function<Object,Object> reference(){return BridgeHandleRoot::echo;}", "java.util.function.Function<Object,Object> reference(){return this::echo;}", 1)
					source = strings.ReplaceAll(source, "BridgeHandleRoot.reference()", "root.reference()")
				} else {
					source = strings.Replace(source, "static java.util.function.Function<Object,Object> reference(){return BridgeHandleRoot::echo;}", "static java.util.function.BiFunction<BridgeHandleRoot,Object,Object> reference(){return BridgeHandleRoot::echo;}", 1)
					source = strings.ReplaceAll(source, "reference().apply(", "reference().apply(root,")
					// The target body ignores this, but unbound reference entry
					// still throws for a null receiver before invoking that body.
					source = strings.Replace(source, "if(root.value!=value", "try{BridgeHandleRoot.reference().apply(null,token);throw new AssertionError(\"lost deferred null check\");}catch(NullPointerException expected){} if(root.value!=value", 1)
				}
			} else {
				source = strings.Replace(source, "public static Object echo(Object value)", "private static Object echo(Object value)", 1)
			}
			if variant == "renamed static" {
				root, driver = "OwnLookupRoot", "OwnLookupDriver"
				source = strings.NewReplacer("BridgeHandleRoot", root, "BridgeHandleDriver", driver, "echo", "relay", "reference", "select").Replace(source)
			}
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{root}, driver, "root-bridge:unrelated-handles:verified\n", nativeLexicalExactSignatures)
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{root + ".java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{root}, driver, "root-bridge:unrelated-handles:verified\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}
