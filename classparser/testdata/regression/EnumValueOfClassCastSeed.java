// Regression seed for enumValueOfClassArgCast. Enum.valueOf requires
// Class<T extends Enum<T>>; a Class<?> argument is rejected ("method valueOf
// cannot be applied"). The source's unchecked `(Class)` cast erases to a no-op
// checkcast on Class and is dropped by javac; the decompiler must re-emit it.
// Kill-switch: JDEC_ENUM_VALUEOF_CLASS_CAST_OFF.
// Recompile: javac --release 8 -d . EnumValueOfClassCastSeed.java
public class EnumValueOfClassCastSeed {
    enum Color { RED, BLUE }

    static <E extends Enum<E>> E parse(Class<?> c, String n) {
        return (E) Enum.valueOf((Class) c, n);
    }
}
