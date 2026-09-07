public class NsmeCatchAdv {
	public Object make(String name) throws Exception {
		try {
			return this.build(name);
		} catch (ClassNotFoundException e) {
			throw new RuntimeException(e);
		}
	}
	Object build(String name) throws ClassNotFoundException, NoSuchMethodException,
			InstantiationException, IllegalAccessException, java.lang.reflect.InvocationTargetException {
		return Class.forName(name).getConstructor(new Class[0]).newInstance(new Object[0]);
	}
}
