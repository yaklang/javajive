package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

const genericAccessConstructorFixture = `class SuffixEffects {static int calls;static final RuntimeException error=new IllegalStateException("identity");}
class SuffixOwner {
 static class Member<T extends Number> {
  final T[][] matrix;final int[] order;final long seed;
  private Member(T[][] matrix,int[] order,long seed){SuffixEffects.calls++;this.matrix=matrix;this.order=order;this.seed=seed;if(seed==Long.MIN_VALUE)throw SuffixEffects.error;}
  Object matrix(){return matrix;}Object order(){return order;}long fold(long n){return n*31L+seed;}
 }
 static<T extends Number>Member<T> create(T[][] matrix,int[] order,long seed){return new Member<T>(matrix,order,seed);}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 int rows=0;for(Integer[][] matrix:new Integer[][][]{null,new Integer[][]{{1,null},{Integer.MIN_VALUE}}})for(int[] order:new int[][]{null,new int[]{2,1,0}})for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){
  SuffixEffects.calls=0;SuffixOwner.Member<Integer> value=null;Throwable error=null;try{value=SuffixOwner.create(matrix,order,seed);}catch(Throwable e){error=e;}
  if(SuffixEffects.calls!=1||error!=(seed==Long.MIN_VALUE?SuffixEffects.error:null))throw new AssertionError("forward once / failure identity",error);
  if(error==null){if(value.matrix()!=matrix||value.order()!=order)throw new AssertionError("array identity");for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})if(value.fold(n)!=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(seed)).longValue())throw new AssertionError("modular arithmetic");}rows++;
 }
 int bridges=0;for(java.lang.reflect.Constructor<?> c:Class.forName("SuffixOwner$Member").getDeclaredConstructors())if(c.isSynthetic()){bridges++;if(c.getTypeParameters().length!=0)throw new AssertionError("invented constructor binder");for(java.lang.reflect.Type p:c.getGenericParameterTypes())if(!(p instanceof Class))throw new AssertionError("compiler access port exposed as generic declaration");}
 if(bridges!=1)throw new AssertionError("original compiler access bridge inventory");System.out.println(rows+":access:constructor:erased:ports:identity");}}
`

func TestAdversarialGenericAccessConstructorRetainsErasedDeclaredPorts(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, release := range []int{5, 6, 7, 8} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("original%d/%s", release, debug), func(t *testing.T) {
				constructorSuffixFamilyRoundTrip(t, genericAccessConstructorFixture, debug, []string{"SuffixOwner$Member"}, "16:access:constructor:erased:ports:identity\n", release)
			})
		}
	}
}

const emptyMemberConstructorFixture = `class SuffixOwner<T> {
 final T token;final long seed;SuffixOwner(T token,long seed){this.token=token;this.seed=seed;}
 class Member {private Member(){}T value(){return token;}long fold(long n){return n*31L+seed;}}
 Member create(){return new Member();}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 int rows=0;for(Object token:new Object[]{null,new Object()})for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){
  SuffixOwner<Object> owner=new SuffixOwner<Object>(token,seed);SuffixOwner<Object>.Member value=owner.create();if(value.value()!=token)throw new AssertionError("enclosing identity");
  for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){if(value.fold(n)!=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(seed)).longValue())throw new AssertionError("modular arithmetic");rows++;}
 }
 Class<?> member=Class.forName("SuffixOwner$Member");if(member.getDeclaringClass()!=SuffixOwner.class||member.getTypeParameters().length!=0)throw new AssertionError("original lexical scope");
 for(java.lang.reflect.Constructor<?> c:member.getDeclaredConstructors())for(java.lang.reflect.Type p:c.getGenericParameterTypes())if(!(p instanceof Class))throw new AssertionError("hidden enclosing port became a declared generic argument");
 System.out.println(rows+":empty:member:constructor:hidden:port:identity");}}
`

func TestAdversarialEmptyMemberConstructorRetainsAbsentGenericSignature(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, release := range []int{5, 6, 7, 8} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("original%d/%s", release, debug), func(t *testing.T) {
				constructorSuffixFamilyRoundTrip(t, emptyMemberConstructorFixture, debug, []string{"SuffixOwner$Member"}, "32:empty:member:constructor:hidden:port:identity\n", release)
			})
		}
	}
}

func TestAdversarialRecursiveBoundAccessConstructorRetainsErasedPorts(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := strings.ReplaceAll(genericAccessConstructorFixture, "T extends Number", "T extends Comparable<T>")
	for _, release := range []int{5, 8} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("original%d/%s", release, debug), func(t *testing.T) {
				constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "16:access:constructor:erased:ports:identity\n", release)
			})
		}
	}
}
func TestAdversarialRecursiveBoundEmptyMemberConstructorRetainsErasedPorts(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := strings.NewReplacer("class SuffixOwner<T>", "class SuffixOwner<T extends Comparable<T>>", "final T token;final long seed;", "private final T token;private final long seed;", "Object token:new Object[]{null,new Object()}", "Integer token:new Integer[]{null,new Integer(123)}", "SuffixOwner<Object>", "SuffixOwner<Integer>").Replace(emptyMemberConstructorFixture)
	for _, release := range []int{5, 8} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("original%d/%s", release, debug), func(t *testing.T) {
				constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member", "SuffixOwner", "SuffixOwner$1"}, "32:empty:member:constructor:hidden:port:identity\n", release)
			})
		}
	}
}

const declaredConstructorRecursiveValue = `interface SuffixElement<E> { E subtract(E other); }
class SuffixScalar implements SuffixElement<SuffixScalar> {final int n;SuffixScalar(int n){this.n=n;}public SuffixScalar subtract(SuffixScalar other){return new SuffixScalar(n-other.n);}}
`

