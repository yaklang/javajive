package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

const anonymousCapturedConstructorFixture = `interface SuffixTask {Object token();long word();}
class SuffixOwner {static SuffixTask create(final Object token,final long word){return new SuffixTask(){public Object token(){return token;}public long word(){return word;}};}}
class SuffixDriver {public static void main(String[]args)throws Exception{Object id=new Object();int rows=0;for(Object token:new Object[]{null,id,"text"})for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){SuffixTask task=SuffixOwner.create(token,word);if(task.token()!=token||task.word()!=word)throw new AssertionError("capture identity and category-two slot");java.lang.reflect.Constructor<?>[] cs=task.getClass().getDeclaredConstructors();if(cs.length!=1||cs[0].getParameterTypes().length!=PHYSICAL||cs[0].getGenericParameterTypes().length!=GENERIC)throw new AssertionError("declared versus physical constructor ports");rows++;}System.out.println(rows+":anonymous:constructor:physical:declared");}}`

func TestAdversarialAnonymousConstructorSignaturePreservesDeclaredPorts(t *testing.T) {
	for _, original := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, member := range []bool{false, true} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%v/member%v/%s", original, member, debug), func(t *testing.T) {
					source := anonymousCapturedConstructorFixture
					ports := 2
					if member {
						source = strings.Replace(source, "static SuffixTask create", "SuffixTask create", 1)
						source = strings.Replace(source, "SuffixOwner.create(token,word)", "new SuffixOwner().create(token,word)", 1)
						ports++
					}
					generic := ports
					if original == ModernJavac {
						generic = 0
					}
					source = strings.ReplaceAll(strings.ReplaceAll(source, "PHYSICAL", fmt.Sprint(ports)), "GENERIC", fmt.Sprint(generic))
					t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
					constructorSuffixFamilyWithOriginalCompilerRoundTrip(t, source, debug, []string{"SuffixOwner$1"}, "15:anonymous:constructor:physical:declared\n", 8, 8, ModernJavac, original)
				})
			}
		}
	}
}
