package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMethodLocalConstructorMetadataKeepsGeneratedFlags(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class LocalMetaOwner{Object make(final long n){class Entry{long get(){return n;}}return new Entry();}static Object stat(final long n){class Node{long get(){return n;}}return new Node();}}`, debug)
			for _, variant := range []string{"original", "static actual absent table", "table length", "table count", "named hidden parameter", "enclosing synthetic instead of mandated", "capture mandated instead of synthetic", "duplicate table", "missing table", "missing signature", "duplicate signature", "wrong signature", "unknown attribute", "older class version", "budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					root, e := Parse(append([]byte(nil), files["LocalMetaOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					path := "LocalMetaOwner$1Entry.class"
					if variant == "static actual absent table" {
						path = "LocalMetaOwner$1Node.class"
					}
					obj, e := Parse(append([]byte(nil), files[path]...))
					if e != nil {
						t.Fatal(e)
					}
					owner, known := originalMethodLocalOwner(obj, root, nil)
					if !known {
						t.Fatal("actual method owner")
					}
					var ctor *MemberInfo
					var parameters *UnparsedAttribute
					var signature *SignatureAttribute
					for _, m := range obj.Methods {
						name, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if name == "<init>" {
							ctor = m
							for _, a := range m.Attributes {
								if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
									parameters = p
								}
								if s, ok := a.(*SignatureAttribute); ok {
									signature = s
								}
							}
						}
					}
					if ctor == nil || signature == nil || variant != "static actual absent table" && parameters == nil {
						t.Fatal("actual original compiler metadata")
					}
					desc, _ := sourceBridgeUTF8(obj, ctor.DescriptorIndex)
					params, _, err := callbinding.Descriptor(desc)
					if err != nil {
						t.Fatal(err)
					}
					var work *workbudget.Budget
					switch variant {
					case "table length":
						parameters.Length++
					case "table count":
						parameters.Info[0]--
					case "named hidden parameter":
						parameters.Info[1] = byte(obj.Fields[0].NameIndex >> 8)
						parameters.Info[2] = byte(obj.Fields[0].NameIndex)
					case "enclosing synthetic instead of mandated":
						parameters.Info[3] = 0x10
					case "capture mandated instead of synthetic":
						parameters.Info[7] = 0x80
					case "duplicate table":
						ctor.Attributes = append(ctor.Attributes, parameters)
					case "missing table", "missing signature":
						keep := []AttributeInfo{}
						for _, a := range ctor.Attributes {
							if variant == "missing table" && a == parameters || variant == "missing signature" && a == signature {
								continue
							}
							keep = append(keep, a)
						}
						ctor.Attributes = keep
					case "duplicate signature":
						ctor.Attributes = append(ctor.Attributes, signature)
					case "wrong signature":
						obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "(I)V"})
						signature.SignatureIndex = uint16(len(obj.ConstantPool))
					case "unknown attribute":
						ctor.Attributes = append(ctor.Attributes, &UnparsedAttribute{Name: "Opaque", Length: 0})
					case "older class version":
						obj.MajorVersion = 51
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					got := nativeMethodLocalConstructorParameters(obj, ctor, params, owner, true, work)
					if got != (variant == "original" || variant == "static actual absent table") {
						t.Fatalf("source regenerated metadata accepted=%v", got)
					}
					if variant == "missing table" || variant == "missing signature" || variant == "older class version" {
						if !nativeMethodLocalConstructorParameters(obj, ctor, params, owner, false, nil) {
							t.Fatal("source refusal must not erase an independently valid physical capture packet")
						}
					}
				})
			}
		})
	}
}
