package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Older javac anonymous constructors may have no Exceptions attribute even
// though their direct SUPER declares a checked exception. The independent
// original factory and reflective observer must retain that physical ABI.
func TestAdversarialConstructorLoweringCheckedSuperRetainsOriginalDeclaration(t *testing.T) {
	for _, release := range []int{4, 8} {
		for _, shape := range []string{"instance", "static"} {
			for _, parent := range []string{"ordinary", "custom renamed", "ordered throws and overload"} {
				for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(fmt.Sprintf("release%d/%s/%s/%s/%s", release, shape, parent, profile, debug), func(t *testing.T) {
							fixture := constructorCheckedSuperFixture
							if shape == "static" {
								fixture = strings.Replace(fixture, "SuffixBase make(", "static SuffixBase make(", 1)
							}
							switch parent {
							case "custom renamed":
								fixture = strings.ReplaceAll(fixture, "java.io.IOException", "ObservedFailure")
								fixture = "class ObservedFailure extends java.io.IOException {ObservedFailure(String text){super(text);}}\n" + fixture
								fixture = strings.ReplaceAll(strings.ReplaceAll(fixture, "SuffixBase", "AlternateBase"), "SuffixWitness", "InitializationTrace")
							case "ordered throws and overload":
								fixture = strings.Replace(fixture, "SuffixBase(Object input)throws java.io.IOException", "SuffixBase(Object input)throws java.io.EOFException,java.io.IOException,java.sql.SQLException,java.lang.IllegalStateException", 1)
								fixture = strings.Replace(fixture, "abstract Object value();", "SuffixBase(boolean unused)throws java.lang.ReflectiveOperationException{} abstract Object value();", 1)
								fixture = strings.Replace(fixture, "final long seed)throws java.io.IOException", "final long seed)throws java.io.IOException,java.sql.SQLException", 1)
							}
							constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner$1"}, "24:checked:super:identity:early:capture:metadata\n", release, 8, profile)
						})
					}
				}
			}
		}
	}
}

const constructorCheckedSuperFixture = `
class SuffixWitness {
 static Object token;static long seed;static SuffixBase escaped;static int visits;
 static java.io.IOException failure;
}
abstract class SuffixBase {
 SuffixBase(Object input)throws java.io.IOException{
  SuffixWitness.escaped=this;
  if(value()!=SuffixWitness.token||seed()!=SuffixWitness.seed)throw new AssertionError("early capture");
  SuffixWitness.visits++;
  if(input==null)throw SuffixWitness.failure;
 }
 abstract Object value();abstract long seed();
}
class SuffixOwner {
 SuffixBase make(Object input,final Object token,final long seed)throws java.io.IOException{
  return new SuffixBase(input){Object value(){return token;}long seed(){return seed;}};
 }
}
class SuffixDriver {
 public static void main(String[]args)throws Exception{
  java.lang.reflect.Constructor ctor=Class.forName("SuffixOwner$1").getDeclaredConstructors()[0];
  if(ctor.getExceptionTypes().length!=0)throw new AssertionError("original anonymous constructor Exceptions absence");
  Object[] tokens={null,"token",new Object()};long[] seeds={Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE};
  int rows=0;SuffixOwner owner=new SuffixOwner();
  for(int t=0;t<tokens.length;t++)for(int s=0;s<seeds.length;s++)for(int f=0;f<2;f++){
   Object token=tokens[t];long seed=seeds[s];boolean fail=f!=0;
   SuffixWitness.token=token;SuffixWitness.seed=seed;SuffixWitness.escaped=null;SuffixWitness.visits=0;
   SuffixWitness.failure=new java.io.IOException("same instance");
   Throwable error=null;SuffixBase actual=null;
   try{actual=owner.make(fail?null:new Object(),token,seed);}catch(Throwable e){error=e;}
   if(error!=(fail?SuffixWitness.failure:null)||SuffixWitness.visits!=1||SuffixWitness.escaped==null||SuffixWitness.escaped.value()!=token||SuffixWitness.escaped.seed()!=seed||!fail&&actual!=SuffixWitness.escaped)throw new AssertionError("checked SUPER effects/identity/capture");
   rows++;
  }
  System.out.println(rows+":checked:super:identity:early:capture:metadata");
 }
}
`
