import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

MODULE = Path(__file__).parents[1] / 'reader.py'


class ReaderTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(MODULE.exists(), 'bulk reader implementation is missing')
        spec = importlib.util.spec_from_file_location('reader', MODULE)
        self.reader = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.reader)
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.file = self.root / 'large file.go'
        self.file.write_text('PRIVATE_CORPUS_SENTINEL\n' * 351)

    def test_gate_blocks_bulk_without_returning_corpus(self):
        result = self.reader.gate({'cwd': str(self.root), 'tool_name': 'Read',
                                  'tool_input': {'file_path': str(self.file)}}, 350)
        self.assertEqual(result['hookSpecificOutput']['permissionDecision'], 'deny')
        self.assertNotIn('PRIVATE_CORPUS_SENTINEL', json.dumps(result))

    def test_ranges_and_threshold(self):
        for params, blocked in [({}, True), ({'limit': 350}, False),
                                ({'offset': 1}, True), ({'offset': 300}, False),
                                ({'limit': 10000}, True), ({'limit': 0}, True)]:
            with self.subTest(params=params):
                result = self.reader.gate({'cwd': str(self.root), 'tool_name': 'Read',
                                          'tool_input': {'file_path': str(self.file), **params}}, 350)
                self.assertEqual(bool(result), blocked)
        self.assertEqual(self.reader.gate({'cwd': str(self.root), 'tool_name': 'Read',
                                          'tool_input': {'file_path': str(self.file)}}, 351), {})

    def test_shell_quotes_multiple_files_and_bounded_head(self):
        for command, blocked in [("cat 'large file.go'", True),
                                  ("head -n 5 'large file.go'", False),
                                  ("cat missing 'large file.go'", True),
                                  ("cat 'large file.go' | cat", True),
                                  ('git status', False)]:
            with self.subTest(command=command):
                self.assertEqual(bool(self.reader.gate({'cwd': str(self.root), 'tool_name': 'Bash',
                    'tool_input': {'command': command}}, 350)), blocked)

    def test_worktree_escape_and_symlink_rejected(self):
        (self.root / 'escape').symlink_to('/etc/hosts')
        for name in ['/etc/hosts', '../outside', 'escape']:
            with self.subTest(name=name), self.assertRaises(ValueError):
                self.reader.resolve_paths(self.root, [name])

    def test_sse_multiline_and_disconnect(self):
        stream = io.BytesIO(b': ping\r\ndata: {"type":\r\ndata: "session.idle"}\r\n\r\n')
        self.assertEqual(list(self.reader.events(stream)), [{'type': 'session.idle'}])

    def test_no_self_report_completion(self):
        messages = [{'info': {'role': 'assistant', 'agent': 'kennel-reader'},
                     'parts': [{'type': 'text', 'text': 'I finished'}]}]
        with self.assertRaises(ValueError):
            self.reader.answer(messages, ['large file.go'], self.root)

    def test_wrong_agent_and_missing_reads_rejected(self):
        for agent in ['build', 'kennel-reader']:
            messages = [{'info': {'role': 'assistant', 'agent': agent,
                                  'time': {'completed': 123}, 'finish': 'stop'},
                         'parts': [{'type': 'text', 'text': 'answer'}]}]
            with self.subTest(agent=agent), self.assertRaises(ValueError):
                self.reader.answer(messages, ['large file.go'], self.root)

    def transport(self, event_type='session.idle', wrong_agent=False, mutate=False, unsafe=False):
        calls = []
        state = self.root / 'state'
        completed = [{'info': {'role': 'assistant', 'agent': 'kennel-reader',
                              'finish': 'stop', 'time': {'completed': 1}, 'tokens': {}, 'cost': 0},
                      'parts': [{'type': 'tool', 'tool': 'read', 'state': {'status': 'completed',
                                 'input': {'filePath': str(self.file)}}},
                                {'type': 'text', 'text': 'Concise answer'}]}]

        class Process:
            pid = 12345

            def poll(self):
                return None

            def wait(self, timeout=None):
                return 0

        def start(argv, **kwargs):
            self.assertEqual(kwargs['cwd'], self.root)
            self.assertIn('--hostname', argv)
            kwargs['stdout'].write('opencode server listening on http://127.0.0.1:12345\n')
            kwargs['stdout'].flush()
            return Process()

        def open_(req, **kwargs):
            url = req if isinstance(req, str) else req.full_url
            method = 'GET' if isinstance(req, str) else req.get_method()
            body = None if isinstance(req, str) or not req.data else json.loads(req.data)
            route = url.removeprefix('http://127.0.0.1:12345')
            calls.append((method, route, body))
            if route == '/event':
                event = {'type': event_type, 'properties': {'sessionID': 'ses_test'}}
                return io.BytesIO(('data: ' + json.dumps(event) + '\n\n').encode())
            payload = None
            if route == '/global/health':
                payload = {'healthy': True, 'version': 'fixture'}
            elif route == '/path':
                payload = {'directory': str(self.root)}
            elif route == '/agent':
                payload = [{'name': 'build' if wrong_agent else 'kennel-reader', 'mode': 'primary',
                            'permission': [{'permission': '*', 'pattern': '*', 'action': 'deny'}]}]
                if unsafe:
                    payload[0]['permission'].append({'permission': 'edit', 'pattern': '*', 'action': 'allow'})
            elif route == '/session':
                payload = {'id': 'ses_test'}
            elif route == '/session/ses_test/message':
                if mutate:
                    self.file.write_text('Changed while worker was reading\n')
                payload = completed
            return io.BytesIO(json.dumps(payload).encode())

        class Opener:
            open = staticmethod(open_)

        with patch.object(self.reader, 'state_root', return_value=state), \
             patch.object(self.reader.sys, 'stderr', io.StringIO()), \
             patch.object(self.reader.socket, 'socket'), \
             patch.object(self.reader.subprocess, 'Popen', side_effect=start), \
             patch.object(self.reader.urllib.request, 'build_opener', return_value=Opener()), \
             patch.object(self.reader.os, 'killpg') as kill:
            try:
                result = self.reader.delegate(self.root, [self.file.name], 'Inventory names', 'provider/model', 2)
            finally:
                self.assertTrue(kill.called, 'owned worker must be stopped on success and failure')
        return result, calls

    def test_async_sse_path_sends_paths_only(self):
        (answer, receipt), calls = self.transport()
        self.assertEqual(answer, 'Concise answer')
        self.assertIn(('GET', '/event', None), calls)
        dispatch = next(c for c in calls if c[1].endswith('/prompt_async'))
        self.assertIn('Inventory names', json.dumps(dispatch))
        self.assertNotIn('PRIVATE_CORPUS_SENTINEL', json.dumps(calls))
        self.assertFalse(any(m == 'POST' and p.endswith('/message') for m, p, _ in calls))
        self.assertEqual(receipt['sha256'].keys(), {self.file.name})

    def test_sse_disconnect_and_provider_error_fail(self):
        for kind in ['unrelated', 'session.error', 'permission.asked']:
            with self.subTest(kind=kind), self.assertRaises(ValueError):
                self.transport(event_type=kind)

    def test_unavailable_primary_agent_fails_before_dispatch(self):
        with self.assertRaisesRegex(ValueError, 'primary worker'):
            self.transport(wrong_agent=True)

    def test_source_change_discards_answer(self):
        with self.assertRaisesRegex(ValueError, 'source changed'):
            self.transport(mutate=True)

    def test_effective_write_permission_fails_preflight(self):
        with self.assertRaisesRegex(ValueError, 'not read-only'):
            self.transport(unsafe=True)

    def test_deadline_stops_worker_without_fallback(self):
        with patch.object(self.reader.queue.Queue, 'get', side_effect=self.reader.queue.Empty), \
             self.assertRaisesRegex(TimeoutError, 'deadline exceeded'):
            self.transport()


if __name__ == '__main__':
    unittest.main()
