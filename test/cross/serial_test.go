package cross

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jj "github.com/yaklang/javajive"
)

const serGenJava = `import java.io.*;
import java.util.*;

public class SerGen {
    public static void main(String[] args) throws Exception {
        String outFile = args[0];
        String which = args.length > 1 ? args[1] : "string";
        Object target;
        switch (which) {
            case "string":
                target = "hello-from-java";
                break;
            case "list": {
                ArrayList<String> l = new ArrayList<>();
                l.add("alpha");
                l.add("beta");
                l.add("gamma");
                target = l;
                break;
            }
            case "map": {
                HashMap<String, String> m = new HashMap<>();
                m.put("key", "value");
                target = m;
                break;
            }
            default:
                target = "default";
        }
        try (FileOutputStream fos = new FileOutputStream(outFile);
             ObjectOutputStream oos = new ObjectOutputStream(fos)) {
            oos.writeObject(target);
        }
    }
}
`

// serialize compiles+runs SerGen to produce a real Java-serialized blob.
func serialize(t *testing.T, which string) []byte {
	t.Helper()
	dir := t.TempDir()
	compileJava(t, dir, map[string]string{"SerGen.java": serGenJava})
	out := filepath.Join(dir, "obj.ser")
	runJava(t, dir, "SerGen", out, which)
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read serialized blob: %v", err)
	}
	if len(raw) < 4 || raw[0] != 0xac || raw[1] != 0xed {
		t.Fatalf("not a java serialization stream: %x", raw[:min(8, len(raw))])
	}
	return raw
}

// TestCrossSerialStringRoundTrip checks an exact byte round-trip for the JDK's
// canonical serialization of a String.
func TestCrossSerialStringRoundTrip(t *testing.T) {
	raw := serialize(t, "string")

	objs, err := jj.ParseSerialized(raw)
	if err != nil {
		t.Fatalf("ParseSerialized: %v", err)
	}
	again := jj.MarshalSerialized(objs...)
	if hex.EncodeToString(again) != hex.EncodeToString(raw) {
		t.Fatalf("string round-trip mismatch:\n have %x\n want %x", again, raw)
	}

	js, err := jj.SerializedToJSON(objs...)
	if err != nil {
		t.Fatalf("SerializedToJSON: %v", err)
	}
	if !strings.Contains(string(js), "hello-from-java") {
		t.Fatalf("json missing value: %s", js)
	}
}

// TestCrossSerialComplexObjects checks that real JDK-serialized collections parse
// and re-marshal to a stable (parse-equivalent) form.
func TestCrossSerialComplexObjects(t *testing.T) {
	for _, tc := range []struct {
		which     string
		classHint string
	}{
		{"list", "java.util.ArrayList"},
		{"map", "java.util.HashMap"},
	} {
		t.Run(tc.which, func(t *testing.T) {
			raw := serialize(t, tc.which)

			objs, err := jj.ParseSerialized(raw)
			if err != nil {
				t.Fatalf("ParseSerialized(%s): %v", tc.which, err)
			}

			js, err := jj.SerializedToJSON(objs...)
			if err != nil {
				t.Fatalf("SerializedToJSON(%s): %v", tc.which, err)
			}
			if !strings.Contains(string(js), tc.classHint) {
				t.Errorf("json for %s missing class hint %q", tc.which, tc.classHint)
			}

			// Re-marshal, then re-parse: the structure must be stable.
			remarshaled := jj.MarshalSerialized(objs...)
			reobjs, err := jj.ParseSerialized(remarshaled)
			if err != nil {
				t.Fatalf("re-ParseSerialized(%s): %v", tc.which, err)
			}
			js2, err := jj.SerializedToJSON(reobjs...)
			if err != nil {
				t.Fatalf("re-SerializedToJSON(%s): %v", tc.which, err)
			}
			if string(js) != string(js2) {
				t.Errorf("%s not stable across marshal/parse round-trip", tc.which)
			}
		})
	}
}
