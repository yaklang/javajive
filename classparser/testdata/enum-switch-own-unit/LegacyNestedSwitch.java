public class LegacyNestedSwitch{public enum Mode{A,B,C}public static int run(Mode first,Mode other,int x){switch(first){case A:return x+1;case B:return x-1;default:return x;}}}
