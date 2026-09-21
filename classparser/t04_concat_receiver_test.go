package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestGenericConcatGetBytesReceiverParenthesized(t *testing.T) {
	const src = `public class ConcatRecvBox {
  static final String SEP = "|";
  public static byte[] pack(Object payload) {
    return (payload + SEP).getBytes();
  }
  public static void main(String[] args) {
    System.out.println(new String(pack("alpha")));
  }
}
`
	origOut, classes := t04CompileRun(t, "8", "ConcatRecvBox", map[string]string{"ConcatRecvBox.java": src})
	if strings.TrimSpace(origOut) != "alpha|" {
		t.Fatalf("original stdout %q", origOut)
	}
	t04RoundTripModes(t, "8", "ConcatRecvBox", origOut, classes, func(t *testing.T, dumped string) {
		if strings.Contains(dumped, "payload + SEP.getBytes()") || strings.Contains(dumped, "var1 + SEP.getBytes()") {
			t.Fatalf("unparenthesized concat receiver:\n%s", dumped)
		}
		if !strings.Contains(dumped, ").getBytes()") {
			t.Fatalf("expected parenthesized concat.getBytes():\n%s", dumped)
		}
	})
	t.Run("logback_kill_switch_does_not_unparen", func(t *testing.T) {
		t.Setenv("JDEC_LOGBACK_REMAINING_OFF", "1")
		res, err := DecompileWithOptions(classes["ConcatRecvBox"], DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(res.Source, "var1 + SEP.getBytes()") && !strings.Contains(res.Source, ").getBytes()") {
			t.Fatalf("kill-switch undid concat parens:\n%s", res.Source)
		}
	})
}

func TestGenericConcatReceiverKillSwitchDoesNotUndoParens(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/EchoEncoder.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_LOGBACK_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(off, "var1 + CoreConstants.LINE_SEPARATOR.getBytes()") &&
		!strings.Contains(off, "(var1 + CoreConstants.LINE_SEPARATOR).getBytes()") {
		t.Fatalf("kill-switch must not undo structural concat parens:\n%s", off)
	}
}
