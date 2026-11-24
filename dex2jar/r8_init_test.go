package dex2jar

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

func r8InitFixture(extraMethods int, withNonTrivialSubCtor bool) *model.Application {
	const (
		parentDesc = "Lr8/Parent;"
		subDesc    = "Lr8/Child;"
		callerDesc = "Lr8/Caller;"
	)
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: parentDesc},
			{Descriptor: subDesc},
			{Descriptor: callerDesc},
			{Descriptor: "I"},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{4}},                   // 1: (I)V
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{6}}, // 2: ([String;)V
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}, // 0
			{Class: parentDesc, Name: "<init>", ProtoIndex: 0},           // 1
			{Class: callerDesc, Name: "make", ProtoIndex: 0},             // 2
			{Class: subDesc, Name: "nonTrivialMarker", ProtoIndex: 0},    // 3
			{Class: callerDesc, Name: "main", ProtoIndex: 2},             // 4
		},
	}
	for i := 0; i < extraMethods; i++ {
		file.Methods = append(file.Methods, model.MethodID{
			Class:      "Ljava/lang/Object;",
			Name:       "pad",
			ProtoIndex: 0,
		})
	}

	parent := model.Class{
		Descriptor:  parentDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 1,
			AccessFlags: 0x10001,
			Method:      file.Methods[1],
			Code: &model.Code{
				RegistersSize: 1, InsSize: 1, OutsSize: 1,
				Instructions: []uint16{
					0x1070, 0x0000, 0x0000, // invoke-direct {v0}, Object.<init>()V
					0x000e, // return-void
				},
			},
		}},
	}

	var subDirect []model.EncodedMethod
	if withNonTrivialSubCtor {
		// Child.<init>()V calls Object.<init>()V plus a dummy method, so it is non-trivial and must NOT be replaced.
		idx := uint32(len(file.Methods))
		file.Methods = append(file.Methods, model.MethodID{Class: subDesc, Name: "<init>", ProtoIndex: 0})
		subDirect = append(subDirect, model.EncodedMethod{
			MethodIndex: idx,
			AccessFlags: 0x10001,
			Method:      file.Methods[idx],
			Code: &model.Code{
				RegistersSize: 1, InsSize: 1, OutsSize: 1,
				Instructions: []uint16{
					0x1070, 0x0000, 0x0000, // invoke-direct {v0}, Object.<init>()V
					0x1071, 0x0003, 0x0000, // invoke-static {}, nonTrivialMarker()V (extra side effect!)
					0x000e,
				},
			},
		})
	}
	sub := model.Class{
		Descriptor:    subDesc,
		Superclass:    parentDesc,
		AccessFlags:   0x0001,
		DirectMethods: subDirect,
	}

	caller := model.Class{
		Descriptor:  callerDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 2,
				AccessFlags: 0x0009,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0002, // new-instance v0, Lr8/Child;
						0x1070, 0x0000, 0x0000, // R8 stripped Child.<init> -> invokes Object.<init>(v0)!
						0x000e, // return-void
					},
				},
			},
			{
				MethodIndex: 4,
				AccessFlags: 0x0009,
				Method:      file.Methods[4],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0002, 0x0000, // invoke-static {}, make()V
						0x000e,
					},
				},
			},
		},
	}

	file.Classes = []model.Class{parent, sub, caller}
	return &model.Application{Files: []*model.File{file}}
}

