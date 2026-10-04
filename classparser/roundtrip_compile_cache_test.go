package javaclassparser

import "testing"

func TestAdversarialSourceFamilyCompileCacheKeepsChangedHelpers(t *testing.T) {
	_, java := t04Tools(t)
	compile := t04SourceFamilyCompiler(t, "8", t.TempDir())
	sources := map[string]string{"CacheDriver.java": `public class CacheDriver {public static void main(String[]args){System.out.println(CacheHelper.value());}}`, "CacheHelper.java": `class CacheHelper {static int value(){return 17;}}`}
	first := compile(t, sources)
	if got := t04RunJava(t, java, first, "CacheDriver"); got != "17\n" {
		t.Fatal("original family oracle", got)
	}
	// Changing a helper must invalidate the entire compiled family even though
	// the consumer source and map identity stay unchanged.
	sources["CacheHelper.java"] = `class CacheHelper {static int value(){return 29;}}`
	second := compile(t, sources)
	if got := t04RunJava(t, java, second, "CacheDriver"); got != "29\n" {
		t.Fatal("changed helper was masked by a stale consumer", got)
	}
	sources["CacheHelper.java"] = `class CacheHelper {static int value(){return 17;}}`
	again := compile(t, sources)
	if again != first || second == first {
		t.Fatal("exact complete family identities were conflated")
	}
	if got := t04RunJava(t, java, again, "CacheDriver"); got != "17\n" {
		t.Fatal("original compiled family was overwritten", got)
	}
}
