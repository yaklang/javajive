import java.math.BigDecimal;
public class ClassCmpSeed<T extends Number> {
    Class<T> handledType() { return null; }
    boolean isBD() {
        return ((Class) this.handledType()) == BigDecimal.class;
    }
}
