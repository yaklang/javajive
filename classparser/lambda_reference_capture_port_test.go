package javaclassparser

import (
	"bytes"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The same verified pack body and SAM descriptors can implement a nullable
// captured arrow. Its original factory lacks an eager null check. Inspect that
// independent bytecode distinction rather than trusting a rec$ name or a
// current source renderer's choice not to inline the capture.
func TestAdversarialVarargsPackBodyCannotSupplyEagerReceiverCheck(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, nullableVarargsReferenceFixture, "NullableReferenceOwner", "none", false)
			obj, err := Parse(bytes.Clone(files["NullableReferenceOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			d.options.SourceCompiler, d.options.TargetSourceVersion = profile, 8
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "lazy" {
					method = m
					for _, a := range m.Attributes {
						if original, ok := a.(*CodeAttribute); ok {
							code = original
						}
					}
				}
			}
			if code == nil {
				t.Fatal("authored nullable factory")
			}
			desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
			d.CurrentMethod = method
			d.FuncCtx = &class_context.ClassContext{ClassName: obj.GetClassName(), FunctionName: "lazy", CurrentMethodDesc: desc, IsStatic: true}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("original opcodes")
			}
			pc := -1
			for _, op := range constructorMotionOps(decoder) {
				if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
					pc = int(op.CurrentOffset)
				}
			}
			request, ok := d.newLambdaMethodSourceRequest(1)
			if !ok {
				t.Fatal("request")
			}
			request.factory = &lambdaSourceFactory{object: obj, method: method, code: code, pc: pc}
			if !request.factory.seal(nil) {
				t.Fatal("factory seal")
			}
			site, ok := d.originalCompilerReferenceSite(request.factory)
			if !ok {
				t.Fatal("authored metafactory")
			}
			var implementation *MemberInfo
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == site.implementation.Member {
					implementation = m
				}
			}
			if _, known := d.originalVarargsAdapterShape(implementation); !known {
				t.Fatal("nullable arrow intentionally has the same ordered pack transfer")
			}
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("NullableVarargsTarget"))
			receiver.IsParam = true
			receiver.MarkOriginalParameter(0)
			before := bytes.Clone(code.Code)
			if p := d.proveCompilerVarargsReference(site.implementation.Member, site.implementation.Description, []values.JavaValue{receiver}, request, d.FuncCtx); p != nil {
				t.Fatal("a pack body invented an eager null check on a nullable lambda capture")
			}
			if !bytes.Equal(before, code.Code) {
				t.Fatal("proof modified original factory")
			}
		})
	}
}
