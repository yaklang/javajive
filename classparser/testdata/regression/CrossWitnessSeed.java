public class CrossWitnessSeed<N> {
    Object captured;

    @SuppressWarnings("unchecked")
    N run(N node) {
        return CrossWitnessPair.pick(node, (N) this.captured);
    }
}
