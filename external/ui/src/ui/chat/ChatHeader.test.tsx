import React from 'react';
import { afterEach, expect, test } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { ChatHeader } from './ChatHeader';

afterEach(() => cleanup());

test('edit mode shows full-width title input class', () => {
  render(<ChatHeader title="Hello" editable onTitleSave={() => {}} />);

  fireEvent.click(screen.getByRole('button', { name: /chat title/i }));

  const input = screen.getByRole('textbox');
  expect(input).toHaveClass('chat-title-input');
});
