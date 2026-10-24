package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// The original member parent remains an external binary dependency. Only the
// child is rebuilt, so javac still sees the parent's implicit enclosing port.
// Reflection supplies null as well as distinct enclosing instances: replacing
// an implicit SUPER with qualified-super must not introduce a new null check.
const partialMemberSuperFixture = `class SuffixWitness {static SuffixAncestor.Base escaped;static Object outer;static String trace;static RuntimeException failure;static boolean fail;}
class SuffixAncestor {class Base {final Object observed;final long word;Base(long word){SuffixWitness.trace+="P";SuffixWitness.escaped=this;observed=capture();if(observed!=SuffixWitness.outer||parent()!=SuffixWitness.outer)throw new AssertionError("early enclosing identity");this.word=word;if(SuffixWitness.fail)throw SuffixWitness.failure;}Object capture(){return null;}Object parent(){return SuffixAncestor.this;}}}
class SuffixOwner extends SuffixAncestor {class Member extends Base {long result;Member(long word){super(word);result=word*31L;SuffixWitness.trace+="C";}Object capture(){return SuffixOwner.this;}}}
class SuffixDriver {public static void main(String[]args)throws Exception{int rows=0;
 java.lang.reflect.Constructor<?> ctor=SuffixOwner.Member.class.getDeclaredConstructor(SuffixOwner.class,long.class);ctor.setAccessible(true);
 for(SuffixOwner outer:new SuffixOwner[]{null,new SuffixOwner(),new SuffixOwner()})for(long word:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){
 SuffixWitness.escaped=null;SuffixWitness.outer=outer;SuffixWitness.trace="";SuffixWitness.failure=new IllegalArgumentException("same");SuffixWitness.fail=fail;
 SuffixOwner.Member value=null;Throwable error=null;try{value=(SuffixOwner.Member)ctor.newInstance(outer,word);}catch(java.lang.reflect.InvocationTargetException e){error=e.getCause();}
 long expected=java.math.BigInteger.valueOf(word).multiply(java.math.BigInteger.valueOf(31)).longValue();SuffixAncestor.Base escaped=SuffixWitness.escaped;
 if(error!=(fail?SuffixWitness.failure:null)||escaped==null||escaped.parent()!=outer||escaped.observed!=outer||escaped.word!=word||!SuffixWitness.trace.equals(fail?"P":"PC")||fail&&value!=null||!fail&&(value!=escaped||value.result!=expected))throw new AssertionError("implicit enclosing port, overflow or failure identity",error);rows++;
 }System.out.println(rows+":partial:member:super:nullable:identity:oracle");}}
`

func TestAdversarialPartialMemberSuperclassKeepsImplicitNullableEnclosingPort(t *testing.T) {
	for _, rename := range []bool{false, true} {
		for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
			for _, shape := range []string{"leaf", "effectful"} {
				for _, target := range []int{8, 16} {
					if profile == NativeJavac8 && target != 8 {
						continue
					}
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(fmt.Sprintf("%s/%s/renamed%v/target%d/%s", profile, shape, rename, target, debug), func(t *testing.T) {
							fixture, owner := partialMemberSuperFixture, "SuffixOwner$Member"
							if rename {
								fixture = strings.NewReplacer("SuffixAncestor", "DifferentAncestor", "SuffixOwner", "DifferentScope", "Member", "Leaf").Replace(fixture)
								owner = "DifferentScope$Leaf"
							}
							if shape == "effectful" {
								fixture = "class SuffixOperand {static long read(long n){SuffixWitness.trace+=\"A\";return n;}}\n" + fixture
								fixture = strings.Replace(fixture, "super(word)", "super(SuffixOperand.read(word))", 1)
								fixture = strings.Replace(fixture, `fail?"P":"PC"`, `fail?"AP":"APC"`, 1)
							}
							constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{owner}, "24:partial:member:super:nullable:identity:oracle\n", 8, target, profile)
						})
					}
				}
			}
		}
	}
}
