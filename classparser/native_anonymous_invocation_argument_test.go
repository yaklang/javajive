package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousArgumentParentRequiresOriginalScopeAndInstructions(t *testing.T) {
	files := nativeCompileClasses(t, `class ArgumentTypeOwner<T>{java.util.function.Function<T,T> make(){return new java.util.function.Function<T,T>(){public T apply(T value){return value;}};}}`)
	for _, variant := range []string{"original", "foreign owner", "foreign query owner", "wrong method", "wrong method descriptor", "wrong constructor", "wrong new", "wrong invocation", "failed family", "foreign family", "missing child", "foreign child", "duplicate method", "duplicate Code", "missing Code", "physical NEW changed", "duplicate Signature", "own formals", "wrong parent", "extra interface", "deep Signature", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["ArgumentTypeOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeAnonymousFamily()
			if p == nil || p.children["ArgumentTypeOwner$1"] == nil {
				t.Fatal("original anonymous family")
			}
			d.nativeAnonymousRoot = p
			child := p.children["ArgumentTypeOwner$1"]
			owner, descriptor, name, methodDesc := child.object.GetClassName(), child.descriptor, "make", "()Ljava/util/function/Function;"
			newPC, pc := child.newPC, child.invokePC
			sourceOwner := root.GetClassName()
			var original *MemberInfo
			var code *CodeAttribute
			for _, m := range root.Methods {
				if n, _ := sourceBridgeUTF8(root, m.NameIndex); n == name {
					original = m
					for _, a := range m.Attributes {
						if body, ok := a.(*CodeAttribute); ok {
							code = body
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original method Code")
			}
			switch variant {
			case "foreign owner":
				owner = "Foreign$1"
			case "foreign query owner":
				sourceOwner = "Foreign"
			case "wrong method":
				name = "other"
			case "wrong method descriptor":
				methodDesc = "(I)Ljava/util/function/Function;"
			case "wrong constructor":
				descriptor = "()V"
			case "wrong new":
				newPC++
			case "wrong invocation":
				pc++
			case "failed family":
				p.failed = true
			case "foreign family":
				p.owner = "Foreign"
			case "missing child":
				delete(p.children, owner)
			case "foreign child":
				child.object = root
			case "duplicate method":
				root.Methods = append(root.Methods, original)
			case "duplicate Code":
				original.Attributes = append(original.Attributes, code)
			case "missing Code":
				original.Attributes = nil
			case "physical NEW changed":
				code.Code = append([]byte(nil), code.Code...)
				code.Code[newPC] = 0
			case "duplicate Signature", "own formals", "wrong parent", "deep Signature":
				for _, a := range child.object.Attributes {
					if sig, ok := a.(*SignatureAttribute); ok {
						if variant == "duplicate Signature" {
							child.object.Attributes = append(child.object.Attributes, sig)
						} else {
							text := "<U:Ljava/lang/Object;>Ljava/lang/Object;Ljava/util/function/Function<TU;TU;>;"
							if variant == "wrong parent" {
								text = "Ljava/lang/Object;Ljava/util/Comparator<TT;>;"
							}
							if variant == "deep Signature" {
								text = "Ljava/lang/Object;" + deepAnonymousArgumentSignature()
							}
							sig.SignatureIndex = sourceBridgePoolString(t, child.object, text)
						}
						break
					}
				}
			case "extra interface":
				child.object.Interfaces = append(child.object.Interfaces, child.object.Interfaces[0])
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			failed := p.failed
			got, known := d.nativeAnonymousArgumentParentSignature(owner, descriptor, sourceOwner, name, methodDesc, newPC, pc)
			if known != (variant == "original") || known && got != "Ljava/lang/Object;Ljava/util/function/Function<TT;TT;>;" {
				t.Fatalf("original parent certificate %s known=%v signature=%s", variant, known, got)
			}
			if p.failed != failed || d.nativeCaptureFailed {
				t.Fatal("read-only query committed a source failure")
			}
		})
	}
}

func deepAnonymousArgumentSignature() string {
	text := "TT;"
	for i := 0; i < 34; i++ {
		text = "Ljava/util/List<" + text + ">;"
	}
	return "Ljava/util/function/Function<" + text + "TT;>;"
}
