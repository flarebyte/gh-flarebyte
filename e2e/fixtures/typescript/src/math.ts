export function add(a: number, b: number): number {
  return a + b;
}

export function magnitude(value: number): number {
  if (value < 0) {
    return -value;
  }
  return value;
}
