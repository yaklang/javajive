import java.util.Map;
import java.util.function.Predicate;

public class WildcardConsumerSeed<K, V> {
    Predicate<? super Map.Entry<K, V>> predicate;

    boolean check(Map.Entry<K, V> e) {
        return this.predicate.test(e);
    }
}
