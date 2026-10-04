package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func listClassFiles(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".class") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func decompileFamily(t *testing.T, srcClassDir, dstJavaDir string) {
	t.Helper()
	if err := os.MkdirAll(dstJavaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range listClassFiles(srcClassDir) {
		base := strings.TrimSuffix(name, ".class")
		if strings.HasSuffix(base, "Oracle") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(srcClassDir, name))
		if err != nil {
			t.Fatal(err)
		}
		src, err := Decompile(raw)
		if err != nil {
			t.Fatalf("decompile %s: %v", name, err)
		}
		javaName := strings.TrimSuffix(name, ".class") + ".java"
		if err := os.WriteFile(filepath.Join(dstJavaDir, javaName), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func compileFamily(t *testing.T, dir, release string, extraFiles ...string) {
	t.Helper()
	t07RequireJavac(t)
	var srcs []string
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".java") {
			srcs = append(srcs, filepath.Join(dir, e.Name()))
		}
	}
	srcs = append(srcs, extraFiles...)
	if len(srcs) == 0 {
		t.Fatal("no java sources")
	}
	args := append([]string{"--release", release, "-d", dir}, srcs...)
	cmd := exec.Command("javac", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("infrastructure javac family: %v\n%s", err, out)
	}
}

func runJavaCP(t *testing.T, cp string, class string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("java"); err != nil {
		t.Fatalf("infrastructure: java missing: %v", err)
	}
	cmdArgs := append([]string{"-cp", cp, class}, args...)
	cmd := exec.Command("java", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", class, err, out)
	}
	return strings.TrimSpace(string(out))
}

func parseClassFile(t *testing.T, dir, name string) *ClassObject {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse %s: %v", name, err)
	}
	return obj
}

func normalizeElement(el *ElementValuePairAttribute) string {
	if el == nil {
		return "<nil>"
	}
	switch el.Tag {
	case 's':
		units := annotationStringUnits(el.Value)
		if units != nil {
			return fmt.Sprintf("s%v", units)
		}
		return fmt.Sprintf("s:%v", el.Value)
	case 'e':
		en, _ := el.Value.(*EnumConstValue)
		if en == nil {
			return "e:?"
		}
		return "e:" + en.TypeName + "." + en.ConstName
	case 'c':
		return fmt.Sprintf("c:%v", el.Value)
	case '@':
		nested, _ := el.Value.(*AnnotationAttribute)
		return "nested:" + normalizeAnnotation(nested)
	case '[':
		arr, _ := el.Value.([]*ElementValuePairAttribute)
		parts := make([]string, 0, len(arr))
		for _, x := range arr {
			parts = append(parts, normalizeElement(x))
		}
		return "arr[" + strings.Join(parts, ",") + "]"
	default:
		switch v := el.Value.(type) {
		case *ConstantIntegerInfo:
			return fmt.Sprintf("%c:%d", el.Tag, v.Value)
		case *ConstantLongInfo:
			return fmt.Sprintf("%c:%d", el.Tag, v.Value)
		case *ConstantFloatInfo:
			return fmt.Sprintf("%c:%f", el.Tag, v.Value)
		case *ConstantDoubleInfo:
			return fmt.Sprintf("%c:%f", el.Tag, v.Value)
		default:
			return fmt.Sprintf("%c:%v", el.Tag, el.Value)
		}
	}
}

func normalizeAnnotation(a *AnnotationAttribute) string {
	if a == nil {
		return "<nil>"
	}
	pairs := append([]*ElementValuePairAttribute(nil), a.ElementValuePairs...)
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Name < pairs[j].Name })
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p == nil {
			continue
		}
		parts = append(parts, p.Name+"="+normalizeElement(p))
	}
	return a.TypeName + "{" + strings.Join(parts, ",") + "}"
}

func methodByName(obj *ClassObject, name string) *MemberInfo {
	if obj == nil {
		return nil
	}
	for _, m := range obj.Methods {
		n, _ := obj.getUtf8(m.NameIndex)
		if n == name {
			return m
		}
	}
	return nil
}

func annotationDefaultOf(obj *ClassObject, elem string) string {
	m := methodByName(obj, elem)
	if m == nil {
		return ""
	}
	for _, attr := range m.Attributes {
		if ad, ok := attr.(*AnnotationDefaultAttribute); ok && ad.DefaultValue != nil {
			return normalizeElement(ad.DefaultValue)
		}
	}
	return ""
}
