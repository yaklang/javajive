package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousBridgeCovarianceUsesClosedOriginalHierarchy(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"p/RootMarker.java": `package p;public interface RootMarker{}`,
		"p/Marker.java":     `package p;public interface Marker extends RootMarker{}`,
		"p/Parent.java":     `package p;public class Parent<U>{}`,
		"p/Token.java":      `package p;public class Token extends Parent<String> implements Marker{}`,
	}, "none", "8")
	for _, variant := range []string{"original", "interface target", "reference arrays", "array dimensional widening", "primitive array Object", "primitive array Cloneable", "primitive array Serializable", "primitive array mismatch", "primitive component not Object", "wider reference component", "nested primitive mismatch", "nested primitive Cloneable", "nested primitive Serializable", "nil source", "nil resolve", "malformed actual", "primitive result", "unknown actual", "unknown formal", "unknown superclass", "unknown second interface", "cyclic interface", "cyclic superclass", "wrong target identity", "wrong CP tag", "superclass is interface", "interface is class", "final superclass", "wrong signature superclass", "wrong signature interface", "duplicate signature", "duplicate interface", "free signature formal", "wrong parent arity", "wildcard parent", "nil attribute", "typed nil signature", "typed nil opaque attribute", "Object cycle", "Object interface", "Object final", "Cloneable class", "Serializable class", "parameterized formal", "graph budget", "memory budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for name, raw := range files {
				o, e := Parse(append([]byte(nil), raw...))
				if e != nil {
					t.Fatal(e)
				}
				objects[name[:len(name)-6]] = o
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			fallback := archive.nativeMemberReader(objects["p/Token"]).nativeAnnotationDeclarationResolver()
			resolve := func(n string) (*ClassObject, bool) {
				if o, ok := objects[n]; ok {
					return o, o != nil
				}
				return fallback(n)
			}
			stableResolve := resolve
			formal, actual, want := "Lp/Parent;", "Lp/Token;", false
			child, parent, marker := objects["p/Token"], objects["p/Parent"], objects["p/Marker"]
			setSig := func(o *ClassObject, text string) {
				pool := NewConstantPoolWithConstant(&o.ConstantPool)
				for _, a := range o.Attributes {
					if s, ok := a.(*SignatureAttribute); ok {
						s.SignatureIndex = uint16(pool.AddUtf8Info(text))
						return
					}
				}
				o.Attributes = append(o.Attributes, &SignatureAttribute{SignatureIndex: uint16(pool.AddUtf8Info(text))})
			}
			var work *workbudget.Budget
			switch variant {
			case "original":
				want = true
			case "interface target":
				formal = "Lp/RootMarker;"
				want = true
			case "reference arrays":
				formal, actual, want = "[Lp/Parent;", "[Lp/Token;", true
			case "array dimensional widening":
				formal, actual, want = "[Ljava/lang/Object;", "[[I", true
			case "primitive array Object":
				formal, actual, want = "Ljava/lang/Object;", "[I", true
			case "primitive array Cloneable":
				formal, actual, want = "Ljava/lang/Cloneable;", "[I", true
			case "primitive array Serializable":
				formal, actual, want = "Ljava/io/Serializable;", "[I", true
			case "primitive array mismatch":
				formal, actual = "[J", "[I"
			case "primitive component not Object":
				formal, actual = "[Ljava/lang/Object;", "[I"
			case "wider reference component":
				formal, actual = "[Lp/Token;", "[Lp/Parent;"
			case "nested primitive mismatch":
				formal, actual = "[[J", "[[I"
			case "nested primitive Cloneable":
				formal, actual, want = "[Ljava/lang/Cloneable;", "[[I", true
			case "nested primitive Serializable":
				formal, actual, want = "[Ljava/io/Serializable;", "[[I", true
			case "nil source":
			case "nil resolve":
				resolve = nil
			case "malformed actual":
				actual = "Lp/Token;garbage"
			case "primitive result":
				actual = "I"
			case "unknown actual":
				objects["p/Token"] = nil
			case "unknown formal":
				objects["p/Parent"] = nil
			case "unknown superclass":
				objects["p/Parent"] = nil
			case "unknown second interface":
				marker.Interfaces = []uint16{uint16(NewConstantPoolWithConstant(&marker.ConstantPool).AddNewClassInfo("p/Missing"))}
			case "cyclic interface":
				marker.Interfaces = []uint16{uint16(NewConstantPoolWithConstant(&marker.ConstantPool).AddNewClassInfo("p/Marker"))}
			case "cyclic superclass":
				parent.SuperClass = uint16(NewConstantPoolWithConstant(&parent.ConstantPool).AddNewClassInfo("p/Token"))
				setSig(parent, "<U:Ljava/lang/Object;>Lp/Token;")
			case "wrong target identity":
				parent.ThisClass = uint16(NewConstantPoolWithConstant(&parent.ConstantPool).AddNewClassInfo("p/Other"))
			case "wrong CP tag":
				child.ConstantPool = append(child.ConstantPool, &ConstantStringInfo{StringIndex: child.ThisClass})
				child.ThisClass = uint16(len(child.ConstantPool))
			case "superclass is interface":
				parent.AccessFlags |= 0x200
			case "interface is class":
				marker.AccessFlags &^= 0x200
			case "final superclass":
				parent.AccessFlags |= 0x10
			case "wrong signature superclass":
				setSig(child, "Ljava/lang/Object;Lp/Marker;")
			case "wrong signature interface":
				setSig(child, "Lp/Parent<Ljava/lang/String;>;Lp/RootMarker;")
			case "duplicate signature":
				for _, a := range child.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						child.Attributes = append(child.Attributes, a)
						break
					}
				}
			case "duplicate interface":
				child.Interfaces = append(child.Interfaces, child.Interfaces[0])
			case "free signature formal":
				setSig(child, "Lp/Parent<TE;>;Lp/Marker;")
			case "wrong parent arity":
				setSig(child, "Lp/Parent<Ljava/lang/String;Ljava/lang/String;>;Lp/Marker;")
			case "wildcard parent":
				setSig(child, "Lp/Parent<*>;Lp/Marker;")
			case "nil attribute":
				child.Attributes = append(child.Attributes, nil)
			case "typed nil signature":
				child.Attributes = append(child.Attributes, (*SignatureAttribute)(nil))
			case "typed nil opaque attribute":
				child.Attributes = append(child.Attributes, (*UnparsedAttribute)(nil))
			case "Object cycle", "Object interface", "Object final", "Cloneable class", "Serializable class":
				name := "java/lang/Object"
				if variant == "Cloneable class" {
					name = "java/lang/Cloneable"
					formal, actual = "Ljava/lang/Cloneable;", "[I"
				}
				if variant == "Serializable class" {
					name = "java/io/Serializable"
					formal, actual = "Ljava/io/Serializable;", "[I"
				}
				o, ok := stableResolve(name)
				if !ok {
					t.Fatal("original platform", name)
				}
				copy := *o
				switch variant {
				case "Object cycle":
					copy.SuperClass = copy.ThisClass
				case "Object interface":
					copy.AccessFlags |= 0x200
				case "Object final":
					copy.AccessFlags |= 0x10
				default:
					copy.AccessFlags &^= 0x200
				}
				resolve = func(n string) (*ClassObject, bool) {
					if n == name {
						return &copy, true
					}
					return stableResolve(n)
				}
			case "parameterized formal":
			case "graph budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			source, e := types.ParseDescriptor(formal)
			if e != nil {
				t.Fatal(e)
			}
			if variant == "nil source" {
				source = nil
			}
			if variant == "parameterized formal" {
				source = types.NewParameterizedType("p.Parent", []types.JavaType{types.NewJavaClass("java.lang.Integer")})
			}
			got := nativeAnonymousBridgeReturnSubstitutable(source, actual, resolve, work)
			if got != want {
				t.Fatalf("%s: implementation %s source %s known=%v want=%v", variant, actual, formal, got, want)
			}
			if strings.HasPrefix(variant, "unknown") && got {
				t.Fatal("missing original evidence granted a path")
			}
		})
	}
}
