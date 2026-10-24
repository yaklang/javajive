package javaclassparser

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

func TestCompilerReferencePrivateExclusionRequiresSeparateOriginalTopLevels(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"AccessCaller.java": `class AccessTarget {
 public void append(Object...items){} private void append(Object item){}
 static class Nested {public void append(Object...items){} private void append(Object item){}}
}
public class AccessCaller {static class Nested {}}`}, "none", "8")
	for _, variant := range []string{"separate top levels", "same owner", "nested caller", "nested target", "target nest host", "caller nest members", "missing original", "changed access", "unknown access", "omitted ordinary", "duplicate declaration", "forged nested permission"} {
		t.Run(variant, func(t *testing.T) {
			caller, target := "AccessCaller", "AccessTarget"
			if variant == "same owner" {
				caller = target
			}
			if variant == "nested caller" || variant == "forged nested permission" {
				caller += "$Nested"
			}
			if variant == "nested target" {
				target += "$Nested"
			}
			original := mustParseLowering(t, files[target+".class"])
			object := mustParseLowering(t, files[caller+".class"])
			if variant == "target nest host" {
				original.Attributes = append(original.Attributes, &UnparsedAttribute{Name: "NestHost", Length: 2, Info: []byte{0, 1}})
			}
			if variant == "caller nest members" {
				object.Attributes = append(object.Attributes, &UnparsedAttribute{Name: "NestMembers", Length: 2, Info: []byte{0, 0}})
			}
			d := NewClassObjectDumper(object)
			d.options.TargetSourceVersion, d.options.SourceCompiler = 8, ModernJavac
			d.foldSiblingResolver = func(n string) ([]byte, bool) {
				if n == target {
					return original.Bytes(), true
				}
				b, k := files[n+".class"]
				return b, k
			}
			base := d.buildInvocationMetadata()
			// Cache the original table independently of the later source-access
			// proof. Missing current bytes cannot grant a stale private exclusion.
			_, _ = base(target)
			if variant == "missing original" {
				d.foldSiblingResolver = func(string) ([]byte, bool) { return nil, false }
			}
			provider := func(n string) (callbinding.Class, bool) {
				cl, k := base(n)
				if n != target {
					return cl, k
				}
				cl.Methods = slices.Clone(cl.Methods)
				for i, m := range cl.Methods {
					if m.Name == "append" && m.Desc == "(Ljava/lang/Object;)V" {
						switch variant {
						case "changed access":
							cl.Methods[i].AccessFlags = 1
						case "unknown access":
							cl.Methods[i].AccessFlagsKnown = false
						case "forged nested permission":
							cl.Methods[i].SourceInaccessible = true
						}
					}
				}
				if variant == "omitted ordinary" {
					cl.Methods = slices.DeleteFunc(cl.Methods, func(m callbinding.Method) bool { return m.Name == "<init>" })
				}
				if variant == "duplicate declaration" {
					cl.Methods = append(cl.Methods, cl.Methods[0])
				}
				return cl, k
			}
			wrapped := d.compilerReferenceAccessProvider(provider)
			cl, known := wrapped(target)
			excluded := false
			if known {
				for _, m := range cl.Methods {
					if m.Name == "append" && m.Desc == "(Ljava/lang/Object;)V" {
						excluded = m.SourceInaccessible
					}
				}
			}
			want := variant == "separate top levels"
			if excluded != want {
				t.Fatalf("known=%v excluded=%v expected=%v", known, excluded, want)
			}
			plan := callbinding.SelectVariableArityReference(callbinding.Witness{Owner: target, Name: "append", Desc: "([Ljava/lang/Object;)V", Kind: callbinding.Virtual}, "(Ljava/lang/Object;)V", wrapped, nil)
			if plan.Supported != want {
				t.Fatalf("selection=%+v wanted=%v", plan, want)
			}
		})
	}
}
