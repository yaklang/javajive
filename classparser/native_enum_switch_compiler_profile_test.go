package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeEnumOwnUnitArchive(t *testing.T, files map[string][]byte, profile SourceCompilerProfile) *JarFS {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authored.jar")
	if err := os.WriteFile(path, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	z, err := NewJarFSFromLocalWithCompilerProfile(path, 8, profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { z.Close() })
	return z
}

func TestNativeEnumSwitchOwnUnitCompilerRejectsUnprovedProfilesAndVersions(t *testing.T) {
	files := nativeEnumOwnUnitFiles(t, "LegacyNestedSwitch", "none")
	for _, variant := range []string{"original", "empty compiler", "modern compiler", "unknown compiler", "wrong source", "nil archive", "nil root", "nil enum", "nil table", "nil helper", "root version", "enum version", "helper version", "root minor", "enum minor", "helper minor", "not enum", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeEnumOwnUnitArchive(t, files, NativeJavac8)
			root, _ := Parse(files["LegacyNestedSwitch.class"])
			enum, _ := Parse(files["LegacyNestedSwitch$Mode.class"])
			helper, _ := Parse(files["LegacyNestedSwitch$1.class"])
			table := nativeEnumSwitchTableProof(helper, nil)
			if table == nil {
				t.Fatal("original complete helper")
			}
			var work *workbudget.Budget
			switch variant {
			case "empty compiler":
				z.sourceCompiler = ""
			case "modern compiler":
				z.sourceCompiler = ModernJavac
			case "unknown compiler":
				z.sourceCompiler = "unknown"
			case "wrong source":
				z.targetSourceVersion = 11
			case "nil archive":
				z = nil
			case "nil root":
				root = nil
			case "nil enum":
				enum = nil
			case "nil table":
				table = nil
			case "nil helper":
				table.object = nil
			case "root version":
				root.MajorVersion = 53
			case "enum version":
				enum.MajorVersion = 51
			case "helper version":
				helper.MajorVersion = 51
			case "root minor":
				root.MinorVersion = 1
			case "enum minor":
				enum.MinorVersion = 1
			case "helper minor":
				helper.MinorVersion = 1
			case "not enum":
				enum.AccessFlags &^= 0x4000
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				_ = work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := z.nativeEnumSwitchOwnUnitCompiler(root, enum, table, work); got != (variant == "original") {
				t.Fatalf("compiler protocol=%v", got)
			}
		})
	}
}

func TestNativeEnumSwitchOwnUnitRequiresMatchingCompilerProfile(t *testing.T) {
	for _, owner := range []string{"LegacySelfEnum", "LegacyNestedSwitch"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(owner+"/"+debug+"/"+string(profile), func(t *testing.T) {
					files := nativeEnumOwnUnitFiles(t, owner, debug)
					z := nativeEnumOwnUnitArchive(t, files, profile)
					root, err := Parse(files[owner+".class"])
					if err != nil {
						t.Fatal(err)
					}
					prepared := z.prepareNativeMemberFamily(root, snapshotJDECEnv())
					if (prepared != nil) != (profile == NativeJavac8) {
						t.Fatalf("original table regeneration: prepared=%v profile=%s", prepared != nil, profile)
					}
				})
			}
		}
	}
}