func TestSynthesizeR8InlinedConstructorsNormalAnd65kOverflow(t *testing.T) {
	// Strict mode leaves R8-bypassed Object.<init> untouched (no synthesis) and rejects it strictly.
	strictApp := r8InitFixture(0, false)
	if _, _, err := applicationToJarWithPolicy(strictApp, FailureStrict); err == nil {
		t.Fatal("FailureStrict unexpectedly accepted un-synthesized Child allocation calling Object.<init>")
	}
	if len(strictApp.Files[0].Classes[1].DirectMethods) != 0 {
		t.Fatalf("FailureStrict unexpectedly synthesized constructors on Child")
	}

	// FailureRecover synthesizes Child.<init>()V -> Parent.<init>()V and rewrites Caller.make.
	recApp := r8InitFixture(0, false)
	jarBytes, detailed, err := applicationToJarWithPolicy(recApp, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover failed: %v", err)
	}
	if detailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs, got %d (%+v)", detailed.StubbedMethods, detailed.Diagnostics)
	}
	if len(recApp.Files[0].Classes[1].DirectMethods) != 1 || recApp.Files[0].Classes[1].DirectMethods[0].Method.Name != "<init>" {
		t.Fatalf("expected synthesized Child.<init>, got %+v", recApp.Files[0].Classes[1].DirectMethods)
	}
	verifyR8JarClasses(t, jarBytes)

	// 65,536-method DEX overflow: synthesized method index exceeds 0xffff -> uses Code.PatchedMethods cleanly.
	overApp := r8InitFixture(65536, false)
	overJar, overDetailed, err := applicationToJarWithPolicy(overApp, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover >65k methods failed: %v", err)
	}
	if overDetailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs on >65k DEX, got %d (%+v)", overDetailed.StubbedMethods, overDetailed.Diagnostics)
	}
	callerCode := overApp.Files[0].Classes[2].DirectMethods[0].Code
	if len(callerCode.PatchedMethods) == 0 {
		t.Fatalf("expected PatchedMethods override when method_ids > 65535")
	}
	verifyR8JarClasses(t, overJar)

	// Non-trivial existing Child.<init> is preserved intact (never overwritten).
	keepApp := r8InitFixture(0, true)
	origLen := len(keepApp.Files[0].Classes[1].DirectMethods[0].Code.Instructions)
	_, _, err = applicationToJarWithPolicy(keepApp, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover preserve non-trivial failed: %v", err)
	}
	if gotLen := len(keepApp.Files[0].Classes[1].DirectMethods[0].Code.Instructions); gotLen != origLen {
		t.Fatalf("non-trivial existing Child.<init> was modified (%d -> %d words)", origLen, gotLen)
	}
}

func verifyR8JarClasses(t *testing.T, jarBytes []byte) {
	t.Helper()
	if len(jarBytes) == 0 {
		t.Fatal("empty JAR")
	}
	zr, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	dir := t.TempDir()
	var sawChild, sawCaller bool
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".class") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		_, copyErr := buf.ReadFrom(rc)
		_ = rc.Close()
		if copyErr != nil {
			t.Fatalf("read %s: %v", f.Name, copyErr)
		}
		raw := buf.Bytes()
		parsed, err := javaclassparser.Parse(raw)
		if err != nil {
			t.Fatalf("Parse %s: %v", f.Name, err)
		}
		if f.Name == "r8/Child.class" {
			sawChild = true
			hasInit := false
			for _, m := range parsed.Methods {
				if parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex)).Value == "<init>" {
					hasInit = true
				}
			}
			if !hasInit {
				t.Fatalf("synthesized r8/Child.class is missing <init>")
			}
		}
		if f.Name == "r8/Caller.class" {
			sawCaller = true
		}
		dst := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !sawChild || !sawCaller {
		t.Fatalf("missing expected classes in JAR (child=%v caller=%v)", sawChild, sawCaller)
	}
	if java, err := exec.LookPath("java"); err == nil {
		if out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "r8.Caller").CombinedOutput(); err != nil {
			t.Fatalf("java -Xverify:all r8.Caller failed: %v\n%s", err, out)
		}
	}
}

func verifyAllR8JarClasses(t *testing.T, jarBytes []byte, mainClass string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	dir := t.TempDir()
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".class") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		_, copyErr := buf.ReadFrom(rc)
		_ = rc.Close()
		if copyErr != nil {
			t.Fatalf("read %s: %v", f.Name, copyErr)
		}
		raw := buf.Bytes()
		if _, err := javaclassparser.Parse(raw); err != nil {
			t.Fatalf("Parse %s: %v", f.Name, err)
		}
		dst := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if java, err := exec.LookPath("java"); err == nil {
		if out, err := exec.Command(java, "-Xverify:all", "-cp", dir, mainClass).CombinedOutput(); err != nil {
			t.Fatalf("java -Xverify:all %s failed: %v\n%s", mainClass, err, out)
		}
	}
}

