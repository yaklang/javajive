package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativePublicAbstractRootBridgeRequiresClosedDeclarationProfile(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"RootBridgePacket.java": publicAbstractRootBridgeFixture("reference")}, "none", "8")
	for _, variant := range []string{"original", "package-private", "final", "synthetic", "interface", "enum", "no-super", "missing-member", "budget", "cancel"} {
		t.Run(variant, func(t *testing.T) {
			copied := make(map[string][]byte, len(files))
			for name, raw := range files {
				copied[name] = append([]byte(nil), raw...)
			}
			root, err := Parse(copied["RootBridgePacket.class"])
			if err != nil {
				t.Fatal(err)
			}
			var work *workbudget.Budget
			switch variant {
			case "package-private":
				root.AccessFlags &^= 0x0001
			case "final":
				root.AccessFlags |= 0x0010
			case "synthetic":
				root.AccessFlags |= 0x1000
			case "interface":
				root.AccessFlags |= 0x0200
			case "enum":
				root.AccessFlags |= 0x4000
			case "no-super":
				root.AccessFlags &^= 0x0020
			case "missing-member":
				delete(copied, "RootBridgePacket$Member.class")
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			copied["RootBridgePacket.class"] = root.Bytes()
			z := nativeArchive(t, copied)
			defer z.Close()
			reader := z.nativeMemberReader(root)
			reader.Work = work
			if (reader.planNativeMemberFamily() != nil) != (variant == "original") {
				t.Fatal("source ownership needs the complete original declaration and matching access profile")
			}
		})
	}
}

func TestNativePublicAbstractRootDoesNotCertifyDirectNew(t *testing.T) {
	fixture := strings.Replace(nativeRootPrivateConstructorFixture, "class RootBridgePacket {", "public class RootBridgePacket {", 1)
	fixture = strings.Replace(fixture, "Member(Object value){super(value);RootBridgeEffects.trace+=\"M\";}", "Member(Object value){super(value);RootBridgeEffects.trace+=\"M\";}static RootBridgePacket direct(Object value){return new RootBridgePacket(value);}", 1)
	fixture = strings.Replace(fixture, "Object token=new Object();int rows=0;", `Object token=new Object();RootBridgeEffects.trace="";try{RootBridgePacket.Member.direct(token);throw new AssertionError("abstract NEW must fail");}catch(InstantiationError expected){if(!RootBridgeEffects.trace.equals(""))throw new AssertionError("constructor ran before abstract NEW failure");}int rows=0;`, 1)
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"RootBridgePacket.java": fixture}, "none", "8")
	root, err := Parse(files["RootBridgePacket.class"])
	if err != nil {
		t.Fatal(err)
	}
	root.AccessFlags |= 0x0400
	files["RootBridgePacket.class"] = root.Bytes()
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, java := t04Tools(t)
	if got := t04RunJava(t, java, original, "RootBridgeDriver"); got != "2:private-root:identity:order:owner\n" {
		t.Fatal(got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	entry := z.nativeMemberEntry(root)
	if entry != nil && entry.family != nil {
		t.Fatal("NEW of an abstract class cannot be emitted as a successful source allocation")
	}
}
