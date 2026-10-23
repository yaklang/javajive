package javaclassparser

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestGenericFunctionalThrowsPublicationKeepsOriginalUse(t *testing.T) {
	genericFunctionalThrowsPublication(t, false)
}
func TestGenericFunctionalThrowsReceiverPublicationKeepsOriginalUse(t *testing.T) {
	genericFunctionalThrowsPublication(t, true)
}

func genericFunctionalThrowsPublication(t *testing.T, receiver bool) {
	t.Helper()
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original required")
	}
	source := `class FunctionalThrowUse {
 static final IllegalStateException failure=new IllegalStateException("same");
 static <E extends Throwable> long choose(long x,java.util.function.Supplier<? extends E> factory)throws E {if(x==17)throw factory.get();return x;}
 static long run(long x){return choose(x,()->failure);}
}`
	if receiver {
		source = strings.Replace(source, "return choose(x,()->failure);", "return new FunctionalThrowContainer<Long>(x==17?null:Long.valueOf(x)).take(()->failure).longValue();", 1)
		source += "\nclass FunctionalThrowContainer<T> {final T value; FunctionalThrowContainer(T value){this.value=value;} <E extends Throwable>T take(java.util.function.Supplier<? extends E> factory)throws E{if(value==null)throw factory.get();return value;} }\n"
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, source, "FunctionalThrowUse", "none", false)
	for _, variant := range []string{"control", "changed code", "changed frame", "changed target", "changed argument", "changed factory", "changed definition", "changed metadata", "missing call", "comment decoy", "duplicated call", "changed call text", "missing definition", "changed definition text", "definition comment", "duplicate definition", "changed source name", "changed receiver", "work exhausted", "storage exhausted", "missing body", "changed return"} {
		t.Run(variant, func(t *testing.T) {
			d := NewClassObjectDumper(mustParseLowering(t, files["FunctionalThrowUse.class"]))
			d.foldSiblingResolver = func(name string) ([]byte, bool) { v, ok := files[name+".class"]; return bytes.Clone(v), ok }
			d.options.TargetSourceVersion = 8
			source, err := d.DumpClass()
			if err != nil {
				t.Fatal(err, source)
			}
			var original *genericFunctionalThrowsScope
			for _, method := range d.dumpedMethodsSet {
				if method.methodName == "run" && method.genericFunctionalSource != nil {
					original = method.genericFunctionalSource
				}
			}
			if original == nil || strings.Contains(source, "jdec$rethrow") {
				t.Fatal("missing actual generic invocation receipt", source)
			}
			reader := *d
			reader.FuncCtx = original.sourceContext
			reader.CurrentMethod = original.method
			proof := *original
			proof.c = &reader
			if !proof.fresh() {
				t.Fatal("completed receipt stale")
			}
			var undo func()
			switch variant {
			case "work exhausted":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "storage exhausted":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "missing body":
				proof.body = nil
			case "changed return":
				old := proof.ret.JavaValue
				proof.ret.JavaValue = values.JavaNull
				undo = func() { proof.ret.JavaValue = old }
			case "changed code":
				old := proof.code.Code[0]
				proof.code.Code[0] ^= 1
				undo = func() { proof.code.Code[0] = old }
			case "changed frame":
				old := proof.code.MaxStack
				proof.code.MaxStack++
				undo = func() { proof.code.MaxStack = old }
			case "changed receiver":
				old := proof.call.Object
				proof.call.Object = values.JavaNull
				undo = func() { proof.call.Object = old }
			case "changed target":
				old := proof.call.FunctionName
				proof.call.FunctionName = "other"
				undo = func() { proof.call.FunctionName = old }
			case "changed argument":
				old := proof.call.Arguments[proof.argument]
				proof.call.Arguments[proof.argument] = values.JavaNull
				undo = func() { proof.call.Arguments[proof.argument] = old }
			case "changed factory":
				old := proof.factory.InstantiatedMtdDesc
				proof.factory.InstantiatedMtdDesc = "()Ljava/lang/Throwable;"
				undo = func() { proof.factory.InstantiatedMtdDesc = old }
			case "changed definition":
				old := proof.definition.JavaValue
				proof.definition.JavaValue = values.JavaNull
				undo = func() { proof.definition.JavaValue = old }
			case "changed source name":
				copy := *reader.FuncCtx
				reader.FuncCtx = &copy
				copy.LocalNames = map[*coreutils.VariableId]string{proof.ref.Id: "otherName"}
			case "changed metadata":
				prior := reader.FuncCtx.InvocationMetadata
				copy := *reader.FuncCtx
				reader.FuncCtx = &copy
				reader.FuncCtx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
					c, ok := prior(name)
					if name == "java/lang/IllegalStateException" {
						c.Parents = []string{"java/lang/Throwable"}
					}
					return c, ok
				}
			}
			if undo != nil {
				defer undo()
			}
			if strings.HasSuffix(variant, "exhausted") || variant == "missing body" || strings.HasPrefix(variant, "changed ") && variant != "changed call text" && variant != "changed definition text" {
				if proof.fresh() {
					t.Fatal("changed evidence remained accepted")
				}
				return
			}
			proof.finished = false
			proof.active = true
			proof.emitting = false
			proof.failed = false
			if len(proof.renders) != 1 {
				t.Fatal("unexpected render count", len(proof.renders))
			}
			r := proof.renders[0]
			packet := r.begin + r.source + r.end
			if strings.Count(proof.sourceBody, r.source) != 1 {
				t.Fatal("ambiguous retained call")
			}
			body := strings.Replace(proof.sourceBody, r.source, packet, 1)
			definition := proof.definitionRender
			defPacket := definition.begin + definition.source + definition.end
			if strings.Count(body, definition.source) != 1 {
				t.Fatal("ambiguous retained definition")
			}
			body = strings.Replace(body, definition.source, defPacket, 1)
			switch variant {
			case "missing definition":
				body = strings.Replace(body, defPacket, "", 1)
			case "changed definition text":
				body = strings.Replace(body, defPacket, definition.begin+definition.source+" "+definition.end, 1)
			case "definition comment":
				body = strings.Replace(body, defPacket, "//"+defPacket+"\n", 1)
			case "duplicate definition":
				body = strings.Replace(body, defPacket, defPacket+defPacket, 1)
			case "missing call":
				body = strings.Replace(body, packet, "0L", 1)
			case "comment decoy":
				body = strings.Replace(body, packet, "0L //"+packet+"\n", 1)
			case "duplicated call":
				body = strings.Replace(body, packet, packet+"+"+packet, 1)
			case "changed call text":
				body = strings.Replace(body, r.source, r.source+"+1L", 1)
			}
			finished, err := proof.finish(reader.FuncCtx, body)
			if variant == "control" {
				if err != nil || finished != original.sourceBody {
					t.Fatal("valid publication", err, finished)
				}
			} else if err == nil || finished != "" {
				t.Fatal("unproved publication accepted", variant)
			}
		})
	}
}
