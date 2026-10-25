package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialAnonymousScopeRequiresExactEnclosingDeclaration(t *testing.T) {
	files := nativeCompileDebugClasses(t, anonymousOverrideSignatureFixture(), "none")
	for _, variant := range []string{"control", "missing owner", "foreign owner", "owner preview", "static method", "abstract method", "missing method", "duplicate method", "changed descriptor", "missing self row", "contradictory owner row", "named local", "missing enclosing", "duplicate enclosing", "initializer", "own formal", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			child := mustParseLowering(t, files["SuffixOwner$1.class"])
			owner := mustParseLowering(t, files["SuffixOwner.class"])
			var method *MemberInfo
			var enclosing *UnparsedAttribute
			for _, m := range owner.Methods {
				n, _ := sourceBridgeUTF8(owner, m.NameIndex)
				if n == "task" {
					method = m
				}
			}
			for _, attr := range child.Attributes {
				if raw, ok := attr.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
					enclosing = raw
				}
			}
			if method == nil || enclosing == nil {
				t.Fatal("fixture declarations")
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign owner":
				owner.ConstantPool[owner.ThisClass-1].(*ConstantClassInfo).NameIndex = uint16(owner.ConstantPoolManager.AddUtf8Info("AnotherOwner"))
			case "owner preview":
				owner.MinorVersion = 65535
			case "static method":
				method.AccessFlags |= StaticFlag
			case "abstract method":
				method.AccessFlags |= 0x400
			case "missing method":
				method.NameIndex = uint16(owner.ConstantPoolManager.AddUtf8Info("other"))
			case "duplicate method":
				owner.Methods = append(owner.Methods, method)
			case "changed descriptor":
				method.DescriptorIndex = uint16(owner.ConstantPoolManager.AddUtf8Info("(I)LSuffixOwner$Task;"))
			case "missing self row", "named local":
				for _, attr := range child.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(child, row.InnerClassInfoIndex)
							if n == child.GetClassName() {
								if variant == "named local" {
									row.InnerNameIndex = uint16(child.ConstantPoolManager.AddUtf8Info("Local"))
								} else {
									row.InnerClassInfoIndex = child.SuperClass
								}
							}
						}
					}
				}
			case "contradictory owner row":
				for _, attr := range owner.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(owner, row.InnerClassInfoIndex)
							if n == child.GetClassName() {
								row.InnerClassAccessFlags ^= 16
							}
						}
					}
				}
			case "missing enclosing":
				enclosing.Name = "OpaqueEnclosing"
			case "duplicate enclosing":
				child.Attributes = append(child.Attributes, enclosing)
			case "initializer":
				binary.BigEndian.PutUint16(enclosing.Info[2:], 0)
			case "own formal":
				for _, attr := range child.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						raw, _ := sourceBridgeUTF8(child, sig.SignatureIndex)
						sig.SignatureIndex = uint16(child.ConstantPoolManager.AddUtf8Info("<X:Ljava/lang/Object;>" + raw))
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d := NewClassObjectDumper(child)
			d.Work = work
			d.foldSiblingResolver = func(n string) ([]byte, bool) {
				if n == "SuffixOwner" {
					if variant == "missing owner" {
						return nil, false
					}
					return owner.Bytes(), true
				}
				b, k := files[n+".class"]
				return b, k
			}
			got, known := d.anonymousGenericScopeOwner()
			if known != (variant == "control") || known && got != "SuffixOwner" {
				t.Fatalf("owner=%q known=%v", got, known)
			}
		})
	}
}

