package javaclassparser

import "testing"

func TestCheckCastFinalMetadataUsesExactPlatformDeclarations(t *testing.T) {
	for _, release := range []int{8, 11, 17, 21} {
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
	if _, ok := jdkInvocationMetadata("java/lang/Integer", 9); ok {
		t.Fatal("unknown platform guessed")
	}
}
