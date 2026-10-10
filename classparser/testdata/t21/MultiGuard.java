public class MultiGuard {
	static String log = "";

	static boolean g1(String s) {
		log += "1";
		return s.startsWith("A");
	}

	static boolean g2(String s) {
		log += "2";
		return s.startsWith("B");
	}

	static String f(Object x) {
		return switch (x) {
			case String s when g1(s) -> "A";
			case String s when g2(s) -> "B";
			case String s -> "C:" + s;
			default -> "D";
		};
	}

	public static void main(String[] a) {
		System.out.println(f("Bxx"));
		System.out.println(f("Axx"));
		System.out.println(f("Z"));
		System.out.println(f(3));
		System.out.println(log);
	}
}
