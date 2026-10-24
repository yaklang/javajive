package javaclassparser

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const constructorParameterDomainFixture = `
class CtorEntryGraph {
 CtorEntryGraph(final long amount,int extra){}
 static class Nested {Nested(String key){}}
 static class Empty {Empty(){}}
 class Member {Member(String key){}}
 static Object local(String key){class Local {Local(String explicit){}}return new Local(key);}
 static Object anonymous(){return new Object(){};}
}
enum CtorEntryEnum { FIRST;CtorEntryEnum(){} }
`

func constructorNameGuardInput(t *testing.T, raw []byte) (*ClassObjectDumper, *MemberInfo, string) {
	t.Helper()
	object, err := Parse(slices.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name == "<init>" {
			descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
			return NewClassObjectDumper(object), method, descriptor
		}
	}
	t.Fatal("authored constructor missing")
	return nil, nil, ""
}

func TestAdversarialConstructorNamesRequireOriginalUnprojectedDomain(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, constructorParameterDomainFixture, "CtorEntryGraph", "none")
	for _, owner := range []string{"CtorEntryGraph", "CtorEntryGraph$Nested", "CtorEntryGraph$Member", "CtorEntryGraph$1Local", "CtorEntryGraph$1", "CtorEntryEnum"} {
		t.Run(owner, func(t *testing.T) {
			d, method, descriptor := constructorNameGuardInput(t, files[owner+".class"])
			names, known := d.originalOrdinaryMethodParameterNames(method, descriptor)
			want := owner == "CtorEntryGraph" || owner == "CtorEntryGraph$Nested"
			if !known || (names != nil) != want {
				t.Fatalf("constructor naming domain: %s names=%v known=%v ordinary=%v", owner, names, known, want)
			}
		})
	}
	for _, variant := range []string{"original", "nonstatic self", "enum class", "interface class", "synthetic class", "static constructor", "abstract constructor", "synthetic constructor", "wrong result", "local enclosing", "nil attribute", "nil inner table", "duplicate inner table", "wrong table length", "wrong table count", "nil row", "wrong CP self", "missing outer", "missing inner", "wrong inner name", "duplicate self", "unknown inner flags", "mandated formal", "synthetic formal", "duplicate declaration", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, method, descriptor := constructorNameGuardInput(t, files["CtorEntryGraph$Nested.class"])
			var table *InnerClassesAttribute
			var self *InnerClassInfo
			for _, a := range d.obj.Attributes {
				if a, ok := a.(*InnerClassesAttribute); ok {
					table = a
					for _, row := range a.Classes {
						name, _ := sourceBridgeClassName(d.obj, row.InnerClassInfoIndex)
						if name == "CtorEntryGraph$Nested" {
							self = row
						}
					}
				}
			}
			if self == nil || table == nil {
				t.Fatal("original static member self row")
			}
			switch variant {
			case "nonstatic self":
				self.InnerClassAccessFlags &^= 8
			case "enum class":
				d.obj.AccessFlags |= 0x4000
			case "interface class":
				d.obj.AccessFlags |= 0x0200
			case "synthetic class":
				d.obj.AccessFlags |= 0x1000
			case "static constructor":
				method.AccessFlags |= 8
			case "abstract constructor":
				method.AccessFlags |= 0x0400
			case "synthetic constructor":
				method.AccessFlags |= 0x1000
			case "wrong result":
				descriptor = "(Ljava/lang/String;)I"
				method.DescriptorIndex = sourceBridgePoolString(t, d.obj, descriptor)
			case "local enclosing":
				d.obj.Attributes = append(d.obj.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Length: 4, Info: []byte{0, 1, 0, 0}})
			case "nil attribute":
				d.obj.Attributes = append(d.obj.Attributes, (*UnparsedAttribute)(nil))
			case "nil inner table":
				d.obj.Attributes = append(d.obj.Attributes, (*InnerClassesAttribute)(nil))
			case "duplicate inner table":
				d.obj.Attributes = append(d.obj.Attributes, table)
			case "wrong table length":
				table.AttrLen++
			case "wrong table count":
				table.NumberOfClasses++
			case "nil row":
				table.Classes[0] = nil
			case "wrong CP self":
				self.InnerClassInfoIndex = self.InnerNameIndex
			case "missing outer":
				self.OuterClassInfoIndex = 0
			case "missing inner":
				self.InnerNameIndex = 0
			case "wrong inner name":
				self.InnerNameIndex = sourceBridgePoolString(t, d.obj, "Another")
			case "duplicate self":
				table.Classes = append(table.Classes, self)
				table.NumberOfClasses++
				table.AttrLen += 8
			case "unknown inner flags":
				self.InnerClassAccessFlags |= 0x0080
			case "mandated formal", "synthetic formal":
				for _, a := range method.Attributes {
					if a, ok := a.(*UnparsedAttribute); ok && a.Name == "MethodParameters" {
						flag := uint16(0x8000)
						if variant == "synthetic formal" {
							flag = 0x1000
						}
						binary.BigEndian.PutUint16(a.Info[3:5], flag)
					}
				}
			case "duplicate declaration":
				d.obj.Methods = append(d.obj.Methods, method)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			names, known := d.originalOrdinaryMethodParameterNames(method, descriptor)
			if allowed := known && names != nil; allowed != (variant == "original") {
				t.Fatalf("unproved constructor naming acquired physical formals: %s names=%v known=%v", variant, names, known)
			}
		})
	}
}

