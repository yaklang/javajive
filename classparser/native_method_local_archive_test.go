package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeMethodLocalSourceRefusesUnprovedCapturesAndTypeUses(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			for _, variant := range []string{"original", "class literal", "local array", "producer capture", "phi capture", "explicit constructor effect", "ordinary field initializer", "nested named scope"} {
				t.Run(variant, func(t *testing.T) {
					source := `class LocalGuardOwner{private long bias;static int trace;Object make(final long seed,final Object token,Object other,boolean flag){class Entry{long get(){return bias+seed;}Object token(){return token;}}return new Entry();}}`
					switch variant {
					case "class literal":
						source = strings.Replace(source, "return new Entry();", "Object value=new Entry();if(flag)return Entry.class;return value;", 1)
					case "local array":
						source = strings.Replace(source, "return new Entry();", "Object value=new Entry();if(flag)return new Entry[1];return value;", 1)
					case "producer capture":
						source = strings.Replace(source, "class Entry", "final Object made=new Object();class Entry", 1)
						source = strings.Replace(source, "return token;", "return made;", 1)
					case "phi capture":
						source = strings.Replace(source, "class Entry", "final Object made=flag?token:other;class Entry", 1)
						source = strings.Replace(source, "return token;", "return made;", 1)
					case "explicit constructor effect":
						source = strings.Replace(source, "class Entry{", "class Entry{Entry(){trace++;}", 1)
					case "ordinary field initializer":
						source = strings.Replace(source, "class Entry{", "class Entry{final long actual=seed;", 1)
					case "nested named scope":
						source = strings.Replace(source, "class Entry{", "class Entry{class Helper{}Object nested(){return new Helper();}", 1)
					}
					files := nativeCompileDebugClasses(t, source, debug)
					z := nativeArchive(t, files)
					defer z.Close()
					root, e := Parse(files["LocalGuardOwner.class"])
					if e != nil {
						t.Fatal(e)
					}
					prepared := z.prepareNativeMemberFamily(root, snapshotJDECEnv())
					if (prepared != nil) != (variant == "original") {
						t.Fatalf("joint source declaration admitted=%v", prepared != nil)
					}
					if variant == "original" {
						entry := z.finishNativeMemberFamily(prepared, z.nativeMemberLookup, false)
						if entry == nil || !nativeMethodLocalSourceComplete(entry.family) || !strings.Contains(entry.source, "class Entry") || !strings.Contains(entry.source, "new Entry()") || strings.Contains(entry.source, "final long val$seed;") {
							t.Fatalf("complete actual source missing %+v", entry)
						}
					}
				})
			}
		})
	}
}