func TestAdversarialAnonymousScopeCompletionPreservesFreshCode(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
	old, fresh, artifact := constructorSuffixFamilyRoundTrip(t, anonymousOverrideSignatureFixture(), "none", []string{"SuffixOwner$1", "SuffixOwner", "SuffixOwner$Pair", "SuffixOwner$Task"}, "12:projected:override:identity:binding\n")
	base := artifact.source.jar.sourceConstructorLowering
	for _, variant := range []string{"control", "opaque metadata", "pending", "stale original", "wrong bound", "wrong enclosing", "duplicate enclosing", "changed constructor", "wrong class", "wrong target", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar, genericActive: base.genericActive}
			original := bytes.Clone(old)
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var work *workbudget.Budget
			switch variant {
			case "opaque metadata":
				obj.ConstantPoolManager.AddUtf8Info("OpaqueLocalScope")
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "OpaqueLocalScope", Length: 3, Info: []byte{5, 7, 9}})
			case "pending":
				a.genericActive = nil
				a.genericPending = base.genericActive
			case "stale original":
				before := mustParseLowering(t, original)
				before.ConstantPoolManager.AddUtf8Info("stale")
				original = before.Bytes()
			case "wrong bound":
				for _, attr := range obj.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<K:Ljava/lang/Number;V:Ljava/lang/Object;>LSuffixOwner$Task<TV;>;"))
					}
				}
			case "wrong enclosing", "duplicate enclosing":
				before := mustParseLowering(t, old)
				var raw *UnparsedAttribute
				for _, attr := range before.Attributes {
					if a, ok := attr.(*UnparsedAttribute); ok && a.Name == "EnclosingMethod" {
						raw = a
					}
				}
				if raw == nil {
					t.Fatal("original edge")
				}
				obj.ConstantPoolManager.AddUtf8Info("EnclosingMethod")
				// Malformed pool links are a conflict too; completion may never overwrite them.
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Length: 4, Info: []byte{0, 0, 0, 0}})
				if variant == "duplicate enclosing" {
					obj.Attributes = append(obj.Attributes, obj.Attributes[len(obj.Attributes)-1])
				}
			case "changed constructor":
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == "<init>" {
						c := constructorLoweringCode(m)
						c.Code = bytes.Clone(c.Code)
						c.Code[0] = 0
					}
				}
			case "wrong class":
				obj.ConstantPool[obj.ThisClass-1].(*ConstantClassInfo).NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("Foreign"))
			case "wrong target":
				obj.MajorVersion = 53
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			saved := bytes.Clone(input)
			out, err := a.restoreMemberGenericScope(original, input, work)
			if !bytes.Equal(saved, input) {
				t.Fatal("input mutation")
			}
			decline := variant == "pending" || variant == "stale original"
			success := variant == "control" || variant == "opaque metadata"
			if decline {
				if err != nil || !bytes.Equal(out, input) {
					t.Fatal("unreturned receipt changed bytes", err)
				}
				return
			}
			if !success {
				if err == nil {
					t.Fatal("accepted changed premise")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := mustParseLowering(t, out)
			if !reflect.DeepEqual(got.ConstantPool[:len(obj.ConstantPool)], obj.ConstantPool) {
				t.Fatal("fresh constant prefix changed")
			}
			for i, m := range obj.Methods {
				if !reflect.DeepEqual(constructorLoweringCode(m), constructorLoweringCode(got.Methods[i])) {
					t.Fatal("fresh Code changed")
				}
			}
			for i, f := range obj.Fields {
				if !reflect.DeepEqual(f, got.Fields[i]) {
					t.Fatal("field changed")
				}
			}
			if again, e := a.restoreMemberGenericScope(original, out, nil); e != nil || !bytes.Equal(out, again) {
				t.Fatal("non-idempotent scope", e)
			}
			if variant == "opaque metadata" {
				found := false
				for _, attr := range got.Attributes {
					if a, ok := attr.(*UnparsedAttribute); ok && a.Name == "OpaqueLocalScope" {
						found = bytes.Equal(a.Info, []byte{5, 7, 9})
					}
				}
				if !found {
					t.Fatal("opaque metadata lost")
				}
			}
		})
	}
}

func TestAdversarialAnonymousCapturedScopeRejectsChangedPortEdges(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
	old, fresh, artifact := constructorSuffixFamilyRoundTrip(t, anonymousCapturedOverrideFixture(), "none", []string{"SuffixOwner$1", "SuffixOwner", "SuffixOwner$Pair", "SuffixOwner$Task"}, "12:projected:override:identity:binding\n")
	input, err := artifact.FinishClass("SuffixOwner$1.class", fresh)
	if err != nil {
		t.Fatal(err)
	}
	base := artifact.source.jar.sourceConstructorLowering
	for _, variant := range []string{"control", "capture field changed", "enclosing slot changed", "local slot changed", "duplicate constructor", "missing capture", "unrelated field"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(input))
			var ctor *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			packet := constructorLoweringPacketFor(obj, ctor, true, false, nil, 3)
			if packet == nil || len(packet.plan.captures) != 3 {
				t.Fatal("three independently typed capture ports")
			}
			code := constructorLoweringCode(ctor)
			switch variant {
			case "capture field changed":
				obj.Fields[1].NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("differentCapture"))
			case "enclosing slot changed":
				code.Code[1] = 0x2a
			case "local slot changed":
				// Decode the capture load and replace its slot with the enclosing one;
				// no field spelling or library name participates in the production proof.
				op := packet.ops[4]
				at := int(op.CurrentOffset)
				if op.Instr.OpCode >= 0x15 && op.Instr.OpCode <= 0x19 {
					code.Code[at+1] = 1
				} else {
					code.Code[at] = 0x2b
				}
			case "duplicate constructor":
				obj.Methods = append(obj.Methods, ctor)
			case "missing capture":
				code.Code[0] = 0
			case "unrelated field":
				obj.Fields = append(obj.Fields, obj.Fields[0])
			}
			raw := obj.Bytes()
			saved := bytes.Clone(raw)
			got, e := base.restoreMemberGenericScope(old, raw, nil)
			if !bytes.Equal(raw, saved) {
				t.Fatal("input mutation")
			}
			if variant == "control" {
				if e != nil || !bytes.Equal(got, raw) {
					t.Fatal("completed capture graph not idempotent", e)
				}
			} else if e == nil {
				t.Fatal("accepted altered capture graph")
			}
		})
	}
}
