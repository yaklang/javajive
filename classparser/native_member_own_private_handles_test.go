package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialOwnPrivateHandleRequiresExactOriginalLookupDeclarationAndCP(t *testing.T) {
	fixture := strings.Replace(nativeRootBridgeHandleSource("both"), "public static Object echo(Object value)", "private static Object echo(Object value)", 1)
	files := nativeCompileClasses(t, fixture)
	for _, scenario := range []string{"original", "foreign lookup", "interface tag", "different kind", "no source", "foreign source object", "no declaration resolver", "unresolved declaration", "same name different object", "no metadata", "incomplete metadata", "public metadata", "static metadata", "varargs metadata", "duplicate metadata", "copied member", "missing member", "duplicate member", "changed declaration", "generic declaration", "changed original handle", "changed original owner", "nested scope", "work", "memory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["BridgeHandleRoot.class"]))
			if err != nil {
				t.Fatal(err)
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			index := archive.originalMemberIndex()
			owner := root.GetClassName()
			d := archive.nativeMemberReader(root)
			source := &nativeMemberClass{object: root, lambdaContext: nativeLambdaImplementationContext{resolve: d.nativeAnnotationDeclarationResolver(), metadata: d.buildInvocationMetadata()}}
			var method *MemberInfo
			var target nativeMemberHandleTarget
			for _, candidate := range root.Methods {
				if name, _ := sourceBridgeUTF8(root, candidate.NameIndex); name == "echo" {
					method = candidate
				}
			}
			for _, candidate := range index.handleTargets[owner] {
				if candidate.name == "echo" {
					target = candidate
				}
			}
			if method == nil || target.name != "echo" || !nativeMemberOwnPrivateHandleSourceClosed(root, source, method, target, nil) {
				t.Fatal("original exact own-private lookup")
			}
			var work *workbudget.Budget
			switch scenario {
			case "foreign lookup":
				target.referencer = "Other"
			case "interface tag":
				target.methodRef = false
			case "different kind":
				target.kind = 7
			case "no source":
				source = nil
			case "foreign source object":
				copy := *root
				source.object = &copy
			case "no declaration resolver":
				source.lambdaContext.resolve = nil
			case "unresolved declaration":
				source.lambdaContext.resolve = func(string) (*ClassObject, bool) { return nil, false }
			case "same name different object":
				copy := *root
				source.lambdaContext.resolve = func(string) (*ClassObject, bool) { return &copy, true }
			case "no metadata":
				source.lambdaContext.metadata = nil
			case "incomplete metadata", "public metadata", "static metadata", "varargs metadata", "duplicate metadata":
				original := source.lambdaContext.metadata
				source.lambdaContext.metadata = func(name string) (callbinding.Class, bool) {
					cl, known := original(name)
					cl.Methods = append([]callbinding.Method(nil), cl.Methods...)
					if scenario == "incomplete metadata" {
						cl.MembersComplete = false
					}
					for i, method := range cl.Methods {
						if method.Name != "echo" {
							continue
						}
						switch scenario {
						case "public metadata":
							cl.Methods[i].Public = true
						case "static metadata":
							cl.Methods[i].Static = false
						case "varargs metadata":
							cl.Methods[i].Varargs = true
						case "duplicate metadata":
							cl.Methods = append(cl.Methods, method)
						}
					}
					return cl, known
				}
			case "copied member":
				copy := *method
				method = &copy
			case "missing member":
				var kept []*MemberInfo
				for _, candidate := range root.Methods {
					if candidate != method {
						kept = append(kept, candidate)
					}
				}
				root.Methods = kept
			case "duplicate member":
				root.Methods = append(root.Methods, method)
			case "changed declaration":
				method.AccessFlags = 9
			case "generic declaration":
				method.Attributes = append(method.Attributes, &SignatureAttribute{})
			case "changed original handle", "changed original owner":
				changed := false
				for _, constant := range root.ConstantPool {
					handle, ok := constant.(*ConstantMethodHandleInfo)
					if !ok || handle == nil {
						continue
					}
					member, known := nativeLambdaOriginalHandleTarget(root, handle)
					if !known || member.owner != owner || member.name != "echo" {
						continue
					}
					if scenario == "changed original handle" {
						handle.ReferenceKind = 5
					} else {
						root.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo).ClassIndex = root.SuperClass
					}
					changed = true
				}
				if !changed {
					t.Fatal("original private CP handle")
				}
			case "nested scope":
				root.Attributes = append(root.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Info: make([]byte, 4)})
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := root.Bytes()
			got := nativeMemberOwnPrivateHandleSourceClosed(root, source, method, target, work)
			if got != (scenario == "original") {
				t.Fatalf("private own lookup closed=%v for %s", got, scenario)
			}
			if !bytes.Equal(before, root.Bytes()) {
				t.Fatal("lookup proof modified original class")
			}
		})
	}
}
