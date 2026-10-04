package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberAncestorSuperFixture = `class AncestorMemberEffects {static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
class AncestorMemberOwner {class Parent {final Object observed;final long number;Parent(Object seed,long n)throws java.io.IOException{AncestorMemberEffects.trace+="P";observed=owner();if(seed==null)throw AncestorMemberEffects.error;number=n;}Object owner(){return AncestorMemberOwner.this;}}
class Layer {class Child extends Parent {Child(Object seed,long n)throws java.io.IOException{super(seed,n);}Object owner(){return AncestorMemberOwner.this;}Object layer(){return Layer.this;}}Child make(Object seed,long n)throws java.io.IOException{return new Child(seed,n);}}Layer layer(){return new Layer();}}
class AncestorMemberDriver {public static void main(String[]args)throws Exception{AncestorMemberOwner a=new AncestorMemberOwner();AncestorMemberOwner b=new AncestorMemberOwner();Object seed=new Object();int rows=0;for(AncestorMemberOwner o:new AncestorMemberOwner[]{a,b}){AncestorMemberOwner.Layer layer=o.layer();for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){AncestorMemberEffects.trace="";AncestorMemberOwner.Layer.Child c=layer.make(seed,n);if(c.owner()!=o||c.observed!=o||c.layer()!=layer||c.number!=n||!AncestorMemberEffects.trace.equals("P")||c.getClass().getDeclaringClass()!=AncestorMemberOwner.Layer.class||!c.getClass().getName().equals("AncestorMemberOwner$Layer$Child"))throw new AssertionError("lexical ancestor/capture/callback/order/binary owner");rows++;}try{layer.make(null,0);throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=AncestorMemberEffects.error)throw new AssertionError("checked identity");}}System.out.println(rows+":ancestor:member:super:identity:callback:checked");}}
`

func TestNativeMemberAncestorSuperclassRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMemberAncestorSuperFixture, "AncestorMemberOwner", "AncestorMemberDriver", "6:ancestor:member:super:identity:callback:checked\n")
}

func TestNativeMemberAncestorSuperclassRenamedRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAncestorSuperFixture, "AncestorMemberOwner", "IndependentAncestorScope")
	fixture = strings.ReplaceAll(fixture, "Parent", "Base")
	fixture = strings.ReplaceAll(fixture, "Layer", "Context")
	testNativePrivateSetterFixture(t, fixture, "IndependentAncestorScope", "AncestorMemberDriver", "6:ancestor:member:super:identity:callback:checked\n")
}

func TestNativeMemberTwoAncestorSuperclassRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeMemberAncestorSuperFixture, "class Layer {class Child", "class Layer {class Middle {class Child", 1)
	fixture = strings.Replace(fixture, "}}Layer layer(){", "}}Middle middle(){return new Middle();}}Layer layer(){", 1)
	fixture = strings.ReplaceAll(fixture, "return Layer.this;", "return Middle.this;")
	fixture = strings.ReplaceAll(fixture, "AncestorMemberOwner.Layer layer=o.layer();", "AncestorMemberOwner.Layer.Middle layer=o.layer().middle();")
	fixture = strings.ReplaceAll(fixture, "AncestorMemberOwner.Layer.Child", "AncestorMemberOwner.Layer.Middle.Child")
	fixture = strings.ReplaceAll(fixture, "AncestorMemberOwner.Layer.class", "AncestorMemberOwner.Layer.Middle.class")
	fixture = strings.ReplaceAll(fixture, "$Layer$Child", "$Layer$Middle$Child")
	testNativePrivateSetterFixture(t, fixture, "AncestorMemberOwner", "AncestorMemberDriver", "6:ancestor:member:super:identity:callback:checked\n")
}

