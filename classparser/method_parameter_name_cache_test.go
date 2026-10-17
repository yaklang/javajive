package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestAdversarialMethodParameterNameCacheRechecksOriginalPacket(t *testing.T) {
	files := methodParameterBoundaryCompile(t, methodParameterBoundaryFixture, "GuardOwner")
	for _, variant := range []string{"name", "MP final flag", "method flag", "major", "Code bytes", "Code limits", "Code exception", "duplicate method", "duplicate Code", "MP CP kind", "remove MP", "MP length", "replace member"} {
		t.Run(variant, func(t *testing.T) {
			d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
			cached, err := d.DumpMethod("read", "(I)I")
			if err != nil || cached == nil || cached.parameterNameStamp == nil {
				t.Fatal("original named packet", err)
			}
			before := d.obj.Bytes()
			method := cached.member
			var table *UnparsedAttribute
			var code *CodeAttribute
			for _, a := range method.Attributes {
				switch a := a.(type) {
				case *UnparsedAttribute:
					if a.Name == "MethodParameters" {
						table = a
					}
				case *CodeAttribute:
					code = a
				}
			}
			if table == nil || code == nil {
				t.Fatal("original MP/Code")
			}
			switch variant {
			case "name":
				binary.BigEndian.PutUint16(table.Info[1:3], sourceBridgePoolString(t, d.obj, "changedNamed"))
			case "MP final flag":
				binary.BigEndian.PutUint16(table.Info[3:5], 16)
			case "method flag":
				method.AccessFlags ^= 0x0020
			case "major":
				d.obj.MajorVersion++
			case "Code bytes":
				code.Code[0] = 0x03
			case "Code limits":
				code.MaxLocals++
			case "Code exception":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
			case "duplicate method":
				copy := *method
				d.obj.Methods = append(d.obj.Methods, &copy)
			case "duplicate Code":
				method.Attributes = append(method.Attributes, code)
			case "MP CP kind":
				index := binary.BigEndian.Uint16(table.Info[1:3])
				d.obj.ConstantPool[index-1] = &ConstantIntegerInfo{Value: 1}
			case "remove MP":
				for i, a := range method.Attributes {
					if a == table {
						method.Attributes = append(method.Attributes[:i], method.Attributes[i+1:]...)
						break
					}
				}
			case "MP length":
				table.Length++
			case "replace member":
				for i, m := range d.obj.Methods {
					if m == method {
						copy := *method
						d.obj.Methods[i] = &copy
					}
				}
			}
			if got, e := d.DumpMethod("read", "(I)I"); e == nil || got != nil {
				t.Fatal("stale ordinary named packet returned", variant, e)
			}
			if d.dumpedMethodsSet["name:read,desc:(I)I"] != cached {
				t.Fatal("stale rejection overwrote retained packet")
			}
			original, e := Parse(before)
			if e != nil {
				t.Fatal(e)
			}
			// Restore the exact original object packets and byte slices. A detached
			// equivalent object deliberately cannot impersonate the stamped owner.
			*d.obj = *original
			if got, e := d.DumpMethod("read", "(I)I"); e == nil || got != nil {
				t.Fatal("detached replacement acquired original packet identity")
			}
		})
	}
}

func TestAdversarialMethodParameterNameCacheRestoredInputAndBudget(t *testing.T) {
	files := methodParameterBoundaryCompile(t, methodParameterBoundaryFixture, "GuardOwner")
	d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
	cached, err := d.DumpMethod("read", "(I)I")
	if err != nil || cached == nil {
		t.Fatal(err)
	}
	code := cached.parameterNameStamp.code
	original := append([]byte(nil), code.Code...)
	code.Code[0] = 0x03
	if _, err := d.DumpMethod("read", "(I)I"); err == nil {
		t.Fatal("changed Code accepted")
	}
	copy(code.Code, original)
	if got, err := d.DumpMethod("read", "(I)I"); err != nil || got != cached {
		t.Fatal("exact original packet no longer reusable", err)
	}
	d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
	if got, err := d.DumpMethod("read", "(I)I"); err == nil || got != nil {
		t.Fatal("cache hit bypassed exhausted fresh-validation work")
	}
	if d.dumpedMethodsSet["name:read,desc:(I)I"] != cached {
		t.Fatal("validation budget failure overwrote original packet")
	}
	d.Work = nil
	if got, err := d.DumpMethod("read", "(I)I"); err != nil || got != cached {
		t.Fatal("fresh retry after validation budget failed", err)
	}
}
