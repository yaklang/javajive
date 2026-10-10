package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeIndependentAnonymousForestRequiresCompleteOwnedScope(t *testing.T) {
	fixture, outer := nativeIndependentForestFixture("nested", "original")
	files := nativeCompileClasses(t, fixture)
	nativeIndependentForestInput(t, files, outer)
	for _, variant := range []string{"original", "no forest", "foreign family", "missing unit", "missing owned object", "foreign object identity", "missing group", "failed group", "changed enclosing owner", "cyclic enclosing owner", "changed method", "physical outer claimed", "assertion protocol", "budget", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			obj, err := Parse(files[outer+"$Bag.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(obj)
			cert := d.originalNativeMemberIndependentRoot()
			if cert == nil {
				t.Fatal("original independent boundary")
			}
			p := d.planNativeMemberFamilyFromRoot(cert)
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("complete owned anonymous forest")
			}
			forest := p.anonymousForest
			name := outer + "$Bag$1$1"
			unit := forest.units[name]
			if unit == nil {
				t.Fatal("original nested unit")
			}
			var work *workbudget.Budget
			switch variant {
			case "no forest":
				p.anonymousForest = nil
			case "foreign family":
				forest.members = nil
			case "missing unit":
				delete(forest.units, name)
			case "missing owned object":
				delete(p.lexicalObjects, name)
			case "foreign object identity":
				copy := *unit.object
				forest.objects[name] = &copy
			case "missing group":
				delete(forest.groups, outer+"$Bag$1")
			case "failed group":
				forest.groups[outer+"$Bag$1"].failed = true
			case "changed enclosing owner", "cyclic enclosing owner":
				owner := "Foreign"
				if variant == "cyclic enclosing owner" {
					owner = name
				}
				cp := NewConstantPoolWithConstant(&unit.object.ConstantPool)
				for _, a := range unit.object.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						binary.BigEndian.PutUint16(raw.Info, uint16(cp.AddNewClassInfo(owner)))
					}
				}
			case "changed method":
				unit.method += "changed"
			case "physical outer claimed":
				original, _ := Parse(files[outer+".class"])
				forest.objects[outer] = original
				p.lexicalObjects[outer] = original
			case "assertion protocol":
				for _, constant := range unit.object.ConstantPool {
					if utf8, ok := constant.(*ConstantUtf8Info); ok && utf8.Value == "token" {
						utf8.Value = nativeAssertionField
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := cert.familyClosed(p, work); got != (variant == "original") {
				t.Fatalf("complete original independent scope %v", got)
			}
			if z.sourceOwnership.bytes != 0 {
				t.Fatal("proof query committed source")
			}
		})
	}
}

func TestNativeIndependentAnonymousForestReadOrderAndConcurrency(t *testing.T) {
	fixture, outer := nativeIndependentForestFixture("nested", "original")
	files := nativeCompileClasses(t, fixture)
	nativeIndependentForestInput(t, files, outer)
	names := []string{outer + "$Bag.class", outer + "$Bag$1.class", outer + "$Bag$1$1.class", outer + "$Bag$1$2.class"}
	var expected [][]byte
	for _, reverse := range []bool{false, true} {
		z := nativeArchive(t, files)
		observed := make([][]byte, len(names))
		for i := range names {
			index := i
			if reverse {
				index = len(names) - 1 - i
			}
			source, err := z.ReadFile(names[index])
			if err != nil {
				t.Fatal(err)
			}
			observed[index] = source
		}
		if expected == nil {
			expected = observed
		} else {
			for i := range names {
				if !bytes.Equal(expected[i], observed[i]) {
					t.Fatal("source depends on original read order", names[i])
				}
			}
		}
		if !strings.Contains(string(observed[0]), "new ForestResult<T>()") {
			t.Fatal("root did not commit anonymous source")
		}
		for i := 1; i < len(names); i++ {
			if !strings.Contains(string(observed[i]), "original anonymous body owned by") {
				t.Fatal("owned anonymous unit was emitted twice", names[i])
			}
		}
		if z.sourceOwnership.bytes != int64(len(observed[0])) {
			t.Fatal("source ownership charged more than its single root")
		}
		z.Close()
	}
	for _, warm := range []bool{false, true} {
		z := nativeArchive(t, files)
		if warm {
			if _, err := z.ReadFile(names[0]); err != nil {
				t.Fatal(err)
			}
		}
		var wait sync.WaitGroup
		results := make([][]byte, 16)
		errors := make([]error, len(results))
		for i := range results {
			wait.Add(1)
			go func(i int) { defer wait.Done(); results[i], errors[i] = z.ReadFile(names[i%len(names)]) }(i)
		}
		wait.Wait()
		for i := range results {
			if errors[i] != nil || !bytes.Equal(results[i], expected[i%len(names)]) {
				t.Fatal("cold/warm concurrent publication", i, errors[i])
			}
		}
		if z.sourceOwnership.bytes != int64(len(expected[0])) {
			t.Fatal("concurrent source charged more than once")
		}
		z.Close()
	}
}
