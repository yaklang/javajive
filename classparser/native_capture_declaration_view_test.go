package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// These AST/metadata controls test declaration admission and atomic mutation.
// The adjacent authored round trips independently test JVM behavior and ABI.
func TestNativeCapturedDeclarationViewRequiresCompleteOriginalTypeAndIdentity(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"CaptureDeclaredOwner.java": captureDeclaredViewFixture3855}, "none", "8")
	for _, change := range []string{"same ID copy", "nil caller", "nil ref", "parameter", "this", "stack alias", "custom ref", "solved declaration", "nil declaration", "blank declaration", "wrong initializer", "reassigned", "branch escape", "missing field", "duplicate field", "wrong field descriptor", "ordinary field", "field Signature", "nil field", "unknown hierarchy", "wrong hierarchy identity", "incomplete hierarchy", "hierarchy cycle", "generic target", "malformed target Signature", "free target formal", "foreign inaccessible target", "malformed descriptor", "wrong copied type", "solved copy", "work", "memory", "depth", "canceled"} {
		t.Run(change, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			owner, _ := Parse(files["CaptureDeclaredOwner.class"])
			childObject, _ := Parse(files["CaptureDeclaredOwner$1.class"])
			d := z.nativeMemberReader(owner)
			d.FuncCtx = &class_context.ClassContext{ClassName: owner.GetClassName()}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			originalProvider := d.FuncCtx.InvocationMetadata
			actual, expected := "LCaptureDeclaredNarrow;", "LCaptureDeclaredWide;"
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("CaptureDeclaredNarrow"))
			initializer := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("CaptureDeclaredNarrow"))
			initializer.IsParam = true
			declaration := statements.NewAssignStatement(ref, initializer, true)
			copy := *ref
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("CaptureDeclaredOwner$1"), OriginPC: 3, HasOriginPC: true, ConstructorCall: &values.FunctionCallExpression{ClassName: "CaptureDeclaredOwner$1", FunctionName: "<init>", Descriptor: "(LCaptureDeclaredWide;)V", Arguments: []values.JavaValue{&copy}, OriginPC: 7, HasOriginPC: true}}
			body := []statements.Statement{declaration, &statements.ReturnStatement{JavaValue: allocation}}
			child := &nativeAnonymousClass{object: childObject}
			var field *MemberInfo
			for _, f := range childObject.Fields {
				if n, _ := sourceBridgeUTF8(childObject, f.NameIndex); n == "val$addresses" {
					field = f
				}
			}
			if field == nil {
				t.Fatal("original capture field")
			}
			switch change {
			case "nil caller":
				d.obj = nil
			case "nil ref":
				ref = nil
			case "parameter":
				ref.IsParam = true
			case "this":
				ref.IsThis = true
			case "stack alias":
				ref.StackVar = initializer
			case "custom ref":
				ref.CustomValue = &values.CustomValue{}
			case "solved declaration":
				ref.WebDeclType = types.NewJavaClass("CaptureDeclaredNarrow")
			case "nil declaration":
				declaration = nil
			case "blank declaration":
				declaration.JavaValue = nil
			case "wrong initializer":
				declaration.JavaValue = values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			case "reassigned":
				body = append(body, statements.NewAssignStatement(ref, initializer, false))
			case "branch escape":
				body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{declaration}}, body[1]}
			case "missing field":
				childObject.Fields = nil
			case "duplicate field":
				childObject.Fields = append(childObject.Fields, field)
			case "wrong field descriptor":
				field.DescriptorIndex = sourceBridgePoolString(t, childObject, actual)
			case "ordinary field":
				field.AccessFlags &^= 0x1000
			case "field Signature":
				field.Attributes = append(field.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, childObject, expected)})
			case "nil field":
				childObject.Fields = append(childObject.Fields, nil)
			case "unknown hierarchy", "wrong hierarchy identity", "incomplete hierarchy", "hierarchy cycle", "generic target", "malformed target Signature", "free target formal", "foreign inaccessible target":
				d.FuncCtx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
					v, known := originalProvider(name)
					if name == "CaptureDeclaredNarrow" {
						switch change {
						case "unknown hierarchy":
							return callbinding.Class{}, false
						case "wrong hierarchy identity":
							v.Name = "Foreign"
						case "incomplete hierarchy":
							v.ParentsComplete = false
						case "hierarchy cycle":
							v.Parents = []string{name}
						}
					}
					if name == "CaptureDeclaredWide" {
						switch change {
						case "generic target":
							v.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
						case "malformed target Signature":
							v.Signature = "bad"
						case "free target formal":
							v.Signature = "Ljava/util/ArrayList<TT;>;"
						}
					}
					return v, known
				}
				if change == "foreign inaccessible target" {
					owner.ConstantPool[owner.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, owner, "foreign/Caller")
				}
			case "malformed descriptor":
				expected = "LCaptureDeclaredWide"
			case "wrong copied type":
				copy.ResetVarType(types.NewJavaClass("java.lang.Object"))
			case "solved copy":
				copy.WebDeclType = types.NewJavaClass("CaptureDeclaredNarrow")
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "depth":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			beforeRef, beforeCopy := "", ""
			if ref != nil {
				beforeRef, _ = values.SourceTypeErasure(ref.Type(), d.FuncCtx)
			}
			beforeCopy, _ = values.SourceTypeErasure(copy.Type(), d.FuncCtx)
			good := change == "same ID copy"
			if got := d.nativeCaptureWidenedDeclaration(body, ref, declaration, child, allocation, "val$addresses", actual, expected); got != good {
				t.Fatalf("admitted=%v expected=%v", got, good)
			}
			if good {
				for _, r := range []*values.JavaRef{ref, &copy} {
					if erased, known := values.SourceTypeErasure(r.WebDeclType, d.FuncCtx); !known || erased != expected {
						t.Fatal("not all identity copies received the declaration view")
					}
				}
				if erased, _ := values.SourceTypeErasure(initializer.Type(), d.FuncCtx); erased != actual {
					t.Fatal("initializer type changed")
				}
			} else {
				if ref != nil {
					if erased, _ := values.SourceTypeErasure(ref.Type(), d.FuncCtx); erased != beforeRef {
						t.Fatal("rejected proof mutated the source ref")
					}
				}
				if erased, _ := values.SourceTypeErasure(copy.Type(), d.FuncCtx); erased != beforeCopy {
					t.Fatal("rejected proof mutated an identity copy")
				}
			}
		})
	}
}
