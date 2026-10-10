package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaReferenceDomainRequiresEveryOriginalReachingProducer(t *testing.T) {
	files := nativeCompileClasses(t, `
interface JoinedRefContract {}
class JoinedRefLeft implements JoinedRefContract {}
class JoinedRefRight implements JoinedRefContract {}
class JoinedRefWords {
 static JoinedRefContract branches(boolean choose,JoinedRefLeft first,JoinedRefRight second){final JoinedRefContract value;if(choose)value=first;else value=second;return value;}
 JoinedRefContract instance(long wide,boolean choose,JoinedRefLeft first,JoinedRefRight second){final JoinedRefContract value;if(choose)value=first;else value=second;return value;}
 static JoinedRefContract selected(boolean choose,JoinedRefLeft first,JoinedRefRight second){final JoinedRefContract value=choose?first:second;return value;}
 static JoinedRefContract nullable(boolean choose,JoinedRefLeft first){final JoinedRefContract value;if(choose)value=first;else value=null;return value;}
 static JoinedRefContract[] arrays(boolean choose,JoinedRefLeft[] first,JoinedRefRight[] second){final JoinedRefContract[] value;if(choose)value=first;else value=second;return value;}
}`)
	obj, err := Parse(files["JoinedRefWords.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		if name == "<init>" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
			arguments, result, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			params := map[int]string{}
			slot := 0
			if method.AccessFlags&StaticFlag == 0 {
				params[0], slot = "LJoinedRefWords;", 1
			}
			for _, argument := range arguments {
				params[slot] = argument
				slot++
				if argument == "J" || argument == "D" {
					slot++
				}
			}
			var code *CodeAttribute
			for _, attribute := range method.Attributes {
				if body, ok := attribute.(*CodeAttribute); ok {
					code = body
				}
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			pc := -1
			for _, op := range constructorMotionOps(decoder) {
				access := core.LocalAccessOf(op.Instr.OpCode)
				if access.Read && !access.Write && params[core.GetRetrieveIdx(op)] == "" {
					if pc != -1 {
						t.Fatal("fixture must have one nonparameter LOAD")
					}
					pc = int(op.CurrentOffset)
				}
			}
			if pc == -1 {
				t.Fatal("missing original LOAD")
			}
			flow := nativeEnumParameterOriginalFlow(decoder, code, nil)
			for _, variant := range []string{"original", "Object", "missing hierarchy", "one unknown arm", "one wrong arm", "incomplete parents", "wrong class identity", "wrong domain", "wrong LOAD PC", "primitive domain", "enum boundary", "extra domain", "budget", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					provider := func(name string) (callbinding.Class, bool) {
						parents, known := map[string][]string{"JoinedRefLeft": {"JoinedRefContract"}, "JoinedRefRight": {"JoinedRefContract"}, "JoinedRefContract": {}, "Rival": {}}[name]
						decl := callbinding.Class{Name: name, Parents: parents, ParentsComplete: known}
						if variant == "one unknown arm" && name == "JoinedRefLeft" {
							known = false
						}
						if variant == "one wrong arm" && name == "JoinedRefLeft" {
							decl.Parents = []string{"Rival"}
						}
						if variant == "incomplete parents" {
							decl.ParentsComplete = false
						}
						if variant == "wrong class identity" {
							decl.Name = "Rival"
						}
						return decl, known
					}
					domain := nativeLocalReferenceDomain{descriptors: map[int]string{pc: result}, metadata: provider}
					reader := NewClassObjectDumper(obj)
					primitive := true
					switch variant {
					case "Object":
						domain.descriptors[pc] = "Ljava/lang/Object;"
					case "missing hierarchy":
						domain.metadata = nil
					case "wrong domain":
						domain.descriptors[pc] = "LRival;"
					case "wrong LOAD PC":
						delete(domain.descriptors, pc)
						domain.descriptors[pc+1] = result
					case "primitive domain":
						domain.descriptors[pc] = "I"
					case "enum boundary":
						primitive = false
					case "extra domain":
						for i := 1000; i < 1065; i++ {
							domain.descriptors[i] = result
						}
					case "budget":
						reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						reader.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					reads, known := reader.nativeTypedLocalReads(method, code, flow, params, primitive, domain)
					read := reads[pc]
					want := variant == "original" || variant == "Object"
					if want != (known && read != nil && read.referenceAssignable && read.descriptor == domain.descriptors[pc]) {
						t.Fatalf("original reference admission=%v/%+v want=%v", known, read, want)
					}
				})
			}
		})
	}
}
