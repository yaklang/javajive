package javaclassparser

import (
	"encoding/json"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// These authored java.probe classes are compiled and read as classfile data;
// they are never loaded into a JVM or installed as platform definitions.
func TestNativePlatformDeclarationIndexDoesNotCertifyConstructorEffects(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"java/probe/Extension.java":    `package java.probe;class Parent{}public class Extension extends Parent{public Extension(){observe();}protected void observe(){}}`,
		"java/probe/CatalogFinal.java": `package java.probe;public final class CatalogFinal{}`,
		"java/probe/FinalApi.java":     `package java.probe;public final class FinalApi{}`,
		"java/probe/Contract.java":     `package java.probe;public interface Contract{}`,
		"vendor/ObserverChild.java":    `package vendor;public class ObserverChild extends java.probe.Extension{static Object escaped;protected void observe(){escaped=this;}}`,
	}, "none", "8")
	objectBytes, known := jdkConstructorClassBytes("java/lang/Object", 8)
	if !known {
		t.Fatal("original Object metadata missing")
	}
	files["java/lang/Object.class"] = objectBytes
	for _, variant := range []string{"original", "no catalog declaration", "missing parent", "parent cycle", "unknown interface", "wrong parent identity", "nil parent"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for _, raw := range files {
				o, e := Parse(raw)
				if e != nil {
					t.Fatal(e)
				}
				objects[o.GetClassName()] = o
			}
			catalog := map[string]json.RawMessage{"java/probe/CatalogFinal": json.RawMessage(`{}`)}
			switch variant {
			case "no catalog declaration":
				catalog = nil
			case "missing parent":
				delete(objects, "java/probe/Parent")
			case "parent cycle":
				o := objects["java/probe/Parent"]
				o.SuperClass = uint16(NewConstantPoolWithConstant(&o.ConstantPool).AddNewClassInfo("java/probe/Extension"))
			case "unknown interface":
				o := objects["java/probe/Extension"]
				o.Interfaces = []uint16{uint16(NewConstantPoolWithConstant(&o.ConstantPool).AddNewClassInfo("java/probe/Unavailable"))}
			case "wrong parent identity":
				objects["java/probe/Parent"] = objects["java/probe/CatalogFinal"]
			case "nil parent":
				objects["java/probe/Parent"] = nil
			}
			selected := nativePlatformDeclarationIndexClasses(objects, catalog)
			complete := variant == "original" || variant == "no catalog declaration"
			if !selected["java/lang/Object"] || !selected["java/probe/Contract"] || selected["java/probe/FinalApi"] || selected["vendor/ObserverChild"] || selected["java/probe/Extension"] != complete || selected["java/probe/Parent"] != complete || selected["java/probe/CatalogFinal"] != (variant != "no catalog declaration") {
				t.Fatalf("metadata root/ancestor selection:%v", selected)
			}
			if !complete {
				return
			}
			// The selected superclass calls an overridable method on THIS;
			// the original child implementation publishes the receiver. Keeping
			// those declarations must never turn that call into an effect proof.
			d := NewClassObjectDumper(objects["vendor/ObserverChild"])
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
			d.FuncCtx = &class_context.ClassContext{}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			remaining := 512
			if !d.constructorChainDoesNotObserve("java/probe/Parent", "()V", map[string]bool{}, map[string]bool{}, &remaining, 0) {
				t.Fatal("nonobserving original parent control was not proved")
			}
			remaining = 512
			if d.constructorChainDoesNotObserve("java/probe/Extension", "()V", map[string]bool{}, map[string]bool{}, &remaining, 0) {
				t.Fatal("metadata membership bypassed constructor receiver observation")
			}
		})
	}
}
