package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberConstructorLexicalArgumentsRequireOriginalBinderAndErasure(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberLexicalArgumentFixture)
	for _, variant := range []string{"original", "missing owner", "copied child", "static child", "generic child", "overloaded child", "duplicate constructor", "missing signature", "duplicate signature", "malformed signature", "generic constructor", "foreign signature variable", "forged source descriptor", "missing current method", "current method shadow", "enclosing method shadow", "duplicate enclosing method", "wrong erasure", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for _, raw := range files {
				object, err := Parse(bytes.Clone(raw))
				if err != nil {
					t.Fatal(err)
				}
				objects[object.GetClassName()] = object
			}
			entry := objects["LexicalEntryOwner$Entry"]
			root := objects["LexicalEntryOwner"]
			caller := objects["LexicalEntryOwner$1"]
			var constructor, enclosing, called *MemberInfo
			descriptor := ""
			for _, method := range entry.Methods {
				name, _ := sourceBridgeUTF8(entry, method.NameIndex)
				if name == "<init>" {
					constructor = method
					descriptor, _ = sourceBridgeUTF8(entry, method.DescriptorIndex)
				}
			}
			for _, method := range root.Methods {
				name, _ := sourceBridgeUTF8(root, method.NameIndex)
				if name == "make" {
					enclosing = method
				}
			}
			for _, method := range caller.Methods {
				name, _ := sourceBridgeUTF8(caller, method.NameIndex)
				desc, _ := sourceBridgeUTF8(caller, method.DescriptorIndex)
				if name == "call" && desc == "()LLexicalEntryOwner$Entry;" {
					called = method
				}
			}
			if constructor == nil || enclosing == nil || called == nil {
				t.Fatal("original declaration missing")
			}
			child := &nativeMemberClass{object: entry, owner: root.GetClassName(), constructors: map[string]*nativeMemberConstructor{descriptor: {sourceDescriptor: "(Ljava/lang/Object;Ljava/lang/Object;)V"}}}
			family := &nativeMemberFamily{lexicalObjects: objects}
			ctx := &class_context.ClassContext{ClassName: caller.GetClassName(), FunctionName: "call", CurrentMethodDesc: "()LLexicalEntryOwner$Entry;", TypeParams: []string{"Alpha", "Beta"}, LexicalTypeParamSignatures: []string{"<Alpha:Ljava/lang/Object;Beta:Ljava/lang/Object;>Ljava/lang/Object;"}}
			args := []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("java.lang.Object")), values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("java.lang.Object"))}
			setSignature := func(object *ClassObject, method *MemberInfo, signature string) {
				var attrs []AttributeInfo
				for _, attr := range method.Attributes {
					if _, isSignature := attr.(*SignatureAttribute); !isSignature {
						attrs = append(attrs, attr)
					}
				}
				method.Attributes = append(attrs, &SignatureAttribute{SignatureIndex: uint16(object.ConstantPoolManager.AddUtf8Info(signature))})
			}
			var work *workbudget.Budget
			want := [2]bool{}
			switch variant {
			case "original":
				want = [2]bool{true, true}
			case "missing owner":
				delete(objects, root.GetClassName())
			case "copied child":
				copy := *entry
				child.object = &copy
			case "static child":
				child.static = true
			case "generic child":
				child.formalCount = 1
			case "overloaded child":
				child.constructors["(LLexicalEntryOwner;)V"] = &nativeMemberConstructor{sourceDescriptor: "()V"}
			case "duplicate constructor":
				entry.Methods = append(entry.Methods, constructor)
			case "missing signature":
				setSignature(entry, constructor, "")
			case "duplicate signature":
				for _, attr := range constructor.Attributes {
					if _, ok := attr.(*SignatureAttribute); ok {
						constructor.Attributes = append(constructor.Attributes, attr)
						break
					}
				}
			case "malformed signature":
				setSignature(entry, constructor, "(TAlpha;broken")
			case "generic constructor":
				setSignature(entry, constructor, "<Alpha:Ljava/lang/Object;>(TAlpha;TBeta;)V")
			case "foreign signature variable":
				setSignature(entry, constructor, "(TUnknown;TUnknown;)V")
			case "forged source descriptor":
				child.constructors[descriptor].sourceDescriptor = "(Ljava/lang/Number;Ljava/lang/Object;)V"
			case "missing current method":
				ctx.CurrentMethodDesc = "()V"
			case "current method shadow":
				setSignature(caller, called, "<Alpha:Ljava/lang/Object;>()LLexicalEntryOwner<TAlpha;TBeta;>.Entry;")
				want = [2]bool{false, true}
			case "enclosing method shadow":
				setSignature(root, enclosing, "<Alpha:Ljava/lang/Object;>(Ljava/lang/Object;Ljava/lang/Object;)Ljava/util/concurrent/Callable<LLexicalEntryOwner<TAlpha;TBeta;>.Entry;>;")
				want = [2]bool{false, true}
			case "duplicate enclosing method":
				root.Methods = append(root.Methods, enclosing)
			case "wrong erasure":
				args[0] = values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("java.lang.Number"))
				want = [2]bool{false, true}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(canceled, workbudget.Limits{})
			}
			got := nativeMemberLexicalConstructorArguments(family, child, descriptor, caller.GetClassName(), args, ctx, work)
			if len(got) != len(args) {
				t.Fatal("argument arity changed")
			}
			for i, arg := range got {
				cast, added := arg.(*values.CastExpression)
				if added != want[i] {
					t.Fatalf("argument %d: cast=%v, want %v", i, added, want[i])
				}
				if added && (!cast.Binding || cast.Value != args[i]) || !added && arg != args[i] {
					t.Fatal("original operand identity or order changed")
				}
				if _, changed := args[i].(*values.CastExpression); changed {
					t.Fatal("original argument packet mutated")
				}
			}
		})
	}
}
