package fixtures;

public enum ConstantBody {
	LE {
		public boolean test(int x, int y) {
			return x <= y;
		}
	},
	LT {
		public boolean test(int x, int y) {
			return x < y;
		}
	},
	PLAIN;

	public boolean test(int x, int y) {
		return false;
	}
}