func TestR8InitCollisionDisambiguatesWithVoidAndReusesCtor(t *testing.T) {
	const (
		midDesc    = "Lr8/WaiterBase;"
		leafDesc   = "Lr8/WaiterLeaf;"
		callerDesc = "Lr8/WaiterCaller;"
	)
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: midDesc},
			{Descriptor: leafDesc},
			{Descriptor: callerDesc},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{5}}, // 1: ([String;)V
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}, // 0
			{Class: midDesc, Name: "<init>", ProtoIndex: 0},              // 1: non-trivial collision
			{Class: midDesc, Name: "mark", ProtoIndex: 0},                // 2
			{Class: callerDesc, Name: "make1", ProtoIndex: 0},            // 3
			{Class: callerDesc, Name: "make2", ProtoIndex: 0},            // 4
			{Class: callerDesc, Name: "main", ProtoIndex: 1},             // 5
		},
	}
	mid := model.Class{
		Descriptor:  midDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 1,
				AccessFlags: 0x10001,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x1070, 0x0000, 0x0000, // Object.<init>()V
						0x0071, 0x0002, 0x0000, // mark()V (makes WaiterBase.<init>()V non-trivial!)
						0x000e,
					},
				},
			},
			{
				MethodIndex: 2,
				AccessFlags: 0x0009,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 0, InsSize: 0, OutsSize: 0,
					Instructions: []uint16{0x000e},
				},
			},
		},
	}
	leaf := model.Class{
		Descriptor:  leafDesc,
		Superclass:  midDesc,
		AccessFlags: 0x0001,
	}
	caller := model.Class{
		Descriptor:  callerDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 3,
				AccessFlags: 0x0009,
				Method:      file.Methods[3],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0002, // new-instance v0, WaiterLeaf
						0x1070, 0x0000, 0x0000, // invoke-direct {v0}, Object.<init>()V
						0x000e,
					},
				},
			},
			{
				MethodIndex: 4,
				AccessFlags: 0x0009,
				Method:      file.Methods[4],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0002, // new-instance v0, WaiterLeaf
						0x1070, 0x0000, 0x0000, // invoke-direct {v0}, Object.<init>()V
						0x000e,
					},
				},
			},
			{
				MethodIndex: 5,
				AccessFlags: 0x0009,
				Method:      file.Methods[5],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0003, 0x0000, // make1()V
						0x0071, 0x0004, 0x0000, // make2()V
						0x000e,
					},
				},
			},
		},
	}
	app := &model.Application{Files: []*model.File{{
		Entry:   "classes.dex",
		Types:   file.Types,
		Protos:  file.Protos,
		Methods: file.Methods,
		Classes: []model.Class{mid, leaf, caller},
	}}}

	jarBytes, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover failed on Void disambiguation: %v", err)
	}
	if detailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs, got %d (%+v)", detailed.StubbedMethods, detailed.Diagnostics)
	}
	// WaiterBase must keep its original <init>()V AND have exactly ONE synthetic <init>(Ljava/lang/Void;)V reused by make1 & make2.
	midPtr := &app.Files[0].Classes[0]
	voidCtorCount := 0
	for _, dm := range midPtr.DirectMethods {
		if dm.Method.Name == "<init>" {
			sig := protoSignature(app.Files[0].Protos[dm.Method.ProtoIndex])
			if sig == "(Ljava/lang/Void;)V" {
				voidCtorCount++
			} else if sig != "()V" {
				t.Fatalf("unexpected WaiterBase.<init> signature %s (Void explosion?)", sig)
			}
		}
	}
	if voidCtorCount != 1 {
		t.Fatalf("expected 1 disambiguating (Ljava/lang/Void;)V ctor on WaiterBase, got %d", voidCtorCount)
	}
	callerCls := &app.Files[0].Classes[2]
	for i := 0; i < 2; i++ {
		if callerCls.DirectMethods[i].Code.PatchedNullArgs[2] != 1 {
			t.Fatalf("make%d PatchedNullArgs[2] = %d, want 1", i+1, callerCls.DirectMethods[i].Code.PatchedNullArgs[2])
		}
	}
	verifyAllR8JarClasses(t, jarBytes, "r8.WaiterCaller")
}

