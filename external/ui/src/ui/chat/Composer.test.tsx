import React from 'react';
import { afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { Composer } from './Composer';

afterEach(() => cleanup());

function renderComposer(opts: { isEmpty: boolean }) {
  return render(
    <Composer
      value=""
      isEmpty={opts.isEmpty}
      mode="agent"
      modes={['agent', 'plan']}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );
}

function renderComposerWithLlm(opts: { isEmpty: boolean }) {
  return render(
    <Composer
      value=""
      isEmpty={opts.isEmpty}
      mode="agent"
      modes={['agent', 'plan']}
      llmModels={['openai/gpt-4o-mini', 'openai/gpt-4o']}
      llmModel="openai/gpt-4o-mini"
      onLlmModelChange={() => {}}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );
}

test('mode menu opens down on start screen', () => {
  renderComposer({ isEmpty: true });

  fireEvent.click(screen.getByRole('button', { name: 'Mode' }));

  const menu = screen.getByRole('menu');
  expect(menu).toHaveClass('opens-down');
});

test('mode menu opens up in active chat composer', () => {
  renderComposer({ isEmpty: false });

  fireEvent.click(screen.getByRole('button', { name: 'Mode' }));

  const menu = screen.getByRole('menu');
  expect(menu).toHaveClass('opens-up');
});

test('yaml model menu opens down on start screen when backends exist', () => {
  renderComposerWithLlm({ isEmpty: true });

  fireEvent.click(screen.getByRole('button', { name: 'Model' }));

  const menu = screen.getByRole('menu');
  expect(menu).toHaveClass('opens-down');
});

test('yaml model menu opens up in active chat composer', () => {
  renderComposerWithLlm({ isEmpty: false });

  fireEvent.click(screen.getByRole('button', { name: 'Model' }));

  const menu = screen.getByRole('menu');
  expect(menu).toHaveClass('opens-up');
});

test('send play disabled when input empty', () => {
  renderComposer({ isEmpty: true });
  expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled();
});

test('send play enabled when draft has text', () => {
  render(
    <Composer
      value="hi"
      isEmpty={true}
      mode="agent"
      modes={['agent', 'plan']}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );
  expect(screen.getByRole('button', { name: 'Send' })).not.toBeDisabled();
});

test('composer highlights plain slash token as chip while editing', () => {
  render(
    <Composer
      value="asdfasf /find-skills asdfasdf"
      isEmpty={false}
      mode="agent"
      modes={['agent', 'plan']}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );
  const chip = screen.getByTestId('composer-skill-chip');
  expect(chip).toHaveTextContent('/find-skills');
  expect(chip).toHaveAttribute('data-skill-name', 'find-skills');
});

test('generating shows stop and calls onStop', () => {
  let stopped = false;
  render(
    <Composer
      value=""
      isEmpty={true}
      generating={true}
      mode="agent"
      modes={['agent', 'plan']}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
      onStop={() => {
        stopped = true;
      }}
    />,
  );
  const b = screen.getByRole('button', { name: 'Stop generation' });
  expect(b).not.toBeDisabled();
  fireEvent.click(b);
  expect(stopped).toBe(true);
});

test('context tooltip percent and Max context follow cap when model max changes', () => {
  const usage = { inputTokens: 800, outputTokens: 200, totalTokens: 1000 };
  const { rerender } = render(
    <Composer
      value=""
      isEmpty={false}
      mode="agent"
      modes={['agent', 'plan']}
      tokenUsage={usage}
      contextPct={1.0}
      maxContextTokens={100000}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );
  const tip = () => screen.getByRole('tooltip').textContent ?? '';
  expect(tip()).toMatch(/1\.0% context used/);
  expect(tip()).toMatch(/Max context 100000/);

  rerender(
    <Composer
      value=""
      isEmpty={false}
      mode="agent"
      modes={['agent', 'plan']}
      tokenUsage={usage}
      contextPct={10.0}
      maxContextTokens={10000}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );
  expect(tip()).toMatch(/10\.0% context used/);
  expect(tip()).toMatch(/Max context 10000/);
});

