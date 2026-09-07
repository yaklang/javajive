public class ThisFirstAdv {
	final java.util.function.Consumer<String> sink;
	ThisFirstAdv(java.util.function.Consumer<String> sink) { this.sink = sink; }
	ThisFirstAdv(StringBuilder sb) { this(sb::append); }
}
