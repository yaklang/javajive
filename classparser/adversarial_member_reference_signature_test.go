package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialMemberReferenceSignatureRetainsOriginalOwnerBinding(t *testing.T) {
	for _, mode := range []string{"native", "physical"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "physical" {
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			}
			constructorSuffixFamilyRoundTrip(t, memberReferenceSignatureFixture, "none", []string{"SuffixOwner", "SuffixOwner$Node"}, "18:owner:binding:identity:null\n")
		})
	}
}

func TestAdversarialMemberReferenceSignaturesPreserveSupplementaryNames(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	const owner, member = "𐐀Owner", "𐐁Cell"
	source := strings.NewReplacer("SuffixOwner", owner, "Node", member).Replace(memberReferenceSignatureFixture)
	constructorSuffixFamilyRoundTrip(t, source, "none", []string{owner, owner + "$" + member}, "18:owner:binding:identity:null\n")
}

func TestAdversarialMemberReferenceSignaturesPreserveNestedAndShadowBindings(t *testing.T) {
	source := strings.Replace(memberReferenceSignatureFixture, "class Node {final T value;Node(T value){this.value=value;}}", "class Node {final T value;Node(T value){this.value=value;}T read(){return SuffixOwner.this.keep(value);}} T keep(T value){return value;}", 1)
	source = strings.Replace(source, "Node identity(Node value){return value;}", `Node identity(Node value){return value;}
 Node[] array(Node[] value){return value;}
 java.util.List<? extends Node[]> nested(java.util.List<? extends Node[]> value){return value;}
 <U extends T> Node dependent(U value,Node result){return result;}
 static <T extends Number> SuffixOwner<T>.Node shadow(SuffixOwner<T>.Node value){return value;}`, 1)
	source = strings.Replace(source, "int rows=0;", `
 java.lang.reflect.Method array=SuffixOwner.class.getDeclaredMethod("array",java.lang.reflect.Array.newInstance(SuffixOwner.Node.class,0).getClass());
 if(!((java.lang.reflect.GenericArrayType)array.getGenericReturnType()).getGenericComponentType().equals(node))throw new AssertionError("array owner");
 java.lang.reflect.Method nested=SuffixOwner.class.getDeclaredMethod("nested",java.util.List.class);
 java.lang.reflect.ParameterizedType list=(java.lang.reflect.ParameterizedType)nested.getGenericReturnType();
 java.lang.reflect.Type upper=((java.lang.reflect.WildcardType)list.getActualTypeArguments()[0]).getUpperBounds()[0];
 if(!((java.lang.reflect.GenericArrayType)upper).getGenericComponentType().equals(node)||!nested.getGenericParameterTypes()[0].equals(list))throw new AssertionError("nested owner");
 java.lang.reflect.Method dependent=SuffixOwner.class.getDeclaredMethod("dependent",Number.class,SuffixOwner.Node.class);
 if(!dependent.getTypeParameters()[0].getBounds()[0].equals(variable)||!dependent.getGenericReturnType().equals(node))throw new AssertionError("dependent method binder");
 java.lang.reflect.Method shadow=SuffixOwner.class.getDeclaredMethod("shadow",SuffixOwner.Node.class);
 java.lang.reflect.ParameterizedType shadowNode=(java.lang.reflect.ParameterizedType)shadow.getGenericReturnType();
 java.lang.reflect.Type shadowVariable=((java.lang.reflect.ParameterizedType)shadowNode.getOwnerType()).getActualTypeArguments()[0];
 if(!shadowVariable.equals(shadow.getTypeParameters()[0])||shadowVariable.equals(variable))throw new AssertionError("method binder shadow");
 int rows=0;`, 1)
	source = strings.Replace(source, "rows++;", `
  SuffixOwner<Integer>.Node[] arrayInput=(SuffixOwner<Integer>.Node[])java.lang.reflect.Array.newInstance(SuffixOwner.Node.class,1);arrayInput[0]=input;
  java.util.List<SuffixOwner<Integer>.Node[]> nestedInput=java.util.Collections.singletonList(arrayInput);
  if(object.array(arrayInput)!=arrayInput||object.nested(nestedInput)!=nestedInput||object.dependent(value,input)!=input||SuffixOwner.shadow(input)!=input||input!=null&&input.read().intValue()!=value)throw new AssertionError("reference behavior");rows++;`, 1)
	for _, renamed := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, compiler := range []struct {
				name    string
				target  int
				profile SourceCompilerProfile
			}{{"modern8", 8, ModernJavac}, {"modern21", 21, ModernJavac}, {"native8", 8, NativeJavac8}} {
				t.Run(fmt.Sprintf("renamed=%v/%s/%s", renamed, debug, compiler.name), func(t *testing.T) {
					t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
					fixture, owner, member := source, "SuffixOwner", "Node"
					if renamed {
						fixture = strings.NewReplacer("SuffixOwner", "RenamedOwner", "Node", "Cell").Replace(source)
						owner, member = "RenamedOwner", "Cell"
					}
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{owner, owner + "$" + member}, "18:owner:binding:identity:null\n", 8, compiler.target, compiler.profile)
				})
			}
		}
	}
}