func TestR8InitPlatformExceptionHierarchyChains(t *testing.T) {
	const (
		midExDesc    = "Lr8/AppMidEx;"
		leafExDesc   = "Lr8/LeafEx;"
		directExDesc = "Lr8/DirectSubEx;"
		callerDesc   = "Lr8/ExCaller;"
	)
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"boom"},
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},           // 0
			{Descriptor: "Ljava/lang/Throwable;"},        // 1
			{Descriptor: "Ljava/lang/Exception;"},        // 2
			{Descriptor: "Ljava/lang/RuntimeException;"}, // 3
			{Descriptor: "Ljava/lang/String;"},           // 4
			{Descriptor: midExDesc},                      // 5
			{Descriptor: leafExDesc},                     // 6
			{Descriptor: directExDesc},                   // 7
			{Descriptor: callerDesc},                     // 8
			{Descriptor: "V"},                            // 9
			{Descriptor: "[Ljava/lang/String;"},          // 10
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{4}},   // 1: (String)V
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{10}}, // 2: ([String;)V
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Throwable;", Name: "<init>", ProtoIndex: 1}, // 0
			{Class: "Ljava/lang/Exception;", Name: "<init>", ProtoIndex: 1}, // 1
			{Class: directExDesc, Name: "<init>", ProtoIndex: 1},            // 2
			{Class: callerDesc, Name: "main", ProtoIndex: 2},                // 3
		},
	}
	appMid := model.Class{
		Descriptor:  midExDesc,
		Superclass:  "Ljava/lang/RuntimeException;",
		AccessFlags: 0x0001,
	}
	leafEx := model.Class{
		Descriptor:  leafExDesc,
		Superclass:  midExDesc,
		AccessFlags: 0x0001,
	}
	directEx := model.Class{
		Descriptor:  directExDesc,
		Superclass:  "Ljava/lang/RuntimeException;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 2,
			AccessFlags: 0x10001,
			Method:      file.Methods[2],
			Code: &model.Code{
				RegistersSize: 2, InsSize: 2, OutsSize: 2,
				Instructions: []uint16{
					0x2070, 0x0000, 0x0010, // invoke-direct {v0, v1}, Throwable.<init>(String)V (skipped RuntimeException)
					0x000e,
				},
			},
		}},
	}
	caller := model.Class{
		Descriptor:  callerDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 3,
			AccessFlags: 0x0009,
			Method:      file.Methods[3],
			Code: &model.Code{
				RegistersSize: 3, InsSize: 1, OutsSize: 2,
				Instructions: []uint16{
					0x0022, 0x0006, // new-instance v0, LeafEx
					0x011a, 0x0000, // const-string v1, "boom"
					0x2070, 0x0001, 0x0010, // invoke-direct {v0, v1}, Exception.<init>(String)V
					0x0022, 0x0007, // new-instance v0, DirectSubEx
					0x2070, 0x0002, 0x0010, // invoke-direct {v0, v1}, DirectSubEx.<init>(String)V
					0x000e,
				},
			},
		}},
	}
	app := &model.Application{Files: []*model.File{{
		Entry:   "classes.dex",
		Strings: file.Strings,
		Types:   file.Types,
		Protos:  file.Protos,
		Methods: file.Methods,
		Classes: []model.Class{appMid, leafEx, directEx, caller},
	}}}

	jarBytes, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover platform exception chains failed: %v", err)
	}
	if detailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs, got %d (%+v)", detailed.StubbedMethods, detailed.Diagnostics)
	}
	verifyAllR8JarClasses(t, jarBytes, "r8.ExCaller")
}

