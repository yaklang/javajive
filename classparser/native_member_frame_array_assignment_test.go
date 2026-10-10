package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberFrameArrayAssignmentRequiresOriginalSourceConversion(t *testing.T) {
	provider := func(name string) (callbinding.Class, bool) {
		parents, known := map[string][]string{
			"sample/Leaf": {"sample/Base"}, "sample/Base": {"sample/Contract"},
			"sample/Contract": {}, "sample/Rival": {},
			"sample/CycleA": {"sample/CycleB"}, "sample/CycleB": {"sample/CycleA"},
		}[name]
		return callbinding.Class{Name: name, Parents: parents, ParentsComplete: known}, known
	}
	for _, test := range []struct {
		array, value string
		want         bool
	}{
		{"[Ljava/lang/Object;", "sample/Leaf", true},
		{"[Lsample/Leaf;", "sample/Leaf", true},
		{"[Lsample/Contract;", "sample/Leaf", true},
		{"[Lsample/Base;", "sample/Rival", false},
		{"[Lsample/Leaf;", "sample/Base", false},
		{"[Ljava/lang/String;", "java/lang/Object", false},
		{"[[Lsample/Contract;", "[Lsample/Leaf;", true},
		{"[[Lsample/Leaf;", "[Lsample/Base;", false},
		{"[[Ljava/lang/Object;", "[[I", true},
		{"[Ljava/lang/Object;", "[I", true},
		{"[[I", "[I", true},
		{"[[I", "[J", false},
		{"[[I", "I", false},
		{"[Ljava/lang/Cloneable;", "[J", true},
		{"[Ljava/io/Serializable;", "[[I", true},
		{"[Lsample/Contract;", "sample/Unknown", false},
		{"[Lsample/Contract;", "sample/CycleA", false},
		{"[I", "sample/Leaf", false},
		{"[", "sample/Leaf", false},
		{"[Lsample/Leaf;trailing", "sample/Leaf", false},
		{"[Lsample/Leaf;", "sample/Leaf;trailing", false},
		{"[Lsample/Leaf;", "", false},
		{"[Lsample/Leaf;", strings.Repeat("[", 256) + "I", false},
	} {
		t.Run(test.array+"<-"+test.value, func(t *testing.T) {
			got := nativeMemberFrameArrayStoreAssignable(frametransfer.RefOf(test.array), frametransfer.T(frametransfer.Int), frametransfer.RefOf(test.value), newConstructorWideningQuery(provider), nil)
			if got != test.want {
				t.Fatalf("source assignment=%v want %v", got, test.want)
			}
		})
	}
	for kind := frametransfer.Top; kind <= frametransfer.UninitNew; kind++ {
		q := newConstructorWideningQuery(provider)
		if got := nativeMemberFrameArrayStoreAssignable(frametransfer.RefOf("[Lsample/Leaf;"), frametransfer.T(frametransfer.Int), frametransfer.Type{Kind: kind, Class: "sample/Leaf"}, q, nil); got != (kind == frametransfer.Ref || kind == frametransfer.Null) {
			t.Errorf("element computational kind %v: accepted=%v", kind, got)
		}
		if got := nativeMemberFrameArrayStoreAssignable(frametransfer.Type{Kind: kind, Class: "[Lsample/Leaf;"}, frametransfer.T(frametransfer.Int), frametransfer.RefOf("sample/Leaf"), q, nil); got != (kind == frametransfer.Ref) {
			t.Errorf("array computational kind %v: accepted=%v", kind, got)
		}
		if got := nativeMemberFrameArrayStoreAssignable(frametransfer.RefOf("[Lsample/Leaf;"), frametransfer.T(kind), frametransfer.RefOf("sample/Leaf"), q, nil); got != (kind == frametransfer.Int) {
			t.Errorf("index computational kind %v: accepted=%v", kind, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, work := range []*workbudget.Budget{workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1}), workbudget.New(ctx, workbudget.Limits{})} {
		if nativeMemberFrameArrayStoreAssignable(frametransfer.RefOf("[Lsample/Leaf;"), frametransfer.T(frametransfer.Int), frametransfer.T(frametransfer.Null), newConstructorWideningQuery(provider), work) {
			t.Fatal("null identity cannot bypass budget/cancellation")
		}
	}
	if nativeMemberFrameArrayStoreAssignable(frametransfer.RefOf("[Lsample/Contract;"), frametransfer.T(frametransfer.Int), frametransfer.RefOf("sample/Leaf"), newConstructorWideningQuery(nil), nil) {
		t.Fatal("unknown hierarchy widened")
	}
}

func TestAdversarialMemberFrameTypedArraySuperclassRoundTrip(t *testing.T) {
	fixture := `
interface TypedStoreContract {}
class TypedStoreBase implements TypedStoreContract {}
class TypedStoreLeaf extends TypedStoreBase {}
class TypedStoreBean {public int field;}
class TypedStoreEffects {static String trace="";}
class TypedStoreParent {final Class type;final TypedStoreContract[] refs;final TypedStoreContract[][] nested;final Object[] primitives;final Object capture;
 TypedStoreParent(Class t,TypedStoreContract[] r,TypedStoreContract[][] n,Object[] p){TypedStoreEffects.trace+="S";type=t;refs=r;nested=n;primitives=p;capture=owner();}Object owner(){return null;}}
public class TypedStoreOwner {final Object token;TypedStoreOwner(Object t){token=t;}
 java.lang.reflect.Field field(java.lang.reflect.Field f){TypedStoreEffects.trace+="F";return f;}
 class Child extends TypedStoreParent {Child(java.lang.reflect.Field f,TypedStoreLeaf leaf,TypedStoreBase base,TypedStoreLeaf[] table,int i,boolean flag,long n){
  super(field(f).getType(),new TypedStoreContract[]{flag?leaf:base,null,table[i]},new TypedStoreContract[][]{new TypedStoreLeaf[]{leaf},null},new Object[]{new int[]{Integer.MIN_VALUE},new long[]{n}});
  TypedStoreEffects.trace+="B";}Object owner(){return TypedStoreOwner.this.token;}}
 Child make(java.lang.reflect.Field f,TypedStoreLeaf leaf,TypedStoreBase base,TypedStoreLeaf[] table,int i,boolean b,long n){return new Child(f,leaf,base,table,i,b,n);}}
class TypedStoreDriver {public static void main(String[]args)throws Exception{Object token=new Object();TypedStoreOwner owner=new TypedStoreOwner(token);java.lang.reflect.Field f=TypedStoreBean.class.getField("field");TypedStoreLeaf leaf=new TypedStoreLeaf();TypedStoreBase base=new TypedStoreBase();TypedStoreLeaf[] table={leaf,null};int rows=0;
 for(boolean flag:new boolean[]{false,true})for(int i=0;i<2;i++)for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){TypedStoreEffects.trace="";TypedStoreOwner.Child c=owner.make(f,leaf,base,table,i,flag,n);
  if(c.type!=int.class||c.refs[0]!=(flag?leaf:base)||c.refs[1]!=null||c.refs[2]!=table[i]||c.nested[0].getClass()!=TypedStoreLeaf[].class||c.nested[0][0]!=leaf||c.nested[1]!=null||((int[])c.primitives[0])[0]!=Integer.MIN_VALUE||((long[])c.primitives[1])[0]!=n||c.capture!=token||!TypedStoreEffects.trace.equals("FSB"))throw new AssertionError("component/rank/covariant widening/identity/order");rows++;}
 TypedStoreEffects.trace="";try{owner.make(null,leaf,base,null,-1,true,0);throw new AssertionError("null producer missing");}catch(NullPointerException e){if(!TypedStoreEffects.trace.equals("F"))throw new AssertionError("producer order");}
 TypedStoreEffects.trace="";try{owner.make(f,leaf,base,table,-1,false,0);throw new AssertionError("bounds missing");}catch(ArrayIndexOutOfBoundsException e){if(!TypedStoreEffects.trace.equals("F"))throw new AssertionError("bounds before SUPER");}
 System.out.println(rows+":typed-array:rank:widening:identity:failure-order");}}
`
	testNativePrivateSetterSourceFixture(t, map[string]string{"TypedStoreOwner.java": fixture}, "TypedStoreOwner", "TypedStoreDriver", "12:typed-array:rank:widening:identity:failure-order\n")
}
