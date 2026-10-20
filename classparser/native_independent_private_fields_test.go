package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestAdversarialIndependentPrivateFieldScopeRequiresOwnedImmutableInputs(t *testing.T) {
	fixture := strings.Replace(nativeIndependentStaticRootProofFixture, "final int base;", "private final int internal=17;final int base;", 1)
	files := nativeCompileIndependentRootFixture(t, "StaticProofOwner", fixture, "none", "8")
	for _, variant := range []string{"original", "foreign root", "missing resolver", "missing child", "child identity", "child version", "direct private field", "duplicate private declaration", "missing field name", "root changed during resolve", "scans", "memory", "canceled", "too many fields"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["StaticProofOwner$Capsule.class"]))
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(bytes.Clone(files["StaticProofOwner$Capsule$Worker.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			data := map[string][]byte{}
			for name, raw := range files {
				data[strings.TrimSuffix(name, ".class")] = bytes.Clone(raw)
			}
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				raw, ok := data[name]
				if variant == "root changed during resolve" {
					for _, field := range root.Fields {
						if text, _ := sourceBridgeUTF8(root, field.NameIndex); text == "internal" {
							field.AccessFlags &^= 2
						}
					}
				}
				return bytes.Clone(raw), ok
			}
			switch variant {
			case "foreign root":
				copy := *root
				d.obj = &copy
			case "missing resolver":
				d.foldSiblingResolver = nil
			case "missing child":
				delete(data, child.GetClassName())
			case "child identity":
				child.ThisClass = uint16(NewConstantPoolWithConstant(&child.ConstantPool).AddNewClassInfo("Foreign"))
				data["StaticProofOwner$Capsule$Worker"] = child.Bytes()
			case "child version":
				child.MajorVersion++
				data[child.GetClassName()] = child.Bytes()
			case "direct private field":
				for _, constant := range child.ConstantPool {
					ref, ok := constant.(*ConstantFieldrefInfo)
					if !ok {
						continue
					}
					owner, _ := sourceBridgeClassName(child, ref.ClassIndex)
					if owner != root.GetClassName() {
						continue
					}
					nt := child.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					name, _ := sourceBridgeUTF8(child, nt.NameIndex)
					if name == "base" {
						nt.NameIndex = sourceBridgePoolString(t, child, "internal")
					}
				}
				data[child.GetClassName()] = child.Bytes()
			case "duplicate private declaration":
				for _, field := range root.Fields {
					if field.AccessFlags&2 != 0 {
						copy := *field
						root.Fields = append(root.Fields, &copy)
						break
					}
				}
			case "missing field name":
				for _, field := range root.Fields {
					if field.AccessFlags&2 != 0 {
						field.NameIndex = 0
					}
				}
			case "scans":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "too many fields":
				for len(root.Fields) <= 4096 {
					root.Fields = append(root.Fields, root.Fields[0])
				}
			}
			known := d.nativeIndependentPrivateFieldsClosed(root)
			if known != (variant == "original") {
				t.Fatalf("private scope=%v expected=%v", known, variant == "original")
			}
		})
	}
}
