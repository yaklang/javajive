package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLexicalPrivateCallRequiresNameableRawOwner(t *testing.T) {
	for _, tc := range []struct {
		name, root, formal string
		forceQualified     bool
		accept             bool
	}{
		{"plain", "Owner", "T", false, true},
		{"hidden default package owner", "Owner", "Owner", false, false},
		{"qualified owner", "probe.Owner", "Owner", false, true},
		{"imported owner with hidden package head", "probe.Owner", "probe", false, true},
		{"hidden required package head", "probe.Owner", "probe", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{tc.formal}}
			if tc.forceQualified {
				ctx.DeclarationSourceName = func(string) (string, bool) { return tc.root, true }
			}
			getter := &nativeMemberPrivateGetter{owner: strings.ReplaceAll(tc.root, ".", "/") + "$Value", field: "original", fieldDescriptor: "()Ljava/lang/Object;", call: &nativeMemberPrivateCall{argumentCount: 1, rawGeneric: true, rawLexicalOwner: []string{strings.ReplaceAll(tc.root, ".", "/"), "Value"}}}
			arg := &values.JavaRef{IsThis: true}
			source, known := nativeMemberPrivateCallSource(getter, []any{arg}, ctx)
			if known != tc.accept || !known && source != "" {
				t.Fatalf("raw owner nameability: known=%v source=%q", known, source)
			}
			if known && !strings.Contains(source, ".Value") {
				t.Fatalf("lost raw outer qualifier: %s", source)
			}
		})
	}
}

