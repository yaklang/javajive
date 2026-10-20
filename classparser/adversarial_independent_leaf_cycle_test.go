package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func independentLeafCycleFixture() string {
	source := strings.Replace(nativeIndependentLeafChainFixture, "class Leaf<T> extends LeafSupport.Base<T>{", "class Leaf<T> extends LeafSupport.Base<T>{LeafSupport.Base<T> peer;", 1)
	return strings.Replace(source, "class Base<T> extends LeafBase<T>{", "class Base<T> extends LeafBase<T>{LeafNamespace.Leaf<T> peer;", 1)
}

func TestAdversarialIndependentLeafCycleRetainsOriginalCallbacksAndTypedPeers(t *testing.T) {
	source := independentLeafCycleFixture()
	needle := `throw new AssertionError("leaf declaration/early enclosing receiver/word/order");`
	// The two fields share a Java name but have different declaring classes and
	// descriptors. Distinct values make a lost/hijacked binding observable.
	checks := `LeafNamespace.Leaf<Object> other=new LeafCaller<Object>(new Object()).make(~word);value.peer=other;((LeafSupport.Base<Object>)value).peer=value;other.peer=value;((LeafSupport.Base<Object>)other).peer=other;if(value.peer!=other||((LeafSupport.Base<Object>)value).peer!=value||value.peer.peer!=other||((LeafSupport.Base<Object>)value).peer.peer!=other)throw new AssertionError("cyclic typed field binding");`
	if !strings.Contains(source, needle) {
		t.Fatal("original callback oracle missing")
	}
	source = strings.Replace(source, needle, needle+checks, 1)
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("original native8 compiler")
				}
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, source, "LeafNamespace", debug, false)
			}, profile, compiler, []string{"LeafNamespace$Leaf", "LeafSupport$Base", "LeafCaller"}, "LeafDriver", "80:independent:static:leaf:early:receiver\n", nativeIndependentLeafChainInput, nil, []string{"normal", "no-core-cleanups"}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialIndependentLeafCycleReadOrderAndConcurrency(t *testing.T) {
	testIndependentLeafReadOrderAndConcurrency(t, independentLeafCycleFixture())
}
