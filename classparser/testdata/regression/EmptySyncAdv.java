public class EmptySyncAdv {
	static final boolean $assertionsDisabled = !EmptySyncAdv.class.desiredAssertionStatus();
	private boolean flag;
	boolean closeInternal(boolean a, boolean b) {
		if (!$assertionsDisabled && Thread.holdsLock(this)) {
			throw new AssertionError();
		} else {
			synchronized (this) {
				if (a) {
					return false;
				}
				if (b) {
					return true;
				}
				this.flag = true;
				return false;
			}
		}
	}
}
