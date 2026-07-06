// Regression seed for jdkCalleeParamIsErasedTypeVar's EnumSet.of branch: `EnumSet.of(E first, E... rest)`
// erases its method-scope `E extends Enum<E>` formals to `java.lang.Enum` in the descriptor, so the naive
// arg-cast logic upcasts the concrete enum argument to raw `(Enum)` -- collapsing javac's inference of E and
// breaking overload resolution ("no suitable method found for of(Enum,Color[])"). Dropping the cast lets
// javac infer E = Color. Mirrors spring ConcurrentReferenceHashMap$Task's constructor.
//
// Recompile: javac -d . EnumSetOfSeed.java
import java.util.EnumSet;

public class EnumSetOfSeed {
    enum Color { RED, GREEN }

    final EnumSet<Color> set;

    public EnumSetOfSeed(Color... options) {
        this.set = options.length == 0 ? EnumSet.noneOf(Color.class) : EnumSet.of(options[0], options);
    }
}
