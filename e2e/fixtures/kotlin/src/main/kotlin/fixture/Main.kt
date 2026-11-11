package fixture

fun add(a: Int, b: Int): Int = a + b

fun unused(value: Boolean): Int = if (value) 1 else 0

fun main() {
    println("sum=${add(2, 3)}")
}
