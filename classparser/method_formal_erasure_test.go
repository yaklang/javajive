package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"reflect"
	"testing"
)

func TestAdversarialMethodFormalErasureIsLexicallyScoped(t *testing.T) {
	for _, name := range []string{"T", "Renamed", "Outer$Token"} {
		t.Run(name, func(t *testing.T) {
			raw := map[string]bool{name: true, "Other": true}
			standalone := map[string]string{name: "java.lang.Number", "Other": "java.lang.Runnable"}
			forced := map[string]string{name: "java.lang.Object", "Other": "java.lang.Runnable"}
			ctx := &class_context.ClassContext{RawEraseTypeVars: raw, StandaloneEraseTypeVars: standalone, ForceParamEraseTypeVars: forced}
			end := maskMethodFormalErasure(ctx, []string{name})
			if ctx.RawEraseTypeVars[name] || ctx.StandaloneEraseTypeVars[name] != "" || ctx.ForceParamEraseTypeVars[name] != "" || !ctx.RawEraseTypeVars["Other"] || ctx.StandaloneEraseTypeVars["Other"] != "java.lang.Runnable" {
				t.Fatal("shadowed or unrelated binding changed")
			}
			if !raw[name] || standalone[name] != "java.lang.Number" || forced[name] != "java.lang.Object" {
				t.Fatal("class context aliased")
			}
			nested := maskMethodFormalErasure(ctx, []string{"Other"})
			if len(ctx.RawEraseTypeVars) != 0 {
				t.Fatal("nested shadow")
			}
			nested()
			if !ctx.RawEraseTypeVars["Other"] || ctx.RawEraseTypeVars[name] {
				t.Fatal("nested scope restoration")
			}
			end()
			if !reflect.DeepEqual(ctx.RawEraseTypeVars, raw) || !reflect.DeepEqual(ctx.StandaloneEraseTypeVars, standalone) || !reflect.DeepEqual(ctx.ForceParamEraseTypeVars, forced) {
				t.Fatal("sibling context leaked")
			}
		})
	}
	maskMethodFormalErasure(nil, []string{"T"})()
	maskMethodFormalErasure(&class_context.ClassContext{}, nil)()
}

func TestAdversarialExternalEnclosingErasureRequiresOriginalOwnership(t *testing.T) {
	files := nativeCompileClasses(t, `class ErasureOwner<T extends Number>{class Member<U extends CharSequence>{} static class Cut<V extends Runnable>{class Leaf{}}}class ErasureOwner$Literal<W>{}`)
	for _, row := range []struct{ name, variable, want string }{
		{"ErasureOwner$Member", "T", "java.lang.Number"},
		{"ErasureOwner$Member", "U", "java.lang.CharSequence"},
		{"ErasureOwner$Cut$Leaf", "V", "java.lang.Runnable"},
		{"ErasureOwner$Cut$Leaf", "T", ""},
		{"ErasureOwner$Literal", "T", ""},
		{"ErasureOwner$Literal", "W", "java.lang.Object"},
	} {
		t.Run(row.name+"/"+row.variable, func(t *testing.T) {
			d := NewClassObjectDumper(mustParseLowering(t, files[row.name+".class"]))
			d.declarationResolver = func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k }
			got, err := d.enclosingTypeParamErasures(map[string]bool{row.variable: true})
			if row.want == "" {
				if err == nil {
					t.Fatal("guessed hidden binder", got)
				}
				return
			}
			if err != nil || got[row.variable] != row.want {
				t.Fatal("original scope erasure", got, err)
			}
		})
	}
}
