package dex2jar_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/dex2jar/lower"
	"github.com/yaklang/javajive/dex2jar/model"
)

func TestAdversarialP1DEXWebSplitAndEntryLoop(t *testing.T) {
	// Scenario:
	// 1. Instance method `process(String p)`: reads `this` and `p`, overwrites `p` (v2) with an
	//    unrelated `int` constant used in a separate loop, and checks cast.
	//    Web Splitting must keep parameter `p` direct (zero `String varX = p;` copy) and give
	//    the int web its own clean JVM local slot.
	// 2. Entry-block PC 0 loop back-edge in static method `pc0Loop(int n)` (`if-le` at block 0).
	const owner = "Ladv/WebSplitProbe;"
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: owner}, {Descriptor: "Ljava/lang/String;"}, {Descriptor: "I"}},
		Strings: []string{"done"},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{1, 2}},
			{ReturnType: "I"},
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{2}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "process", ProtoIndex: 0},
			{Class: "Ljava/lang/String;", Name: "length", ProtoIndex: 1},
			{Class: owner, Name: "pc0Loop", ProtoIndex: 2},
		},
	}
	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexAccessPublic,
		VirtualMethods: []model.EncodedMethod{
			{
				MethodIndex: 0,
				AccessFlags: dexAccessPublic,
				Method:      file.Methods[0],
				// v1=this, v2=String p, v3=int limit
				Code: &model.Code{
					RegistersSize: 4,
					InsSize:       3,
					OutsSize:      1,
					Instructions: []uint16{
						0x106e, 0x0001, 0x0002, // 0: invoke-virtual {v2}, String.length()I
						0x000a,         // 3: move-result v0
						0x1212,         // 4: const/4 v2, 1   (reuses String param register v2 as int!)
						0x3035, 0x0004, // 5: if-ge v0, v3, +4 (to offset 9)
						0x00d8, 0x0200, // 7: add-int/lit8 v0, v0, 2 (note v2=1 still live)
						0x0090, 0x0200, // 9: add-int v0, v0, v2
						0x000f, // 11: return v0
					},
				},
			},
		},
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 2,
				AccessFlags: dexAccessPublic | dexAccessStatic,
				Method:      file.Methods[2],
				// v0=int n (param) reused directly as counter starting at offset 0 (backedge jumps to PC 0!)
				Code: &model.Code{
					RegistersSize: 1,
					InsSize:       1,
					Instructions: []uint16{
						0x0439, 0x0000, // 0: if-nez v0, +4? Wait: add-int/lit8 then goto PC 0
					},
				},
			},
		},
	}
	// Replace pc0Loop with valid terminating PC 0 loop:
	// 0: if-eqz v0, +4 (exit)
	// 2: add-int/lit8 v0, v0, -1
	// 4: goto -4 (PC 0)
	// 5: return v0
	cls.DirectMethods[0].Code.Instructions = []uint16{
		0x0038, 0x0005, // 0: if-eqz v0, +5 -> offset 5
		0xffd8, 0x0000, // 2: add-int/lit8 v0, v0, -1? wait: aa=v0,bb=-1 -> 0xff00
		0xfc28, // 4: goto -4 -> offset 0? check goto offset math: 4 + (-4) = 0 -> 0xfc28
		0x000f, // 5: return v0
	}
	// In dex 22b add-int/lit8 vAA, vBB, #+CC: word0=(vAA<<8)|0xd8, word1=(lit8<<8)|vBB
	cls.DirectMethods[0].Code.Instructions = []uint16{
		0x0038, 0x0005, // 0: if-eqz v0, +5 -> 5
		0x00d8, 0xff00, // 2: add-int/lit8 v0, v0, -1
		0xfc28, // 4: goto -4 -> 0
		0x000f, // 5: return v0
	}

	raw, err := lower.Class(file, cls)
	if err != nil {
		t.Fatalf("lower.Class failed: %v", err)
	}
	src := decompileLoweredClass(t, raw)
	if strings.Contains(src, classparser.DecompileStubMarker) {
		t.Fatalf("unexpected stub method:\n%s", src)
	}
	if !strings.Contains(src, "process(") || !strings.Contains(src, "pc0Loop(") {
		t.Fatalf("expected both methods in decompiled source:\n%s", src)
	}
	if !strings.Contains(src, ".length()") {
		t.Fatalf("expected String.length() call on direct String param:\n%s", src)
	}
}

