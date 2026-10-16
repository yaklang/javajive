package javaclassparser

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeRawFieldSourceProjection(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"p/Base.java": `package p;public class Base<T>{protected T payload;}`, "q/FieldOwner.java": `package q;public class FieldOwner<E> extends p.Base<E>{public class Writer{public E put(E next){return payload=next;}}}`}, "none", "8")
	for _, variant := range []string{"original", "fixed instantiation", "raw superclass", "bounded root", "wrong class signature parent", "missing declaration signature", "unbound field formal", "wrong field erasure", "duplicate field signature", "nil field attribute", "static formal field", "constant field", "missing declaring class", "wrong class identity", "copied provider declaration", "parent cycle", "duplicate class signature", "free root formal", "dependent root bound", "bounded declaration", "graph budget", "memory budget", "canceled", "wrong parent arity", "nongeneric parameterized parent", "owner wrong header", "owner malformed signature", "copied field", "removed field", "duplicate field identity", "static root field", "parent bounded compatible erasure", "wildcard superclass", "bounded wildcard superclass", "nil root attribute", "typed nil root attribute", "nil owner attribute"} {
		for _, use := range []nativeInheritedFieldUse{nativeInheritedFieldErasedRead, nativeInheritedFieldRawWrite} {
			t.Run(fmt.Sprintf("%s/use%d", variant, use), func(t *testing.T) {
				objects := map[string]*ClassObject{}
				for n, b := range files {
					o, e := Parse(b)
					if e != nil {
						t.Fatal(e)
					}
					objects[n[:len(n)-6]] = o
				}
				root, owner := objects["q/FieldOwner"], objects["p/Base"]
				field := owner.Fields[0]
				setSig := func(o *ClassObject, s string) {
					pool := NewConstantPoolWithConstant(&o.ConstantPool)
					found := false
					for _, a := range o.Attributes {
						if sig, ok := a.(*SignatureAttribute); ok {
							sig.SignatureIndex = uint16(pool.AddUtf8Info(s))
							found = true
						}
					}
					if !found {
						o.Attributes = append(o.Attributes, &SignatureAttribute{SignatureIndex: uint16(pool.AddUtf8Info(s))})
					}
				}
				resolver := func(n string) (*ClassObject, bool) { o, ok := objects[n]; return o, ok }
				var work *workbudget.Budget
				want := (variant == "fixed instantiation" && use == nativeInheritedFieldErasedRead) || variant == "original" || variant == "raw superclass" || variant == "bounded root"
				switch variant {
				case "fixed instantiation":
					setSig(root, "Lp/Base<Ljava/lang/String;>;")
				case "raw superclass":
					for i, a := range root.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							root.Attributes = append(root.Attributes[:i], root.Attributes[i+1:]...)
							break
						}
					}
				case "bounded root":
					setSig(root, "<E:Ljava/lang/Number;>Lp/Base<TE;>;")
				case "wrong class signature parent":
					setSig(root, "<E:Ljava/lang/Object;>LWrong<TE;>;")
				case "missing declaration signature":
					for i, a := range owner.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							owner.Attributes = append(owner.Attributes[:i], owner.Attributes[i+1:]...)
							break
						}
					}
				case "unbound field formal":
					for _, a := range field.Attributes {
						if sig, ok := a.(*SignatureAttribute); ok {
							sig.SignatureIndex = uint16(NewConstantPoolWithConstant(&owner.ConstantPool).AddUtf8Info("TU;"))
						}
					}
				case "wrong field erasure":
					for _, a := range field.Attributes {
						if sig, ok := a.(*SignatureAttribute); ok {
							sig.SignatureIndex = uint16(NewConstantPoolWithConstant(&owner.ConstantPool).AddUtf8Info("Ljava/lang/String;"))
						}
					}
				case "duplicate field signature":
					field.Attributes = append(field.Attributes, field.Attributes[0])
				case "nil field attribute":
					field.Attributes = append(field.Attributes, nil)
				case "static formal field":
					field.AccessFlags |= 8
				case "constant field":
					field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
				case "missing declaring class":
					delete(objects, "p/Base")
				case "wrong class identity":
					owner.ThisClass = uint16(NewConstantPoolWithConstant(&owner.ConstantPool).AddNewClassInfo("Wrong"))
				case "copied provider declaration":
					copy := *owner
					objects["p/Base"] = &copy
				case "parent cycle":
					root.SuperClass = uint16(NewConstantPoolWithConstant(&root.ConstantPool).AddNewClassInfo("q/FieldOwner"))
				case "duplicate class signature":
					for _, a := range root.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							root.Attributes = append(root.Attributes, a)
							break
						}
					}
				case "free root formal":
					setSig(root, "Lp/Base<TE;>;")
				case "dependent root bound":
					setSig(root, "<E:TU;U:Ljava/lang/Number;>Lp/Base<TE;>;")
				case "bounded declaration":
					setSig(owner, "<T:Ljava/lang/Number;>Ljava/lang/Object;")
				case "copied field":
					copy := *field
					field = &copy
				case "removed field":
					owner.Fields = nil
				case "duplicate field identity":
					owner.Fields = append(owner.Fields, field)
				case "static root field":
					field.AccessFlags |= 8
				case "parent bounded compatible erasure":
					setSig(owner, "<T:Ljava/lang/Object;:Ljava/io/Serializable;>Ljava/lang/Object;")
				case "wildcard superclass":
					setSig(root, "<E:Ljava/lang/Object;>Lp/Base<*>;")
				case "bounded wildcard superclass":
					setSig(root, "<E:Ljava/lang/Object;>Lp/Base<+TE;>;")
				case "nil root attribute":
					root.Attributes = append(root.Attributes, nil)
				case "typed nil root attribute":
					root.Attributes = append(root.Attributes, (*UnparsedAttribute)(nil))
				case "nil owner attribute":
					owner.Attributes = append(owner.Attributes, nil)
				case "wrong parent arity":
					setSig(root, "<E:Ljava/lang/Object;>Lp/Base<TE;TE;>;")
				case "nongeneric parameterized parent":
					setSig(owner, "Ljava/lang/Object;")
				case "owner wrong header":
					setSig(owner, "<T:Ljava/lang/Object;>LWrong;")
				case "owner malformed signature":
					setSig(owner, "<T:Ljava/lang/Object;>Ljava/lang/Object;garbage")
				case "graph budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory budget":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				generic, known := nativeMemberInheritedFieldSourceSignature(root, owner, field, "Ljava/lang/Object;", use, resolver, work)
				if known != want || known && !generic {
					t.Fatalf("generic=%v known=%v want=%v", generic, known, want)
				}
			})
		}
	}
}
