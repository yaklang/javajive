package javaclassparser

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Metadata/source inspection only; historical JAR code is never host executed.
func TestReviewedLogbackPrivateNestBridges(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	jar, err := zip.OpenReader(filepath.Join(home, ".m2/repository/ch/qos/logback/logback-core/1.4.14/logback-core-1.4.14.jar"))
	if err != nil {
		t.Skipf("optional original jar: %v", err)
	}
	defer jar.Close()
	files := map[string]*zip.File{}
	for _, f := range jar.File {
		files[f.Name] = f
	}
	cache := map[string][]byte{}
	resolve := func(n string) ([]byte, bool) {
		if b, ok := cache[n]; ok {
			return b, true
		}
		f := files[n+".class"]
		if f == nil {
			return nil, false
		}
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		cache[n] = b
		return b, true
	}
	for _, test := range []struct{ owner, caller, name, desc string }{
		{"ch/qos/logback/core/net/AbstractSocketAppender", "ch/qos/logback/core/net/AbstractSocketAppender$1", "connectSocketAndDispatchEvents", "()V"},
		{"ch/qos/logback/core/net/server/ConcurrentServerRunner", "ch/qos/logback/core/net/server/ConcurrentServerRunner$ClientWrapper", "addClient", "(Lch/qos/logback/core/net/server/Client;)V"},
		{"ch/qos/logback/core/net/server/ConcurrentServerRunner", "ch/qos/logback/core/net/server/ConcurrentServerRunner$ClientWrapper", "removeClient", "(Lch/qos/logback/core/net/server/Client;)V"},
		{"ch/qos/logback/core/spi/AbstractComponentTracker", "ch/qos/logback/core/spi/AbstractComponentTracker$2", "isEntryStale", "(Lch/qos/logback/core/spi/AbstractComponentTracker$Entry;J)Z"},
		{"ch/qos/logback/core/spi/AbstractComponentTracker", "ch/qos/logback/core/spi/AbstractComponentTracker$3", "isEntryDoneLingering", "(Lch/qos/logback/core/spi/AbstractComponentTracker$Entry;J)Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, ok := resolve(test.owner)
			if !ok {
				t.Fatal("missing original owner")
			}
			owner, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, m := range owner.Methods {
				n, _ := owner.getUtf8(m.NameIndex)
				d, _ := owner.getUtf8(m.DescriptorIndex)
				if n == test.name && d == test.desc {
					if m.AccessFlags&2 == 0 || m.AccessFlags&8 != 0 {
						t.Fatal("original target is not private instance")
					}
					found = true
				}
			}
			if !found {
				t.Fatal("original exact descriptor missing")
			}
			for _, setting := range []string{"", "1"} {
				dumper := NewClassObjectDumper(owner)
				dumper.foldSiblingResolver = resolve
				dumper.FuncCtx = nil
				objects := map[string]*ClassObject{test.owner: owner}
				load := func(n string) (*ClassObject, bool) {
					if o, ok := objects[n]; ok {
						return o, true
					}
					b, ok := resolve(n)
					if !ok {
						return nil, false
					}
					o, e := Parse(b)
					if e != nil {
						t.Fatal(e)
					}
					objects[n] = o
					return o, true
				}
				plan, e := dumper.buildPrivateNestPlan(owner, load)
				if e != nil {
					t.Fatal(e)
				}
				key := privateNestTarget{test.name, test.desc}
				if plan == nil || plan.bridges[key] == "" {
					t.Fatal("original reciprocal nest invocation did not prove bridge")
				}
				bridge := plan.bridges[key]
				sites := 0
				for s := range plan.sites {
					if s.caller == test.caller && s.target == key {
						sites++
					}
				}
				if sites == 0 {
					t.Fatal("missing exact caller owner/name/desc/kind/PC")
				}
				env := map[string]string{"JDEC_NEST_PRIVATE_PACKAGE_OFF": setting}
				for _, n := range []string{test.owner, test.caller} {
					bytes, _ := resolve(n)
					result, e := DecompileWithOptions(bytes, DecompileOptions{Mode: Precision, Resolve: resolve, EnvSnapshot: env})
					if e != nil {
						t.Fatal(e)
					}
					if len(result.StubMethods) > 0 {
						t.Fatalf("source stub: %v", result.StubMethods)
					}
					if !strings.Contains(result.Source, bridge+"(") {
						t.Fatalf("%s=%q: proven bridge missing from %s\n%s", "JDEC_NEST_PRIVATE_PACKAGE_OFF", setting, n, result.Source)
					}
					if n == test.owner {
						if !strings.Contains(result.Source, "private ") || !strings.Contains(result.Source, ")."+test.name+"(") {
							t.Fatalf("original private invocation not retained by bridge: %s", result.Source)
						}
					}
				}
			}
		})
	}
}
