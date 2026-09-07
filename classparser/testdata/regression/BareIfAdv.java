public class BareIfAdv {
	void startArray(int n) {}
	void writeComma() {}
	void sibling(int n) {
		int i = 0;
		while (i < n) {
			i++;
		}
	}
	void write(boolean jsonb, java.util.List<?> list) {
		if (jsonb) {
			int size = list.size();
			startArray(size);
			int j = 0;
			while (j < size) {
				list.get(j);
				j++;
			}
		} else {
			int i = 0;
			while (i < list.size()) {
				if (i != 0) {
					writeComma();
				}
				list.get(i);
				i++;
			}
		}
	}
}
