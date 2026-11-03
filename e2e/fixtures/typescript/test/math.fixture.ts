import assert from 'node:assert/strict';
import { test } from 'node:test';
import { answer } from '../src/cli';
import { add } from '../src/math';

test('adds numbers', () => {
  assert.equal(process.env.TYPESCRIPT_FIXTURE_PRELOAD, 'loaded');
  assert.equal(add(2, 3), 5);
  assert.equal(answer(), 5);
});

test.skip('skipped test', () => {});
