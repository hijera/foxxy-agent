import { expect, test } from 'vitest';
import { stableMemoryCopilotItemId } from './memoryStableId';

test('same memoryRowId yields same id for live row and API reload', () => {
  expect(stableMemoryCopilotItemId('row-abc', 3)).toBe(stableMemoryCopilotItemId('row-abc', 99));
});

test('sanitizes odd characters but stays deterministic', () => {
  expect(stableMemoryCopilotItemId('g:uuid/x', 1)).toBe('mc_g_uuid_x');
});

test('fallback uses turn index when row id empty', () => {
  expect(stableMemoryCopilotItemId('', 2)).toBe('mc_turn_2');
});
