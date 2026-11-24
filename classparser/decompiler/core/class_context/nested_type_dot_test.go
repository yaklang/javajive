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

func TestNestedTypeShouldDotArchiveFlatSiblingWithoutOuter(t *testing.T) {
	// R8 can shrink or inline the outer class while leaving the flat $ sibling
	// in the archive. Lookup of the full binary name must win over the missing outer.
	f := &ClassContext{PackageName: "com.example.caller"}
	f.SiblingClassAccessible = func(name string) (bool, bool) {
		switch name {
		case "androidx/emoji2/text/EmojiProcessor$Callback":
			return true, true
		case "androidx/emoji2/text/EmojiProcessor":
			return false, true
		default:
			return false, false
		}
	}
	if f.nestedTypeShouldDot("androidx.emoji2.text", "EmojiProcessor$Callback") {
		t.Fatal("archive flat sibling with shrunk outer was dotted into nonexistent Outer.Inner")
	}
}

func TestR8HorizontalMergedDollarOneImportedWhenAccessible(t *testing.T) {
	f := &ClassContext{PackageName: "com.example.caller"}
	f.SiblingClassAccessible = func(name string) (bool, bool) {
		return name == "androidx/cardview/widget/CardView$1", true
	}

	got := f.ShortTypeName("androidx.cardview.widget.CardView$1")
	if got != "CardView$1" {
		t.Fatalf("ShortTypeName(accessible CardView$1) = %q, want CardView$1", got)
	}
	_ = f.ShortTypeName("com.other.Anon$1")

	imports := f.GetAllImported()
	if len(imports) != 1 || imports[0] != "androidx.cardview.widget.CardView$1" {
		t.Fatalf("GetAllImported() = %#v, want [androidx.cardview.widget.CardView$1]", imports)
	}
}