const nativeMemberAncestorGenericSuperFixture = `class AncestorGenericEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
class AncestorGenericOwner<T>{class Parent {final Object observed;final T[] values;Parent(Object seed,T[] kept)throws java.io.IOException{AncestorGenericEffects.trace+="P";observed=owner();if(seed==null)throw AncestorGenericEffects.error;values=kept;}Object owner(){return AncestorGenericOwner.this;}}
class Layer{class Child extends Parent{Child(Object seed,T[] kept)throws java.io.IOException{super(seed,kept);}Object owner(){return AncestorGenericOwner.this;}Object layer(){return Layer.this;}}Child make(Object seed,T[] kept)throws java.io.IOException{return new Child(seed,kept);}}Layer layer(){return new Layer();}}
class AncestorGenericDriver{public static void main(String[]args)throws Exception{AncestorGenericOwner<Object>a=new AncestorGenericOwner<>();AncestorGenericOwner<Object>b=new AncestorGenericOwner<>();Object seed=new Object();int rows=0;for(AncestorGenericOwner<Object>o:java.util.Arrays.asList(a,b)){AncestorGenericOwner<Object>.Layer layer=o.layer();for(Object[]v:new Object[][]{null,new Object[]{seed},new String[]{"original"}}){AncestorGenericEffects.trace="";AncestorGenericOwner<Object>.Layer.Child c=layer.make(seed,v);if(c.owner()!=o||c.observed!=o||c.layer()!=layer||c.values!=v||!AncestorGenericEffects.trace.equals("P")||c.getClass().getDeclaringClass()!=AncestorGenericOwner.Layer.class||!c.getClass().getName().equals("AncestorGenericOwner$Layer$Child"))throw new AssertionError("generic owner/array/callback/order");rows++;}try{layer.make(null,new Object[]{seed});throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=AncestorGenericEffects.error)throw new AssertionError("checked identity");}}System.out.println(rows+":generic:ancestor:member:identity");}}
`

func TestNativeMemberGenericAncestorSuperclassRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMemberAncestorGenericSuperFixture, "AncestorGenericOwner", "AncestorGenericDriver", "6:generic:ancestor:member:identity\n")
}

func TestNativeMemberGenericTwoAncestorSuperclassRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeMemberAncestorGenericSuperFixture, "class Layer{class Child", "class Layer{class Middle{class Child", 1)
	fixture = strings.Replace(fixture, "}}Layer layer(){", "}}Middle middle(){return new Middle();}}Layer layer(){", 1)
	fixture = strings.ReplaceAll(fixture, "return Layer.this;", "return Middle.this;")
	fixture = strings.ReplaceAll(fixture, "AncestorGenericOwner<Object>.Layer layer=o.layer();", "AncestorGenericOwner<Object>.Layer.Middle layer=o.layer().middle();")
	fixture = strings.ReplaceAll(fixture, "AncestorGenericOwner<Object>.Layer.Child", "AncestorGenericOwner<Object>.Layer.Middle.Child")
	fixture = strings.ReplaceAll(fixture, "AncestorGenericOwner.Layer.class", "AncestorGenericOwner.Layer.Middle.class")
	fixture = strings.ReplaceAll(fixture, "$Layer$Child", "$Layer$Middle$Child")
	fixture = strings.ReplaceAll(fixture, "AncestorGenericOwner", "UnrelatedGenericScope")
	testNativePrivateSetterFixture(t, fixture, "UnrelatedGenericScope", "AncestorGenericDriver", "6:generic:ancestor:member:identity\n")
}

