package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"strings"
	"testing"
)

func TestAdversarialLambdaSourceFormalProjectionNeedsOriginalDeclaration(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("native8 original metadata oracle")
	}
	source := `import java.util.function.Predicate; public class UncapturedLambdaNames {public static GenericProfileContract factory(String captured){return active->active.test(captured);}}`
	extras := map[string]string{"GenericProfileContract.java": `import java.util.function.Predicate;public interface GenericProfileContract{boolean matches(Predicate<String> active);}`}
	files := compileLambdaParameterMetadataFixtureWithSources(t, compiler, NativeJavac8, source, "UncapturedLambdaNames", "none", extras, true)
	variants := []string{"original", "missing provider", "wrong owner", "not public", "not interface", "incomplete methods", "incomplete parents", "generic owner", "missing signature", "method variable", "unbound variable", "primitive argument", "wrong erasure", "malformed signature", "duplicate member", "static member", "bridge member", "wrong method name", "wrong factory", "wrong role", "changed physical type", "changed physical name", "work", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			d, method, _, _ := lambdaParameterTestDumper(t, files["UncapturedLambdaNames.class"], "(Ljava/lang/String;Ljava/util/function/Predicate;)Z")
			p, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || p == nil || !p.factory.seal(nil) {
				t.Fatal("original metadata/factory")
			}
			d.CurrentMethod = method
			d.FuncCtx = &class_context.ClassContext{ClassName: d.obj.GetClassName(), FunctionName: p.name, CurrentMethodDesc: p.descriptor, IsStatic: true, LocalNames: map[*coreutils.VariableId]string{}}
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("java.util.function.Predicate"))
			ref.IsParam = true
			ref.MarkOriginalParameter(p.samSlot())
			ref.Id.SetName(p.names[len(p.captures)])
			binding := &lambdaParameterBinding{metadata: p, refs: []*values.JavaRef{ref}, request: methodSourceRequest{role: methodSourceLambdaBody, serial: 1, captureCount: p.factoryCaptureCount(), factory: p.factory}}
			declaration := callbinding.Class{Name: "GenericProfileContract", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "matches", Desc: p.samDescriptor, Public: true, Signature: "(Ljava/util/function/Predicate<Ljava/lang/String;>;)Z"}}}
			d.FuncCtx.InvocationMetadata = func(name string) (callbinding.Class, bool) { return declaration, name == "GenericProfileContract" }
			switch variant {
			case "missing provider":
				d.FuncCtx.InvocationMetadata = nil
			case "wrong owner":
				declaration.Name = "Foreign"
			case "not public":
				declaration.Public = false
			case "not interface":
				declaration.IsInterface = false
			case "incomplete methods":
				declaration.MembersComplete = false
			case "incomplete parents":
				declaration.ParentsComplete = false
			case "generic owner":
				declaration.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			case "missing signature":
				declaration.Methods[0].Signature = ""
			case "method variable":
				declaration.Methods[0].Signature = "<T:Ljava/lang/Object;>(Ljava/util/function/Predicate<TT;>;)Z"
			case "unbound variable":
				declaration.Methods[0].Signature = "(Ljava/util/function/Predicate<TT;>;)Z"
			case "primitive argument":
				declaration.Methods[0].Signature = "(Ljava/util/function/Predicate<I>;)Z"
			case "wrong erasure":
				declaration.Methods[0].Signature = "(Ljava/util/List<Ljava/lang/String;>;)Z"
			case "malformed signature":
				declaration.Methods[0].Signature += "extra"
			case "duplicate member":
				declaration.Methods = append(declaration.Methods, declaration.Methods[0])
			case "static member":
				declaration.Methods[0].Static = true
			case "bridge member":
				declaration.Methods[0].Bridge = true
			case "wrong method name":
				declaration.Methods[0].Name = "foreign"
			case "wrong factory":
				f := *p.factory
				f.pc++
				binding.request.factory = &f
			case "wrong role":
				binding.request.role = methodSourceDeclaration
			case "changed physical type":
				ref.ResetVarType(types.NewJavaClass("java.lang.Object"))
			case "changed physical name":
				ref.Id.SetName("foreign")
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			rawBefore, _ := values.SourceTypeErasure(ref.Type(), d.FuncCtx)
			types, proved := d.originalLambdaSourceFormals(binding)
			if proved != (variant == "original") {
				t.Fatalf("projection=%v expected=%v", proved, variant == "original")
			}
			if proved && (len(types) != 1 || !strings.Contains(types[0].String(d.FuncCtx), "String")) {
				t.Fatal("missing concrete generic formal")
			}
			rawAfter, _ := values.SourceTypeErasure(ref.Type(), d.FuncCtx)
			if rawBefore != rawAfter || !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("source header changed physical original bindings")
			}
		})
	}
}
