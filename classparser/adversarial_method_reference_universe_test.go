package javaclassparser

import (
	"strings"
	"testing"
)

const methodReferenceUniverseFixture = `
import java.util.*;import java.util.function.*;
class SuffixList extends ArrayList<Object> {
 int indices,objects;boolean fail;final RuntimeException error=new IllegalStateException("identity");
 public Object remove(int n){indices++;if(fail)throw error;return super.remove(n);}
 public boolean remove(Object o){objects++;throw new AssertionError("wrong overload");}}
public class SuffixOwner {
 public static ToIntFunction<Integer> number(){return Integer::intValue;}
 public static IntConsumer erase(SuffixList input){return input::remove;}
 public static void pipeline(ArrayList<Integer> indices,SuffixList values){indices.stream().sorted(Comparator.reverseOrder()).mapToInt(Integer::intValue).forEach(values::remove);}}
class SuffixDriver {public static void main(String[]args){
 if(SuffixOwner.class.getDeclaredMethods().length!=3)throw new AssertionError("original declaration universe");
 int rows=0;ToIntFunction<Integer> number=SuffixOwner.number();
 for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){if(number.applyAsInt(n)!=n)throw new AssertionError("unboxing");rows++;}
 try{number.applyAsInt(null);throw new AssertionError("null argument");}catch(NullPointerException expected){rows++;}
 try{((ToIntFunction)number).applyAsInt("bad");throw new AssertionError("raw input");}catch(ClassCastException expected){rows++;}
 try{SuffixOwner.erase(null);throw new AssertionError("eager bound null");}catch(NullPointerException expected){rows++;}
 SuffixList values=new SuffixList();values.addAll(Arrays.asList("a","b","c"));IntConsumer remove=SuffixOwner.erase(values);
 remove.accept(1);if(!values.equals(Arrays.asList("a","c"))||values.indices!=1||values.objects!=0)throw new AssertionError("primitive overload");rows++;
 try{remove.accept(99);throw new AssertionError("deferred bounds");}catch(IndexOutOfBoundsException expected){if(values.indices!=2||values.objects!=0)throw new AssertionError("bounds effects");rows++;}
 values.fail=true;try{remove.accept(0);throw new AssertionError("failure identity");}catch(RuntimeException error){if(error!=values.error||values.indices!=3)throw new AssertionError("wrong failure",error);rows++;}
 values=new SuffixList();values.addAll(Arrays.asList("a","b","c","d","e"));SuffixOwner.pipeline(new ArrayList<Integer>(Arrays.asList(1,3)),values);
 if(!values.equals(Arrays.asList("a","c","e"))||values.indices!=2||values.objects!=0)throw new AssertionError("ordered pipeline");rows++;
 try{SuffixOwner.pipeline(new ArrayList(Arrays.asList("bad")),values);throw new AssertionError("pipeline raw argument");}catch(ClassCastException expected){if(values.indices!=2||values.objects!=0)throw new AssertionError("raw adapter ordering");rows++;}
 System.out.println(rows+":method-reference:overload:identity:eager:raw:universe");}}
`

func TestAdversarialMethodReferencesPreserveOriginalDeclarationUniverse(t *testing.T) {
	for _, pkg := range []string{"ordinary.references", "org.mockito.references"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(pkg+"/"+debug, func(t *testing.T) {
				fixture := "package " + pkg + ";\n" + methodReferenceUniverseFixture
				owner := strings.ReplaceAll(pkg, ".", "/") + "/SuffixOwner"
				constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{owner}, "13:method-reference:overload:identity:eager:raw:universe\n", 8, 8, ModernJavac, pkg)
			})
		}
	}
}
