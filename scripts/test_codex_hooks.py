"""Regression checks for Codex's Windows hook launcher, without a model or Codex API."""

import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
import uuid


REPO = Path(__file__).resolve().parents[1]
CONFIG = json.loads((REPO / '.codex/hooks.json').read_text(encoding='utf-8'))


@unittest.skipUnless(os.name == 'nt', 'Windows command launchers require Windows')
class WindowsHookLauncherTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='codex-hooks-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name) / 'worktree Кириллица'
        hooks = self.root / '.codex/hooks'
        hooks.mkdir(parents=True)
        subprocess.run(['git', 'init', str(self.root)], check=True, capture_output=True)
        shutil.copy2(REPO / '.codex/hooks/attach_rules.py', hooks / 'attach_rules.py')
        rules = self.root / '.cursor/rules'
        rules.mkdir(parents=True)
        (rules / 'always.mdc').write_text('---\nalwaysApply: true\n---\nALWAYS-RULE\n', encoding='utf-8')
        (rules / 'scoped.mdc').write_text('---\nglobs: internal/**/*.go\n---\nSCOPED-RULE\n', encoding='utf-8')
        self.nested = self.root / 'internal/nested'
        self.nested.mkdir(parents=True)
        state = Path(self.temporary.name) / 'state'
        state.mkdir()
        self.environment = dict(os.environ, TEMP=str(state), TMP=str(state))
        self.environment['PATH'] = str(Path(sys.executable).parent) + os.pathsep + self.environment['PATH']

    def invoke(self, event, shell, payload, nested=False):
        command = CONFIG['hooks'][event][0]['hooks'][0]['commandWindows']
        shell_options = {}
        if shell == 'cmd':
            args = command
            shell_options['shell'] = True
        else:
            # Windows PowerShell normally decodes native stdout with the OEM code page.
            # Force it explicitly so a developer's UTF-8 profile cannot hide the bug.
            command = '[Console]::OutputEncoding = [Text.Encoding]::GetEncoding(866); ' + command
            args = ['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', command]
        result = subprocess.run(args, input=json.dumps(payload, ensure_ascii=False).encode('utf-8'),
                                cwd=self.nested if nested else self.root, env=self.environment,
                                capture_output=True, timeout=15, **shell_options)
        self.assertEqual(result.returncode, 0, result.stderr.decode('utf-8', errors='replace'))
        return result.stdout.decode('utf-8').strip()

    def test_session_start_with_cyrillic_root_in_both_shells(self):
        for shell in ['cmd', 'powershell']:
            with self.subTest(shell=shell):
                payload = {'hook_event_name': 'SessionStart', 'source': 'startup', 'session_id': str(uuid.uuid4())}
                result = json.loads(self.invoke('SessionStart', shell, payload))
                self.assertIn('ALWAYS-RULE', result['hookSpecificOutput']['additionalContext'])

    def test_scoped_rule_from_nested_cwd_and_unicode_absolute_path_once(self):
        for shell in ['cmd', 'powershell']:
            with self.subTest(shell=shell):
                path = (self.nested / 'пример.go').resolve().as_posix()
                payload = {'hook_event_name': 'PreToolUse', 'session_id': str(uuid.uuid4()),
                           'tool_input': {'command': f'*** Begin Patch\n*** Update File: {path}\n*** End Patch'}}
                result = json.loads(self.invoke('PreToolUse', shell, payload, nested=True))
                self.assertIn('SCOPED-RULE', result['hookSpecificOutput']['additionalContext'])
                self.assertEqual(self.invoke('PreToolUse', shell, payload, nested=True), '')


class SessionStartMatcherTest(unittest.TestCase):
    def test_fork_is_a_supported_start_source(self):
        matcher = CONFIG['hooks']['SessionStart'][0]['matcher']
        self.assertIsNotNone(re.search(matcher, 'fork'))


if __name__ == '__main__':
    unittest.main(verbosity=2)
