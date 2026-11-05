import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;
import javax.tools.Diagnostic;
import javax.tools.DiagnosticCollector;
import javax.tools.JavaCompiler;
import javax.tools.JavaFileObject;
import javax.tools.StandardJavaFileManager;
import javax.tools.ToolProvider;
import com.sun.source.util.JavacTask;

/** Parses source without attribution, annotation processing, or execution. */
public final class ParseOnly {
  public static void main(String[] args) throws Exception {
    if (args.length == 0) {
      System.err.println("java ParseOnly Source.java ...");
      System.exit(2);
    }
    JavaCompiler compiler = ToolProvider.getSystemJavaCompiler();
    if (compiler == null) {
      System.err.println("UNAVAILABLE: full JDK required");
      System.exit(2);
    }
    DiagnosticCollector<JavaFileObject> diagnostics = new DiagnosticCollector<>();
    try (StandardJavaFileManager files = compiler.getStandardFileManager(
        diagnostics, Locale.ROOT, StandardCharsets.UTF_8)) {
      Iterable<? extends JavaFileObject> inputs =
          files.getJavaFileObjectsFromStrings(Arrays.asList(args));
      List<String> options = Arrays.asList("-proc:none", "-encoding", "UTF-8");
      JavacTask task = (JavacTask) compiler.getTask(null, files, diagnostics, options, null, inputs);
      task.parse();
      boolean invalid = false;
      for (Diagnostic<?> diagnostic : diagnostics.getDiagnostics()) {
        if (diagnostic.getKind() == Diagnostic.Kind.ERROR) {
          invalid = true;
          System.err.println(diagnostic.getCode() + "@" + diagnostic.getLineNumber()
              + ":" + diagnostic.getMessage(Locale.ROOT));
        }
      }
      System.out.println(invalid ? "INVALID" : "PARSED_NOT_TYPECHECKED");
      if (invalid) {
        System.exit(1);
      }
    }
  }
}
