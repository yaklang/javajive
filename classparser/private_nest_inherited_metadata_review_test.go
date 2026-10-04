package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrivateNestBridgeRequiresCompleteInheritedNameMetadata(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	const source = `
class OriginalNestBridgeParent {Object inherited(Object token){return token;}}
public class InheritedNestMetadataReview {
 static class Owner extends OriginalNestBridgeParent {private Object target(Object token){return token;}}
 static class Reader {static Object read(Owner owner,Object token){return owner.target(token);}}
}
`
	file := filepath.Join(dir, "InheritedNestMetadataReview.java")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "11", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("original: %v\n%s", err, out)
	}
	rawClasses := map[string][]byte{}
	objects := map[string]*ClassObject{}
	files, err := filepath.Glob(filepath.Join(dir, "*.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		object, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		rawClasses[object.GetClassName()] = raw
		objects[object.GetClassName()] = object
	}
	owner := objects["InheritedNestMetadataReview$Owner"]
	if owner == nil {
		t.Fatal("missing original owner")
	}
	for _, hideParent := range []bool{false, true} {
		dumper := NewClassObjectDumper(owner)
		dumper.foldSiblingResolver = func(name string) ([]byte, bool) {
			if hideParent && name == "OriginalNestBridgeParent" {
				return nil, false
			}
			b, ok := rawClasses[name]
			return b, ok
		}
		plan, err := dumper.buildPrivateNestPlan(owner, func(name string) (*ClassObject, bool) { object, ok := objects[name]; return object, ok })
		if err != nil {
			t.Fatal(err)
		}
		if hideParent {
			if plan != nil {
				t.Fatalf("missing inherited declaration table admitted bridge: %#v", plan)
			}
			continue
		}
		if plan == nil || len(plan.bridges) != 1 || len(plan.sites) != 1 {
			t.Fatalf("complete original metadata has no exact bridge: %#v", plan)
		}
	}
}
