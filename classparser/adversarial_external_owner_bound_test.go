package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Declaration resolution supplies lexical type bindings without enrolling an
// external enclosing class in the reconstruction. The untouched factory calls
// the original erased value() descriptor, and reflection checks its binder.
func externalOwnerBoundFixture(bound string) string {
	values := `null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")`
	if bound == "CharSequence" {
		values = `null,"identity",new StringBuilder("mutable"),new StringBuffer("separate")`
	}
	return fmt.Sprintf(`class SuffixOwner<T extends %s> {
 final T seed; SuffixOwner(T seed){this.seed=seed;}
 class Member {Member(){} T value(){return SuffixOwner.this.seed;}}
 Member make(){return new Member();}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 Class<?> member=SuffixOwner.Member.class;
 java.lang.reflect.Method value=member.getDeclaredMethod("value");
 if(value.getReturnType()!=%s.class||!value.getGenericReturnType().equals(SuffixOwner.class.getTypeParameters()[0])||member.getTypeParameters().length!=0)throw new AssertionError("original erased ABI and binder");
 int rows=0;
 for(%s n:new %s[]{%s}) {
  SuffixOwner<%s> owner=new SuffixOwner<%s>(n);
  SuffixOwner<%s>.Member first=owner.make(),second=owner.make();
  if(first==second||first.value()!=n||second.value()!=n||value.invoke(first)!=n)throw new AssertionError("original call descriptor and reference identity");rows++;
 }
 System.out.println(rows+":external:bound:erasure:identity");
}}`, bound, bound, bound, bound, values, bound, bound, bound)
}

func TestAdversarialIndependentMemberRetainsExternalOwnerBound(t *testing.T) {
	for _, bound := range []string{"Number", "CharSequence"} {
		for rootIndex, root := range []string{"SuffixOwner", "RenamedBoundScope", "泛型𐐀Owner"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				for _, compiler := range []struct {
					profile SourceCompilerProfile
					target  int
				}{{ModernJavac, 8}, {ModernJavac, 11}, {NativeJavac8, 8}} {
					// Keep the classpath root ASCII for JDK URLClassPath decoding;
					// the actual Java declaration/file names retain non-BMP text.
					t.Run(fmt.Sprintf("%s/scope%d/%s/%v-%d", bound, rootIndex, debug, compiler.profile, compiler.target), func(t *testing.T) {
						constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(externalOwnerBoundFixture(bound), "SuffixOwner", root), debug, []string{root + "$Member"}, "4:external:bound:erasure:identity\n", 8, compiler.target, compiler.profile)
					})
				}
			}
		}
	}
}
