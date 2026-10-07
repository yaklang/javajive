package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// A local can participate only under its exact original EnclosingMethod and
// capture packet. That scope does not relax the marker's symbolic-use closure.
func TestNativeMethodLocalPrivateBridgeRequiresExactOwnedScopeAndSymbols(t *testing.T) {
	files := nativeCompileClasses(t, nativeMethodLocalPrivateBridgeFixture)
	for _, scenario := range []string{"original", "absent local", "foreign local object", "missing lexical object", "missing root", "wrong method", "wrong capture PC", "failed family", "budget", "canceled", "foreign constructor owner", "fieldref alias", "interface alias", "method handle alias", "dynamic alias", "marker literal", "marker array"} {
		t.Run(scenario, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["OwnedLocalPrivateRoot.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original source family")
			}
			const user = "OwnedLocalPrivateRoot$1Entry"
			local := p.methodLocals[user]
			if local == nil {
				t.Fatal("original local")
			}
			obj := local.object
			allocations, known := z.nativeMemberReader(obj).nativeMemberAllocations(p)
			if !known {
				t.Fatal("original NEW packets")
			}
			index := z.originalMemberIndex()
			var work *workbudget.Budget
			var bridgeIndex int
			var ref *ConstantMethodrefInfo
			marker := ""
			for _, bridges := range p.bridgeOwners() {
				for _, bridge := range bridges {
					marker = bridge.marker
				}
			}
			for i, constant := range obj.ConstantPool {
				r, ok := constant.(*ConstantMethodrefInfo)
				if !ok {
					continue
				}
				nt := obj.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				descriptor, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
				if p.constructorBridges("OwnedLocalPrivateRoot$Value")[descriptor] != nil {
					bridgeIndex = i + 1
					ref = r
				}
			}
			if marker == "" || ref == nil {
				t.Fatal("original private bridge symbol")
			}
			scopeFailure := true
			switch scenario {
			case "original":
				scopeFailure = false
			case "absent local":
				delete(p.methodLocals, user)
			case "foreign local object":
				local.object, _ = Parse(append([]byte(nil), files[user+".class"]...))
			case "missing lexical object":
				delete(p.lexicalObjects, user)
			case "missing root":
				delete(p.lexicalObjects, p.owner)
			case "wrong method":
				local.owner.method = "foreign"
			case "wrong capture PC":
				local.constructor.capturePCs[local.constructor.enclosingField]++
			case "failed family":
				p.failed = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			default:
				scopeFailure = false
				switch scenario {
				case "foreign constructor owner":
					ref.ClassIndex = obj.ThisClass
				case "fieldref alias":
					obj.ConstantPool[bridgeIndex-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
				case "interface alias":
					obj.ConstantPool[bridgeIndex-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
				case "method handle alias":
					obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceIndex: uint16(bridgeIndex), ReferenceKind: 8})
				case "dynamic alias":
					obj.ConstantPool = append(obj.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: ref.NameAndTypeIndex})
				case "marker literal", "marker array":
					pool := NewConstantPoolWithConstant(&obj.ConstantPool)
					classIndex := pool.AddNewClassInfo(marker)
					changed := false
					for _, m := range obj.Methods {
						name, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if name != "eval" {
							continue
						}
						for _, attr := range m.Attributes {
							if code, ok := attr.(*CodeAttribute); ok {
								if code.Code[len(code.Code)-1] != byte(core.OP_LRETURN) {
									t.Fatal("original result")
								}
								addition := []byte{byte(core.OP_LDC_W), byte(classIndex >> 8), byte(classIndex), byte(core.OP_POP)}
								if scenario == "marker array" {
									addition = []byte{byte(core.OP_ICONST_0), byte(core.OP_ANEWARRAY), byte(classIndex >> 8), byte(classIndex), byte(core.OP_POP)}
								}
								prefix := append([]byte(nil), code.Code[:len(code.Code)-1]...)
								code.Code = append(append(prefix, addition...), byte(core.OP_LRETURN))
								code.MaxStack++
								changed = true
							}
						}
					}
					if !changed {
						t.Fatal("original result method")
					}
				}
			}
			if got := z.nativeMemberJointBridgeReferencesClosed(p, index, work); got != (scenario == "original") {
				t.Fatalf("marker scope/symbol closure=%v", got)
			}
			if scenario == "original" || scopeFailure {
				if got := nativeMemberJointBridgeCallersClosed(p, obj, allocations, work); got != (scenario == "original") {
					t.Fatalf("owned local caller=%v", got)
				}
			}
		})
	}
}
