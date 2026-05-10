import { expect, test } from 'vitest';
import {
  segmentComposerSlashSpans,
  slugSlashesForUserBubbleMarkdown,
  stripCoddySkillMarkdownLinks,
} from './segmentComposerSlashSpans';

test('segmentComposerSlashSpans plain slash mid-line', () => {
  const s = segmentComposerSlashSpans('asdfasf /find-skills asdfasdf');
  expect(s).toEqual([
    { type: 'text', value: 'asdfasf ' },
    { type: 'slash', literal: '/find-skills', name: 'find-skills' },
    { type: 'text', value: ' asdfasdf' },
  ]);
});

test('segmentComposerSlashSpans line start slash', () => {
  expect(segmentComposerSlashSpans('/foo')).toEqual([
    { type: 'slash', literal: '/foo', name: 'foo' },
  ]);
});

test('segmentComposerSlashSpans skips letter before slash', () => {
  expect(segmentComposerSlashSpans('x/foo')).toEqual([{ type: 'text', value: 'x/foo' }]);
});

test('stripCoddySkillMarkdownLinks restores plain slash token', () => {
  expect(stripCoddySkillMarkdownLinks('a [/demo](coddy-skill:demo) b')).toBe('a /demo b');
});

test('stripCoddySkillMarkdownLinks leaves mismatched href', () => {
  const s = 'a [/demo](coddy-skill:other) b';
  expect(stripCoddySkillMarkdownLinks(s)).toBe(s);
});

test('slugSlashesForUserBubbleMarkdown for Markdown chip render', () => {
  expect(slugSlashesForUserBubbleMarkdown('hi /demo there')).toBe(
    'hi [/demo](coddy-skill:demo) there',
  );
});

test('slugSlashesForUserBubbleMarkdown strips legacy first then chips', () => {
  expect(slugSlashesForUserBubbleMarkdown('x [/a](coddy-skill:a) /b')).toBe(
    'x [/a](coddy-skill:a) [/b](coddy-skill:b)',
  );
});
