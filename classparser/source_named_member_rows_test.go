package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialNamedMemberRowsRequireReciprocalReturnedDeclarations(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	original, raw, artifact := constructorSuffixFamilyRoundTrip(t, projectedOverrideSignatureFixture, "none", []string{"SuffixOwner", "SuffixOwner$Pair", "SuffixOwner$Task", "SuffixOwner$Sub"}, "12:projected:override:identity:binding\n")
	base := artifact.source.jar.sourceConstructorLowering
	for _, variant := range []string{"control", "idempotent", "opaque attribute", "unreturned owner", "pending owner", "stale owner", "unreturned child", "pending child", "wrong outer", "wrong name", "wrong flags", "anonymous replacement", "duplicate row", "original conflict", "fresh version", "fresh class identity", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			generic := map[string]constructorNativeSourceOrigin{}
			for k, v := range base.genericActive {
				generic[k] = v
			}
			origin := base.namedRowsActive["SuffixOwner"]
			a := &sourceConstructorLoweringAuthority{jar: base.jar, namedRowsActive: map[string]constructorNativeSourceOrigin{"SuffixOwner": origin}, genericActive: generic}
			obj, old := mustParseLowering(t, raw), bytes.Clone(original)
			var work *workbudget.Budget
			switch variant {
			case "unreturned owner":
				a.namedRowsActive = nil
			case "pending owner":
				a.namedRowsPending = a.namedRowsActive
				a.namedRowsActive = nil
			case "stale owner":
				o := mustParseLowering(t, old)
				o.ConstantPoolManager.AddUtf8Info("stale original")
				old = o.Bytes()
			case "unreturned child":
				delete(generic, "SuffixOwner$Sub")
			case "pending child":
				a.genericPending = map[string]constructorNativeSourceOrigin{"SuffixOwner$Sub": generic["SuffixOwner$Sub"]}
				delete(generic, "SuffixOwner$Sub")
			case "opaque attribute":
				obj.ConstantPoolManager.AddUtf8Info("OpaqueNamedData")
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "OpaqueNamedData", Length: 3, Info: []byte{2, 4, 8}})
			case "fresh class identity":
				obj.ThisClass = uint16(obj.ConstantPoolManager.AddNewClassInfo("ForeignOwner"))
			case "fresh version":
				obj.MinorVersion = 65535
			case "wrong outer", "wrong name", "wrong flags", "anonymous replacement", "duplicate row":
				var table *InnerClassesAttribute
				for _, attr := range obj.Attributes {
					if v, ok := attr.(*InnerClassesAttribute); ok {
						table = v
					}
				}
				if table == nil {
					obj.ConstantPoolManager.AddUtf8Info("InnerClasses")
					table = &InnerClassesAttribute{}
					obj.Attributes = append(obj.Attributes, table)
				}
				row := &InnerClassInfo{InnerClassInfoIndex: uint16(obj.ConstantPoolManager.AddNewClassInfo("SuffixOwner$Sub")), OuterClassInfoIndex: obj.ThisClass, InnerNameIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("Sub"))}
				switch variant {
				case "wrong outer":
					row.OuterClassInfoIndex = obj.SuperClass
				case "wrong name":
					row.InnerNameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("OtherSub"))
				case "wrong flags":
					row.InnerClassAccessFlags = StaticFlag
				case "anonymous replacement":
					row.OuterClassInfoIndex = 0
					row.InnerNameIndex = 0
				}
				table.Classes = append(table.Classes, row)
				if variant == "duplicate row" {
					table.Classes = append(table.Classes, row)
				}
				table.NumberOfClasses = uint16(len(table.Classes))
				table.AttrLen = uint32(2 + 8*len(table.Classes))
			case "original conflict":
				o := mustParseLowering(t, old)
				for _, attr := range o.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							name, _ := sourceBridgeClassName(o, row.InnerClassInfoIndex)
							if name == "SuffixOwner$Sub" {
								row.InnerNameIndex = uint16(o.ConstantPoolManager.AddUtf8Info("OtherSub"))
							}
						}
					}
				}
				old = o.Bytes()
				origin.originalHash = sha256.Sum256(old)
				a.namedRowsActive["SuffixOwner"] = origin
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			if variant == "idempotent" {
				var e error
				input, e = a.restoreNamedMemberRows(old, input, nil)
				if e != nil {
					t.Fatal(e)
				}
			}
			saved := bytes.Clone(input)
			out, e := a.restoreNamedMemberRows(old, input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("input modified")
			}
			switch variant {
			case "wrong outer", "wrong name", "wrong flags", "anonymous replacement", "duplicate row", "original conflict", "fresh version", "fresh class identity", "budget", "canceled":
				if e == nil || out != nil {
					t.Fatal("unproved rows accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if variant == "unreturned owner" || variant == "pending owner" || variant == "stale owner" {
				if !bytes.Equal(input, out) {
					t.Fatal("unpublished owner edited")
				}
				return
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			rows, e := sourceAnonymousInnerRows(after, nil)
			if e != nil {
				t.Fatal(e)
			}
			_, hasSub := rows["SuffixOwner$Sub"]
			if hasSub != (variant != "unreturned child" && variant != "pending child") {
				t.Fatal("unpublished child relation", hasSub)
			}
			if !reflect.DeepEqual(before.Methods, after.Methods) || !reflect.DeepEqual(before.Fields, after.Fields) || len(after.ConstantPool) < len(before.ConstantPool) || !reflect.DeepEqual(before.ConstantPool, after.ConstantPool[:len(before.ConstantPool)]) {
				t.Fatal("fresh Code, fields, or existing constants changed")
			}
			for _, attr := range before.Attributes {
				if _, table := attr.(*InnerClassesAttribute); table {
					continue
				}
				found := false
				for _, next := range after.Attributes {
					if reflect.DeepEqual(attr, next) {
						found = true
					}
				}
				if !found {
					t.Fatal("unrelated class attribute changed")
				}
			}
			if again, e := a.restoreNamedMemberRows(old, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("not idempotent", e)
			}
		})
	}
}
