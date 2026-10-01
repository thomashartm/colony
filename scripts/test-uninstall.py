#!/usr/bin/env python3
"""Exercise uninstall in disposable homes without touching real settings."""
import json
import fcntl
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import unittest

REPO = Path(__file__).resolve().parent.parent


class UninstallTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory(prefix='motley-uninstall-build-')
        cls.binary = Path(cls.build.name) / 'motley'
        subprocess.run(['go', 'build', '-o', str(cls.binary), './cmd/motley'], cwd=REPO, check=True)

    @classmethod
    def tearDownClass(cls):
        cls.build.cleanup()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='motley-uninstall-test-')
        self.addCleanup(self.temp.cleanup)
        self.home = Path(self.temp.name) / "user's home"
        self.config = self.home / 'config with spaces'
        self.commands = self.home / '.local/bin'
        self.commands.mkdir(parents=True)
        self.env = dict(os.environ, HOME=str(self.home), XDG_CONFIG_HOME=str(self.config),
                        XDG_STATE_HOME=str(self.home / 'state'), CODEX_HOME=str(self.home / 'custom codex'))
        shutil.copyfile(self.binary, self.commands / 'motley')
        (self.commands / 'mtly').symlink_to('motley')
        self.claude = self.home / '.claude/settings.json'
        self.codex = self.home / 'custom codex/hooks.json'
        for agent, path in [('claude', self.claude), ('codex', self.codex)]:
            value = {'custom': {'preserve': True}, 'hooks': {'Stop': [
                {'matcher': '*', 'hooks': [
                    {'type': 'command', 'command': 'motley report --agent '+agent},
                    {'type': 'command', 'command': 'my-hook'}]},
                {'hooks': [{'type': 'command', 'command': 'motley report --agent '+agent}]}]}}
            if agent == 'codex':
                value['description'] = 'User description\nAdded motley schema = 1 status hooks.'
            self.write(path, json.dumps(value).encode())
        self.plugin = self.config / 'opencode/plugins/motley.ts'
        self.write(self.plugin, (REPO / 'integrations/opencode/motley.ts').read_bytes())
        self.tmux = self.home / '.tmux.conf'
        self.write(self.tmux, ('# user config\nset -g mouse on\n# motley installer — schema = 1\n'
                              + 'source-file '+shlex.quote(str(self.config / 'motley/motley.tmux.conf'))+'\n').encode())
        self.keep = {}
        for relative in ['projects/api/work.txt', 'worktrees/api/changes.txt', 'state/motley/members/one.toml',
                         'config with spaces/motley/config.toml', 'config with spaces/motley/motley.tmux.conf',
                         '.motley/install-history/previous.log', '.bashrc', '.zshrc', '.profile',
                         'config with spaces/fish/conf.d/motley.fish']:
            path = self.home / relative
            data = ('preserve '+relative+'\n').encode()
            self.write(path, data)
            self.keep[path] = data

    @staticmethod
    def write(path, data):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)

    def run_uninstall(self, success=True):
        run = subprocess.run(['bash', str(REPO / 'uninstall.sh')], env=self.env, text=True,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(run.returncode, 0 if success else 1, run.stdout+run.stderr)
        self.assertFalse(run.stderr, run.stderr)
        self.assertTrue(all(line.startswith('• ') for line in run.stdout.splitlines()), run.stdout)
        history = Path(next(line[len('• History: '):] for line in run.stdout.splitlines()
                            if line.startswith('• History: ')))
        self.assertTrue(history.is_file())
        self.assertEqual(history.stat().st_mode & 0o777, 0o600)
        for path, data in self.keep.items():
            self.assertEqual(path.read_bytes(), data, str(path))
        return run.stdout, history

    def test_bash(self):
        self.check_remove_and_repeat('bash')

    def test_zsh(self):
        self.check_remove_and_repeat('zsh')

    def test_fish(self):
        self.check_remove_and_repeat('fish')

    def check_remove_and_repeat(self, shell):
        self.env['SHELL'] = '/bin/'+shell
        original = self.claude.read_bytes()
        output, history = self.run_uninstall()
        self.assertIn('✅', output)
        self.assertFalse((self.commands / 'motley').exists())
        self.assertFalse((self.commands / 'mtly').is_symlink())
        self.assertFalse(self.plugin.exists())
        for path in [self.claude, self.codex]:
            root = json.loads(path.read_bytes())
            self.assertTrue(root['custom']['preserve'])
            self.assertEqual(root['hooks']['Stop'], [{'matcher': '*', 'hooks': [
                {'type': 'command', 'command': 'my-hook'}]}])
        self.assertEqual(json.loads(self.codex.read_bytes())['description'], 'User description')
        self.assertEqual(self.tmux.read_text(), '# user config\nset -g mouse on\n')
        self.assertTrue(any(p.read_bytes() == original for p in self.claude.parent.glob('*.motley-uninstall-backup-*')))
        before = history.read_bytes()
        output2, history2 = self.run_uninstall()
        self.assertIn('Already uninstalled', output2)
        self.assertNotEqual(history, history2)
        self.assertEqual(history.read_bytes(), before)

    def test_partial_install_and_binary_alias(self):
        (self.commands / 'mtly').unlink()
        shutil.copyfile(self.binary, self.commands / 'mtly')
        (self.commands / 'motley').unlink()
        self.claude.unlink()
        self.run_uninstall()
        self.assertFalse((self.commands / 'mtly').exists())

    def test_unrelated_executable_is_not_run_or_removed(self):
        (self.commands / 'motley').write_text('#!/bin/sh\nexit 99\n')
        before = self.claude.read_bytes()
        output, history = self.run_uninstall(False)
        self.assertIn('Not a Motley executable', output)
        self.assertEqual(self.claude.read_bytes(), before)
        self.assertIn('Traceback', history.read_text())
        self.assertTrue((self.commands / 'mtly').is_symlink())

    def test_replaced_alias_is_preserved(self):
        (self.commands / 'mtly').unlink()
        (self.commands / 'mtly').symlink_to('other-tool')
        self.run_uninstall(False)
        self.assertEqual(os.readlink(self.commands / 'mtly'), 'other-tool')
        self.assertTrue((self.commands / 'motley').is_file())

    def test_modified_plugin_is_preserved(self):
        self.plugin.write_text('// user replacement\n')
        self.run_uninstall(False)
        self.assertEqual(self.plugin.read_text(), '// user replacement\n')
        self.assertTrue((self.commands / 'motley').is_file())

    def test_invalid_settings_prevent_any_removal(self):
        self.codex.write_text('{invalid')
        before = self.claude.read_bytes()
        self.run_uninstall(False)
        self.assertEqual(self.claude.read_bytes(), before)
        self.assertTrue(self.plugin.is_file())
        self.assertTrue((self.commands / 'motley').is_file())

    def test_settings_symlink_is_preserved(self):
        target = self.home / 'user-settings.json'
        self.claude.rename(target)
        self.claude.symlink_to(target)
        original = target.read_bytes()
        self.run_uninstall(False)
        self.assertTrue(self.claude.is_symlink())
        self.assertEqual(target.read_bytes(), original)

    def test_concurrent_update_blocks_uninstall(self):
        with (self.commands / '.motley-update.lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            output, _ = self.run_uninstall(False)
            self.assertIn('Another update or uninstall is running', output)
        self.assertTrue((self.commands / 'motley').is_file())
        self.assertTrue(self.plugin.is_file())


if __name__ == '__main__':
    unittest.main()
