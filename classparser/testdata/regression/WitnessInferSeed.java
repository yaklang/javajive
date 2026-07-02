import java.util.List;

public class WitnessInferSeed {
    static <N> void sink(List<N> list, N node) {
    }

    @SuppressWarnings("unchecked")
    static <N> void run(List<N> list, Object o) {
        sink(list, (N) o);
    }
}
