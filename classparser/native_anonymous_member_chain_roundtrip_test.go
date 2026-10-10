package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeAnonymousMixedChainFixture = `class MixedEffects{static String trace="";static boolean fail;static final IllegalArgumentException error=new IllegalArgumentException("same");static long arg(long n){trace+="A";return n;}}
class MixedParent{final Object observed;MixedParent(long n){MixedEffects.trace+="P";observed=origin();if(n!=Long.MIN_VALUE)throw new AssertionError("argument");if(MixedEffects.fail)throw MixedEffects.error;}Object origin(){return null;}Object layer(){return null;}Object middle(){return null;}}
class NestedOwner{class Layer{class Middle{MixedParent make(long n){return new MixedParent(MixedEffects.arg(n)){Object origin(){return NestedOwner.this;}Object layer(){return Layer.this;}Object middle(){return Middle.this;}};}}}}
class MixedDriver{public static void main(String[]args)throws Exception{NestedOwner root=new NestedOwner();java.lang.reflect.Constructor<?> lc=NestedOwner.Layer.class.getDeclaredConstructor(NestedOwner.class);lc.setAccessible(true);java.lang.reflect.Constructor<?> mc=NestedOwner.Layer.Middle.class.getDeclaredConstructor(NestedOwner.Layer.class);mc.setAccessible(true);int rows=0;for(boolean rootNull:new boolean[]{false,true})for(boolean layerNull:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true}){NestedOwner.Layer layer=(NestedOwner.Layer)lc.newInstance(new Object[]{rootNull?null:root});NestedOwner.Layer.Middle middle=(NestedOwner.Layer.Middle)mc.newInstance(new Object[]{layerNull?null:layer});MixedEffects.trace="";MixedEffects.fail=fail;try{MixedParent c=middle.make(Long.MIN_VALUE);if(layerNull||fail||c.observed!=(rootNull?null:root)||c.origin()!=(rootNull?null:root)||c.layer()!=layer||c.middle()!=middle||!MixedEffects.trace.equals("AP"))throw new AssertionError("capture/identity/order");if(!c.getClass().getName().equals("NestedOwner$Layer$Middle$1")||!c.getClass().isAnonymousClass()||c.getClass().getEnclosingClass()!=NestedOwner.Layer.Middle.class||!c.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("lexical ownership");java.lang.reflect.Field cf=c.getClass().getDeclaredField("this$2");if(!cf.isSynthetic()||cf.getType()!=NestedOwner.Layer.Middle.class||cf.getModifiers()!=0x1010)throw new AssertionError("capture metadata");}catch(NullPointerException e){if(!layerNull||!MixedEffects.trace.equals("AP"))throw new AssertionError("nullable intermediate priority",e);}catch(IllegalArgumentException e){if(layerNull||!fail||e!=MixedEffects.error||!MixedEffects.trace.equals("AP"))throw new AssertionError("parent error identity/priority",e);}rows++;}System.out.println(rows+":identity:nullable:priority");}}
`

func TestNativeAnonymousNamedCaptureChainRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousMixedChainFixture, "NestedOwner", "MixedDriver", "8:identity:nullable:priority\n")
}
func TestNativeAnonymousNamedCaptureChainRenamedRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(nativeAnonymousMixedChainFixture, "NestedOwner", "ChangedRoot"), "Layer", "Level"), "Middle", "Frame")
	testNativePrivateSetterFixture(t, fixture, "ChangedRoot", "MixedDriver", "8:identity:nullable:priority\n")
}
func TestNativeAnonymousNamedCaptureChainRequiresExactOriginalIRPath(t *testing.T) {
	testNativeAnonymousLexicalCaptureChain(t, nativeAnonymousMixedChainFixture, "NestedOwner$Layer$Middle$1", true)
}
func TestNativeAnonymousNamedCaptureChainRequiresWholeOriginalReceiverPath(t *testing.T) {
	testNativeAnonymousLexicalReceiverPath(t, nativeAnonymousMixedChainFixture, "NestedOwner$Layer$Middle$1", true)
}

// This valid original reads a different enclosing instance. Its nominal field
// type is identical to the lexical owner's; assignability is not a THIS proof.
func TestNativeAnonymousNamedCaptureChainRefusesForeignReceiver(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousMixedChainFixture, `Object middle(){return null;}`, `Object middle(){return null;}Object foreign(NestedOwner.Layer other){return null;}`)
	fixture = strings.ReplaceAll(fixture, `Object middle(){return Middle.this;}`, `Object middle(){return Middle.this;}Object foreign(NestedOwner.Layer other){return NestedOwner.this;}`)
	fixture = strings.ReplaceAll(fixture, `if(!c.getClass().getName()`, `NestedOwner foreign=new NestedOwner();NestedOwner.Layer other=foreign.new Layer();if(c.foreign(other)!=foreign)throw new AssertionError("foreign original receiver");if(!c.getClass().getName()`)
	files := nativeCompileClasses(t, fixture)
	obj, err := Parse(files["NestedOwner$Layer$Middle$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name != "foreign" {
			continue
		}
		for _, a := range m.Attributes {
			if code, ok := a.(*CodeAttribute); ok {
				// ALOAD_0, three consecutive enclosing GETFIELDs, ARETURN. Preserve
				// the original final Layer-to-root field reference, using the argument.
				if len(code.Code) != 11 || code.Code[0] != 42 || code.Code[7] != 180 || code.Code[10] != 176 {
					t.Fatal("original three-hop witness")
				}
				code.Code = append([]byte{43}, code.Code[7:]...)
				code.Attributes = nil
				code.AttrLen = uint32(12 + len(code.Code))
				changed++
			}
		}
	}
	if changed != 1 {
		t.Fatal("original foreign method")
	}
	files["NestedOwner$Layer$Middle$1.class"] = obj.Bytes()
	original := t.TempDir()
	_, java := t04Tools(t)
	for name, raw := range files {
		if err = os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "MixedDriver"); got != "8:identity:nullable:priority\n" {
		t.Fatalf("independent foreign original=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["NestedOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	d := z.nativeMemberReader(root)
	members := d.planNativeMemberFamily()
	if members == nil {
		t.Fatal("named original family")
	}
	if d.planNativeMemberAnonymousScopes(members) {
		t.Fatal("foreign receiver licensed as lexical THIS")
	}
}
