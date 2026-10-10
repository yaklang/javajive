record Pair(int a, int b) {}

public class RecDecon {
	static int f(Object o) {
		return switch (o) {
			case Pair(int a, int b) -> a + b;
			case String s -> s.length();
			default -> -1;
		};
	}

	public static void main(String[] args) {
		System.out.println(f(new Pair(2, 3)));
		System.out.println(f("ab"));
		System.out.println(f(7));
	}
}