func TestAdversarialP1AABZeroPanicsAndNewlyRecoveredClasses(t *testing.T) {
	aab := filepath.Join("..", "testdata", "external", "bundletool-install-time.aab")
	if _, err := os.Stat(aab); err != nil {
		t.Skipf("external AAB fixture unavailable: %v", err)
	}
	jarPath := filepath.Join(t.TempDir(), "aab-p1.jar")
	detailed, err := dex2jar.DexFileToJarWithOptions(aab, jarPath, dex2jar.ConvertOptions{
		Policy: dex2jar.FailureRecover,
	})
	if err != nil {
		t.Fatalf("DexFileToJarWithOptions: %v", err)
	}
	if detailed.Summary.Classes != 1298 {
		t.Fatalf("expected 1298 classes, got %d", detailed.Summary.Classes)
	}

	fs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("NewJarFSFromLocal: %v", err)
	}
	defer fs.Close()
	fs.SetRetainUnprovedSource(true)

	// Classes that previously had class failures, checked-escape failures, sentinel leaks,
	// constructor prefix stubs, multi-continuation forward region stubs, or analysis budget failures
	// and now decompile cleanly with 0 stubs:
	zeroStubTargets := []string{
		"com/google/android/samples/dynamicfeatures/MainActivity.class",
		"androidx/constraintlayout/widget/ConstraintLayout.class",
		"androidx/fragment/app/Fragment.class",
		"androidx/appcompat/widget/i0.class",
		"a/g/e/d.class",
		"a/g/e/i.class",
		"com/google/android/material/bottomsheet/BottomSheetBehavior.class",
		"com/google/android/material/appbar/AppBarLayout$BaseBehavior.class",
		"com/google/android/material/snackbar/SnackbarContentLayout.class",
	}
	for _, entry := range zeroStubTargets {
		raw, rerr := fs.ReadFile(entry)
		if rerr != nil {
			t.Fatalf("ReadFile(%s): %v", entry, rerr)
		}
		src := string(raw)
		if strings.Contains(src, "// decompile dump failed") {
			t.Fatalf("%s failed class decompilation:\n%s", entry, src)
		}
		if strings.Contains(src, classparser.DecompileStubMarker) {
			t.Fatalf("%s still contains degraded stub method:\n%s", entry, src)
		}
		if entry == "a/g/e/d.class" || entry == "a/g/e/i.class" {
			if strings.Contains(src, "= Exception") {
				t.Fatalf("%s leaked exception sentinel:\n%s", entry, src)
			}
		}
	}

	// Verify a/e/a/j/c no longer fails analysis budget, and u0 no longer panics.
	cRaw, rerr := fs.ReadFile("a/e/a/j/c.class")
	if rerr != nil {
		t.Fatalf("ReadFile(a/e/a/j/c): %v", rerr)
	}
	if strings.Contains(string(cRaw), "analysis_budget_exceeded") {
		t.Fatalf("a/e/a/j/c still hit analysis_budget_exceeded:\n%s", string(cRaw))
	}
	u0Raw, rerr := fs.ReadFile("b/a/a/b/a/a/u0.class")
	if rerr != nil {
		t.Fatalf("ReadFile(u0): %v", rerr)
	}
	u0Src := string(u0Raw)
	if strings.Contains(u0Src, "panic:") || !strings.Contains(u0Src, "class u0") {
		t.Fatalf("u0 decompilation regressed or panicked:\n%s", u0Src)
	}
}