// The original return type is Outer<T>.Node: T belongs to Outer, and Node
// declares no formals. A flattened source Node<T> must not change reflection's
// generic declaration identity or the original method's reference behavior.
const memberReferenceSignatureFixture = `class SuffixOwner<T extends Number> {
 class Node {final T value;Node(T value){this.value=value;}}
 Node identity(Node value){return value;}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 java.lang.reflect.Method method=SuffixOwner.class.getDeclaredMethod("identity",SuffixOwner.Node.class);
 java.lang.reflect.ParameterizedType node=(java.lang.reflect.ParameterizedType)method.getGenericReturnType();
 if(node.getRawType()!=SuffixOwner.Node.class||node.getActualTypeArguments().length!=0)throw new AssertionError("member arity");
 java.lang.reflect.ParameterizedType owner=(java.lang.reflect.ParameterizedType)node.getOwnerType();
 if(owner.getRawType()!=SuffixOwner.class||owner.getActualTypeArguments().length!=1)throw new AssertionError("owner arguments");
 java.lang.reflect.TypeVariable<?> variable=(java.lang.reflect.TypeVariable<?>)owner.getActualTypeArguments()[0];
 if(variable.getGenericDeclaration()!=SuffixOwner.class||!variable.getName().equals("T")||!method.getGenericParameterTypes()[0].equals(node))throw new AssertionError("original binder");
 int rows=0;for(int value:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE})for(boolean empty:new boolean[]{false,true})for(int repeat=0;repeat<3;repeat++){
  SuffixOwner<Integer> object=new SuffixOwner<Integer>();SuffixOwner<Integer>.Node input=empty?null:object.new Node(value);
  SuffixOwner<Integer>.Node result=object.identity(input);if(result!=input||result!=null&&result.value.intValue()!=value)throw new AssertionError("identity");rows++;
 }System.out.println(rows+":owner:binding:identity:null");
}}
`

func TestAdversarialMemberReferenceMultipleOwnerArgumentsKeepOrder(t *testing.T) {
	const fixture = `class SuffixOwner<A extends Number,B extends CharSequence>{
 A first(A value){return value;}B second(B value){return value;}
 class Pair{final A a;final B b;Pair(A a,B b){this.a=a;this.b=b;}A readFirst(){return SuffixOwner.this.first(a);}B readSecond(){return SuffixOwner.this.second(b);}}
 Pair identity(Pair value){return value;}
}
class SuffixDriver{public static void main(String[]args)throws Exception{
 java.lang.reflect.Method method=SuffixOwner.class.getDeclaredMethod("identity",SuffixOwner.Pair.class);
 java.lang.reflect.ParameterizedType result=(java.lang.reflect.ParameterizedType)method.getGenericReturnType();
 java.lang.reflect.Type[] owner=((java.lang.reflect.ParameterizedType)result.getOwnerType()).getActualTypeArguments();
 java.lang.reflect.TypeVariable<?>[] formals=SuffixOwner.class.getTypeParameters();
 if(result.getActualTypeArguments().length!=0||owner.length!=2||!owner[0].equals(formals[0])||!owner[1].equals(formals[1])||!method.getGenericParameterTypes()[0].equals(result))throw new AssertionError("ordered original owner arguments");
 int rows=0;for(int value:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE})for(String text:new String[]{null,"","字"})for(boolean empty:new boolean[]{false,true}){
  SuffixOwner<Integer,String> outer=new SuffixOwner<Integer,String>();SuffixOwner<Integer,String>.Pair input=empty?null:outer.new Pair(value,text);
  SuffixOwner<Integer,String>.Pair output=outer.identity(input);if(output!=input||output!=null&&(output.readFirst().intValue()!=value||output.readSecond()!=text))throw new AssertionError("two binder identity");rows++;
 }System.out.println(rows+":two:owner:arguments:identity");
}}
`
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, compiler := range []struct {
			name    string
			target  int
			profile SourceCompilerProfile
		}{{"modern8", 8, ModernJavac}, {"modern21", 21, ModernJavac}, {"native8", 8, NativeJavac8}} {
			t.Run(debug+"/"+compiler.name, func(t *testing.T) {
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
				constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner", "SuffixOwner$Pair"}, "18:two:owner:arguments:identity\n", 8, compiler.target, compiler.profile)
			})
		}
	}
}
