// Seed for the generic-declaration detection fix (JDEC_GENERIC_DECL_DETECT_OFF).
//
// addMissingGeneratedLocalDecls uses generatedLocalDeclRe to learn which `varN` slots are already
// declared before it injects `Object varN = null;` for any that look undeclared. That regex's type
// token forbids spaces, so a declaration whose type is a multi-argument / wildcard generic ending in a
// bare wildcard -- `Map<K, Map<V, ?>> var2 = ...` -- is NOT recognized: the run immediately before
// `var2` is `?>>`, which does not start with an identifier char. var2 then looks undeclared and a bogus
// `Object var2 = null;` is injected that duplicates the real declaration (javac "variable var2 is
// already defined"). Mirrors guava MapMakerInternalMap$StrongKeyWeakValueSegment /
// $WeakKeyWeakValueSegment.setWeakValueReferenceForTesting, whose second parameter type is
// `WeakValueReference<K, V, ? extends InternalEntry<K, V, ?>>`.
//
// The fix additionally scans each rendered line with the space-tolerant, line-anchored
// castEscapeDeclLineRe (skipping keyword-led pseudo-types) so such generic declarations are recognized
// and the phantom is not injected.
//
// The intervening `read()` call plus the `consume(var2)` use keep the `var2 = var1` copy from being
// propagated away, so the generic-typed declaration survives to the dumper -- the exact residual shape.
import java.util.Map;

public class GenericDeclSeed<K, V> {
    void set(Map<K, Map<V, ?>> src) {
        Map<K, Map<V, ?>> dst = src;
        Map<K, Map<V, ?>> prev = read();
        consume(dst);
        prev.clear();
    }

    Map<K, Map<V, ?>> read() {
        return null;
    }

    static void consume(Object o) {
    }
}