func TestR8InitCatchBlockAllocationSynthesizesCtorWithoutLeakingTryUninit(t *testing.T) {
	const (
		parentDesc = "Lr8/CatchParent;"
		childDesc  = "Lr8/CatchChild;"
		callerDesc = "Lr8/CatchCaller;"
	)
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},  // 0
			{Descriptor: parentDesc},            // 1
			{Descriptor: childDesc},             // 2
			{Descriptor: callerDesc},            // 3
			{Descriptor: "V"},                   // 4
			{Descriptor: "[Ljava/lang/String;"}, // 5
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{5}}, // 1: ([String;)V
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}, // 0
			{Class: parentDesc, Name: "<init>", ProtoIndex: 0},           // 1
			{Class: callerDesc, Name: "main", ProtoIndex: 1},             // 2
		},
	}
	parent := model.Class{
		Descriptor:  parentDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 1,
			AccessFlags: 0x10001,
			Method:      file.Methods[1],
			Code: &model.Code{
				RegistersSize: 1, InsSize: 1, OutsSize: 1,
				Instructions: []uint16{
					0x1070, 0x0000, 0x0000, // invoke-direct {v0}, Object.<init>()V
					0x000e,
				},
			},
		}},
	}
	child := model.Class{
		Descriptor:  childDesc,
		Superclass:  parentDesc,
		AccessFlags: 0x0001,
	}
	catchAddr := uint32(5)
	caller := model.Class{
		Descriptor:  callerDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 2,
			AccessFlags: 0x0009,
			Method:      file.Methods[2],
			Code: &model.Code{
				RegistersSize: 3, InsSize: 1, OutsSize: 1,
				Instructions: []uint16{
					0x0222, 0x0001, // 0..1: new-instance v2, CatchParent (uninit in try block only)
					0x0012,         // 2: const/4 v0, #0
					0x0027,         // 3: throw v0 (triggers NPE into catch-all)
					0x000e,         // 4: return-void
					0x000d,         // 5: move-exception v0
					0x0122, 0x0002, // 6..7: new-instance v1, CatchChild
					0x1070, 0x0001, 0x0001, // 8..10: invoke-direct {v1}, CatchParent.<init>()V
					0x000e, // 11: return-void
				},
				Tries: []model.Try{{
					StartAddress:     0,
					InstructionCount: 4,
					Handler:          model.CatchHandler{CatchAllAddress: &catchAddr},
				}},
			},
		}},
	}
	app := &model.Application{Files: []*model.File{{
		Entry:   "classes.dex",
		Types:   file.Types,
		Protos:  file.Protos,
		Methods: file.Methods,
		Classes: []model.Class{parent, child, caller},
	}}}

	jarBytes, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover catch-block R8 ctor synthesis failed: %v", err)
	}
	if detailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs, got %d (%+v)", detailed.StubbedMethods, detailed.Diagnostics)
	}
	verifyAllR8JarClasses(t, jarBytes, "r8.CatchCaller")
}

func TestR8InitSuperclassMethodAllocatingSubclass(t *testing.T) {
	const (
		baseDesc  = "Lr8/BaseFactory;"
		childDesc = "Lr8/SubItem;"
	)
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},  // 0
			{Descriptor: baseDesc},              // 1
			{Descriptor: childDesc},             // 2
			{Descriptor: "V"},                   // 3
			{Descriptor: "[Ljava/lang/String;"}, // 4
		},
		Protos: []model.Proto{
			{ReturnType: "V"},       // 0: ()V
			{ReturnType: baseDesc},  // 1: ()Lr8/BaseFactory;
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{4}}, // 2: ([String;)V
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}, // 0
			{Class: baseDesc, Name: "<init>", ProtoIndex: 0},             // 1
			{Class: baseDesc, Name: "createChild", ProtoIndex: 1},        // 2
			{Class: baseDesc, Name: "main", ProtoIndex: 2},               // 3
		},
	}
	base := model.Class{
		Descriptor:  baseDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 1,
				AccessFlags: 0x10001,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x1070, 0x0000, 0x0000, // invoke-direct {v0}, Object.<init>()V
						0x000e,
					},
				},
			},
			{
				MethodIndex: 2,
				AccessFlags: 0x0009,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0002, // new-instance v0, Lr8/SubItem;
						0x1070, 0x0001, 0x0000, // invoke-direct {v0}, BaseFactory.<init>()V (callee.Class == c.Descriptor)
						0x0011, // return-object v0
					},
				},
			},
			{
				MethodIndex: 3,
				AccessFlags: 0x0009,
				Method:      file.Methods[3],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0002, 0x0000, // invoke-static {}, createChild()BaseFactory
						0x000e,
					},
				},
			},
		},
	}
	child := model.Class{
		Descriptor:  childDesc,
		Superclass:  baseDesc,
		AccessFlags: 0x0001,
	}
	app := &model.Application{Files: []*model.File{{
		Entry:   "classes.dex",
		Types:   file.Types,
		Protos:  file.Protos,
		Methods: file.Methods,
		Classes: []model.Class{base, child},
	}}}

	jarBytes, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover superclass factory failed: %v", err)
	}
	if detailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs, got %d (%+v)", detailed.StubbedMethods, detailed.Diagnostics)
	}
	verifyAllR8JarClasses(t, jarBytes, "r8.BaseFactory")
}

