import React from 'react';
import { afterEach, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render } from '@testing-library/react';
import { MemoryCopilotMessage } from './MemoryCopilotMessage';

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

test('busy wall clock respects live cap near thinking', () => {
  vi.useFakeTimers();
  const t0 = 1_716_000_000_000;
  vi.setSystemTime(t0);

  render(
    <MemoryCopilotMessage
      recallStatus="in_progress"
      persistStatus="idle"
      recallText=""
      persistText=""
      memoryWallStartedAtMs={t0 - 100}
      memoryWallLiveCapMs={100}
    />,
  );

  const dur = () => document.querySelector('.thinking-dur')?.textContent;
  expect(dur()).toBe('100ms');

  vi.advanceTimersByTime(10_000);
  expect(dur()).toBe('100ms');
});

test('memory details stay closed by default like thinking', () => {
  const { container } = render(
    <MemoryCopilotMessage
      memoryStatus="completed"
      recallStatus="completed"
      persistStatus="idle"
      recallText=""
      persistText=""
      memoryText="Already on disk"
    />,
  );
  expect(container.querySelector('details')?.getAttribute('open')).toBeNull();
});

test('new React key remounts row and resets details open (reload-from-API used random ids before)', () => {
  const shared = {
    recallStatus: 'completed' as const,
    persistStatus: 'idle' as const,
    recallText: '',
    persistText: '',
    memoryStatus: 'completed' as const,
    memoryText: 'ctx',
  };
  const { container, rerender } = render(
    <MemoryCopilotMessage {...shared} key="live-stream-id" />,
  );
  fireEvent.click(container.querySelector('summary')!);
  expect(container.querySelector('details')?.open).toBe(true);

  rerender(<MemoryCopilotMessage {...shared} key="reload-from-api-id" />);
  expect(container.querySelector('details')?.open).toBe(false);
});

test('memory details stay open across prop updates after user opens them', () => {
  const completedProps = {
    memoryStatus: 'completed' as const,
    recallStatus: 'completed' as const,
    persistStatus: 'idle' as const,
    recallText: '',
    persistText: '',
    memoryText: 'ctx',
  };
  const { container, rerender } = render(
    <MemoryCopilotMessage
      memoryStatus="in_progress"
      recallStatus="in_progress"
      persistStatus="idle"
      recallText=""
      persistText=""
      memoryText="streaming"
    />,
  );
  const summary = container.querySelector('summary');
  expect(summary).toBeTruthy();
  fireEvent.click(summary!);
  expect(container.querySelector('details')?.open).toBe(true);

  rerender(
    <MemoryCopilotMessage
      {...completedProps}
      memoryText="Already on disk\nmore text after assistant finished"
    />,
  );
  expect(container.querySelector('details')?.open).toBe(true);
});

test('when main thinking streams, cap label wins over completed recall server duration', () => {
  vi.useFakeTimers();
  const t0 = 1_716_000_000_000;
  vi.setSystemTime(t0);

  render(
    <MemoryCopilotMessage
      mainThinkingInProgress
      recallStatus="completed"
      persistStatus="idle"
      recallText="x"
      persistText=""
      recallDurationMs={5000}
      memoryWallStartedAtMs={t0 - 500}
      memoryWallLiveCapMs={120}
    />,
  );

  const dur = () => document.querySelector('.thinking-dur')?.textContent;
  expect(dur()).toBe('120ms');
});
