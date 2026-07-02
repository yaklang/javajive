// Regression seed for the same-package simple-name shadowing fix (JDEC_SAMEPKG_FQ_OFF). Compiled from
// THREE sources across two packages; only SamePkgFQSeed.class is kept as the seed (the decompiler
// recovers the clash from SamePkgFQSeed's OWN constant pool, so it reproduces standalone):
//
//   // fqseed/Widget.java
//   package fqseed;
//   public class Widget<T> { public T value; }
//
//   // fqseed/other/Widget.java
//   package fqseed.other;
//   public class Widget { public int id; }
//
//   // fqseed/SamePkgFQSeed.java  (this unit)
//
// SamePkgFQSeed references BOTH fqseed.Widget<T> (its OWN package, generic) and fqseed.other.Widget
// (imported, non-generic). The single-type-import of the latter shadows the same-package simple name,
// so the return type MUST be written fully-qualified `fqseed.Widget<String>` to compile -- a bare
// `Widget<String>` binds to the non-generic fqseed.other.Widget ("type Widget does not take
// parameters"). Real hit: fastjson2 ObjectWriterCreatorASM (com.alibaba.fastjson2.writer.FieldWriter<T>
// vs com.alibaba.fastjson2.internal.asm.FieldWriter).
package fqseed;

import fqseed.other.Widget;

public class SamePkgFQSeed {
    public fqseed.Widget<String> make() {
        return new fqseed.Widget<String>();
    }

    public void use(Widget w) {
        w.id = 1;
    }
}
