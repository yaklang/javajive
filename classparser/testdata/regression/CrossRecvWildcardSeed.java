import java.lang.annotation.Annotation;

// Regression seed for crossRecvWildcardReturnCast: a method whose declared return type mentions a type
// variable (`Class<A>`) returns an instance call on a NON-`this` receiver (the field `this.holder`, a
// jar-internal class) whose recovered generic return is a WILDCARD parameterization of the SAME erasure
// (`Class<? extends Annotation>`). javac captures the wildcard to CAP#1 and rejects the bare return; the
// source carried an unchecked `(Class<A>)` cast the bytecode dropped. Mirrors spring
// TypeMappedAnnotation.getType() -> this.mapping.getAnnotationType().
//
// Recompile: javac -d . CrossRecvWildcardSeed.java
public class CrossRecvWildcardSeed<A extends Annotation> {
    static class Holder {
        Class<? extends Annotation> getKind() {
            return Annotation.class;
        }
    }

    private final Holder holder = new Holder();

    @SuppressWarnings("unchecked")
    public Class<A> getType() {
        return (Class<A>) this.holder.getKind();
    }
}
