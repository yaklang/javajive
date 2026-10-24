package javaclassparser

import (
	"strings"
	"sync"
	"testing"
)

func TestNativeSourceTransactionRoutesLocalAndRootReadsToSameReceipt(t *testing.T) {
	fixture := strings.Replace(nativeCyclicDependencyFixture, "class CycleScopeA{", `class CycleScopeA{Object local(final Object token){class Entry{Object get(){return token;}}return new Entry();}`, 1)
	files := nativeCompileClasses(t, fixture)
	const local = "CycleScopeA$1Entry"
	for _, order := range [][]string{{local, "CycleScopeA", "CycleScopeB"}, {"CycleScopeA", "CycleScopeB", local}, {"CycleScopeB", local, "CycleScopeA"}} {
		t.Run(strings.Join(order, ":"), func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			localObject, e := Parse(files[local+".class"])
			if e != nil {
				t.Fatal(e)
			}
			owner, known := z.nativeMemberTransactionOwner(localObject)
			if !known || owner != "CycleScopeA" {
				t.Fatalf("original local owner=%q known=%v", owner, known)
			}
			for _, n := range order {
				source, e := z.ReadFile(n + ".class")
				if e != nil {
					t.Fatal(e)
				}
				if n == local && !strings.Contains(string(source), "original method-local declaration regenerated") {
					t.Fatalf("transaction-owned local emitted as independent source:\n%s", source)
				}
			}
			root, e := Parse(files["CycleScopeA.class"])
			if e != nil {
				t.Fatal(e)
			}
			first, second := z.nativeMemberTransactionEntry(root), z.nativeMemberTransactionEntry(localObject)
			if first == nil || first != second || first.family == nil || first.family.methodLocals[local] == nil || first.family.methodLocals[local].source == "" {
				t.Fatal("root/local reads did not share a complete published ownership receipt")
			}
			before := z.nativeMembersBytes
			var group sync.WaitGroup
			errors := make(chan string, 18)
			for i := 0; i < 18; i++ {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					n := order[i%len(order)]
					source, e := z.ReadFile(n + ".class")
					if e != nil || n == local && !strings.Contains(string(source), "original method-local declaration regenerated") {
						errors <- n
					}
				}(i)
			}
			group.Wait()
			close(errors)
			for n := range errors {
				t.Errorf("concurrent original source ownership failed for %s", n)
			}
			if z.nativeMembersBytes != before {
				t.Fatal("repeated local reads reserved source again")
			}
		})
	}
}

// A routing key is neither a guessed lexical edge nor a publication license.
// Corrupt just one original witness while preserving the binary spelling.
func TestNativeSourceTransactionLocalRoutingNeedsCorroboratedOriginalMethod(t *testing.T) {
	fixture := strings.Replace(nativeCyclicDependencyFixture, "class CycleScopeA{", `class CycleScopeA{Object local(final Object token){class Entry{Object get(){return token;}}return new Entry();}`, 1)
	original := nativeCompileClasses(t, fixture)
	const local = "CycleScopeA$1Entry"
	for _, variant := range []string{"original", "missing enclosing class", "wrong enclosing identity", "missing exact method", "missing reciprocal row", "duplicate enclosing attribute", "initializer scope", "wrong local name", "disabled publication"} {
		t.Run(variant, func(t *testing.T) {
			files := make(map[string][]byte, len(original))
			for n, b := range original {
				files[n] = append([]byte(nil), b...)
			}
			object, err := Parse(files[local+".class"])
			if err != nil {
				t.Fatal(err)
			}
			parent, err := Parse(files["CycleScopeA.class"])
			if err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "missing enclosing class":
				delete(files, "CycleScopeA.class")
			case "wrong enclosing identity":
				files["CycleScopeA.class"] = files["CycleScopeB.class"]
			case "missing exact method":
				for _, m := range parent.Methods {
					n, _ := sourceBridgeUTF8(parent, m.NameIndex)
					if n == "local" {
						m.NameIndex = sourceBridgePoolString(t, parent, "differentMethod")
					}
				}
				files["CycleScopeA.class"] = parent.Bytes()
			case "missing reciprocal row":
				for _, a := range parent.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for i, row := range table.Classes {
							n, _ := sourceBridgeClassName(parent, row.InnerClassInfoIndex)
							if n == local {
								table.Classes = append(table.Classes[:i], table.Classes[i+1:]...)
								break
							}
						}
					}
				}
				files["CycleScopeA.class"] = parent.Bytes()
			case "duplicate enclosing attribute", "initializer scope":
				for _, a := range object.Attributes {
					if enclosing, ok := a.(*UnparsedAttribute); ok && enclosing.Name == "EnclosingMethod" {
						if variant == "duplicate enclosing attribute" {
							object.Attributes = append(object.Attributes, enclosing)
						} else {
							enclosing.Info[2], enclosing.Info[3] = 0, 0
						}
						break
					}
				}
			case "wrong local name":
				for _, a := range object.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(object, row.InnerClassInfoIndex)
							if n == local {
								row.InnerNameIndex = sourceBridgePoolString(t, object, "Other")
							}
						}
					}
				}
			case "disabled publication":
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			}
			files[local+".class"] = object.Bytes()
			archive := nativeArchive(t, files)
			defer archive.Close()
			owner, known := archive.nativeMemberOriginalSourceOwner(object)
			transactionOwner, transactionKnown := archive.nativeMemberTransactionOwner(object)
			if owner != transactionOwner || known != transactionKnown {
				t.Fatal("ordinary and transactional routing diverged")
			}
			wantRoot := variant == "original" || variant == "disabled publication"
			if (known && owner == "CycleScopeA") != wantRoot {
				t.Fatalf("root routing=%q/%v; original witnesses valid=%v", owner, known, wantRoot)
			}
			if variant == "disabled publication" {
				if entry := archive.nativeMemberEntry(object); entry != nil {
					t.Fatal("a routing key overrode the disabled source policy")
				}
			}
		})
	}
}
