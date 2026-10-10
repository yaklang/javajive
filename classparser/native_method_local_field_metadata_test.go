package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMethodLocalHiddenFieldsRetainActualErasedMetadata(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class LocalFieldOwner<T extends Number>{<T extends CharSequence> Object make(final T token,final java.util.List<String> items,final long seed){class Entry {Object token(){return token;}Object items(){return items;}long get(){return seed;}}return new Entry();} static <T extends CharSequence> Object stat(final T token){class Node {Object get(){return token;}}return new Node();}}`, debug)
			for _, variant := range []string{"original", "static original", "capture Synthetic attribute", "capture both encodings", "enclosing Synthetic attribute", "duplicate Synthetic", "nonempty Synthetic", "nil Synthetic", "Synthetic plus Signature", "extra capture Signature", "enclosing Signature", "unknown attribute", "duplicate field Signature", "inconsistent method erasure", "invalid free method type", "budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					root, e := Parse(files["LocalFieldOwner.class"])
					if e != nil {
						t.Fatal(e)
					}
					path := "LocalFieldOwner$1Entry.class"
					if variant == "static original" {
						path = "LocalFieldOwner$1Node.class"
					}
					obj, e := Parse(files[path])
					if e != nil {
						t.Fatal(e)
					}
					owner, known := originalMethodLocalOwner(obj, root, nil)
					if !known {
						t.Fatal("original method owner")
					}
					ctor, known := originalMethodLocalDefaultConstructor(obj, root, nil)
					if !known {
						t.Fatal("original default constructor")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					d := z.nativeMemberReader(root)
					sites, known := d.nativeMethodLocalParameterAllocations(obj, owner, ctor)
					if !known {
						t.Fatal("original parameter captures")
					}
					local := &nativeMethodLocalClass{object: obj, owner: owner, constructor: ctor, allocations: sites}
					var capture, enclosing *MemberInfo
					for _, f := range obj.Fields {
						n, _ := sourceBridgeUTF8(obj, f.NameIndex)
						if len(f.Attributes) != 0 {
							t.Fatalf("actual javac hidden field %s carries unexpected attributes %v", n, f.Attributes)
						}
						if n == ctor.enclosingField {
							enclosing = f
						} else if n == "val$token" {
							capture = f
						}
					}
					if capture == nil {
						t.Fatal("original generic capture")
					}
					addSig := func(f *MemberInfo) {
						obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "TT;"})
						f.Attributes = append(f.Attributes, &SignatureAttribute{SignatureIndex: uint16(len(obj.ConstantPool))})
					}
					var work *workbudget.Budget
					switch variant {
					case "capture Synthetic attribute":
						capture.AccessFlags &^= 0x1000
						capture.Attributes = append(capture.Attributes, &SyntheticAttribute{})
					case "capture both encodings":
						capture.Attributes = append(capture.Attributes, &SyntheticAttribute{})
					case "enclosing Synthetic attribute":
						if enclosing == nil {
							t.Fatal("missing original enclosing capture")
						}
						enclosing.AccessFlags &^= 0x1000
						enclosing.Attributes = append(enclosing.Attributes, &SyntheticAttribute{})
					case "duplicate Synthetic":
						capture.Attributes = append(capture.Attributes, &SyntheticAttribute{}, &SyntheticAttribute{})
					case "nonempty Synthetic":
						capture.Attributes = append(capture.Attributes, &SyntheticAttribute{AttrLen: 1})
					case "nil Synthetic":
						capture.Attributes = append(capture.Attributes, (*SyntheticAttribute)(nil))
					case "Synthetic plus Signature":
						capture.Attributes = append(capture.Attributes, &SyntheticAttribute{})
						addSig(capture)
					case "extra capture Signature":
						addSig(capture)
					case "enclosing Signature":
						if enclosing == nil {
							t.Fatal("enclosing capture")
						}
						addSig(enclosing)
					case "unknown attribute":
						capture.Attributes = append(capture.Attributes, &UnparsedAttribute{Name: "RuntimeInvisibleAnnotations", Length: 0})
					case "duplicate field Signature":
						addSig(capture)
						addSig(capture)
					case "inconsistent method erasure", "invalid free method type":
						for _, a := range owner.declaration.Attributes {
							if sig, ok := a.(*SignatureAttribute); ok {
								raw := "<T:Ljava/lang/Number;>(TT;Ljava/util/List<Ljava/lang/String;>;J)Ljava/lang/Object;"
								if variant == "invalid free method type" {
									raw = "<T::Ljava/lang/CharSequence;>(TX;Ljava/util/List<Ljava/lang/String;>;J)Ljava/lang/Object;"
								}
								root.ConstantPool = append(root.ConstantPool, &ConstantUtf8Info{Value: raw})
								sig.SignatureIndex = uint16(len(root.ConstantPool))
							}
						}
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					if got := nativeMethodLocalCaptureMetadata(root, local, work); got != (variant == "original" || variant == "static original" || variant == "capture Synthetic attribute" || variant == "capture both encodings" || variant == "enclosing Synthetic attribute") {
						t.Fatalf("hidden field metadata accepted %v", got)
					}
				})
			}
		})
	}
}
