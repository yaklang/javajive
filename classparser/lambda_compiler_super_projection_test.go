package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestSuperReferenceProjectionRequiresCompleteConcreteOriginalDeclaration(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original javac8 required")
	}
	fixture := "interface GenericContract<A>{Object take(A value,java.io.IOException failure)throws java.io.IOException;}\n" + strings.ReplaceAll(superReferenceErasureFixture, "class GenericParent<T> {", "class GenericParent<T> implements GenericContract<T> {")
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "SuffixOwner", "none", false)
	for _, variant := range []string{"control", "raw superclass", "wrong instantiation", "unbound superclass", "missing superclass", "incomplete members", "incomplete parents", "provider identity", "signature identity", "method signature", "source competitor", "unknown generic rival", "changed erased input", "changed return", "missing ancestor", "ancestor signature identity", "ancestor incomplete", "ancestor flags", "ancestor edge removed", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(files["SuffixOwner.class"]))
			if variant == "raw superclass" || variant == "wrong instantiation" || variant == "unbound superclass" {
				signature := "LGenericParent;"
				if variant == "wrong instantiation" {
					signature = "LGenericParent<Ljava/lang/Integer;>;"
				}
				if variant == "unbound superclass" {
					signature = "LGenericParent<TMissing;>;"
				}
				for _, attr := range object.Attributes {
					if a, ok := attr.(*SignatureAttribute); ok {
						a.SignatureIndex = uint16(object.ConstantPoolManager.AddUtf8Info(signature))
					}
				}
			}
			d := NewClassObjectDumper(object)
			d.FuncCtx = &class_context.ClassContext{ClassName: object.GetClassName()}
			d.options.Resolve = func(name string) ([]byte, bool) {
				if variant == "missing ancestor" && name == "GenericContract" {
					return nil, false
				}
				if variant == "missing superclass" && name == "GenericParent" {
					return nil, false
				}
				b, known := files[name+".class"]
				return b, known
			}
			d.foldSiblingResolver = d.options.Resolve
			var shape *lambdaVarargsAdapter
			for _, method := range object.Methods {
				if method.AccessFlags == 0x1002 {
					shape = d.originalSuperReferenceShape(method)
				}
			}
			if shape == nil {
				t.Fatal("missing actual super-reference packet")
			}
			ctx := d.FuncCtx
			provider := d.buildInvocationMetadata()
			ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
				cl, known := provider(name)
				if name == "GenericContract" {
					cl.Methods = slices.Clone(cl.Methods)
					switch variant {
					case "ancestor signature identity":
						cl.Signature = "<A:Ljava/lang/Number;>Ljava/lang/Object;"
					case "ancestor incomplete":
						cl.MembersComplete = false
					case "ancestor flags":
						for i := range cl.Methods {
							cl.Methods[i].AccessFlags = 1
						}
					}
				}
				if name != "GenericParent" {
					return cl, known
				}
				cl.Methods = slices.Clone(cl.Methods)
				switch variant {
				case "incomplete members":
					cl.MembersComplete = false
				case "incomplete parents":
					cl.ParentsComplete = false
				case "provider identity":
					cl.Name = "OtherParent"
				case "signature identity":
					cl.Signature = "<T:Ljava/lang/String;>Ljava/lang/Object;"
				case "ancestor edge removed":
					cl.Parents = []string{"java/lang/Object"}
				}
				for i := range cl.Methods {
					m := &cl.Methods[i]
					if m.Name != "take" {
						continue
					}
					switch variant {
					case "method signature":
						m.Signature = "(Ljava/lang/String;Ljava/io/IOException;)Ljava/lang/Object;"
					case "changed erased input":
						m.Desc = "(Ljava/lang/String;Ljava/io/IOException;)Ljava/lang/Object;"
					case "changed return":
						m.Desc = "(Ljava/lang/Object;Ljava/io/IOException;)Ljava/lang/String;"
					case "source competitor", "unknown generic rival":
						rival := *m
						rival.Desc = "(Ljava/lang/String;Ljava/io/IOException;)Ljava/lang/Object;"
						rival.Signature, rival.Generic = "", variant == "unknown generic rival"
						cl.Methods = append(cl.Methods, rival)
					}
				}
				return cl, known
			}
			if variant == "budget" {
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			if variant == "canceled" {
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(cancelled, workbudget.Limits{})
			}
			plan := d.compilerSuperReferenceSelection(shape, compilerReferenceSite{actual: "(Ljava/lang/String;Ljava/io/IOException;)Ljava/lang/Object;"}, ctx)
			if plan.Supported != (variant == "control") {
				t.Fatalf("concrete super projection=%v: %s", plan.Supported, plan.Reason)
			}
			if plan.Supported && (plan.Selected.Desc != shape.member.Description || !slices.Equal(plan.ExpandedParameters, []string{"Ljava/lang/Object;", "Ljava/io/IOException;"})) {
				t.Fatal("source lookup changed the physical declaration or helper ports")
			}
		})
	}
}
