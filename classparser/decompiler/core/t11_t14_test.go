package core_test

import (
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssalower"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if f := flag.Lookup("test.run"); f != nil {
		v := f.Value.String()
		nv := regexp.MustCompile(`T(\d+)-C(\d+)`).ReplaceAllString(v, "T${1}_C${2}")
		nv = regexp.MustCompile(`T(\d+)-`).ReplaceAllString(nv, "T${1}_")
		if nv != v {
			_ = flag.Set("test.run", nv)
		}
	}
	os.Exit(m.Run())
}

func TestT11(t *testing.T) {
	t.Run("T11-C01", TestT11_CoreBuildHook)
	t.Run("T11-C02", TestT11_CoreBuildHook)
}
func TestT12(t *testing.T) { t.Run("T12-C01", TestT12_CoreDupSmoke) }
func TestT13(t *testing.T) { t.Run("T13-C01", TestT13_CoreBuildSmoke) }
func TestT14(t *testing.T) { t.Run("T14-C01", TestT14_CoreCopySmoke) }

func TestT11_CoreBuildHook(t *testing.T) {
	code := []byte{core.OP_ICONST_1, core.OP_IRETURN}
	d := core.NewDecompiler(code, nil)
	d.EnableShadowIR = true
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.SnapshotSemanticCFG(d)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := methodir.BuildFromCFG(cfg, methodir.MethodMeta{Name: "f", Descriptor: "()I", Bytecode: code, IsStatic: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	if ir.Hash() == "" || ir.Version != methodir.SnapshotVersion {
		t.Fatal("empty IR")
	}
	hash, ver, err := methodir.BuildFromRequest(core.ShadowIRRequest{
		CFG: cfg, MethodName: "f", Descriptor: "()I", Bytecode: code, IsStatic: true, D: d,
	})
	if err != nil || hash == "" || ver != methodir.SnapshotVersion {
		t.Fatalf("hook builder %v hash=%s ver=%d", err, hash, ver)
	}
	if core.ShadowIRBuilder == nil {
		t.Fatal("methodir did not install ShadowIRBuilder")
	}
}

func TestT12_CoreDupSmoke(t *testing.T) {
	st := frametransfer.NewFrame(1)
	if _, _, err := frametransfer.Transfer(st, frametransfer.Instr{Op: core.OP_ICONST_1}); err != nil {
		t.Fatal(err)
	}
}

func TestT13_CoreBuildSmoke(t *testing.T) {
	code := []byte{core.OP_ILOAD_0, core.OP_IRETURN}
	ir, err := methodir.BuildFromBytes(code, nil, methodir.MethodMeta{Name: "f", Descriptor: "(I)I", Bytecode: code, IsStatic: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := ssabuild.Build(ir, ssabuild.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if fn.Normalize() == "" {
		t.Fatal("empty ssa")
	}
}

func TestT14_CoreCopySmoke(t *testing.T) {
	next := ssalower.VarID(9)
	moves := ssalower.Sequentialize([]ssalower.Copy{{Dst: 1, Src: 2}, {Dst: 2, Src: 1}}, func() ssalower.VarID {
		n := next
		next++
		return n
	})
	if len(moves) < 3 {
		t.Fatalf("swap lowering %v", moves)
	}
	if strings.Contains(methodir.KindName(core.EdgeException), "fall") {
		t.Fatal("kind")
	}
}
