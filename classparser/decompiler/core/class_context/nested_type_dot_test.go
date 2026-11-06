package class_context

import "testing"

// An archive-only sibling lookup is authoritative for the flat spelling. A
// Kotlin stdlib class in kotlin.collections referring to kotlin.RequiresOptIn's
// nested Level type must use the emitted flat unit, not the stdlib dotted
// shortcut. External JDK nested types keep the dotted spelling.
func TestNestedTypeShouldDotPrefersArchiveSiblingLookup(t *testing.T) {
	f := &ClassContext{PackageName: "kotlin.collections"}
	f.SiblingSuperTypes = func(name string) ([]string, bool) {
		return nil, name == "kotlin/RequiresOptIn"
	}
	f.SiblingClassAccessible = func(name string) (bool, bool) {
		return true, name == "kotlin/RequiresOptIn"
	}
	if f.nestedTypeShouldDot("kotlin", "RequiresOptIn$Level") {
		t.Fatal("archive sibling was dotted instead of using its flat source unit")
	}
	if !f.nestedTypeShouldDot("java.util", "Map$Entry") {
		t.Fatal("external stdlib nested type was flattened")
	}

	// Without an archive callback a stdlib outer may be an ambient JDK class
	// rather than an emitted sibling; stdlib names must stay dotted.
	f.SiblingClassAccessible = nil
	if !f.nestedTypeShouldDot("kotlin", "RequiresOptIn$Level") {
		t.Fatal("stdlib nested type was flattened without an archive callback")
	}
	if !f.nestedTypeShouldDot("java.util", "Map$Entry") {
		t.Fatal("external stdlib nested type was flattened without an archive callback")
	}

	// A non-stdlib external type may still use the legacy resolver fallback.
	f.SiblingSuperTypes = func(name string) ([]string, bool) {
		return nil, name == "example/Outer"
	}
	if f.nestedTypeShouldDot("example", "Outer$Inner") {
		t.Fatal("resolved third-party sibling was dotted without an archive callback")
	}
}
