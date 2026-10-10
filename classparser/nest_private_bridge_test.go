package javaclassparser

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestPrivateNestBridgeRequiresOriginalAccessEvidence(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "NestBridgeProofReview.java")
	const source = `public class NestBridgeProofReview {private Object pick(Object value){return value;}static Object jdec$private$4e65737442726964676550726f6f66526576696577$0(Object value){return value;}static class Reader {static Object read(NestBridgeProofReview owner,Object value){return owner.pick(value);}}}`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "17", "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("original: %v %s", err, out)
	}
	loadFresh := func() map[string]*ClassObject {
		objects := map[string]*ClassObject{}
		files, _ := filepath.Glob(filepath.Join(dir, "*.class"))
		for _, file := range files {
			raw, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			o, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			objects[o.GetClassName()] = o
		}
		return objects
	}
	for _, name := range []string{"positive", "missing-host-list", "missing-reciprocal", "malformed-host-list", "malformed-nesthost", "duplicate-host-list", "wrong-owner-declaration", "static-target", "synthetic-target", "method-formal-intersection", "interface-owner", "missing-sibling", "wrong-sibling-identity", "old-class-version"} {
		t.Run(name, func(t *testing.T) {
			objects := loadFresh()
			owner := objects["NestBridgeProofReview"]
			reader := objects["NestBridgeProofReview$Reader"]
			remove := func(o *ClassObject, attrName string) {
				var attrs []AttributeInfo
				for _, a := range o.Attributes {
					if raw, ok := a.(*UnparsedAttribute); !ok || raw.Name != attrName {
						attrs = append(attrs, a)
					}
				}
				o.Attributes = attrs
			}
			switch name {
			case "missing-host-list":
				remove(owner, "NestMembers")
			case "missing-reciprocal":
				remove(reader, "NestHost")
			case "malformed-host-list":
				for _, a := range owner.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "NestMembers" {
						raw.Info = append(raw.Info, 0)
					}
				}
			case "malformed-nesthost":
				owner.Attributes = append(owner.Attributes, &UnparsedAttribute{Name: "NestHost", Info: []byte{0}})
			case "duplicate-host-list":
				for _, a := range owner.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "NestMembers" {
						owner.Attributes = append(owner.Attributes, raw)
						break
					}
				}
			case "wrong-owner-declaration", "static-target", "synthetic-target", "method-formal-intersection":
				for _, m := range owner.Methods {
					n, _ := owner.getUtf8(m.NameIndex)
					if n == "pick" {
						switch name {
						case "wrong-owner-declaration":
							m.AccessFlags &^= 2
						case "static-target":
							m.AccessFlags |= 8
						case "synthetic-target":
							m.AccessFlags |= 0x1000
						case "method-formal-intersection":
							cp := NewConstantPoolWithConstant(&owner.ConstantPool)
							index := cp.AddUtf8Info("<T:Ljava/lang/Object;:Ljava/lang/Runnable;>(TT;)TT;")
							m.Attributes = append(m.Attributes, &SignatureAttribute{SignatureIndex: uint16(index)})
						}
					}
				}
			case "missing-sibling":
				delete(objects, "NestBridgeProofReview$Reader")
			case "wrong-sibling-identity":
				objects["NestBridgeProofReview$Reader"] = owner
			case "old-class-version":
				owner.MajorVersion = 52
			case "interface-owner":
				owner.AccessFlags |= 0x200
			}
			d := NewClassObjectDumper(owner)
			plan, e := d.buildPrivateNestPlan(owner, func(n string) (*ClassObject, bool) { o, ok := objects[n]; return o, ok })
			if e != nil {
				t.Fatal(e)
			}
			if name != "positive" {
				if plan != nil && len(plan.bridges) > 0 {
					t.Fatalf("unproven bridge accepted: %#v", plan)
				}
				return
			}
			if plan == nil || len(plan.bridges) != 1 || len(plan.sites) != 1 {
				t.Fatalf("missing exact access proof: %#v", plan)
			}
			if got := plan.bridges[privateNestTarget{"pick", "(Ljava/lang/Object;)Ljava/lang/Object;"}]; got != "jdec$private$4e65737442726964676550726f6f66526576696577$0$" {
				t.Fatalf("bridge did not avoid exact encoded original member: %q", got)
			}
			caller := NewClassObjectDumper(reader)
			caller.FuncCtx = &class_context.ClassContext{ClassName: reader.GetClassName(), FunctionName: "read", CurrentMethodDesc: "(LNestBridgeProofReview;Ljava/lang/Object;)Ljava/lang/Object;"}
			caller.foldSiblingResolver = func(n string) ([]byte, bool) { b, e := os.ReadFile(filepath.Join(dir, n+".class")); return b, e == nil }
			caller.wirePrivateNestBridges()
			var site privateNestSite
			for s := range plan.sites {
				site = s
			}
			if _, ok := caller.FuncCtx.PrivateNestBridge(owner.GetClassName(), "pick", site.target.descriptor, site.kind, site.pc); !ok {
				t.Fatal("exact invoke rejected")
			}
			for _, bad := range []struct {
				name, desc string
				kind       uint8
				pc         int
			}{{"other", site.target.descriptor, site.kind, site.pc}, {"pick", "()Ljava/lang/Object;", site.kind, site.pc}, {"pick", site.target.descriptor, uint8(values.InvokeStatic), site.pc}, {"pick", site.target.descriptor, site.kind, site.pc + 1}} {
				if _, ok := caller.FuncCtx.PrivateNestBridge(owner.GetClassName(), bad.name, bad.desc, bad.kind, bad.pc); ok {
					t.Fatalf("invented invoke admitted: %#v", bad)
				}
			}
			caller.FuncCtx.FunctionName = "other"
			if _, ok := caller.FuncCtx.PrivateNestBridge(owner.GetClassName(), "pick", site.target.descriptor, site.kind, site.pc); ok {
				t.Fatal("wrong method context admitted")
			}
		})
	}
}

