package javaclassparser

import (
	"strings"
	"testing"
)

// A member's own formal does not absorb its owner's arguments. Flattening
// Outer<A>.Cell<B> to Outer$Cell<B> still owes the complete original owner
// graph at publication, independently of the executable methods' erasures.
const ownMemberReferenceFixture = `class SuffixOwner<A extends Number>{
 private long seed;SuffixOwner(long seed){this.seed=seed;}
 class Cell<B extends CharSequence>{final B value;Cell(B value){this.value=value;}
  long score(){return seed*31L+(value==null?-1:value.length());}}
 <X extends CharSequence> Cell<X> identity(Cell<X> value){return value;}
 <X extends CharSequence> Cell<X>[] array(Cell<X>[] value){return value;}
 <X extends CharSequence> java.util.List<? extends Cell<X>[]> nested(java.util.List<? extends Cell<X>[]> value){return value;}
}
class SuffixDriver{public static void main(String[]args)throws Exception{
 java.lang.reflect.Method method=SuffixOwner.class.getDeclaredMethod("identity",SuffixOwner.Cell.class);
 java.lang.reflect.ParameterizedType cell=(java.lang.reflect.ParameterizedType)method.getGenericReturnType();
 java.lang.reflect.ParameterizedType owner=(java.lang.reflect.ParameterizedType)cell.getOwnerType();
 if(owner.getRawType()!=SuffixOwner.class||!owner.getActualTypeArguments()[0].equals(SuffixOwner.class.getTypeParameters()[0])||cell.getRawType()!=SuffixOwner.Cell.class||!cell.getActualTypeArguments()[0].equals(method.getTypeParameters()[0])||!method.getGenericParameterTypes()[0].equals(cell))throw new AssertionError("separate owner and member binders");
 if(SuffixOwner.Cell.class.getTypeParameters().length!=1||!SuffixOwner.Cell.class.getTypeParameters()[0].getName().equals("B"))throw new AssertionError("original member arity");
 java.lang.reflect.Method array=SuffixOwner.class.getDeclaredMethod("array",java.lang.reflect.Array.newInstance(SuffixOwner.Cell.class,0).getClass());
 java.lang.reflect.ParameterizedType element=(java.lang.reflect.ParameterizedType)((java.lang.reflect.GenericArrayType)array.getGenericReturnType()).getGenericComponentType();
 if(!element.getOwnerType().equals(owner)||!element.getActualTypeArguments()[0].equals(array.getTypeParameters()[0]))throw new AssertionError("array binder");
 int rows=0;for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(String value:new String[]{null,"","字𝄞"})for(boolean absent:new boolean[]{false,true}){
  SuffixOwner<Integer> instance=new SuffixOwner<Integer>(seed);
  SuffixOwner<Integer>.Cell<String> input=absent?null:instance.new Cell<String>(value);
  SuffixOwner<Integer>.Cell<String> output=instance.identity(input);
  long expected=java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(value==null?-1:value.length())).longValue();
  if(output!=input||output!=null&&(output.value!=value||output.score()!=expected))throw new AssertionError("owner/cell behavior");
  SuffixOwner<Integer>.Cell<String>[] cells=(SuffixOwner<Integer>.Cell<String>[])java.lang.reflect.Array.newInstance(SuffixOwner.Cell.class,1);cells[0]=input;
  java.util.List<SuffixOwner<Integer>.Cell<String>[]> list=java.util.Collections.singletonList(cells);
  if(instance.array(cells)!=cells||instance.nested(list)!=list)throw new AssertionError("nested identity");rows++;
 }System.out.println(rows+":own:member:owner:binding:overflow");
}}
`

func TestAdversarialOwnMemberReferenceRetainsBothBinderDeclarations(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			name, member, source := "SuffixOwner", "Cell", ownMemberReferenceFixture
			if renamed {
				name, member = "RenamedGraph", "Entry"
				source = strings.NewReplacer("SuffixOwner", name, "Cell", member).Replace(source)
			}
			t.Run(name+"/"+debug, func(t *testing.T) {
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
				constructorSuffixFamilyRoundTrip(t, source, debug, []string{name, name + "$" + member}, "30:own:member:owner:binding:overflow\n")
			})
		}
	}
}

func TestAdversarialOwnMemberReferenceRetainsCompilerBoundaryContracts(t *testing.T) {
	for _, compiler := range []struct {
		name             string
		original, target int
		profile          SourceCompilerProfile
	}{
		{"legacy-original", 5, 8, ModernJavac},
		{"modern-output", 8, 21, ModernJavac},
		{"native-output", 8, 8, NativeJavac8},
	} {
		t.Run(compiler.name, func(t *testing.T) {
			t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, ownMemberReferenceFixture, "none", []string{"SuffixOwner", "SuffixOwner$Cell"}, "30:own:member:owner:binding:overflow\n", compiler.original, compiler.target, compiler.profile)
		})
	}
}

func TestAdversarialOwnMemberReferenceConstructorRetainsPhysicalOuterPort(t *testing.T) {
	source := strings.Replace(ownMemberReferenceFixture, " <X extends CharSequence> Cell<X> identity", ` class Envelope {final Cell<String> value;Envelope(Cell<String> value){this.value=value;}}
 <X extends CharSequence> Cell<X> identity`, 1)
	source = strings.Replace(source, " int rows=0;", ` java.lang.reflect.Type[] constructor=SuffixOwner.Envelope.class.getDeclaredConstructors()[0].getGenericParameterTypes();
 if(constructor.length!=1)throw new AssertionError("original hidden outer port");
 java.lang.reflect.ParameterizedType captured=(java.lang.reflect.ParameterizedType)constructor[0];
 if(captured.getRawType()!=SuffixOwner.Cell.class||!captured.getOwnerType().equals(owner)||captured.getActualTypeArguments()[0]!=String.class)throw new AssertionError("constructor owner/member arguments");
 int rows=0;`, 1)
	source = strings.Replace(source, "  SuffixOwner<Integer>.Cell<String> output=", `  SuffixOwner<Integer>.Envelope envelope=instance.new Envelope(input);if(envelope.value!=input)throw new AssertionError("constructor reference identity");
  SuffixOwner<Integer>.Cell<String> output=`, 1)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			constructorSuffixFamilyRoundTrip(t, source, debug, []string{"SuffixOwner", "SuffixOwner$Cell", "SuffixOwner$Envelope"}, "30:own:member:owner:binding:overflow\n")
		})
	}
}
