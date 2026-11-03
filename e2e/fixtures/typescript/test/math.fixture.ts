import { expect, test } from 'bun:test';
import { answer } from '../src/cli';
import { add } from '../src/math';

test('adds numbers', () => {
  expect(process.env.TYPESCRIPT_FIXTURE_PRELOAD).toBe('loaded');
  expect(add(2, 3)).toBe(5);
  expect(answer()).toBe(5);
});

test.skip('skipped test', () => {});
