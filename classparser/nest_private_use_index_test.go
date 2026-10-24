package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialNestPrivateUseIndexNeedsEveryReciprocalPeer(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, compiler, ModernJavac, nestUnusedPrivateFixture, "ClosedPrivateOwner", "none", nil, 11, false)
	for _, variant := range []string{"closed", "different name", "different descriptor", "field reference", "symbolic subclass", "method reference", "method handle", "missing peer", "wrong peer", "missing host claim", "wrong host claim", "duplicate host claim", "wrong attribute length", "duplicate member", "bad member operand", "preview peer", "future peer", "work", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := modernNestTestObjects(t, files)
			host, peer := objects["ClosedPrivateOwner"], objects["ClosedPrivateOwner$Peer"]
			var field, method *MemberInfo
			for _, member := range host.Fields {
				if name, _ := sourceBridgeUTF8(host, member.NameIndex); name == "scale" {
					field = member
				}
			}
			for _, member := range host.Methods {
				if name, _ := sourceBridgeUTF8(host, member.NameIndex); name == "product" {
					method = member
				}
			}
			if field == nil || method == nil {
				t.Fatal("original declarations")
			}
			query := field
			cp := NewConstantPoolWithConstant(&peer.ConstantPool)
			switch variant {
			case "field reference", "symbolic subclass", "different name", "different descriptor", "bad member operand":
				owner, name, desc := "ClosedPrivateOwner", "scale", "J"
				if variant == "symbolic subclass" {
					owner = "AnyDerivedOwner"
				} else if variant == "different name" {
					name = "differentScale"
				} else if variant == "different descriptor" {
					desc = "I"
				}
				ref := &ConstantFieldrefInfo{ConstantMemberrefInfo: *cp.newMemberrefInfo(owner, name, desc)}
				if variant == "bad member operand" {
					ref.NameAndTypeIndex = 0
				}
				cp.AppendConstantInfo(ref)
			case "method reference", "method handle":
				query = method
				index := cp.AddNewMethodInfo("ClosedPrivateOwner", "product", "(J)J")
				if variant == "method handle" {
					cp.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: uint16(index)})
				}
			case "missing peer":
				delete(objects, peer.GetClassName())
			case "wrong peer":
				objects[peer.GetClassName()] = host
			case "missing host claim":
				modernNestTestRemoveAttribute(peer, "NestHost")
			case "wrong host claim":
				raw := modernNestTestAttribute(peer, "NestHost")
				binary.BigEndian.PutUint16(raw.Info, uint16(cp.AddNewClassInfo("DifferentHost")))
			case "duplicate host claim":
				peer.Attributes = append(peer.Attributes, modernNestTestAttribute(peer, "NestHost"))
			case "wrong attribute length":
				modernNestTestAttribute(peer, "NestHost").Length++
			case "duplicate member":
				raw := modernNestTestAttribute(host, "NestMembers")
				raw.Info = append(raw.Info, raw.Info[2:]...)
				binary.BigEndian.PutUint16(raw.Info, 2)
				raw.Length = uint32(len(raw.Info))
			case "preview peer":
				peer.MinorVersion = 65535
			case "future peer":
				peer.MajorVersion = 66
			}
			d := NewClassObjectDumper(host)
			d.declarationResolver = func(name string) ([]byte, bool) {
				object := objects[name]
				if object == nil {
					return nil, false
				}
				return object.Bytes(), true
			}
			switch variant {
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "allocation":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := bytes.Clone(host.Bytes())
			wantDemoted := variant != "closed" && variant != "different name" && variant != "different descriptor"
			if demoted := d.nestDemotePrivateMember(query); demoted != wantDemoted {
				t.Fatalf("widened=%v want %v", demoted, wantDemoted)
			}
			if !bytes.Equal(before, host.Bytes()) {
				t.Fatal("read-only use proof changed input")
			}
			if !wantDemoted && d.nestDemotePrivateMember(query) {
				t.Fatal("memoized immutable declaration proof changed")
			}
		})
	}
}