// A parent's physical enclosing argument may be independent of the child's
// lexical enclosing object. A matching erased type does not prove identity.
func TestNativeMemberAncestorSuperclassRefusesForeignQualifiedOuter(t *testing.T) {
	fixture := strings.Replace(nativeMemberAncestorSuperFixture, "Object owner(){return AncestorMemberOwner.this;}", "Object declaredOwner(){return AncestorMemberOwner.this;}Object owner(){return AncestorMemberOwner.this;}", 1)
	fixture = strings.Replace(fixture, "Child(Object seed,long n)", "Child(AncestorMemberOwner other,Object seed,long n)", 1)
	fixture = strings.Replace(fixture, "super(seed,n);", "other.super(seed,n);", 1)
	fixture = strings.Replace(fixture, "Child make(Object seed,long n)", "Child make(AncestorMemberOwner other,Object seed,long n)", 1)
	fixture = strings.Replace(fixture, "new Child(seed,n)", "new Child(other,seed,n)", 1)
	fixture = strings.ReplaceAll(fixture, "layer.make(seed,n)", "layer.make(o==a?b:a,seed,n)")
	fixture = strings.ReplaceAll(fixture, "layer.make(null,0)", "layer.make(o==a?b:a,null,0)")
	fixture = strings.Replace(fixture, "c.owner()!=o||", "c.declaredOwner()!=(o==a?b:a)||c.owner()!=o||", 1)
	files := nativeCompileClasses(t, fixture)
	_, java := t04Tools(t)
	original := t.TempDir()
	for n, raw := range files {
		if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "AncestorMemberDriver"); got != "6:ancestor:member:super:identity:callback:checked\n" {
		t.Fatalf("valid original foreign outer=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["AncestorMemberOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	if z.nativeMemberReader(root).planNativeMemberFamily() != nil {
		t.Fatal("foreign SUPER enclosing operand accepted as lexical identity")
	}
}

func TestNativeMemberGenericAncestorThisDelegationRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeMemberAncestorGenericSuperFixture, "Child(Object seed,T[] kept)", "Child(T[] kept)throws java.io.IOException{this(new Object(),kept);}Child(Object seed,T[] kept)", 1)
	fixture = strings.Replace(fixture, "return new Child(seed,kept);", "if(seed==null)return new Child(seed,kept);return new Child(kept);", 1)
	testNativePrivateSetterFixture(t, fixture, "AncestorGenericOwner", "AncestorGenericDriver", "6:generic:ancestor:member:identity\n")
}

func TestNativeMemberAncestorSuperclassKeepsNullEnclosingIdentity(t *testing.T) {
	fixture := nativeMemberAncestorSuperFixture[:strings.Index(nativeMemberAncestorSuperFixture, "class AncestorMemberDriver")] + `class AncestorMemberDriver{public static void main(String[]args)throws Exception{AncestorMemberOwner.Layer layer=new AncestorMemberOwner().layer();int changed=0;for(java.lang.reflect.Field f:layer.getClass().getDeclaredFields())if(f.getType()==AncestorMemberOwner.class){f.setAccessible(true);f.set(layer,null);changed++;}if(changed!=1)throw new AssertionError("capture metadata");AncestorMemberEffects.trace="";AncestorMemberOwner.Layer.Child c=layer.make(new Object(),Long.MIN_VALUE);if(c.owner()!=null||c.observed!=null||c.layer()!=layer||c.number!=Long.MIN_VALUE||!AncestorMemberEffects.trace.equals("P"))throw new AssertionError("implicit null outer must not acquire a qualified-allocation null check");System.out.println("null:enclosing:identity:preserved");}}`
	testNativePrivateSetterFixture(t, fixture, "AncestorMemberOwner", "AncestorMemberDriver", "null:enclosing:identity:preserved\n")
}

func TestNativeMemberAncestorSuperclassKeepsIntermediateNullFailure(t *testing.T) {
	fixture := strings.Replace(nativeMemberAncestorSuperFixture, "class Layer {class Child", "class Layer {class Middle {class Child", 1)
	fixture = strings.Replace(fixture, "}}Layer layer(){", "}}Middle middle(){return new Middle();}}Layer layer(){", 1)
	fixture = fixture[:strings.Index(fixture, "class AncestorMemberDriver")] + `class AncestorMemberDriver{public static void main(String[]args)throws Exception{AncestorMemberOwner.Layer.Middle middle=new AncestorMemberOwner().layer().middle();int changed=0;for(java.lang.reflect.Field f:middle.getClass().getDeclaredFields())if(f.getType()==AncestorMemberOwner.Layer.class){f.setAccessible(true);f.set(middle,null);changed++;}if(changed!=1)throw new AssertionError("capture metadata");AncestorMemberEffects.trace="";try{middle.make(new Object(),7);throw new AssertionError("missing original lexical dereference failure");}catch(NullPointerException expected){if(!AncestorMemberEffects.trace.equals(""))throw new AssertionError("failure moved after parent effects");}System.out.println("intermediate:null:before:parent");}}`
	testNativePrivateSetterFixture(t, fixture, "AncestorMemberOwner", "AncestorMemberDriver", "intermediate:null:before:parent\n")
}
