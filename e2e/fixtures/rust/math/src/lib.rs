/// Adds two numbers.
///
/// ```
/// assert_eq!(flarebyte_rust_math::add(2, 3), 5);
/// ```
pub fn add(left: i32, right: i32) -> i32 {
    left + right
}

pub fn untested() -> i32 {
    42
}

#[cfg(test)]
mod tests {
    #[test]
    fn adds_numbers() {
        assert_eq!(super::add(2, 3), 5);
    }

    #[test]
    #[ignore = "fixture exercises ignored tests"]
    fn ignored_test() {}
}
