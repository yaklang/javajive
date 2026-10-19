package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Named compiler-generated enum parameters are part of the original physical
// declarations. A valid enum must not flatten its entire enclosing family just
// because values/valueOf/the hidden constructor have parameter tables.
const memberEnumParameterMetadataFixture = `
public class EnumParameterOwner {
 static class Scope {
  private enum Choice { FIRST,SECOND; }
  static Object select(int selector,final Object first,Object second){return Choice.values()[selector&1]==Choice.FIRST?first:second;}
 }
 public static Object select(int selector,final Object first,Object second){return Scope.select(selector,first,second);}
}
class EnumParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;Object shared=new Object(),other=new Object();Object[] values={null,shared,other};
  for(int selector:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object first:values)for(Object second:values){
   Object actual=EnumParameterOwner.select(selector,first,second);Object expected=(selector&1)==0?first:second;
   if(actual!=expected)throw new AssertionError("enum selection/identity/overflow/null");rows++;
  }
  Class<?>[] outer=EnumParameterOwner.class.getDeclaredClasses();if(outer.length!=1||outer[0]!=EnumParameterOwner.Scope.class||outer[0].getDeclaringClass()!=EnumParameterOwner.class)throw new AssertionError("outer original member declaration");
  Class<?>[] inner=outer[0].getDeclaredClasses();if(inner.length!=1||!inner[0].isEnum()||inner[0].getDeclaringClass()!=outer[0]||inner[0].getEnclosingClass()!=outer[0]||!java.lang.reflect.Modifier.isPrivate(inner[0].getModifiers()))throw new AssertionError("nested enum original owner/privacy");
  Object[] constants=inner[0].getEnumConstants();if(constants.length!=2||!((Enum)constants[0]).name().equals("FIRST")||((Enum)constants[1]).ordinal()!=1)throw new AssertionError("constant order");
  System.out.println(rows+":enum:named-parameters:owners:identity");
 }
}`

func TestAdversarialNamedEnumParametersKeepOriginalNestedDeclarationFamily(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for original enum lowering oracle")
				}
			}
			for _, owner := range []string{"EnumParameterOwner", "RenamedParameterFamily"} {
				t.Run(owner, func(t *testing.T) {
					source := strings.ReplaceAll(memberEnumParameterMetadataFixture, "EnumParameterOwner", owner)
					testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, source, owner, debug, true)
					}, profile, compiler, []string{owner}, "EnumParameterDriver", "45:enum:named-parameters:owners:identity\n", nil, func(t *testing.T, files map[string][]byte) *JarFS {
						return nativeEnumOwnUnitArchive(t, files, profile)
					}, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, true, nativeLexicalExactSignatures, func(t *testing.T, name string, original, rebuilt []byte) {
						t.Helper()
						if a, b := lambdaParameterMetadataTables(t, original), lambdaParameterMetadataTables(t, rebuilt); a != b {
							t.Fatalf("original physical enum parameter metadata changed for %s:\n%s\n%s", name, a, b)
						}
						if a, b := constructorInvocationThrowsDeclarations(t, original), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
							t.Fatalf("original complete enum declaration inventory changed for %s:\n%s\n%s", name, a, b)
						}
					})
				})
			}
		})
	}
}
