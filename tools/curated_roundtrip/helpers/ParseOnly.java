import java.nio.file.*;
import java.util.*;
import javax.tools.*;
import com.sun.source.util.JavacTask;
/** Parse only: no annotation processing, no attribution, no target execution. */
public final class ParseOnly {
 public static void main(String[] args) throws Exception {
  if (args.length == 0) { System.err.println("java ParseOnly Source.java ..."); System.exit(2); }
  JavaCompiler compiler=ToolProvider.getSystemJavaCompiler();
  if(compiler==null){System.err.println("UNAVAILABLE: full JDK required");System.exit(2);}
  DiagnosticCollector<JavaFileObject> diagnostics=new DiagnosticCollector<>();
  try(StandardJavaFileManager fm=compiler.getStandardFileManager(diagnostics,Locale.ROOT,java.nio.charset.StandardCharsets.UTF_8)) {
   Iterable<? extends JavaFileObject> files=fm.getJavaFileObjectsFromStrings(Arrays.asList(args));
   JavacTask task=(JavacTask)compiler.getTask(null,fm,diagnostics,List.of("-proc:none","-encoding","UTF-8"),null,files);
   task.parse();boolean bad=false;
   for(Diagnostic<?> d:diagnostics.getDiagnostics()) if(d.getKind()==Diagnostic.Kind.ERROR){bad=true;System.err.println(d.getCode()+"@"+d.getLineNumber()+":"+d.getMessage(Locale.ROOT));}
   System.out.println(bad?"INVALID":"PARSED_NOT_TYPECHECKED");if(bad)System.exit(1);
  }
 }
}
