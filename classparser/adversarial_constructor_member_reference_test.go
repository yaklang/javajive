package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// An enclosing type variable belongs to the owner declaration even when a
// constructor accepts another member of that owner. Physical reconstruction
// must preserve both the hidden receiver port and the visible generic port.
func TestAdversarialConstructorMemberReferenceBinding(t *testing.T) {
	for _, mode := range []string{"native", "physical"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, renamed := range []bool{false, true} {
				for _, compiler := range []struct {
					name    string
					target  int
					profile SourceCompilerProfile
				}{{"modern8", 8, ModernJavac}, {"modern21", 21, ModernJavac}, {"native8", 8, NativeJavac8}} {
					t.Run(fmt.Sprintf("%s/%s/%v/%s", mode, debug, renamed, compiler.name), func(t *testing.T) {
						if mode == "physical" {
							t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
						}
						fixture, owner, item, holder := constructorMemberReferenceFixture, "SuffixOwner", "Item", "Holder"
						if renamed {
							owner, item, holder = "RelocatedOwner", "Cell", "Container"
							fixture = strings.NewReplacer("SuffixOwner", owner, "Item", item, "Holder", holder).Replace(fixture)
						}
						constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{owner + "$" + holder, owner + "$" + item}, "10:constructor:owner:binding:identity\n", 8, compiler.target, compiler.profile)
					})
				}
			}
		}
	}
}

func TestAdversarialFieldOnlyMemberReferenceBinding(t *testing.T) {
	const fixture = `class SuffixOwner<T extends Number> {
 class Item {final T value;Item(T value){this.value=value;}}
 Item stored; void put(Object value){stored=(Item)value;} Object get(){return stored;}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 java.lang.reflect.ParameterizedType item=(java.lang.reflect.ParameterizedType)SuffixOwner.class.getDeclaredField("stored").getGenericType();
 java.lang.reflect.ParameterizedType owner=(java.lang.reflect.ParameterizedType)item.getOwnerType();
 if(item.getActualTypeArguments().length!=0||!owner.getActualTypeArguments()[0].equals(SuffixOwner.class.getTypeParameters()[0]))throw new AssertionError("field-only original binder");
 int rows=0;for(int n:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){
  SuffixOwner<Integer> object=new SuffixOwner<Integer>();SuffixOwner<Integer>.Item value=object.new Item(n);object.put(value);
  if(object.get()!=value||object.stored.value.intValue()!=n)throw new AssertionError("identity");object.put(null);if(object.get()!=null)throw new AssertionError("null");
  try{object.put(new Object());throw new AssertionError("missing checkcast");}catch(ClassCastException expected){}if(object.get()!=null)throw new AssertionError("store before cast");rows++;
 }System.out.println(rows+":field-only:owner:identity:checkcast");
}}
`
	for _, mode := range []string{"native", "physical"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "physical" {
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			}
			constructorSuffixFamilyRoundTrip(t, fixture, "none", []string{"SuffixOwner", "SuffixOwner$Item"}, "3:field-only:owner:identity:checkcast\n")
		})
	}
}

const constructorMemberReferenceFixture = `class SuffixOwner<T extends Number> {
 class Item {final T value;Item(T value){this.value=value;}}
 class Holder {final Item item;Holder(Item item){this.item=item;}T read(){return item==null?null:item.value;}}
 Item item(T value){return new Item(value);}
 Holder make(Item item){return new Holder(item);}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 java.lang.reflect.Constructor<?> constructor=SuffixOwner.Holder.class.getDeclaredConstructor(SuffixOwner.class,SuffixOwner.Item.class);
 java.lang.reflect.Type[] parameters=constructor.getGenericParameterTypes();
 if(parameters.length!=1)throw new AssertionError("visible generic port count");
 java.lang.reflect.ParameterizedType member=(java.lang.reflect.ParameterizedType)parameters[0];
 if(member.getRawType()!=SuffixOwner.Item.class||member.getActualTypeArguments().length!=0)throw new AssertionError("member declaration arity");
 java.lang.reflect.ParameterizedType owner=(java.lang.reflect.ParameterizedType)member.getOwnerType();
 if(owner.getRawType()!=SuffixOwner.class||!owner.getActualTypeArguments()[0].equals(SuffixOwner.class.getTypeParameters()[0]))throw new AssertionError("original owner binder");
 if(!SuffixOwner.Holder.class.getDeclaredField("item").getGenericType().equals(member))throw new AssertionError("field binder");
 int rows=0;for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean empty:new boolean[]{false,true}){
  SuffixOwner<Integer> outer=new SuffixOwner<Integer>();SuffixOwner<Integer>.Item item=empty?null:outer.item(value);
  SuffixOwner<Integer>.Holder holder=outer.make(item);
  if(holder.item!=item||holder.read()!=(item==null?null:item.value))throw new AssertionError("reference identity");rows++;
 }System.out.println(rows+":constructor:owner:binding:identity");
}}
`
