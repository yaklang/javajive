package javaclassparser

import (
	"strconv"
	"strings"
	"testing"
)

func TestNativeSerializableReferencePublicationCannotBorrowDifferentDeclarations(t *testing.T) {
	files := nativeCompileDebugClasses(t, serializableStaticReferenceFixture, "none")
	_, java := t04Tools(t)
	if got := t04RunJava(t, java, adversarialAssertionClasses(t, files), "SerializationDriver"); !strings.HasSuffix(got, "44:serializable:static:identity:protocol\n") {
		t.Fatal("valid unchanged original oracle", got)
	}
	for _, change := range []string{"original", "foreign class", "foreign graph", "removed cache", "missing rendered source", "different rendered source", "dropped graph", "changed original helper", "factory removed from source", "quoted factory", "duplicated factory", "nested member borrowing", "renamed declaration", "changed declaration descriptor", "changed capturing class", "keyword capturing class", "keyword capturing package"} {
		t.Run(change, func(t *testing.T) {
			old := AfterMembersDumped
			defer func() { AfterMembersDumped = old }()
			observed := false
			AfterMembersDumped = func(c *ClassObjectDumper) {
				if c.obj.GetClassName() != "SerializableReferences" {
					return
				}
				observed = true
				plan := c.serializableReferencePlan
				if plan == nil || len(plan.sites) != 2 {
					t.Fatal("original protocol source plan missing")
				}
				var parent *dumpedMethods
				for _, body := range c.dumpedMethodsSet {
					if body != nil && body.member == plan.sites[0].method && body.serializableReferences != nil {
						if parent != nil && parent != body {
							t.Fatal("duplicate original source declaration")
						}
						parent = body
					}
				}
				packet := c.serializableReferencePackets[0]
				emission := c.serializableReferenceEmissions[packet]
				if parent == nil || packet == nil || emission == nil || len(emission.fingerprints) == 0 {
					t.Fatal("original completed source receipt missing")
				}
				text := ""
				for _, fingerprint := range emission.fingerprints {
					for pos := 0; pos+fingerprint.size <= len(parent.code); pos++ {
						window := parent.code[pos : pos+fingerprint.size]
						if actual, known := serializableReferenceFingerprintOf(window); known && actual == fingerprint {
							text = window
							break
						}
					}
				}
				if text == "" {
					t.Fatal("observed direct-reference source window missing")
				}
				switch change {
				case "foreign class":
					c.obj = &ClassObject{}
				case "changed capturing class", "keyword capturing class", "keyword capturing package":
					owner := "OtherCapturingClass"
					if change == "keyword capturing class" {
						owner = "switch"
					} else if change == "keyword capturing package" {
						owner = "switch/SerializableReferences"
					}
					pool := NewConstantPoolWithConstant(&c.obj.ConstantPool)
					self := serializableConstant(c.obj, c.obj.ThisClass).(*ConstantClassInfo)
					self.NameIndex = uint16(pool.AddUtf8Info(owner))
				case "foreign graph":
					copy := *plan
					parent.serializableReferences.plan = &copy
				case "removed cache":
					delete(c.dumpedMethodsSet, parent.sourceCacheKey)
				case "missing rendered source":
					emission.fingerprints = nil
				case "different rendered source":
					changed, known := serializableReferenceFingerprintOf(strings.ReplaceAll(text, "::Aa", "::BB"))
					if !known {
						t.Fatal("changed reference has no fingerprint")
					}
					emission.fingerprints = []serializableReferenceFingerprint{changed}
				case "dropped graph":
					parent.serializableReferences.counts = nil
				case "changed original helper":
					pool := NewConstantPoolWithConstant(&c.obj.ConstantPool)
					found := false
					for _, constant := range c.obj.ConstantPool {
						if s, ok := constant.(*ConstantStringInfo); ok && s != nil {
							value, _ := sourceBridgeUTF8(c.obj, s.StringIndex)
							if value == "Invalid lambda deserialization" {
								s.StringIndex = uint16(pool.AddUtf8Info("different original failure"))
								found = true
								break
							}
						}
					}
					if !found {
						t.Fatal("original failure literal missing")
					}
				case "factory removed from source":
					parent.code = strings.Replace(parent.code, text, "null", 1)
				case "quoted factory":
					parent.code = strings.Replace(parent.code, text, strconv.Quote(text), 1)
				case "duplicated factory":
					parent.code = strings.Replace(parent.code, text, text+";"+text, 1)
				case "nested member borrowing":
					parent.code = "static class SeparateScope {" + parent.code + "}"
				case "renamed declaration":
					parent.code = strings.Replace(parent.code, "first()", "renamed()", 1)
				case "changed declaration descriptor":
					parent.code = strings.Replace(parent.code, "first()", "first(int ignored)", 1)
				}
			}
			z := nativeArchive(t, files)
			defer z.Close()
			source, err := z.ReadFile("SerializableReferences.class")
			if !observed {
				t.Fatal("production archive publication hook not reached", err)
			}
			published := err == nil && len(source) != 0 && !strings.Contains(string(source), DecompileStubMarker) && !strings.Contains(string(source), "decompile dump failed")
			if published != (change == "original") {
				t.Fatalf("source publication authority=%t change=%s err=%v\n%s", published, change, err, source)
			}

		})
	}
}
