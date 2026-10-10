package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// Countermodels exercise discovery's sealed enum role. They are not valid
// Java program positives and cannot substitute for the compiled roundtrip.
func TestNativeEnumAnonymousRoleRequiresSealedAllocationAndOriginalConstructor(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ConstantPacketOwner.java": nativeEnumConstantPacketFixture}, "none", "8")
	for _, variant := range []string{"original", "nil family", "missing body", "copied body seal", "missing lexical object", "missing enum parent", "missing enum synthesis", "foreign allocation pc", "foreign constant", "fresh version", "wrong class flags", "named self", "missing Code", "extra constructor effect", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(bytes.Clone(files["ConstantPacketOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original enum family")
			}
			name := "ConstantPacketOwner$Mode$1"
			body := p.enumConstants[name]
			if body == nil {
				t.Fatal("original enum body")
			}
			obj, err := Parse(bytes.Clone(files[name+".class"]))
			if err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "nil family":
				p = nil
			case "missing body":
				delete(p.enumConstants, name)
			case "copied body seal":
				copy := *body
				p.enumConstants[name] = &copy
			case "missing lexical object":
				delete(p.lexicalObjects, name)
			case "missing enum parent":
				delete(p.children, body.owner)
			case "missing enum synthesis":
				p.children[body.owner].enumSynthesis = nil
			case "foreign allocation pc":
				body.plan.newPC++
			case "foreign constant":
				body.plan.constant = "OTHER"
			case "fresh version":
				obj.MajorVersion++
			case "wrong class flags":
				obj.AccessFlags = 0x20
			case "named self":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if n == name {
								row.InnerNameIndex = 1
							}
						}
					}
				}
			case "missing Code", "extra constructor effect":
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n != "<init>" {
						continue
					}
					var keep []AttributeInfo
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							if variant == "missing Code" {
								continue
							}
							// Preserve the forwarding call, but throw null before
							// the original return. Unreachable padding is not an
							// observable constructor effect and is a poor mutant.
							code.Code = append(bytes.Clone(code.Code[:len(code.Code)-1]), 0x01, 0xbf, 0xb1)
						}
						keep = append(keep, a)
					}
					m.Attributes = keep
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.nativeMemberEnumConstantAnonymousRole(p, obj); got != (variant == "original") {
				t.Fatalf("enum role %t variant%s", got, variant)
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("lost resource refusal")
			}
		})
	}
}
func TestNativeEnumAnonymousRoleExhaustsClassFlagWords(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ConstantPacketOwner.java": nativeEnumConstantPacketFixture}, "none", "8")
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["ConstantPacketOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	d := z.nativeMemberReader(root)
	p := d.planNativeMemberFamily()
	if p == nil {
		t.Fatal("original family")
	}
	original, err := Parse(bytes.Clone(files["ConstantPacketOwner$Mode$1.class"]))
	if err != nil {
		t.Fatal(err)
	}
	for flags := 0; flags < 1<<16; flags++ {
		obj := *original
		obj.AccessFlags = uint16(flags)
		if got := d.nativeMemberEnumConstantAnonymousRole(p, &obj); got != (flags == 0x4030) {
			t.Fatalf("flags %04x role%t", flags, got)
		}
	}
}
