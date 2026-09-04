// Regression seed for enclosingTypeVarArgCast on newEntry (JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF).
// `Helper.newEntry(seg, key, hash, E next)` is fed a raw InternalEntry (erased chain
// head). The source's `(E)` cast drops; javac then rejects "InternalEntry cannot be
// converted to E". Real hit: guava MapMakerInternalMap$Segment.put.
// Recompile: javac --release 8 -d . NewEntryTypeVarCastSeed.java
public class NewEntryTypeVarCastSeed<K, V, E extends NewEntryTypeVarCastSeed.InternalEntry<K, V, E>> {
    interface InternalEntry<K, V, E> {
        E getNext();
    }

    interface Helper<K, V, E> {
        E newEntry(Object seg, K key, int hash, E next);
    }

    // RAW helper: matches MapMakerInternalMap.entryHelper as seen from Segment (descriptor
    // erases E to InternalEntry; recovering the Signature would drop a synthesized (E) via
    // calleeParamIsErasedTypeVar). enclosingTypeVarArgCast re-emits it because E is in scope.
    Helper entryHelper;

    E put(K key, InternalEntry rawNext) {
        return (E) this.entryHelper.newEntry(this, key, 0, (E) rawNext);
    }
}
