package javaclassparser

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialInheritedBridgeAncestorRequiresClosedDeclarationGraph(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	fixture := strings.Replace(inheritedCovariantBridgeFixture, "static class Leaf", "static class Layer extends BridgeOuter {} static class Leaf", 1)
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "BridgeOuter", "none", false)
	insertAuthoredBridgeSuperclass(t, files, "BridgeOuter", "Layer")
	for _, variant := range []string{"control", "missing intermediary", "missing declaration", "foreign intermediary", "cycle", "duplicate declaration", "private shadow", "static shadow", "final shadow", "abstract shadow", "interface intermediary", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			child := mustParseLowering(t, files["BridgeOuter$Leaf.class"])
			middle := mustParseLowering(t, files["BridgeOuter$Layer.class"])
			parent := mustParseLowering(t, files["BridgeOuter.class"])
			var bridge, declaration *MemberInfo
			for _, m := range child.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			const descriptor = "(JDLjava/lang/Object;)Ljava/lang/String;"
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				d, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
				if n == "read" && d == descriptor {
					declaration = m
				}
			}
			if bridge == nil || declaration == nil {
				t.Fatal("original bridge and ancestor declaration")
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign intermediary":
				middle.ThisClass = uint16(middle.ConstantPoolManager.AddNewClassInfo("ForeignLayer"))
			case "cycle":
				middle.SuperClass = middle.ThisClass
			case "duplicate declaration":
				parent.Methods = append(parent.Methods, declaration)
			case "private shadow", "static shadow", "final shadow", "abstract shadow":
				flags := map[string]uint16{"private shadow": 2, "static shadow": 9, "final shadow": 17, "abstract shadow": 1025}
				middle.Methods = append(middle.Methods, &MemberInfo{AccessFlags: flags[variant], NameIndex: uint16(middle.ConstantPoolManager.AddUtf8Info("read")), DescriptorIndex: uint16(middle.ConstantPoolManager.AddUtf8Info(descriptor))})
			case "interface intermediary":
				middle.AccessFlags |= 0x0200
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d := NewClassObjectDumper(child)
			d.Work = work
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "BridgeOuter$Layer" {
					return middle.Bytes(), variant != "missing intermediary"
				}
				if name == "BridgeOuter" {
					return parent.Bytes(), variant != "missing declaration"
				}
				b, ok := files[name+".class"]
				return b, ok
			}
			if (d.inheritedBridgeOriginal(bridge) != nil) != (variant == "control") {
				t.Fatal("unproved ancestor declaration", variant)
			}
		})
	}
}
