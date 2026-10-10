import java.lang.reflect.RecordComponent;
import java.lang.reflect.Type;

public record GenericBox<T>(@Ann("head") T item, String label) {
	public static void main(String[] a) {
		RecordComponent[] cs = GenericBox.class.getRecordComponents();
		System.out.println(cs.length);
		System.out.println(cs[0].getName());
		System.out.println(cs[0].getGenericType().getTypeName());
		System.out.println(cs[1].getName());
		System.out.println(cs[1].getType().getName());
		Ann ann = cs[0].getAnnotation(Ann.class);
		System.out.println(ann != null ? ann.value() : "missing");
		GenericBox<Integer> b = new GenericBox<>(7, "ok");
		System.out.println(b.item() + ":" + b.label());
		System.out.println(GenericBox.class.isRecord());
	}
}