func TestR8InitEnumPlatformSuperclassCtor(t *testing.T) {
	const enumDesc = "Lr8/Color;"
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"RED"},
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Enum;"},    // 0
			{Descriptor: enumDesc},              // 1
			{Descriptor: "Ljava/lang/String;"},  // 2
			{Descriptor: "I"},                   // 3
			{Descriptor: "V"},                   // 4
			{Descriptor: "[Ljava/lang/String;"}, // 5
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{2, 3}}, // 1: (String, I)V
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{5}},        // 2: ([String;)V
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 1}, // 0
			{Class: enumDesc, Name: "<clinit>", ProtoIndex: 0},         // 1
			{Class: enumDesc, Name: "main", ProtoIndex: 2},             // 2
		},
	}
	color := model.Class{
		Descriptor:  enumDesc,
		Superclass:  "Ljava/lang/Enum;",
		AccessFlags: 0x4011,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 1,
				AccessFlags: 0x10008,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 0, OutsSize: 3,
					Instructions: []uint16{
						0x0022, 0x0001, // new-instance v0, Lr8/Color;
						0x011a, 0x0000, // const-string v1, "RED"
						0x0212,         // const/4 v2, #0
						0x3070, 0x0000, 0x0210, // invoke-direct {v0, v1, v2}, Enum.<init>(String, I)V
						0x000e,
					},
				},
			},
			{
				MethodIndex: 2,
				AccessFlags: 0x0009,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 0,
					Instructions:  []uint16{0x000e},
				},
			},
		},
	}
	app := &model.Application{Files: []*model.File{{
		Entry:   "classes.dex",
		Strings: file.Strings,
		Types:   file.Types,
		Protos:  file.Protos,
		Methods: file.Methods,
		Classes: []model.Class{color},
	}}}

	jarBytes, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover Enum.<init> chain failed: %v", err)
	}
	if detailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs, got %d (%+v)", detailed.StubbedMethods, detailed.Diagnostics)
	}
	verifyAllR8JarClasses(t, jarBytes, "r8.Color")
}

