// Seed for externalNestedEnumSourceName (guava AbstractFuture/AggregateFutureState/InterruptibleTask
// `@ReflectionSupport(value=ReflectionSupport$Level.FULL)`): a class annotated with an annotation whose
// value is a NESTED enum constant. The bytecode stores the enum type as the binary descriptor
// `LAnnoEnumAnno$Kind;`; rendering it with the flat `$` name (`AnnoEnumAnno$Kind.FULL`) is unresolvable
// in Java source -- javac rejects it ("an enum annotation value must be an enum constant"). When the
// enum owner is NOT a decompiled sibling (foldSiblingResolver miss) the fix rewrites it to the dotted
// source name `AnnoEnumAnno.Kind.FULL`.
@AnnoEnumAnno(value = AnnoEnumAnno.Kind.FULL)
public class AnnoEnumSeed {
    private AnnoEnumSeed() {
    }
}
