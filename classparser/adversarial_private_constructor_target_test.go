package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// A caller retained from the original JAR still invokes the private-access
// constructor descriptor even when the new source compiler supports nests.
// Source target selection must not silently remove that binary entry point.
func TestAdversarialPrivateConstructorPortsAcrossPhysicalTargets(t *testing.T) {
	for _, target := range []int{8, 9, 10, 11, 16, 21} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			fixture := strings.Replace(externalOwnerBoundFixture("Number"), "Member(){}", "private Member(){}", 1)
			constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, "none", []string{"SuffixOwner$Member"}, "4:external:bound:erasure:identity\n", 8, target, ModernJavac)
		})
	}
}

func TestAdversarialPrivateConstructorPortsAcrossOwnedFamilyTargets(t *testing.T) {
	const fixture = `class SuffixOwner {
 static int calls;
 interface Op {int apply(int value);}
 private static final class Node implements Op {
  private Node(){calls++;}
  public int apply(int value){return value*31+7;}
 }
 static Op make(){return new Node();}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 Class<?> type=Class.forName("SuffixOwner$Node");
 if(type.getDeclaredConstructors().length!=2||!java.lang.reflect.Modifier.isPrivate(type.getDeclaredConstructor().getModifiers()))throw new AssertionError("original constructor inventory");
 int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  SuffixOwner.Op a=SuffixOwner.make(),b=SuffixOwner.make();
  if(a==b||a.apply(n)!=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).intValue())throw new AssertionError("constructor identity and arithmetic");rows++;
 }if(SuffixOwner.calls!=10)throw new AssertionError("constructor effects");System.out.println(rows+":ports:identity:arithmetic:effects");}}
`
	for _, target := range []int{8, 11, 16, 21} {
		for _, marker := range []string{"empty", "static anonymous", "capturing anonymous", "switch table"} {
			for _, root := range []string{"SuffixOwner", "RenamedPortOwner"} {
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(fmt.Sprintf("%d/%s/%s/%s", target, marker, root, debug), func(t *testing.T) {
						source := fixture
						owners := []string{root, root + "$Node", root + "$Op", root + "$1"}
						if marker == "static anonymous" {
							source = strings.Replace(source, "static int calls;", "static int calls; static Object token=new Object(){public int hashCode(){return 31;}};", 1)
							source = strings.Replace(source, "int rows=0;", "if(SuffixOwner.token.hashCode()!=31)throw new AssertionError(\"anonymous behavior\");int rows=0;", 1)
						} else if marker == "capturing anonymous" {
							source = strings.Replace(source, "static int calls;", "static int calls; Object token(final int seed){return new Object(){public int hashCode(){return seed+calls;}};}", 1)
							source = strings.Replace(source, "int rows=0;", "Object token=new SuffixOwner().token(31);if(token.hashCode()!=31)throw new AssertionError(\"anonymous capture\");int rows=0;", 1)
							source = strings.Replace(source, "if(SuffixOwner.calls!=10)", "if(token.hashCode()!=41)throw new AssertionError(\"captured observation\");if(SuffixOwner.calls!=10)", 1)
						} else if marker == "switch table" {
							source = strings.Replace(source, "interface Op", "enum Choice{LEFT,RIGHT} static int select(Choice choice){switch(choice){case LEFT:return 3;case RIGHT:return 7;default:throw new AssertionError();}} interface Op", 1)
							source = strings.Replace(source, "int rows=0;", "if(SuffixOwner.select(SuffixOwner.Choice.LEFT)!=3||SuffixOwner.select(SuffixOwner.Choice.RIGHT)!=7)throw new AssertionError(\"switch marker behavior\");int rows=0;", 1)
							owners = append(owners, root+"$Choice")
						}
						source = strings.ReplaceAll(source, "SuffixOwner", root)
						constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, source, debug, owners, "5:ports:identity:arithmetic:effects\n", 8, target, ModernJavac)
					})
				}
			}
		}
	}
}
