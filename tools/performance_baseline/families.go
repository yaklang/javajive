package performance_baseline

import (
	"crypto/sha256"
	"fmt"

	"github.com/yaklang/javajive/classparser/classes"
)

type Family struct {
	Meta  FamilyMeta
	Bytes []byte
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}

func family(kind, scale string, n int, shape, internal string, src string, reviewed bool, raw []byte) Family {
	return Family{
		Meta: FamilyMeta{
			ID:            fmt.Sprintf("%s/%s", kind, scale),
			Kind:          kind,
			ScaleLabel:    scale,
			N:             n,
			Shape:         shape,
			ClassInternal: internal,
			SHA256:        sha256Hex(raw),
			SizeBytes:     len(raw),
			Source:        src,
			Reviewed:      reviewed,
		},
		Bytes: raw,
	}
}

func mustSynthetic(internal, desc string, run methodSpec) []byte {
	raw, err := makeSyntheticClass(internal, desc, run)
	if err != nil {
		panic(err)
	}
	return raw
}

func loopFamily(n int, scale string) Family {
	name := fmt.Sprintf("t32/LoopN%d", n)
	src := javaSourceLoop("t32", fmt.Sprintf("LoopN%d", n), n)
	raw := mustJavacFamily(name, src)
	return family("loop", scale, n, "sequential_counted_loops", name, "javac_release8_xverify_all", false, raw)
}

func slotFamily(n int, scale string) Family {
	name := fmt.Sprintf("t32/SlotN%d", n)
	src := javaSourceSlot("t32", fmt.Sprintf("SlotN%d", n), n)
	raw := mustJavacFamily(name, src)
	return family("slot", scale, n, "local_slot_stores_with_phi_merge", name, "javac_release8_xverify_all", false, raw)
}

func deepFamily(n int, scale string) Family {
	name := fmt.Sprintf("t32/DeepExprN%d", n)
	src := javaSourceDeep("t32", fmt.Sprintf("DeepExprN%d", n), n)
	raw := mustJavacFamily(name, src)
	return family("deep_expr", scale, n, "left_fold_iadd_chain", name, "javac_release8_xverify_all", false, raw)
}

func handlerDenseFamily(n int, scale string) Family {
	name := fmt.Sprintf("t32/HandlerDenseN%d", n)
	src := javaSourceHandlerDense("t32", fmt.Sprintf("HandlerDenseN%d", n), n)
	raw := mustJavacFamily(name, src)
	return family("handler_dense", scale, n, "nested_exception_handlers", name, "javac_release8_xverify_all", false, raw)
}

func handlerSparseFamily(n int, scale string) Family {
	name := fmt.Sprintf("t32/HandlerSparseN%d", n)
	src := javaSourceHandlerSparse("t32", fmt.Sprintf("HandlerSparseN%d", n), n)
	raw := mustJavacFamily(name, src)
	return family("handler_sparse", scale, n, "disjoint_exception_handlers", name, "javac_release8_xverify_all", false, raw)
}

func tinyRealFamily() Family {
	raw, err := classes.FS.ReadFile("LogicalOperationMini.class")
	if err != nil {
		panic(err)
	}
	internal := "org/benf/cfr/reader/LogicalOperationMini"
	if err := verifyAllClassBytes(internal, raw); err != nil {
		panic(err)
	}
	return family("tiny_real", "reviewed", 0, "reviewed_repo_testdata_LogicalOperationMini", internal, "classparser/classes/static/LogicalOperationMini.class", true, raw)
}

// FrozenFamilies returns the T32 input families. Generation is deterministic.
func FrozenFamilies() []Family {
	n := ScaleBaseLoopSlotDeep
	hn := ScaleBaseHandler
	return []Family{
		loopFamily(n, "N"),
		loopFamily(2*n, "2N"),
		loopFamily(4*n, "4N"),
		slotFamily(n, "N"),
		slotFamily(2*n, "2N"),
		slotFamily(4*n, "4N"),
		deepFamily(n, "N"),
		deepFamily(2*n, "2N"),
		deepFamily(4*n, "4N"),
		handlerDenseFamily(hn, "N"),
		handlerDenseFamily(2*hn, "2N"),
		handlerDenseFamily(4*hn, "4N"),
		handlerSparseFamily(hn, "N"),
		handlerSparseFamily(2*hn, "2N"),
		handlerSparseFamily(4*hn, "4N"),
		tinyRealFamily(),
	}
}

func familyByID(id string) (Family, error) {
	for _, f := range FrozenFamilies() {
		if f.Meta.ID == id {
			return f, nil
		}
	}
	return Family{}, fmt.Errorf("unknown family %q", id)
}

func scaleFamilyIDs(kind string) []string {
	return []string{kind + "/N", kind + "/2N", kind + "/4N"}
}
