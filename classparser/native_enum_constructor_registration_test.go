package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumConstructorRegistrationRequiresOriginalBridge(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ConstantPacketOwner.java": nativeEnumConstantPacketFixture}, "none", "8")
	for _, variant := range []string{"original", "nil body", "nil object", "failed family", "missing body", "copied body", "missing lexical object", "missing parent", "missing synthesis", "wrong current", "copied current", "missing bridge", "bridge target", "bridge code", "super pc", "super descriptor", "allocation pc", "constant", "body code", "budget", "memory", "canceled"} {
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
				t.Fatal("original family")
			}
			body := p.enumConstants["ConstantPacketOwner$Mode$1"]
			if body == nil {
				t.Fatal("original enum body")
			}
			d.nativeMemberRoot, d.nativeMemberCurrent = p, p.children[body.owner]
			bridge := d.nativeMemberCurrent.accessBridges[body.superDescriptor]
			if bridge == nil {
				t.Fatal("original bridge")
			}
			switch variant {
			case "nil body":
				body = nil
			case "nil object":
				body.object = nil
			case "failed family":
				p.failed = true
			case "missing body":
				delete(p.enumConstants, body.object.GetClassName())
			case "copied body":
				copy := *body
				body = &copy
			case "missing lexical object":
				delete(p.lexicalObjects, body.object.GetClassName())
			case "missing parent":
				delete(p.children, body.owner)
			case "missing synthesis":
				d.nativeMemberCurrent.enumSynthesis = nil
			case "wrong current":
				d.nativeMemberCurrent = nil
			case "copied current":
				copy := *d.nativeMemberCurrent
				d.nativeMemberCurrent = &copy
			case "missing bridge":
				delete(d.nativeMemberCurrent.accessBridges, body.superDescriptor)
			case "bridge target":
				bridge.target = "()V"
			case "bridge code":
				for _, a := range bridge.method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
					}
				}
			case "super pc":
				body.superPC++
			case "super descriptor":
				body.superDescriptor = "()V"
			case "allocation pc":
				body.plan.newPC++
			case "constant":
				body.plan.constant = "FOREIGN"
			case "body code":
				for _, m := range body.object.Methods {
					if name, _ := sourceBridgeUTF8(body.object, m.NameIndex); name == "<init>" {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								code.Code = append(bytes.Clone(code.Code[:len(code.Code)-1]), 0x01, 0xbf, 0xb1)
							}
						}
					}
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
			got, known := d.nativeEnumConstantConstructorRegistration(body)
			if known != (variant == "original") {
				t.Fatalf("original constructor registration %t %s", known, got)
			}
			if known && !strings.HasPrefix(got, "/*"+nativeConstructorRegistrationPrefix+"ConstantPacketOwner$Mode:") {
				t.Fatalf("unowned event %s", got)
			}
		})
	}
}
