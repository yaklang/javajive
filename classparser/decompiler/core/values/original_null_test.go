package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalNullLiteralRequiresUnchangedActualProducer(t *testing.T) {
	for _, change := range []string{"original", "zero PC", "negative PC", "excess PC", "copy", "synthetic object null", "string type", "numeric type", "missing type", "changed value", "UTF16 units", "missing origin", "nil node"} {
		t.Run(change, func(t *testing.T) {
			value := NewOriginalNullLiteral(19)
			switch change {
			case "zero PC":
				value = NewOriginalNullLiteral(0)
			case "negative PC":
				value = NewOriginalNullLiteral(-1)
			case "excess PC":
				value = NewOriginalNullLiteral(65536)
			case "copy":
				copy := *value
				value = &copy
			case "synthetic object null":
				value = NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "string type":
				value.JavaType = types.NewJavaClass("java.lang.String")
			case "numeric type":
				value.JavaType = types.NewJavaPrimer(types.JavaInteger)
			case "missing type":
				value.JavaType = nil
			case "changed value":
				value.Data = "not null"
			case "UTF16 units":
				value.Units = []uint16{'n', 'u', 'l', 'l'}
			case "missing origin":
				value.originalNull = nil
			case "nil node":
				value = nil
			}
			pc, known := value.OriginalNullPC()
			want := change == "original" || change == "zero PC"
			wantPC := 19
			if change == "zero PC" {
				wantPC = 0
			}
			if known != want || known && pc != wantPC {
				t.Fatalf("origin=%d/%v want=%v", pc, known, want)
			}
		})
	}
}
