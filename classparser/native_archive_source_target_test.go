package javaclassparser

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNativeArchiveSourceTargetIsImmutableAndSeparateFromReleaseLookup(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class SourceTargetOwner {static int value(){return 7;}}`, "none")
	path := filepath.Join(t.TempDir(), "original.jar")
	if e := os.WriteFile(path, t23Zip(t, files), 0600); e != nil {
		t.Fatal(e)
	}
	for _, target := range []int{0, 8, 9, 11, 17, 21} {
		t.Run(strconv.Itoa(target), func(t *testing.T) {
			z, e := NewJarFSFromLocalWithSourceVersion(path, target, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer z.Close()
			obj, e := Parse(files["SourceTargetOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			if z.nativeMemberReader(obj).options.TargetSourceVersion != target {
				t.Fatal("member reader lost compilation profile")
			}
			before := z.ZipFS.TargetRelease()
			for _, release := range []int{9, 11, 17} {
				view := z.sourceReleaseView(release)
				if view == nil || view.targetSourceVersion != target || view.nativeMemberReader(obj).options.TargetSourceVersion != target || view.ZipFS.TargetRelease() != release || z.sourceReleaseView(release) != view {
					t.Fatal("namespace changed source profile or cache identity")
				}
			}
			if z.ZipFS.TargetRelease() != before {
				t.Fatal("source target changed original archive lookup")
			}
		})
	}
	for _, target := range []int{-1, 1, 7, 22} {
		if z, e := NewJarFSFromLocalWithSourceVersion(path, target, nil); e == nil || z != nil {
			t.Fatalf("invalid source target accepted: %d", target)
		}
	}
	legacy, e := NewJarFSFromLocalWithResolver(path, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer legacy.Close()
	if legacy.targetSourceVersion != 0 {
		t.Fatal("legacy constructor no longer infers the class profile")
	}
}
