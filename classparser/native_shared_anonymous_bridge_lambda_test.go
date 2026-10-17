package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// The bridge transaction must compose only the same committed anonymous
// object's original factory. An alias or stale implementation cannot borrow
// the constructor marker's independent certificate.
func TestNativeSharedAnonymousBridgeLambdaRequiresOriginalFactory(t *testing.T) {
	files := nativeCompileClasses(t, sharedAnonymousBridgeLambdaFixture)
	for _, variant := range []string{"original", "cached site", "no forest", "foreign forest", "missing marker unit", "missing implementation", "ordinary alias", "unused dynamic alias", "condy alias", "deleted original factory", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["SharedMarkerOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			prepared := z.prepareNativeMemberFamilyUnpublished(root, nil)
			if prepared == nil {
				t.Fatal("complete original marker/lambda family")
			}
			p := prepared.family
			forest := p.anonymousForest
			if forest == nil {
				t.Fatal("joint forest")
			}
			child := forest.units["SharedMarkerOwner$1"]
			if child == nil || child.lambdaImplementation == nil {
				t.Fatal("same marker receiver")
			}
			object := child.object
			index := z.originalMemberIndex()
			var dynamic *ConstantInvokeDynamicInfo
			for _, constant := range object.ConstantPool {
				if v, ok := constant.(*ConstantInvokeDynamicInfo); ok {
					if dynamic != nil {
						t.Fatal("ambiguous original factory")
					}
					dynamic = v
				}
			}
			if dynamic == nil {
				t.Fatal("original factory missing")
			}
			var work *workbudget.Budget
			switch variant {
			case "cached site":
				for _, sites := range child.lambdaImplementation.lambdaContext.factorySites {
					for _, site := range sites {
						site.pc = 65535
					}
				}
			case "no forest":
				p.anonymousForest = nil
			case "foreign forest":
				forest.members = &nativeMemberFamily{}
			case "missing marker unit":
				delete(forest.units, object.GetClassName())
			case "missing implementation":
				child.lambdaImplementation = nil
			case "ordinary alias":
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: object.ThisClass, NameAndTypeIndex: dynamic.NameAndTypeIndex}})
			case "unused dynamic alias":
				copy := *dynamic
				object.ConstantPool = append(object.ConstantPool, &copy)
			case "condy alias":
				object.ConstantPool = append(object.ConstantPool, &ConstantDynamicInfo{BootstrapMethodAttrIndex: dynamic.BootstrapMethodAttrIndex, NameAndTypeIndex: dynamic.NameAndTypeIndex})
			case "deleted original factory":
				deleted := false
				for _, method := range object.Methods {
					for _, attribute := range method.Attributes {
						if code, ok := attribute.(*CodeAttribute); ok {
							decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
							if decoder.ParseOpcode() != nil {
								t.Fatal("original opcodes")
							}
							for _, op := range decoder.Opcodes() {
								if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
									code.Code[int(op.CurrentOffset)] = core.OP_NOP
									deleted = true
								}
							}
						}
					}
				}
				if !deleted {
					t.Fatal("original factory site")
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := z.nativeMemberJointBridgeReferencesClosed(p, index, work); got != (variant == "original" || variant == "cached site") {
				t.Fatalf("joint marker/lambda certificate %v", got)
			}
		})
	}
}
