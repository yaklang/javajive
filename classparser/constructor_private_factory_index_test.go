package javaclassparser

import (
	"encoding/json"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// These authored java.probe classfiles are metadata only and never executed.
// Keeping a private implementation's bytes grants neither source accessibility
// nor permission to publish the retained input from its constructor.
func TestAdversarialConstructorPrivateFactoryIndexSeparatesDataAndPermission(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"java/probe/CopyFactories.java": `package java.probe;public class CopyFactories{public static Object copy(Object v){return new Hidden(v);}public static Object observe(Object v){return new Observer(v);}private static class Base{final Object ref;Base(Object v){ref=v;}}private static final class Hidden extends Base{Hidden(Object v){super(v);}}private static final class Observer{Observer(Object v){v.toString();}}private static final class Unrelated{}}`,
		"vendor/CopyProofRoot.java":     `package vendor;public final class CopyProofRoot{}`,
	}, "none", "8")
	object, ok := jdkConstructorClassBytes("java/lang/Object", 8)
	if !ok {
		t.Fatal("original Object evidence missing")
	}
	files["java/lang/Object.class"] = object
	objects := map[string]*ClassObject{}
	for _, raw := range files {
		o, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		objects[o.GetClassName()] = o
	}
	selected := nativePlatformDeclarationIndexClasses(objects, map[string]json.RawMessage{})
	for _, name := range []string{"java/probe/CopyFactories", "java/probe/CopyFactories$Hidden", "java/probe/CopyFactories$Base", "java/probe/CopyFactories$Observer", "java/lang/Object"} {
		if !selected[name] {
			t.Fatal("missing original private factory allocation or ancestor", name)
		}
	}
	if selected["java/probe/CopyFactories$Unrelated"] || selected["vendor/CopyProofRoot"] {
		t.Fatal("unrelated private class acquired an index root")
	}
	for _, row := range []struct {
		name string
		want bool
	}{{"copy", true}, {"observe", false}} {
		t.Run(row.name, func(t *testing.T) {
			d := NewClassObjectDumper(objects["vendor/CopyProofRoot"])
			d.constructorReceiverFinalizerSilent = true
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			d.FuncCtx = &class_context.ClassContext{}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			fuel := 512
			member := values.NewJavaClassMember("java/probe/CopyFactories", row.name, "(Ljava/lang/Object;)Ljava/lang/Object;", nil)
			retained, known := d.constructorConfinedCopyFactory(member, []constructorEffectValue{{kind: 'L', receiver: true}}, &fuel, map[string]bool{}, 0)
			if known != row.want || known && !retained {
				t.Fatalf("index membership/copy permission: retained=%v known=%v", retained, known)
			}
		})
	}
}
