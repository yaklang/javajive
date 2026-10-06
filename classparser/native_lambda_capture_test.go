package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestNativeLambdaCaptureNamesRequireDescriptorSeededWordOrigins(t *testing.T) {
	for _, mode := range []string{"static", "instance"} {
		source := `class CaptureWordOwner{int base;static java.util.function.Supplier<Object> make(final Object first,final long wide,final double fraction){return ()->first.toString()+wide+fraction;}}`
		if mode == "instance" {
			source = strings.Replace(source, "static java.util.function.Supplier", "java.util.function.Supplier", 1)
			source = strings.Replace(source, "first.toString()+", "base+first.toString()+", 1)
		}
		files := nativeCompileClasses(t, source)
		for _, variant := range []string{"original", "equal source names", "wide interior word", "wrong method context", "unregistered implementation", "missing capture tuple", "oversized capture tuple", "unseeded source flag", "receiver flag", "changed initial value"} {
			t.Run(mode+"/"+variant, func(t *testing.T) {
				root, err := Parse(append([]byte(nil), files["CaptureWordOwner.class"]...))
				if err != nil {
					t.Fatal(err)
				}
				c := NewClassObjectDumper(root)
				var name, desc string
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if strings.HasPrefix(n, "lambda$") {
						c.CurrentMethod = m
						name = n
						desc, _ = sourceBridgeUTF8(root, m.DescriptorIndex)
					}
				}
				if c.CurrentMethod == nil {
					t.Fatal("missing original implementation")
				}
				c.FuncCtx = &class_context.ClassContext{}
				c.FuncCtx.FunctionName = name
				c.FuncCtx.CurrentMethodDesc = desc
				c.lambdaMethods[name] = []string{desc}
				offset, word := 0, 0
				if mode == "instance" {
					offset, word = 1, 1
				}
				c.lambdaCaptureCount[name+desc] = 3 + offset
				for index, typ := range []types.JavaType{types.NewJavaClass("java.lang.Object"), types.NewJavaPrimer("long"), types.NewJavaPrimer("double")} {
					ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
					ref.IsParam = true
					if variant != "unseeded source flag" {
						ref.MarkOriginalParameter(word)
					}
					switch variant {
					case "equal source names":
						ref.Id.SetName("same")
					case "wide interior word":
						if index == 1 {
							ref = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
							ref.IsParam = true
							ref.MarkOriginalParameter(word + 1)
						}
					case "wrong method context":
						c.FuncCtx.FunctionName = "other"
					case "unregistered implementation":
						delete(c.lambdaMethods, name)
					case "missing capture tuple":
						delete(c.lambdaCaptureCount, name+desc)
					case "oversized capture tuple":
						c.lambdaCaptureCount[name+desc] = 100
					case "receiver flag":
						ref.IsThis = true
					case "changed initial value":
						ref.Val = values.NewJavaLiteral(1, types.NewJavaPrimer("int"))
					}
					got, valid := c.nativeLambdaParameterCaptureName(ref)
					want := variant == "original" || variant == "equal source names" || variant == "wide interior word" && index != 1
					if valid != want || valid && got != fmt.Sprintf("\x00LCAP%d\x00", index+offset) {
						t.Fatalf("descriptor word %d result=%q/%v", word, got, valid)
					}
					word++
					if index > 0 {
						word++
					}
				}
			})
		}
	}
}

func TestNativeLambdaCaptureBindingUsesDeclarationOriginThroughRefCopies(t *testing.T) {
	source := `class CaptureAliasOwner{static java.util.function.Supplier<Object> make(final Object first,final long wide,final double fraction){return ()->first.toString()+wide+fraction;}}`
	files := nativeCompileClasses(t, source)
	for _, variant := range []string{"original use", "copy without parameter flag", "equal name different declaration"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(append([]byte(nil), files["CaptureAliasOwner.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			c := NewClassObjectDumper(root)
			var name, desc string
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				if strings.HasPrefix(n, "lambda$") {
					c.CurrentMethod = m
					name = n
					desc, _ = sourceBridgeUTF8(root, m.DescriptorIndex)
				}
			}
			c.FuncCtx = &class_context.ClassContext{ClassName: "CaptureAliasOwner", FunctionName: name, CurrentMethodDesc: desc}
			c.lambdaMethods[name] = []string{desc}
			c.lambdaCaptureCount[name+desc] = 3
			typesList := []types.JavaType{types.NewJavaClass("java.lang.Object"), types.NewJavaPrimer("long"), types.NewJavaPrimer("double")}
			slots := []int{0, 1, 3}
			fields := map[string]int{"val$first": 0, "val$wide": 1, "val$fraction": 2}
			var params, args []values.JavaValue
			for i, typ := range typesList {
				decl := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
				decl.IsParam = true
				decl.MarkOriginalParameter(slots[i])
				decl.Id.SetName("same")
				params = append(params, decl)
				use := decl
				if variant == "copy without parameter flag" {
					copy := *decl
					copy.IsParam = false
					use = &copy
				}
				if variant == "equal name different declaration" {
					use = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
					use.Id.SetName("same")
				}
				args = append(args, use)
			}
			child := &nativeAnonymousClass{descriptor: "(Ljava/lang/Object;JD)V", method: name + desc, fields: fields}
			family := &nativeAnonymousFamily{owner: "CaptureAliasOwner", children: map[string]*nativeAnonymousClass{"CaptureAliasOwner$1": child}}
			c.nativeAnonymousRoot = family
			call := &values.FunctionCallExpression{ClassName: "CaptureAliasOwner$1", FunctionName: "<init>", Descriptor: child.descriptor, Arguments: args, OriginPC: 7, HasOriginPC: true}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("CaptureAliasOwner$1"), ConstructorCall: call, OriginPC: 3, HasOriginPC: true}
			body := []statements.Statement{&statements.ReturnStatement{JavaValue: allocation}}
			c.prepareNativeCaptureBindings(body, params)
			if family.failed != (variant == "equal name different declaration") {
				t.Fatalf("binding failed=%v", family.failed)
			}
			if family.failed {
				return
			}
			for i, arg := range args {
				ref := arg.(*values.JavaRef)
				if c.FuncCtx.SourceCaptureStable == nil || !c.FuncCtx.SourceCaptureStable(7, ref.Id) || c.FuncCtx.LocalNames[ref.Id] != fmt.Sprintf("\x00LCAP%d\x00", i) {
					t.Fatal("lost original declaration identity/word or borrowed equal spelling")
				}
			}
		})
	}
}
