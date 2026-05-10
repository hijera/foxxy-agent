import { expect, test } from 'vitest';
import { SHELL_STACK_MAX_WIDTH_PX, shellStackMaxWidthMediaQuery } from './shellBreakpoint';

test('shell stack breakpoint matches CSS tier (1199 / 1200)', () => {
  expect(SHELL_STACK_MAX_WIDTH_PX).toBe(1199);
  expect(shellStackMaxWidthMediaQuery).toBe('(max-width: 1199px)');
});
