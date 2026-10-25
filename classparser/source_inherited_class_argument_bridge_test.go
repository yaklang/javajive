package javaclassparser

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialInheritedBridgeClassArgumentsRequireOriginalScope(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, inheritedClassArgumentBridgeFixture("list", "C", "BridgeOuter"), "BridgeOuter", "none", false)
	for _, variant := range []string{"control", "renamed binder", "missing owner signature", "duplicate owner signature", "wrong physical parent", "free nested variable", "unused method formal", "method-dependent array", "free method bound", "bad bound grammar", "direct dependent port", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			child := mustParseLowering(t, files["BridgeOuter$Leaf.class"])
			owner := mustParseLowering(t, files["BridgeOuter.class"])
			var bridge, target *MemberInfo
			var classSig, methodSig *SignatureAttribute
			for _, m := range child.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			for _, m := range owner.Methods {
				name, _ := sourceBridgeUTF8(owner, m.NameIndex)
				if name == "read" && m.AccessFlags == 0x81 {
					target = m
				}
			}
			if bridge == nil || target == nil {
				t.Fatal("actual generic varargs declaration")
			}
			for _, a := range owner.Attributes {
				if s, ok := a.(*SignatureAttribute); ok {
					classSig = s
				}
			}
			for _, a := range target.Attributes {
				if s, ok := a.(*SignatureAttribute); ok {
					methodSig = s
				}
			}
			if classSig == nil || methodSig == nil {
				t.Fatal("actual class and target Signature")
			}
			cs, _ := sourceBridgeUTF8(owner, classSig.SignatureIndex)
			ms, _ := sourceBridgeUTF8(owner, methodSig.SignatureIndex)
			var work *workbudget.Budget
			switch variant {
			case "renamed binder":
				cs = strings.ReplaceAll(strings.Replace(cs, "<C:", "<Element:", 1), "TC;", "TElement;")
				ms = strings.ReplaceAll(ms, "TC;", "TElement;")
			case "missing owner signature":
				var kept []AttributeInfo
				for _, a := range owner.Attributes {
					if a != classSig {
						kept = append(kept, a)
					}
				}
				owner.Attributes = kept
			case "duplicate owner signature":
				owner.Attributes = append(owner.Attributes, classSig)
			case "wrong physical parent":
				cs = strings.Replace(cs, "LBridgeBase<TC;>;", "Ljava/lang/Object;", 1)
			case "free nested variable":
				ms = strings.ReplaceAll(ms, "TC;", "TUnbound;")
			case "unused method formal":
				// A declared but unused binder cannot change an invocation port.
				ms = "<Unused:Ljava/lang/Object;>" + ms
			case "method-dependent array":
				// Same List[] erasure, but inference can change the element type.
				ms = "<U::Ljava/util/List<TC;>;>(JD[TU;)Ljava/lang/String;"
			case "free method bound":
				ms = "<U:Ljava/lang/Comparable<TMissing;>;>" + ms
			case "bad bound grammar":
				cs = strings.ReplaceAll(cs, "Comparable<TC;>", "Comparable<I>")
			case "direct dependent port":
				cs = "<C::Ljava/util/List<Ljava/lang/String;>;>LBridgeBase<TC;>;"
				ms = "(JD[TC;)Ljava/lang/String;"
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			classSig.SignatureIndex = uint16(owner.ConstantPoolManager.AddUtf8Info(cs))
			methodSig.SignatureIndex = uint16(owner.ConstantPoolManager.AddUtf8Info(ms))
			d := NewClassObjectDumper(child)
			d.Work = work
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "BridgeOuter" {
					return owner.Bytes(), true
				}
				b, ok := files[name+".class"]
				return b, ok
			}
			if (d.inheritedBridgeOriginal(bridge) != nil) != (variant == "control" || variant == "renamed binder" || variant == "unused method formal") {
				t.Fatal("unproved declaring scope", variant)
			}
		})
	}
}
