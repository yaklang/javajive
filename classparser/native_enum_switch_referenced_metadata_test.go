package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeEnumSwitchReferencedDeclarationsRequireExactClosure(t *testing.T) {
	for _, deep := range []bool{false, true} {
		files := nativeCompileSourceReleaseClasses(t, nativeNestedEnumReferenceSources("MetadataTableOwner", deep), "none", "8")
		helper := ""
		for name, raw := range files {
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			if nativeEnumSwitchTableProof(obj, nil) != nil {
				if helper != "" {
					t.Fatal("ambiguous compiler artifact")
				}
				helper = name
			}
		}
		if helper == "" {
			t.Fatal("missing original helper")
		}
		for _, variant := range []string{"original", "nil packet", "nil object", "nil resolver", "nil table", "wrong owner", "missing enum", "nil enum", "foreign enum", "missing ancestor", "duplicate enum self", "duplicate declaration table", "missing enum self", "anonymous enum", "wrong reference flags", "wrong reference owner", "wrong reference name", "duplicate reference", "missing reference", "extra reference", "missing helper self", "duplicate helper table", "work", "memory", "canceled"} {
			t.Run(map[bool]string{false: "member", true: "nested"}[deep]+"/"+variant, func(t *testing.T) {
				objects := map[string]*ClassObject{}
				for name, raw := range files {
					obj, e := Parse(raw)
					if e != nil {
						t.Fatal(e)
					}
					objects[name[:len(name)-6]] = obj
				}
				obj := objects[helper[:len(helper)-6]]
				packet := nativeEnumSwitchTableProof(obj, nil)
				if packet == nil {
					t.Fatal("original executable certificate")
				}
				var enum string
				for _, array := range packet.tables {
					enum = array.enum
				}
				declaration := objects[enum]
				var own *InnerClassesAttribute
				var self *InnerClassInfo
				for _, a := range declaration.Attributes {
					if rows, ok := a.(*InnerClassesAttribute); ok {
						own = rows
						for _, row := range rows.Classes {
							name, _ := sourceBridgeClassName(declaration, row.InnerClassInfoIndex)
							if name == enum {
								self = row
							}
						}
					}
				}
				if own == nil || self == nil {
					t.Fatal("missing original declaration")
				}
				outer, _ := sourceBridgeClassName(declaration, self.OuterClassInfoIndex)
				var metadata *InnerClassesAttribute
				var reference int
				for _, a := range obj.Attributes {
					if rows, ok := a.(*InnerClassesAttribute); ok {
						metadata = rows
						for i, row := range rows.Classes {
							name, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if name == enum {
								reference = i
							}
						}
					}
				}
				if metadata == nil {
					t.Fatal("missing artifact metadata")
				}
				resolve := func(name string) (*ClassObject, bool) { v, ok := objects[name]; return v, ok }
				var work *workbudget.Budget
				switch variant {
				case "nil packet":
					packet = nil
				case "nil object":
					packet.object = nil
				case "nil resolver":
					resolve = nil
				case "nil table":
					for name := range packet.tables {
						packet.tables[name] = nil
					}
				case "wrong owner":
				case "missing enum":
					delete(objects, enum)
				case "nil enum":
					objects[enum] = nil
				case "foreign enum":
					objects[enum] = objects[outer]
				case "missing ancestor":
					delete(objects, outer)
				case "duplicate enum self":
					own.Classes = append(own.Classes, self)
				case "duplicate declaration table":
					declaration.Attributes = append(declaration.Attributes, own)
				case "missing enum self":
					for i, row := range own.Classes {
						if row == self {
							own.Classes = append(own.Classes[:i], own.Classes[i+1:]...)
							break
						}
					}
				case "anonymous enum":
					self.OuterClassInfoIndex = 0
					self.InnerNameIndex = 0
				case "wrong reference flags":
					metadata.Classes[reference].InnerClassAccessFlags ^= 1
				case "wrong reference owner":
					metadata.Classes[reference].OuterClassInfoIndex = obj.SuperClass
				case "wrong reference name":
					metadata.Classes[reference].InnerNameIndex = sourceBridgePoolString(t, obj, "Other")
				case "duplicate reference":
					metadata.Classes = append(metadata.Classes, metadata.Classes[reference])
				case "missing reference":
					metadata.Classes = append(metadata.Classes[:reference], metadata.Classes[reference+1:]...)
				case "extra reference":
					metadata.Classes = append(metadata.Classes, &InnerClassInfo{InnerClassInfoIndex: obj.SuperClass})
				case "missing helper self":
					for i, row := range metadata.Classes {
						name, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
						if name == obj.GetClassName() {
							metadata.Classes = append(metadata.Classes[:i], metadata.Classes[i+1:]...)
							break
						}
					}
				case "duplicate helper table":
					obj.Attributes = append(obj.Attributes, metadata)
				case "work":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				owner := "MetadataTableOwner"
				if variant == "wrong owner" {
					owner = "ForeignOwner"
				}
				if got := nativeEnumSwitchReferencedMetadata(packet, owner, resolve, work); got != (variant == "original") {
					t.Fatalf("original declaration closure admission=%v", got)
				}
			})
		}
	}
}
