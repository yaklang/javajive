package javaclassparser

import "testing"

func TestAdversarialJarReleaseSelectionExactOriginalNames(t *testing.T) {
	worlds := []jarReleaseOracleWorld{
		{name: "manifest-alias-last-canonical-false.jar", multi: false, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "META-INF//MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "META-INF/versions/9/probe.txt", hex: "5639"}}},
		{name: "manifest-alias-last-canonical-true.jar", multi: true, payload: "V9", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "META-INF//MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "META-INF/versions/9/probe.txt", hex: "5639"}}},
		{name: "manifest-alias-first-canonical-false.jar", multi: false, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF//MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "META-INF/versions/9/probe.txt", hex: "5639"}}},
		{name: "manifest-alias-first-canonical-true.jar", multi: true, payload: "V9", entries: []jarReleaseOracleEntry{{name: "META-INF//MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "META-INF/versions/9/probe.txt", hex: "5639"}}},
		{name: "noncanonical-version-0.jar", multi: true, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "./META-INF/versions/09/probe.txt", hex: "5639"}}},
		{name: "noncanonical-version-1.jar", multi: true, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "META-INF\\versions\\9\\probe.txt", hex: "5639"}}},
		{name: "noncanonical-version-2.jar", multi: true, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "META-INF/versions/09/probe.txt", hex: "5639"}}},
		{name: "noncanonical-version-3.jar", multi: true, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "META-INF/versions/9//probe.txt", hex: "5639"}}},
		{name: "noncanonical-version-4.jar", multi: true, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "./META-INF/versions/9/probe.txt", hex: "5639"}}},
		{name: "base-alias-overwrites-forest.jar", multi: false, payload: "BASE", entries: []jarReleaseOracleEntry{{name: "META-INF/MANIFEST.MF", hex: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a"}, {name: "probe.txt", hex: "42415345"}, {name: "./probe.txt", hex: "414c494153"}, {name: "META-INF/versions/9/probe.txt", hex: "5639"}}},
	}
	for _, world := range worlds {
		t.Run(world.name, func(t *testing.T) {
			entries := []jarReleaseTestEntry{}
			for _, e := range world.entries {
				entries = append(entries, jarReleaseTestEntry{e.name, jarReleaseTestHex(t, e.hex)})
			}
			z := jarReleaseTestLegacy(t, jarReleaseTestZIP(t, entries), 21)
			flag, err := z.RuntimeMultiRelease()
			if err != nil || flag != world.multi {
				t.Fatalf("original physical manifest=%v %v want %v", flag, err, world.multi)
			}
			raw, err := z.readJarEntry("probe.txt")
			if err != nil || string(raw) != world.payload {
				t.Fatalf("selected raw bytes=%q %v want JDK URL %q", raw, err, world.payload)
			}
		})
	}
}
