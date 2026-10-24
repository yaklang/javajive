package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialOrdinaryReferenceCounterClassifiesOriginalFactories(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Fatal("native Java 8 compiler required")
	}
	fixture := strings.Replace(localVarargsReceiverFixture, "public class LocalReceiverOwner {", `public class LocalReceiverOwner {
 public static java.util.function.ToIntFunction<String> before(){return String::length;}
 public static java.util.function.Function<Object,Object> after(){return ReferenceOperations::requireNonNull;}`, 1)
	fixture = strings.Replace(fixture, "$BODY", "LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);return receiver::append;", 1)
	fixture += `
class ReferenceOperations {public static <T>T requireNonNull(T value){if(value==null)throw new NullPointerException();return value;}}
`
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "LocalReceiverOwner", "none", true)
	obj := mustParseLowering(t, files["LocalReceiverOwner.class"])
	d := NewClassObjectDumper(obj)
	d.options.SourceCompiler, d.options.TargetSourceVersion = NativeJavac8, 8
	d.foldSiblingResolver = func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
	d.FuncCtx = &class_context.ClassContext{ClassName: obj.GetClassName()}
	d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
	for _, m := range obj.Methods {
		code := constructorLoweringCode(m)
		if code == nil {
			continue
		}
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
		if err := decoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		for _, op := range constructorMotionOps(decoder) {
			if op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
				continue
			}
			f := &lambdaSourceFactory{object: obj, method: m, code: code, pc: int(op.CurrentOffset)}
			if !f.seal(nil) {
				t.Fatal("factory seal")
			}
			id, ordinary := d.originalOrdinaryReferenceSite(f)
			if ordinary != (name != "make") {
				cl, known := d.FuncCtx.InvocationMetadata(id.Owner)
				t.Errorf("%s ordinary=%v identity=%+v metadata=%v name=%s complete=%v", name, ordinary, id, known, cl.Name, cl.MembersComplete)
			}
			if ordinary {
				provider := d.FuncCtx.InvocationMetadata
				d.FuncCtx.InvocationMetadata = nil
				if _, accepted := d.originalOrdinaryReferenceSite(f); accepted {
					t.Error("an unknown declaration borrowed a non-adapter role")
				}
				d.FuncCtx.InvocationMetadata = provider
			}
		}
	}
	if !d.compilerReferenceFlatScope() {
		t.Fatal("mixed ordinary/adapter scope not classified")
	}
}

func TestAdversarialMixedReferenceFinalSourceReopensOrdinaryOccurrences(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Fatal("native Java 8 compiler required")
	}
	fixture := strings.Replace(localVarargsReceiverFixture, "$BODY", `java.util.function.ToIntFunction<String> first=String::length;LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);Consumer<Object> action=receiver::append;java.util.function.ToIntFunction<String> second=String::length;if(first.applyAsInt("abc")!=3||second.applyAsInt("abcd")!=4)throw new AssertionError("ordinary reference");return action;`, 1)
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "LocalReceiverOwner", "none", true)
	for _, variant := range []string{"control", "comment is not reference", "lost occurrence", "reordered occurrences", "changed packet", "unfinished parent", "missing seal", "detached method", "changed factory PC", "changed value PC", "changed binding", "foreign source class", "foreign source method", "foreign source descriptor", "changed static context", "lost declaration metadata", "extra source occurrence", "changed final source", "work budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["LocalReceiverOwner.class"]))
			d := NewClassObjectDumper(obj)
			d.options = DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return bytes.Clone(b), ok }}
			d.foldSiblingResolver = d.options.Resolve
			var source string
			var methods []*dumpedMethods
			err := jdecenv.Run(map[string]string{}, func() error {
				var e error
				source, e = d.DumpClass()
				if e == nil {
					methods, e = d.DumpMethods()
				}
				return e
			})
			if err != nil || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
				t.Fatal("original mixed source did not close", err)
			}
			var frame *lambdaMethodEmission
			for _, method := range methods {
				if method.lambdaEmission != nil && len(method.lambdaEmission.references) == 2 {
					if frame != nil {
						t.Fatal("nonunique parent")
					}
					frame = method.lambdaEmission
				}
			}
			if frame == nil {
				t.Fatal("ordinary writer occurrences absent")
			}
			event := &frame.references[0]
			switch variant {
			case "comment is not reference":
				source = "/* Unknown::method */\n" + source
			case "lost occurrence":
				frame.references = frame.references[1:]
			case "reordered occurrences":
				frame.references[0], frame.references[1] = frame.references[1], frame.references[0]
			case "changed packet":
				event.packet = "Other::length"
			case "unfinished parent":
				frame.complete = false
			case "missing seal":
				frame.seal = nil
			case "detached method":
				m := *event.receipt.factory.method
				event.receipt.factory.method = &m
			case "changed factory PC":
				event.receipt.factory.pc++
			case "changed value PC":
				event.receipt.value.OriginPC++
			case "changed binding":
				event.receipt.identity.Member = "hashCode"
			case "foreign source class":
				event.context.ClassName = "Other"
			case "foreign source method":
				event.context.FunctionName = "other"
			case "foreign source descriptor":
				event.context.CurrentMethodDesc = "()V"
			case "changed static context":
				event.context.IsStatic = !event.context.IsStatic
			case "lost declaration metadata":
				d.FuncCtx.InvocationMetadata = nil
			case "extra source occurrence":
				source = strings.Replace(source, "String::length", "true ? String::length : String::length", 1)
			case "changed final source":
				source = strings.Replace(source, "String::length", "String::hashCode", 1)
			case "work budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "control" || variant == "comment is not reference"
			if got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); got != want {
				t.Fatalf("final mixed source=%v variant=%s", got, variant)
			}
		})
	}
}