func TestAdversarialConstructorNameCacheClosesOriginalProjectionRole(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, constructorParameterDomainFixture, "CtorEntryGraph", "none")
	for _, variant := range []string{"nonstatic", "enclosing", "enum", "table", "budget"} {
		t.Run(variant, func(t *testing.T) {
			d, method, descriptor := constructorNameGuardInput(t, files["CtorEntryGraph$Nested.class"])
			names, known := d.originalOrdinaryMethodParameterNames(method, descriptor)
			if !known || !slices.Equal(names, []string{"key"}) {
				t.Fatal("original constructor names")
			}
			stamp, known := d.newMethodParameterNameCacheStamp(method, descriptor, names)
			if !known || stamp == nil || !d.methodParameterNameCacheClosed(stamp) {
				t.Fatal("original constructor name packet")
			}
			switch variant {
			case "nonstatic":
				for _, a := range d.obj.Attributes {
					if a, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range a.Classes {
							row.InnerClassAccessFlags &^= 8
						}
					}
				}
			case "enclosing":
				d.obj.Attributes = append(d.obj.Attributes, &UnparsedAttribute{Name: "EnclosingMethod"})
			case "enum":
				d.obj.AccessFlags |= 0x4000
			case "table":
				stamp.table.Info[0] = 0
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if d.methodParameterNameCacheClosed(stamp) {
				t.Fatal("constructor cache accepted changed physical/source projection", variant)
			}
		})
	}
}

func TestAdversarialEmptyConstructorNameCacheCannotLoseProjectionProof(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, constructorParameterDomainFixture, "CtorEntryGraph", "none")
	d, method, descriptor := constructorNameGuardInput(t, files["CtorEntryGraph$Empty.class"])
	// A legal, authored count=0 table is still a present naming requirement.
	// nil means a domain was not proved; equal arity zero cannot replace it.
	NewConstantPoolWithConstant(&d.obj.ConstantPool).AddUtf8Info("MethodParameters")
	method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: 1, Info: []byte{0}})
	names, known := d.originalOrdinaryMethodParameterNames(method, descriptor)
	if !known || names == nil || len(names) != 0 {
		t.Fatal("original legal empty constructor parameter table")
	}
	stamp, known := d.newMethodParameterNameCacheStamp(method, descriptor, names)
	if !known || stamp == nil || !d.methodParameterNameCacheClosed(stamp) {
		t.Fatal("original empty constructor naming packet")
	}
	d.obj.Attributes = append(d.obj.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Length: 4, Info: []byte{0, 1, 0, 0}})
	if d.methodParameterNameCacheClosed(stamp) {
		t.Fatal("equal zero arity replaced the lost physical/source naming proof")
	}
}
