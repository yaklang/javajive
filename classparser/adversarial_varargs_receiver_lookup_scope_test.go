package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

const varargsErasureLookupFixture = `import java.util.function.Function;
class ErasurePackProducer {public String apply(String...values){return "pack:"+String.valueOf(values[0]);}}
interface ErasureOther {String apply(String value);}
class ErasureBoth extends ErasurePackProducer implements ErasureOther {public String apply(String value){return "fixed:"+value;}}
public class ErasureReferenceOwner {
 static <X extends ErasurePackProducer & ErasureOther>Function<String,String> make(X receiver){return ((ErasurePackProducer)receiver)::apply;}
}
class ErasureReferenceDriver {
 public static void main(String[]args){Function<String,String>action=ErasureReferenceOwner.make(new ErasureBoth());for(String value:new String[]{null,"x"}){String got=action.apply(value);if(!got.equals("pack:"+String.valueOf(value)))throw new AssertionError("original source lookup cast lost");System.out.println(got);}}
}`

// The cast is erased from bytecode but fixes Java member lookup. A solver's
// concrete erasure view of the input cannot erase the declaration's additional
// intersection-bound overload. Until that source qualifier is certified, this
// representation must refuse, leaving the existing body recovery available.
func TestAdversarialVarargsReferenceCannotBorrowOnlyReceiverErasure(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, varargsErasureLookupFixture, "ErasureReferenceOwner", "none", true)
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "ErasureReferenceDriver"); got != "pack:null\npack:x\n" {
				t.Fatal("independent source lookup oracle", got)
			}
			object, err := Parse(bytes.Clone(files["ErasureReferenceOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options.SourceCompiler, d.options.TargetSourceVersion = profile, 8
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range object.Methods {
				name, _ := sourceBridgeUTF8(object, m.NameIndex)
				if name == "make" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original factory")
			}
			descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
			d.CurrentMethod = method
			d.FuncCtx = &class_context.ClassContext{ClassName: object.GetClassName(), FunctionName: "make", CurrentMethodDesc: descriptor, IsStatic: true}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("original opcodes")
			}
			pc := -1
			for _, op := range constructorMotionOps(decoder) {
				if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
					pc = int(op.CurrentOffset)
				}
			}
			request, known := d.newLambdaMethodSourceRequest(1)
			if !known {
				t.Fatal("request")
			}
			request.factory = &lambdaSourceFactory{object: object, method: method, code: code, pc: pc}
			if !request.factory.seal(nil) {
				t.Fatal("factory seal")
			}
			site, known := d.originalCompilerReferenceSite(request.factory)
			if !known {
				t.Fatal("original metafactory")
			}
			// Deliberately provide a solved erasure view. The original declaration,
			// rather than this inferred JavaRef type, must still constrain source lookup.
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("ErasurePackProducer"))
			receiver.IsParam = true
			receiver.MarkOriginalParameter(0)
			if proof := d.proveCompilerVarargsReference(site.implementation.Member, site.implementation.Description, []values.JavaValue{receiver}, request, d.FuncCtx); proof != nil {
				t.Fatal("receiver erasure concealed an original intersection lookup/cast obligation")
			}
		})
	}
}
