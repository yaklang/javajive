package lower

import (
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

func TestDebugParameterRegistersUseWideSlots(t *testing.T) {
	code := &model.Code{RegistersSize: 8, InsSize: 5}
	proto := model.Proto{Parameters: []string{"J", "I", "D"}}
	got := debugParameterRegisters(code, proto, dexStatic)
	want := []uint16{3, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("registers = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("registers = %v, want %v", got, want)
		}
	}
}

func TestAnnotationIndexesAndFallback(t *testing.T) {
	annotation := model.Annotation{Type: "LIndexed;"}
	class := &model.Class{
		FieldAnnotations:     []model.FieldAnnotation{{FieldIndex: 7, Annotations: []model.Annotation{annotation}}},
		MethodAnnotations:    []model.MethodAnnotation{{MethodIndex: 8, Annotations: []model.Annotation{annotation}}},
		ParameterAnnotations: []model.ParameterAnnotation{{MethodIndex: 9, Parameters: [][]model.Annotation{{annotation}}}},
		FieldAnnotationIndex: map[uint32]int{7: 0}, MethodAnnotationIndex: map[uint32]int{8: 0}, ParameterAnnotationIndex: map[uint32]int{9: 0},
	}
	if got := fieldAnnotationsOf(class, 7); len(got) != 1 || got[0].Type != annotation.Type {
		t.Fatalf("indexed field annotations = %#v", got)
	}
	if got := methodAnnotationsOf(class, 8); len(got) != 1 || got[0].Type != annotation.Type {
		t.Fatalf("indexed method annotations = %#v", got)
	}
	if got := parameterAnnotationsOf(class, 9); len(got) != 1 || len(got[0]) != 1 || got[0][0].Type != annotation.Type {
		t.Fatalf("indexed parameter annotations = %#v", got)
	}
	class.FieldAnnotationIndex = nil
	class.MethodAnnotationIndex = nil
	class.ParameterAnnotationIndex = nil
	if len(fieldAnnotationsOf(class, 7)) != 1 || len(methodAnnotationsOf(class, 8)) != 1 || len(parameterAnnotationsOf(class, 9)) != 1 {
		t.Fatal("annotation slice fallback failed")
	}
}

func BenchmarkMethodAnnotationIndexScale(b *testing.B) {
	const count = 100000
	class := &model.Class{
		MethodAnnotations:     make([]model.MethodAnnotation, count),
		MethodAnnotationIndex: make(map[uint32]int, count),
	}
	for index := range class.MethodAnnotations {
		class.MethodAnnotations[index] = model.MethodAnnotation{MethodIndex: uint32(index)}
		class.MethodAnnotationIndex[uint32(index)] = index
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = methodAnnotationsOf(class, uint32(i%count))
	}
}
