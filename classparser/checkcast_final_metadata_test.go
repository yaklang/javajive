package javaclassparser

import "testing"

func TestCheckCastFinalMetadataUsesExactPlatformDeclarations(t *testing.T) {
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		for _, name := range []string{"java/lang/Integer", "java/lang/String"} {
			decl, ok := jdkInvocationMetadata(name, release)
			if !ok || decl.Name != name || !decl.Final || decl.IsInterface || !decl.ParentsComplete {
				t.Fatalf("release%d original final declaration %s missing", release, name)
			}
		}
		runnable, ok := jdkInvocationMetadata("java/lang/Runnable", release)
		if !ok || runnable.Name != "java/lang/Runnable" || !runnable.IsInterface || runnable.Final {
			t.Fatalf("release%d original Runnable declaration missing", release)
		}
		object, ok := jdkInvocationMetadata("java/lang/Object", release)
		if !ok || object.Final {
			t.Fatalf("release%d Object incorrectly marked final", release)
		}
	}
	// Releases9/16 are explicit original catalog profiles. Every gap and
	// future release must still fail closed rather than borrowing a profile.
	for _, release := range []int{0, 7, 10, 12, 13, 14, 15, 18, 19, 20, 22, 99} {
		if _, ok := jdkInvocationMetadata("java/lang/Integer", release); ok {
			t.Fatalf("unknown platform%d guessed", release)
		}
	}
}
