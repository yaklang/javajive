// Regression seed for reachingRefSlotNullReassignMerge (JDEC_REF_SLOT_NULL_REASSIGN_MERGE_OFF).
//
// A typed local is stored (`Character c = Character.valueOf(chars[i])`), conditionally reassigned null
// on ONE branch (`if (c.charValue() == 0) c = null;`), then read after the branch (`sink(c)`). The typed
// def and the `= null` def both reach the post-branch read, so it is ONE variable whose type is the typed
// def's (null is assignable to any reference). Without this merge, AssignVarGuarded — seeing the null
// value carry no concrete type to match the current Character ref — mints a FRESH Object variable for the
// null store; the typed def is then left with only its in-branch `c.charValue()` use, so the single-use
// variable fold splices `Character.valueOf(chars[i]).charValue()` into the condition and DROPS the typed
// store, and the post-branch read binds to the fresh var that is assigned only on the null branch ->
// javac "variable might not have been initialized". Mirrors snakeyaml Resolver.addImplicitResolver and
// commons-lang3 LocaleUtils.countriesByLanguage.
public class NullReassignSeed {
    static void sink(Character c) {
    }

    void run(char[] chars, int i) {
        Character c = Character.valueOf(chars[i]);
        if (c.charValue() == 0) {
            c = null;
        }
        sink(c);
    }
}
