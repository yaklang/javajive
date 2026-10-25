package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialAnonymousConstructorCompletionComposesWithGenericScope(t *testing.T) {
	t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := strings.NewReplacer(
		"interface SuffixTask {Object token();", "interface SuffixTask<X> {X token();",
		"class SuffixOwner {", "class SuffixOwner<X> {",
		"static SuffixTask create(final Object token", "SuffixTask<X> create(final X token",
		"SuffixOwner.create(token,word)", "new SuffixOwner<Object>().create(token,word)",
		"new SuffixTask(){public Object token()", "new SuffixTask<X>(){public X token()",
		"PHYSICAL", "3", "||cs[0].getGenericParameterTypes().length!=GENERIC", "").Replace(anonymousCapturedConstructorFixture)
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("%v/%s", profile, debug), func(t *testing.T) {
				constructorSuffixFamilyWithOriginalCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner$1", "SuffixOwner", "SuffixTask"}, "15:anonymous:constructor:physical:declared\n", 8, 8, ModernJavac, profile)
			})
		}
	}
}

func TestAdversarialAnonymousConstructorCompletionComposesWithAssertionStorage(t *testing.T) {
	t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
	fixture := strings.NewReplacer(
		"public long word(){return word;}", "public long word(){assert word>=0:token;return word;}",
		"if(task.token()!=token||task.word()!=word)throw new AssertionError(\"capture identity and category-two slot\");", "if(task.token()!=token)throw new AssertionError(\"capture identity\");Throwable thrown=null;long actual=0;try{actual=task.word();}catch(Throwable e){thrown=e;}boolean failed=SuffixOwner.class.desiredAssertionStatus()&&word<0;if((thrown instanceof AssertionError)!=failed||thrown!=null&&!(thrown instanceof AssertionError)||!failed&&actual!=word)throw new AssertionError(\"assertion owner / word\",thrown);if(failed&&!String.valueOf(token).equals(thrown.getMessage()))throw new AssertionError(\"assertion detail\");",
		"PHYSICAL", "2", "||cs[0].getGenericParameterTypes().length!=GENERIC", "").Replace(anonymousCapturedConstructorFixture)
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("%v/%s", profile, debug), func(t *testing.T) {
				constructorSuffixFamilyWithOriginalCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner$1"}, "15:anonymous:constructor:physical:declared\n", 8, 8, ModernJavac, profile)
			})
		}
	}
}