func TestNativeLexicalPrivateCallRequiresOriginalScopeAndPacket(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, lexicalPrivateCallFixture, debug)
		for _, variant := range []string{"original", "dependent bound", "method shadow", "missing owner", "foreign owner pointer", "missing outer", "wrong outer bound", "unbound outer", "inner cannot repair outer bound", "wrong outer superclass", "duplicate outer signature", "nil outer signature", "missing outer signature", "unbound method", "wrong method descriptor", "wrong method bound", "wrong signature throws", "duplicate method signature", "nil method signature", "static target", "public target", "static cut", "foreign lexical edge", "duplicate lexical edge", "missing reciprocal edge", "wrong reciprocal flags", "local enclosing evidence", "no lexical authority", "budget", "memory", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				objects := map[string]*ClassObject{}
				for name, raw := range files {
					if strings.HasPrefix(name, "LexicalCallOwner") {
						obj, e := Parse(append([]byte(nil), raw...))
						if e != nil {
							t.Fatal(e)
						}
						objects[obj.GetClassName()] = obj
					}
				}
				owner := objects["LexicalCallOwner$Value"]
				outer := objects["LexicalCallOwner"]
				if owner == nil || outer == nil {
					t.Fatal("missing original owner")
				}
				var bridge, target *MemberInfo
				for _, m := range owner.Methods {
					n, _ := sourceBridgeUTF8(owner, m.NameIndex)
					d, _ := sourceBridgeUTF8(owner, m.DescriptorIndex)
					if n == "access$000" {
						bridge = m
					}
					if n == "choose" && d == "(Ljava/util/List;J)Ljava/lang/Number;" {
						target = m
					}
				}
				if bridge == nil || target == nil {
					t.Fatal("missing original private packet")
				}
				self := func(obj *ClassObject) *InnerClassInfo {
					for _, a := range obj.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							for _, r := range table.Classes {
								n, _ := sourceBridgeClassName(obj, r.InnerClassInfoIndex)
								if n == owner.GetClassName() {
									return r
								}
							}
						}
					}
					t.Fatal("missing original lexical edge")
					return nil
				}
				// Both admission APIs need the same original lexical certificate. A
				// signature alone may not license this outer-dependent method.
				if nativeMemberPrivateCallProof(owner, bridge, nil) != nil {
					t.Fatal("closed raw-own proof borrowed a free outer formal")
				}
				var work *workbudget.Budget
				switch variant {
				case "dependent bound": // The original U extends T environment is the witness.
				case "method shadow":
					nativeReplaceOriginalSignature(t, owner, target.Attributes, "<U:Ljava/lang/Number;>(Ljava/util/List<-TU;>;J)TU;")
				case "missing owner":
					delete(objects, owner.GetClassName())
				case "foreign owner pointer":
					copy, e := Parse(files["LexicalCallOwner$Value.class"])
					if e != nil {
						t.Fatal(e)
					}
					objects[owner.GetClassName()] = copy
				case "missing outer":
					delete(objects, outer.GetClassName())
				case "wrong outer bound":
					nativeReplaceOriginalSignature(t, outer, outer.Attributes, "<T:Ljava/lang/CharSequence;U:TT;>Ljava/lang/Object;")
				case "unbound outer":
					nativeReplaceOriginalSignature(t, outer, outer.Attributes, "<T:TFree;U:TT;>Ljava/lang/Object;")
				case "inner cannot repair outer bound":
					nativeReplaceOriginalSignature(t, outer, outer.Attributes, "<T:Ljava/lang/Number<TFree;>;U:TT;>Ljava/lang/Object;")
					owner.Attributes = append(owner.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, owner, "<Free:Ljava/lang/Object;>Ljava/lang/Object;")})
				case "wrong outer superclass":
					nativeReplaceOriginalSignature(t, outer, outer.Attributes, "<T:Ljava/lang/Number;U:TT;>Ljava/util/ArrayList<TU;>;")
				case "duplicate outer signature":
					for _, a := range outer.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							outer.Attributes = append(outer.Attributes, a)
							break
						}
					}
				case "nil outer signature":
					outer.Attributes = append(outer.Attributes, (*SignatureAttribute)(nil))
				case "missing outer signature":
					var kept []AttributeInfo
					for _, a := range outer.Attributes {
						if _, ok := a.(*SignatureAttribute); !ok {
							kept = append(kept, a)
						}
					}
					outer.Attributes = kept
				case "unbound method":
					nativeReplaceOriginalSignature(t, owner, target.Attributes, "(Ljava/util/List<-TFree;>;J)TU;")
				case "wrong method descriptor":
					nativeReplaceOriginalSignature(t, owner, target.Attributes, "(Ljava/util/ArrayList<-TU;>;J)TU;")
				case "wrong method bound":
					nativeReplaceOriginalSignature(t, owner, target.Attributes, "<U:Ljava/lang/CharSequence;>(Ljava/util/List<-TU;>;J)TU;")
				case "wrong signature throws":
					nativeReplaceOriginalSignature(t, owner, target.Attributes, "(Ljava/util/List<-TU;>;J)TU;^Ljava/lang/Exception;")
				case "duplicate method signature":
					for _, a := range target.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							target.Attributes = append(target.Attributes, a)
							break
						}
					}
				case "nil method signature":
					target.Attributes = append(target.Attributes, (*SignatureAttribute)(nil))
				case "static target":
					target.AccessFlags |= 8
				case "public target":
					target.AccessFlags = (target.AccessFlags &^ 2) | 1
				case "static cut":
					self(owner).InnerClassAccessFlags |= 8
				case "foreign lexical edge":
					self(owner).OuterClassInfoIndex = nativeWideningTestPoolClass(t, owner, "OtherSameNamedOwner")
				case "duplicate lexical edge":
					for _, a := range owner.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							table.Classes = append(table.Classes, self(owner))
							break
						}
					}
				case "missing reciprocal edge":
					for _, a := range outer.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							var kept []*InnerClassInfo
							for _, r := range table.Classes {
								n, _ := sourceBridgeClassName(outer, r.InnerClassInfoIndex)
								if n != owner.GetClassName() {
									kept = append(kept, r)
								}
							}
							table.Classes = kept
						}
					}
				case "wrong reciprocal flags":
					self(outer).InnerClassAccessFlags |= 8
				case "local enclosing evidence":
					owner.Attributes = append(owner.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Info: []byte{0, 0, 0, 0}})
				case "no lexical authority":
					objects = nil
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				p := nativeMemberPrivateAccessProofWithDeclarations(owner, bridge, nil, work, objects)
				want := variant == "original" || variant == "dependent bound" || variant == "method shadow"
				if (p != nil) != want {
					t.Fatalf("admitted=%v want=%v", p != nil, want)
				}
				if p != nil && (p.call == nil || p.call.static || p.call.inherited || p.call.methodFormalCount != 0 || strings.Join(p.call.rawLexicalOwner, ".") != "LexicalCallOwner.Value") {
					t.Fatal("lost original raw outer qualification")
				}
			})
		}
	}
}
