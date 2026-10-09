package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const independentRootProofFixture = `abstract class ProofParent{ProofParent(int expected){if(read()!=expected)throw new AssertionError("early root capture");}abstract int read();}interface ProofFactory{ProofParent make();}abstract class ObservingRootParent{ObservingRootParent(){marker();}abstract int marker();}class ProofOwner{static ProofFactory factory(final int token){return new ProofFactory(){int marker(){return token;}public ProofParent make(){return new ProofParent(token){int read(){return marker();}};}};}}`

func TestAdversarialAnonymousIndependentRootRequiresOriginalScope(t *testing.T) {
	base := nativeCompileIndependentRootFixture(t, "ProofOwner", independentRootProofFixture, "none", "7")
	for _, variant := range []string{"original", "nonfinal", "wrong owner", "owner identity", "instance method", "missing method", "duplicate method", "missing owner", "free field binder", "capture is ordinary field", "wrong capture store", "observing parent", "missing parent", "missing self row", "budget", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(base["ProofOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			owner, err := Parse(base["ProofOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			cp := NewConstantPoolWithConstant(&root.ConstantPool)
			switch variant {
			case "nonfinal":
				root.AccessFlags &^= 0x10
			case "wrong owner":
				for _, a := range root.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						binary.BigEndian.PutUint16(raw.Info, uint16(cp.AddNewClassInfo("AbsentOwner")))
					}
				}
			case "owner identity":
				owner.ThisClass = uint16(NewConstantPoolWithConstant(&owner.ConstantPool).AddNewClassInfo("ForeignOwner"))
			case "instance method", "missing method", "duplicate method":
				for _, m := range owner.Methods {
					name, _ := sourceBridgeUTF8(owner, m.NameIndex)
					if name == "factory" {
						switch variant {
						case "instance method":
							m.AccessFlags &^= 8
						case "missing method":
							m.NameIndex = uint16(NewConstantPoolWithConstant(&owner.ConstantPool).AddUtf8Info("other"))
						case "duplicate method":
							owner.Methods = append(owner.Methods, m)
						}
						break
					}
				}
			case "free field binder":
				root.Fields[0].Attributes = append(root.Fields[0].Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(cp.AddUtf8Info("TT;"))})
			case "capture is ordinary field":
				root.Fields[0].AccessFlags &^= 0x1000
			case "wrong capture store":
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "<init>" {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								if len(code.Code) < 3 || code.Code[2] != byte(core.OP_PUTFIELD) {
									t.Fatal("original capture store fixture")
								}
								code.Code[2] = byte(core.OP_PUTSTATIC)
							}
						}
					}
				}
			case "observing parent", "missing parent":
				parent := "ObservingRootParent"
				if variant == "missing parent" {
					parent = "AbsentParent"
				}
				root.SuperClass = uint16(cp.AddNewClassInfo(parent))
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "<init>" {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								for i := 0; i+2 < len(code.Code); i++ {
									if code.Code[i] == byte(core.OP_INVOKESPECIAL) {
										ref := nativeConstantMember(root.ConstantPool[binary.BigEndian.Uint16(code.Code[i+1:i+3])-1])
										if ref == nil {
											t.Fatal("constructor reference")
										}
										ref.ClassIndex = root.SuperClass
										break
									}
								}
							}
						}
					}
				}
			case "missing self row":
				for _, a := range root.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							name, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if name == root.GetClassName() {
								row.InnerNameIndex = uint16(cp.AddUtf8Info("Named"))
							}
						}
					}
				}
			}
			files := map[string][]byte{}
			for n, raw := range base {
				files[n] = raw
			}
			files["ProofOwner$1.class"] = root.Bytes()
			files["ProofOwner.class"] = owner.Bytes()
			if variant == "missing owner" {
				delete(files, "ProofOwner.class")
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			switch variant {
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			p := d.originalNativeAnonymousIndependentRoot()
			if (p != nil) != (variant == "original") {
				t.Fatalf("independent root admission=%v", p != nil)
			}
			if p != nil && (!p.validFor(d) || d.planNativeAnonymousGroup(nil, nil) != nil) {
				t.Fatal("certificate did not retain distinct anonymous source boundary")
			}
		})
	}
}

func TestAdversarialAnonymousIndependentRootCachedCertificateCannotReplaceOriginal(t *testing.T) {
	files := nativeCompileIndependentRootFixture(t, "ProofOwner", independentRootProofFixture, "none", "7")
	for _, variant := range []string{"original", "nil", "different object", "wrong lexical owner", "wrong lexical method", "changed original static context", "changed original capture"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["ProofOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			p := d.originalNativeAnonymousIndependentRoot()
			if p == nil {
				t.Fatal("baseline independent certificate")
			}
			switch variant {
			case "nil":
				p = nil
			case "different object":
				p.object, err = Parse(files["ProofOwner$1.class"])
				if err != nil {
					t.Fatal(err)
				}
			case "wrong lexical owner":
				p.lexicalOwner = "Foreign"
			case "wrong lexical method":
				p.method = "factory()LProofFactory;"
			case "changed original static context":
				owner, err := Parse(files["ProofOwner.class"])
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range owner.Methods {
					n, _ := sourceBridgeUTF8(owner, m.NameIndex)
					if n == "factory" {
						m.AccessFlags &^= 8
					}
				}
				prior := d.foldSiblingResolver
				d.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "ProofOwner" {
						return owner.Bytes(), true
					}
					return prior(name)
				}
			case "changed original capture":
				root.Fields[0].AccessFlags &^= 0x1000
			}
			if p.validFor(d) != (variant == "original") {
				t.Fatal("cached certificate replaced original proof")
			}
			plan := d.planNativeAnonymousOwnedGroup(nil, nil, p)
			if (plan != nil) != (variant == "original") {
				t.Fatal("cached certificate changed ownership admission")
			}
		})
	}
}

func TestAdversarialAnonymousIndependentRootRequiresArchiveClosure(t *testing.T) {
	files := nativeCompileIndependentRootFixture(t, "ProofOwner", independentRootProofFixture, "none", "7")
	root, err := Parse(files["ProofOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	d := z.nativeMemberReader(root)
	certificate := d.originalNativeAnonymousIndependentRoot()
	p := d.planNativeAnonymousOwnedGroup(nil, nil, certificate)
	p = d.validateNativeAnonymousGroup(p, nil, nil)
	if p == nil {
		t.Fatal("original closed group")
	}
	for _, variant := range []string{"closed", "external type user", "external handle", "missing index", "invalid index", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			index := &nativeMemberIndex{valid: true, handles: map[string]bool{}, typeUsers: map[string]map[string]bool{"ProofOwner$1$1": {"ProofOwner$1": true, "ProofOwner$1$1": true}}}
			var work *workbudget.Budget
			switch variant {
			case "external type user":
				index.typeUsers["ProofOwner$1$1"]["UnrelatedOwner"] = true
			case "external handle":
				index.handles["ProofOwner$1$1"] = true
			case "missing index":
				index = nil
			case "invalid index":
				index.valid = false
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if nativeAnonymousPrefixArchiveClosed(p, index, work) != (variant == "closed") {
				t.Fatal("archive closure admitted an unowned native type")
			}
		})
	}
}
