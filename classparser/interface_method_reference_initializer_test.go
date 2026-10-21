package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialInterfaceMethodReferenceReceiptReopensOriginalFactoryAndTypedSource(t *testing.T) {
	compiler, _ := t04Tools(t)
	const fixture = `import java.util.function.*;
interface InterfaceReferenceReceipt {
 IntUnaryOperator WORD=value->value+1;
 Function<Object,String> FORMAT=InterfaceReferenceReceipt::format;
 static String format(Object value){return String.valueOf(value);}
}`
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, fixture, "InterfaceReferenceReceipt", "none", true)
	for _, scenario := range []string{"original", "lost references", "changed identity", "changed factory PC", "unsealed factory", "detached method", "opaque source", "changed source flag", "changed capture", "changed instantiated type", "changed original handle kind", "changed original member", "changed original erased SAM", "changed original instantiated SAM", "duplicate bootstrap table", "changed final reference", "work", "memory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["InterfaceReferenceReceipt.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			source, err := d.DumpClass()
			if err != nil {
				t.Fatal("original reference source", err)
			}
			methods, err := d.DumpMethods()
			p := d.interfaceLambdaInitializer
			if err != nil || p == nil || len(p.references) != 1 || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
				t.Fatal("original typed reference receipt", err)
			}
			r := p.references[0]
			site := object.ConstantPool[binary.BigEndian.Uint16(r.factory.code.Code[r.factory.pc+1:r.factory.pc+3])-1].(*ConstantInvokeDynamicInfo)
			var table *BootstrapMethodsAttribute
			for _, attribute := range object.Attributes {
				if candidate, ok := attribute.(*BootstrapMethodsAttribute); ok {
					table = candidate
				}
			}
			bootstrap := table.BootstrapMethods[site.BootstrapMethodAttrIndex]
			switch scenario {
			case "lost references":
				p.references = nil
			case "changed identity":
				r.identity.Member = "other"
			case "changed factory PC":
				r.factory.pc++
			case "unsealed factory":
				r.factory.sealed = false
			case "detached method":
				copy := *r.factory.method
				r.factory.method = &copy
			case "opaque source":
				v := values.NewCustomValue(func(*class_context.ClassContext) string { t.Fatal("opaque source callback ran"); return "" }, func() types.JavaType { return types.NewJavaClass("java.util.function.Function") })
				v.Flag, v.IsMethodRef, v.NoOuterCapture, v.CapturesKnown = "lambda", true, true, true
				v.OriginPC, v.HasOriginPC = r.factory.pc, true
				v.InstantiatedMtdDesc = r.identity.Instantiated
				v.MarkOriginalLambdaFactory(r.factory.pc)
				r.value = v
			case "changed source flag":
				r.value.IsMethodRef = false
			case "changed capture":
				r.value.NoOuterCapture = false
			case "changed instantiated type":
				r.value.InstantiatedMtdDesc = "()V"
			case "changed original handle kind":
				object.ConstantPool[bootstrap.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo).ReferenceKind = 5
			case "changed original member":
				h := object.ConstantPool[bootstrap.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
				ref := object.ConstantPool[h.ReferenceIndex-1].(*ConstantInterfaceMethodrefInfo)
				nt := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				object.ConstantPool[nt.NameIndex-1].(*ConstantUtf8Info).Value = "other"
			case "changed original erased SAM", "changed original instantiated SAM":
				index := 0
				if scenario == "changed original instantiated SAM" {
					index = 2
				}
				mt := object.ConstantPool[bootstrap.BootstrapArguments[index]-1].(*ConstantMethodTypeInfo)
				object.ConstantPool[mt.DescriptorIndex-1].(*ConstantUtf8Info).Value = "()V"
			case "duplicate bootstrap table":
				object.Attributes = append(object.Attributes, table)
			case "changed final reference":
				source = strings.Replace(source, "::format", "::other", 1)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := object.Bytes()
			got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods)
			if got != (scenario == "original") {
				t.Fatalf("interface original reference source closed=%v for %s", got, scenario)
			}
			if !bytes.Equal(before, object.Bytes()) {
				t.Fatal("reference closure changed original bytes")
			}
		})
	}
}
