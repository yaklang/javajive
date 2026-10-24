package javaclassparser

import (
	"context"
	"os"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A complete concrete Signature must erase to the actual target descriptor.
// Matching only its outer erased name would admit unbound variables, malformed
// nested arguments or a different invocation port.
func TestAdversarialInheritedBridgeRequiresClosedTargetSignature(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	fixture := inheritedVarargsBridgeFixture("java.lang.Class<?>", "java.lang.Class", "new java.lang.Class<?>[]{String.class,int.class}", "BridgeOuter")
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "BridgeOuter", "none", false)
	const descriptor = "(JD[Ljava/lang/Class;)Ljava/lang/String;"
	const signature = "(JD[Ljava/lang/Class<*>;)Ljava/lang/String;"
	for _, variant := range []string{"control", "plain signature", "matching throws", "empty", "malformed", "free variable", "nested free variable", "method formal", "different array rank", "different return", "primitive type argument", "throws mismatch", "duplicate", "invalid pool index", "oversized", "work budget", "allocation budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			child := mustParseLowering(t, files["BridgeOuter$Leaf.class"])
			parent := mustParseLowering(t, files["BridgeOuter.class"])
			var bridge, target *MemberInfo
			for _, m := range child.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			for _, m := range parent.Methods {
				name, _ := sourceBridgeUTF8(parent, m.NameIndex)
				desc, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
				if name == "read" && desc == descriptor {
					target = m
				}
			}
			if bridge == nil || target == nil {
				t.Fatal("actual original target")
			}
			var attr *SignatureAttribute
			for _, a := range target.Attributes {
				if s, ok := a.(*SignatureAttribute); ok {
					if attr != nil {
						t.Fatal("fixture duplicate signature")
					}
					attr = s
				}
			}
			if attr == nil {
				t.Fatal("fixture missing actual generic signature")
			}
			got, valid := sourceBridgeUTF8(parent, attr.SignatureIndex)
			if !valid || got != signature {
				t.Fatal("fixture signature", got)
			}
			altered := signature
			var work *workbudget.Budget
			switch variant {
			case "plain signature":
				altered = descriptor
			case "matching throws":
				altered += "^Ljava/io/IOException;"
			case "empty":
				altered = ""
			case "malformed":
				altered = signature + "garbage"
			case "free variable":
				altered = "(JDTT;)Ljava/lang/String;"
			case "nested free variable":
				altered = "(JD[Ljava/lang/Class<TT;>;)Ljava/lang/String;"
			case "method formal":
				altered = "<T:Ljava/lang/Object;>" + signature
			case "different array rank":
				altered = "(JD[[Ljava/lang/Class<*>;)Ljava/lang/String;"
			case "different return":
				altered = "(JD[Ljava/lang/Class<*>;)Ljava/lang/Object;"
			case "primitive type argument":
				altered = "(JD[Ljava/lang/Class<I>;)Ljava/lang/String;"
			case "throws mismatch":
				altered += "^Ljava/lang/RuntimeException;"
			case "duplicate":
				target.Attributes = append(target.Attributes, attr)
			case "oversized":
				for len(altered) <= 4096 {
					altered += "^Ljava/io/IOException;"
				}
			case "work budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "allocation budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			attr.SignatureIndex = uint16(parent.ConstantPoolManager.AddUtf8Info(altered))
			if variant == "invalid pool index" {
				attr.SignatureIndex = 0
			}
			d := NewClassObjectDumper(child)
			d.Work = work
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "BridgeOuter" {
					return parent.Bytes(), true
				}
				b, ok := files[name+".class"]
				return b, ok
			}
			want := variant == "control" || variant == "plain signature" || variant == "matching throws"
			if (d.inheritedBridgeOriginal(bridge) != nil) != want {
				t.Fatal("unproved target signature", variant)
			}
		})
	}
}
