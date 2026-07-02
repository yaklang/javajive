public class SuperCtorTypeVarSeed<N> {
    N node;

    SuperCtorTypeVarBaseSeed<N> make() {
        return new SuperCtorTypeVarBaseSeed<N>("t", this.node) {
        };
    }
}
