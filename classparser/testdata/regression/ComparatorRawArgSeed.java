// Seed for the raw `(Comparator)` cast on a JDK sort/search static (JDEC_COMPARATOR_RAW_ARG_OFF).
//
// Source `Arrays.sort((K[]) arr, comparator)` compiles (T=K, comparator is Comparator<? super K>), but
// the `(K[])` array cast erases to `(Object[])` in bytecode; the decompiler renders `Arrays.sort(
// (Object[]) arr, this.comparator)`, and javac -- re-resolving against sort(T[], Comparator<? super T>)
// with T inferred as Object -- rejects the `Comparator<? super K>` capture ("no suitable method for
// sort(Object[], Comparator<CAP#1>)"). Mirrors guava ImmutableSortedMap$Builder.build /
// ImmutableSortedMultiset$Builder / ImmutableList. A raw `(Comparator)` cast makes it an unchecked,
// behaviour-preserving call. The Comparator position is identified by the stable descriptor parameter,
// and lambda/method-ref comparator args are excluded (raw would erase their inferred param types).
import java.util.Arrays;
import java.util.Comparator;

public class ComparatorRawArgSeed<K> {
    private final Comparator<? super K> comparator;

    public ComparatorRawArgSeed(Comparator<? super K> c) {
        this.comparator = c;
    }

    void order(Object[] arr) {
        Arrays.sort((K[]) arr, this.comparator);
    }
}