// Decode the emitted token back to the original binary identity, including
// punctuation and Unicode that cannot be reconstructed from flattened names.
func TestPrivateNestBridgeOwnerIdentityIsLosslessAndDisjoint(t *testing.T) {
	owners := []string{"A", "A$B", "A/B", "AB", "a", "pkg/Outer$Base", "pkg/Outer$Child", "包/类", "pkg/𐐀"}
	seen := map[string]string{}
	for _, owner := range owners {
		for _, index := range []int{0, 1, 10, 100} {
			name, ok := privateNestBridgeBaseName(owner, index)
			if !ok {
				t.Fatalf("valid owner rejected: %q", owner)
			}
			pieces := strings.Split(strings.TrimPrefix(name, "jdec$private$"), "$")
			if len(pieces) != 2 {
				t.Fatalf("ambiguous identity delimiter: %q", name)
			}
			decoded, err := hex.DecodeString(pieces[0])
			if err != nil || string(decoded) != owner {
				t.Fatalf("lossy owner identity %q -> %q", owner, name)
			}
			for extra := 0; extra < 4; extra++ {
				candidate := name + strings.Repeat("$", extra)
				if prior, exists := seen[candidate]; exists {
					t.Fatalf("owner/target/collision routes overlap: %q / %q at %q", prior, owner, candidate)
				}
				seen[candidate] = owner
			}
		}
	}
	for _, invalid := range []struct {
		owner string
		index int
	}{{"", 0}, {"Owner", -1}, {strings.Repeat("A", 32761), 0}} {
		if _, ok := privateNestBridgeBaseName(invalid.owner, invalid.index); ok {
			t.Fatal("unrepresentable owner identity accepted")
		}
	}
	name, ok := privateNestBridgeBaseName(strings.Repeat("A", 32760), 0)
	if !ok || len(name) != 65535 {
		t.Fatalf("exact UTF8 name boundary: len=%d ok=%t", len(name), ok)
	}
}

func TestPrivateNestBridgeNameBudgetBoundsIdentityExpansion(t *testing.T) {
	const limit = 1 << 20
	for _, tc := range []struct {
		previous, next, want int
		ok                   bool
	}{{0, 1, 1, true}, {limit - 1, 1, limit, true}, {limit, 0, limit, true}, {limit, 1, 0, false}, {0, limit + 1, 0, false}, {-1, 1, 0, false}, {1, -1, 0, false}, {int(^uint(0) >> 1), 1, 0, false}} {
		got, ok := privateNestBridgeNameBudget(tc.previous, tc.next)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("retained owner proof bytes %d+%d: got %d/%t want %d/%t", tc.previous, tc.next, got, ok, tc.want, tc.ok)
		}
	}
}
