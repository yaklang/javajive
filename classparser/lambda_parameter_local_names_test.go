package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialLocalLambdaNamesRequireFreshTypedSingleDefinition(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original local-capture metadata")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, localLambdaParameterFixture, "UncapturedLambdaNames", "none")
	for _, descriptor := range []string{"([II)I", "(J)J", "(Ljava/util/Map;Ljava/lang/Object;Ljava/util/List;)V"} {
		variants := []string{"original", "capture not final", "enclosing name collision", "missing store", "wrong load slot", "duplicate local write", "shared factory", "work", "memory", "canceled"}
		if descriptor == "(J)J" {
			variants = append(variants, "wide interior write")
		}
		for _, variant := range variants {
			t.Run(descriptor+"/"+variant, func(t *testing.T) {
				d, method, table, _ := lambdaParameterTestDumper(t, files["UncapturedLambdaNames.class"], descriptor)
				original, known := d.originalLambdaParameterMetadata(method, nil)
				if !known || original == nil || len(original.captures) != 1 || original.captures[0].local == nil {
					t.Fatal("original local capture was not proved")
				}
				read, code := original.captures[0].local, original.factory.code
				switch variant {
				case "capture not final":
					binary.BigEndian.PutUint16(table.Info[3:5], 0x1000)
				case "enclosing name collision":
					name := "initial"
					if descriptor[1] == 'L' {
						name = "source"
					}
					binary.BigEndian.PutUint16(table.Info[1:3], sourceBridgePoolString(t, d.obj, name))
				case "missing store":
					code.Code[read.storePC] = core.OP_POP
					if descriptor == "(J)J" {
						code.Code[read.storePC] = core.OP_POP2
					}
				case "wrong load slot":
					code.Code[read.pc] = core.OP_ALOAD_0
					if descriptor == "(J)J" {
						code.Code[read.pc] = core.OP_LLOAD_0
					}
				case "duplicate local write":
					prefix := []byte{core.OP_ACONST_NULL, core.OP_ASTORE, byte(read.slot)}
					if descriptor == "(J)J" {
						prefix = []byte{core.OP_LCONST_0, core.OP_LSTORE, byte(read.slot)}
					}
					code.Code = append(prefix, code.Code...)
				case "wide interior write":
					code.Code = append([]byte{core.OP_ICONST_0, core.OP_ISTORE, byte(read.slot + 1)}, code.Code...)
				case "shared factory":
					code.Code = append(bytes.Clone(code.Code[:len(code.Code)-1]), code.Code...)
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
				p, known := d.originalLambdaParameterMetadata(method, d.Work)
				if (known && p != nil) != (variant == "original") {
					t.Fatalf("unproved typed local acquired a source binder: %v, %v", p != nil, known)
				}
				if !bytes.Equal(before, d.obj.Bytes()) {
					t.Fatal("local capture naming query modified original bytes")
				}
			})
		}
	}
}

func TestAdversarialLocalLambdaFinalNamesRequireCompletedOriginalEmission(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original local-capture metadata")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, localLambdaParameterFixture, "UncapturedLambdaNames", "none")
	for _, variant := range []string{"original", "analysis context", "uncompleted frame", "changed event", "foreign owner", "lost name", "same spelling new identity", "changed seed", "missing witness", "wrong original store", "lost context name", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["UncapturedLambdaNames.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			if err = jdecenv.Run(map[string]string{}, func() error { _, err := d.DumpClass(); return err }); err != nil {
				t.Fatal("original completed source", err)
			}
			var binding *lambdaParameterBinding
			for _, receipt := range d.lambdaMethodSourceBodies {
				if receipt != nil && receipt.body != nil && receipt.body.lambdaParameterBinding != nil && receipt.body.lambdaParameterBinding.metadata.descriptor == "([II)I" {
					binding = receipt.body.lambdaParameterBinding
				}
			}
			if binding == nil || !binding.capturedNamesClosed(nil) {
				t.Fatal("original typed local and final source naming receipt")
			}
			capture, ok := binding.captured[0].(*values.JavaRef)
			if !ok || capture == nil || capture.Id == nil || binding.captureEmission == nil {
				t.Fatal("original captured source declaration")
			}
			var work *workbudget.Budget
			switch variant {
			case "analysis context":
				binding.captureEmission = nil
			case "uncompleted frame":
				binding.captureEmission.complete = false
			case "changed event":
				binding.captureEmission.children[0].packet += " /* changed */"
			case "foreign owner":
				copy := *binding.captureEmission.body
				copy.member = &MemberInfo{}
				binding.captureEmission.body = &copy
			case "lost name":
				binding.captureContext.LocalNames[capture.Id] = "another"
			case "same spelling new identity":
				capture.Id = coreutils.NewRootVariableId()
				capture.Id.SetName(binding.metadata.names[0])
				binding.captureContext.LocalNames[capture.Id] = binding.metadata.names[0]
			case "changed seed":
				capture.Val = values.NewJavaLiteral(nil, capture.Type())
			case "missing witness":
				binding.captured[0] = values.NewJavaRef(capture.Id, capture.Val, capture.Type())
			case "wrong original store":
				binding.metadata.captures[0].local.storePC++
			case "lost context name":
				delete(binding.captureContext.LocalNames, capture.Id)
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if known := binding.capturedNamesClosed(work); known != (variant == "original") {
				t.Fatal("unproved local identity/name acquired a completed factory emission", variant, known)
			}
		})
	}
}
