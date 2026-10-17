package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestAdversarialJavaIdentifierAlphabetMatchesJDK21(t *testing.T) {
	home := os.Getenv("JAVA_HOME")
	if home == "" {
		t.Fatal("JAVA_HOME must select the pinned JDK21 identifier oracle")
	}
	dir := t.TempDir()
	source := `import java.io.*;class IdentifierOracle{public static void main(String[]args)throws Exception{DataOutputStream out=new DataOutputStream(new BufferedOutputStream(System.out));out.writeInt(Integer.parseInt(System.getProperty("java.specification.version")));for(int c=0;c<=Character.MAX_CODE_POINT;c++){int flags=(Character.isJavaIdentifierStart(c)?1:0)|(Character.isJavaIdentifierPart(c)?2:0)|(Character.isIdentifierIgnorable(c)?4:0);out.writeByte(flags);}out.close();}}`
	p := filepath.Join(dir, "IdentifierOracle.java")
	if e := os.WriteFile(p, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command(filepath.Join(home, "bin/javac"), "-proc:none", "--release", "8", "-d", dir, p).CombinedOutput(); e != nil {
		t.Fatalf("compile trusted Character oracle: %v\n%s", e, out)
	}
	raw, e := exec.Command(filepath.Join(home, "bin/java"), "-XX:-UsePerfData", "-Xmx64m", "-XX:+UseSerialGC", "-XX:ActiveProcessorCount=1", "-cp", dir, "IdentifierOracle").Output()
	if e != nil {
		t.Fatal(e)
	}
	if len(raw) != 4+utf8.MaxRune+1 || binary.BigEndian.Uint32(raw[:4]) != 21 {
		t.Fatalf("require full pinned JDK21 code-point oracle, bytes=%d", len(raw))
	}
	for c := rune(0); c <= utf8.MaxRune; c++ {
		s := string(c)
		flags := raw[4+int(c)]
		start := flags&1 != 0 && s != "_"
		part := flags&2 != 0 && flags&4 == 0
		if got := class_context.SafeIdentifier(s) == s; got != start {
			t.Fatalf("U+%04X stable start=%v expected=%v from independent Character oracle", c, got, start)
		}
		if got := class_context.SafeIdentifier("a"+s) == "a"+s; got != part {
			t.Fatalf("U+%04X stable part=%v expected=%v (javac removes ignorable characters)", c, got, part)
		}
	}
	t.Logf("verified stable start/part behavior over all %d code points against the independent JDK21 Character oracle", utf8.MaxRune+1)
}