func TestAdversarialApplicationRecursiveBoundConstructorPorts(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, member := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("member%v/%s", member, debug), func(t *testing.T) {
				owners := []string{"SuffixOwner$Member"}
				fixture := declaredConstructorRecursiveValue + strings.NewReplacer("T extends Number", "T extends SuffixElement<T>", "Integer[][]", "SuffixScalar[][]", "{{1,null},{Integer.MIN_VALUE}}", "{{new SuffixScalar(1),null},{new SuffixScalar(Integer.MIN_VALUE)}}", "Member<Integer>", "Member<SuffixScalar>").Replace(genericAccessConstructorFixture)
				expected := "16:access:constructor:erased:ports:identity\n"
				if member {
					owners = append(owners, "SuffixOwner", "SuffixOwner$1")
					fixture = declaredConstructorRecursiveValue + strings.NewReplacer("class SuffixOwner<T>", "class SuffixOwner<T extends SuffixElement<T>>", "final T token;final long seed;", "private final T token;private final long seed;", "Object token:new Object[]{null,new Object()}", "SuffixScalar token:new SuffixScalar[]{null,new SuffixScalar(123)}", "SuffixOwner<Object>", "SuffixOwner<SuffixScalar>").Replace(emptyMemberConstructorFixture)
					expected = "32:empty:member:constructor:hidden:port:identity\n"
				}
				constructorSuffixFamilyRoundTrip(t, fixture, debug, owners, expected, 5)
			})
		}
	}
}

func TestAdversarialParameterizedPrefixAccessConstructorErasure(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := "interface SuffixMeta<X> {}\n" + strings.NewReplacer(
		"final T[][] matrix;", "final SuffixMeta<T> meta;final T[][] matrix;",
		"(T[][] matrix,int[] order,long seed)", "(SuffixMeta<T> meta,T[][] matrix,int[] order,long seed)",
		"SuffixEffects.calls++;this.matrix", "SuffixEffects.calls++;this.meta=meta;this.matrix",
		"new Member<T>(matrix,order,seed)", "new Member<T>(meta,matrix,order,seed)",
		"SuffixOwner.create(matrix,order,seed)", "SuffixOwner.create(new SuffixMeta<Integer>(){},matrix,order,seed)").Replace(genericAccessConstructorFixture)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "16:access:constructor:erased:ports:identity\n", 5)
		})
	}
}

func TestAdversarialPureGenericAccessConstructorDeclaredPorts(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := strings.NewReplacer("SuffixEffects.calls++;", "", "if(seed==Long.MIN_VALUE)throw SuffixEffects.error;", "", "SuffixEffects.calls!=1||error!=(seed==Long.MIN_VALUE?SuffixEffects.error:null)", "SuffixEffects.calls!=0||error!=null").Replace(genericAccessConstructorFixture)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "16:access:constructor:erased:ports:identity\n", 5)
		})
	}
}
func TestAdversarialMemberConstructorEnclosingReadKeepsUndeclaredPort(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := strings.NewReplacer("class SuffixOwner<T>", "class SuffixOwner<T extends Comparable<T>>", "final T token;final long seed;", "private final T token;private final long seed;", "Object token:new Object[]{null,new Object()}", "Integer token:new Integer[]{null,new Integer(123)}", "SuffixOwner<Object>", "SuffixOwner<Integer>", "class Member {private Member(){}", "class Member {final long snapshot;private Member(){snapshot=seed;}", "long fold(long n){return n*31L+seed;}", "long fold(long n){return n*31L+snapshot;}").Replace(emptyMemberConstructorFixture)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member", "SuffixOwner", "SuffixOwner$1"}, "32:empty:member:constructor:hidden:port:identity\n", 5)
		})
	}
}

// Alpha-renaming an application type must not change a constructor's declared
// generic contract. These are authored lookalikes, not library fixtures: the
// legacy text layer used a simple name as permission to promote erased ports.
func TestAdversarialErasedConstructorDeclarationsAreIndependentOfTypeNames(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, kind := range []string{"access", "enclosing"} {
		for _, bound := range []string{"NeutralElement", "FieldElement", "RealFieldElement"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(kind+"/"+bound+"/"+debug, func(t *testing.T) {
					fixture := declaredConstructorRecursiveValue + strings.NewReplacer("T extends Number", "T extends SuffixElement<T>", "Integer[][]", "SuffixScalar[][]", "{{1,null},{Integer.MIN_VALUE}}", "{{new SuffixScalar(1),null},{new SuffixScalar(Integer.MIN_VALUE)}}", "Member<Integer>", "Member<SuffixScalar>").Replace(genericAccessConstructorFixture)
					owners := []string{"SuffixOwner$Member"}
					expected := "16:access:constructor:erased:ports:identity\n"
					if kind == "enclosing" {
						fixture = declaredConstructorRecursiveValue + strings.NewReplacer("class SuffixOwner<T>", "class SuffixOwner<T extends SuffixElement<T>>", "final T token;final long seed;", "private final T token;private final long seed;", "Object token:new Object[]{null,new Object()}", "SuffixScalar token:new SuffixScalar[]{null,new SuffixScalar(123)}", "SuffixOwner<Object>", "SuffixOwner<SuffixScalar>").Replace(emptyMemberConstructorFixture)
						owners = []string{"OpenIntToFieldHashMap$Member", "OpenIntToFieldHashMap", "OpenIntToFieldHashMap$1"}
						fixture = strings.ReplaceAll(fixture, "SuffixOwner", "OpenIntToFieldHashMap")
						expected = "32:empty:member:constructor:hidden:port:identity\n"
					}
					fixture = strings.ReplaceAll(fixture, "SuffixElement", bound)
					constructorSuffixFamilyRoundTrip(t, fixture, debug, owners, expected, 5)
				})
			}
		}
	}
}