func TestR8InitPreservesAllocationAcrossTryCatch(t *testing.T) {
	const (
		parentDesc = "Lr8/TryParent;"
		subDesc    = "Lr8/TryChild;"
		callerDesc = "Lr8/TryCaller;"
	)
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: parentDesc},
			{Descriptor: subDesc},
			{Descriptor: callerDesc},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
			{Descriptor: "Ljava/lang/RuntimeException;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{5}}, // 1: ([String;)V
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0},          // 0
			{Class: parentDesc, Name: "<init>", ProtoIndex: 0},                    // 1
			{Class: callerDesc, Name: "mayThrow", ProtoIndex: 0},                  // 2
			{Class: callerDesc, Name: "make", ProtoIndex: 0},                      // 3
			{Class: callerDesc, Name: "main", ProtoIndex: 1},                      // 4
			{Class: "Ljava/lang/RuntimeException;", Name: "<init>", ProtoIndex: 0}, // 5
		},
	}
	parent := model.Class{
		Descriptor:  parentDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 1,
			AccessFlags: 0x10001,
			Method:      file.Methods[1],
			Code: &model.Code{
				RegistersSize: 1, InsSize: 1, OutsSize: 1,
				Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x000e},
			},
		}},
	}
	sub := model.Class{
		Descriptor:  subDesc,
		Superclass:  parentDesc,
		AccessFlags: 0x0001,
	}
	catchAddr := uint32(6)
	caller := model.Class{
		Descriptor:  callerDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: 0x0001,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 2,
				AccessFlags: 0x0009,
				Method:      file.Methods[2], // mayThrow(): throws new RuntimeException()
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0006, // new-instance v0, RuntimeException
						0x1070, 0x0005, 0x0000, // invoke-direct {v0}, RuntimeException.<init>()V
						0x0027, // throw v0
					},
				},
			},
			{
				MethodIndex: 3,
				AccessFlags: 0x0009,
				Method:      file.Methods[3], // make(): allocate before try, catch exception, init after merge
				Code: &model.Code{
					RegistersSize: 2, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0002, // 0..1: new-instance v0, Lr8/TryChild;
						0x0071, 0x0002, 0x0000, // 2..4: invoke-static {}, mayThrow()V
						0x0228,                 // 5: goto +2 -> offset 7
						0x010d,                 // 6: catch-all: move-exception v1
						0x1070, 0x0000, 0x0000, // 7..9: invoke-direct {v0}, Object.<init>()V (R8 bypassed TryChild.<init>)
						0x000e,                 // 10: return-void
					},
					Tries: []model.Try{{
						StartAddress:     2,
						InstructionCount: 3,
						Handler:          model.CatchHandler{CatchAllAddress: &catchAddr},
					}},
				},
			},
			{
				MethodIndex: 4,
				AccessFlags: 0x0009,
				Method:      file.Methods[4],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0003, 0x0000, // invoke-static {}, make()V
						0x000e,
					},
				},
			},
		},
	}

	app := &model.Application{Files: []*model.File{{
		Entry:   "classes.dex",
		Types:   file.Types,
		Protos:  file.Protos,
		Methods: file.Methods,
		Classes: []model.Class{parent, sub, caller},
	}}}

	jarBytes, detailed, err := applicationToJarWithPolicy(app, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover try/catch R8 init failed: %v", err)
	}
	if detailed.StubbedMethods != 0 {
		t.Fatalf("expected 0 stubs, got %d (%+v)", detailed.StubbedMethods, detailed.Diagnostics)
	}
	verifyAllR8JarClasses(t, jarBytes, "r8.TryCaller")

	// Negative test: allocation created INSIDE try after a potentially-throwing
	// instruction must NOT propagate into catch.
	negCatchAddr := uint32(7)
	negMethod := &model.EncodedMethod{
		MethodIndex: 3,
		AccessFlags: 0x0009,
		Method:      file.Methods[3],
		Code: &model.Code{
			RegistersSize: 2, InsSize: 0, OutsSize: 1,
			Instructions: []uint16{
				0x0071, 0x0002, 0x0000, // 0..2: invoke-static {}, mayThrow()V (throws BEFORE new-instance!)
				0x0022, 0x0002,         // 3..4: new-instance v0, Lr8/TryChild;
				0x0428,                 // 5..6? Wait: 0x0028 requires 1u goto+5:
				0x010d,                 // 6: nop / catch target? Let's match exact offsets below
			},
		},
	}
	negMethod.Code.Instructions = []uint16{
		0x0071, 0x0002, 0x0000, // 0..2: invoke-static {}, mayThrow()V
		0x0022, 0x0002,         // 3..4: new-instance v0, Lr8/TryChild;
		0x0528,                 // 5: goto +5 -> 10
		0x0000,                 // 6: padding nop (inside try)
		0x1070, 0x0000, 0x0000, // 7..9: catch handler: invoke-direct {v0}, Object.<init>()V
		0x000e,                 // 10: return-void
	}
	negMethod.Code.Tries = []model.Try{{
		StartAddress:     0,
		InstructionCount: 5,
		Handler:          model.CatchHandler{CatchAllAddress: &negCatchAddr},
	}}
	negInsns, err := ir.Decode(negMethod.Code, ir.Context{Entry: "classes.dex", Class: callerDesc, Method: "neg"})
	if err != nil {
		t.Fatalf("ir.Decode neg: %v", err)
	}
	hierarchy := map[string]*model.Class{
		parentDesc: &app.Files[0].Classes[0],
		subDesc:    &app.Files[0].Classes[1],
		callerDesc: &app.Files[0].Classes[2],
	}
	if sites := findR8PatchSites(app.Files[0], &app.Files[0].Classes[2], negMethod, negInsns, false, hierarchy); len(sites) != 0 {
		t.Fatalf("expected 0 patch sites when new-instance occurs inside try after throwing insn, got %+v", sites)
	}
}
