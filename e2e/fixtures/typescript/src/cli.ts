import { add } from './math';

export function answer(): number {
  return add(2, 3);
}

if (import.meta.main) {
  console.log(answer());
}
