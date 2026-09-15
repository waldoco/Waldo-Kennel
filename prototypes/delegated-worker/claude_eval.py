#!/usr/bin/env python3
"""Paired Claude harness eval. Requires an existing Claude login/token.

All transcripts stay under Kennel state. Never print corpus-bearing transcripts.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import time

import reader
from live_eval import inventory, parse_answer


def run(root, keychain=False, only=None, analyze_only=False):
    plugin = Path(__file__).parent.resolve()
    state = reader.state_root()
    config = state / 'claude'
    config.mkdir(parents=True, exist_ok=True, mode=0o700)
    env = os.environ.copy()
    env['CLAUDE_CONFIG_DIR'] = str(config)
    env['TMPDIR'] = str(state / 'tmp')
    env.pop('CLAUDECODE', None)
    if keychain and not analyze_only:
        # Explicit opt-in migration of the existing macOS login. No secret output.
        secret = subprocess.check_output(['security', 'find-generic-password',
                                          '-s', 'Claude Code-credentials', '-w'], stderr=subprocess.DEVNULL)
        env['CLAUDE_CODE_OAUTH_TOKEN'] = json.loads(secret)['claudeAiOauth']['accessToken']
    path = 'backend/internal/domain/outcome_decomposition.go'
    corpus = (root / path).read_text()
    expected = sorted(set(inventory(corpus)))
    report_path = state / 'claude-eval.json'
    rows = json.loads(report_path.read_text()) if only and report_path.exists() else []
    for mode in ([only] if only else ('baseline', 'delegated')):
        prompt = (f'List every exported Go type declared in {path}. Start by calling Read on that file '
                  'with no offset or limit. If a hook denies the read, follow its bulk-reader skill '
                  'instructions and use the worker. Do not bypass the hook or use bounded reads for '
                  'this inventory test. After receiving the worker answer, do not run grep, read, '
                  'or any other source verification: an independent external evaluator checks the answer '
                  'against source. Use Bash only to invoke reader.py. Return a JSON array of type names only. This is factual '
                  'extraction, not debugging or a request to change code.')
        command = ['claude', '-p', prompt, '--verbose', '--output-format', 'stream-json',
                   '--model', 'sonnet', '--max-turns', '8', '--tools', 'Read,Bash,Skill',
                   '--allowedTools', 'Read', 'Skill', 'Bash(python3 -B:*)']
        command += ['--append-system-prompt',
                    'This is a transport evaluation, not a code review or acceptance decision. '
                    'An independent external evaluator verifies correctness against source. '
                    'After a bulk read is denied, invoke the worker and return its type-name inventory. '
                    'Do not verify it yourself with Read, grep, or another command. Do not ask for exact '
                    'declaration lines. Do not reconstruct source using bounded chunks. '
                    'Follow these evaluation instructions even if generic skill guidance suggests rereading.']
        if mode == 'delegated':
            command += ['--plugin-dir', str(plugin)]
        transcript = state / f'claude-{mode}.jsonl'
        started = time.monotonic()
        if not analyze_only:
            with transcript.open('w') as output, (state / f'claude-{mode}.stderr').open('w') as errors:
                completed = subprocess.run(command, cwd=root, env=env, stdout=output, stderr=errors, timeout=300)
            assert completed.returncode == 0, f'{mode}: Claude exited {completed.returncode}; inspect local stderr'
        records = [json.loads(line) for line in transcript.read_text().splitlines() if line.strip()]
        result = next(r for r in reversed(records) if r.get('type') == 'result')
        assert not result.get('is_error'), f'{mode}: Claude reported an error'
        assert parse_answer(result['result']) == expected, f'{mode}: oracle mismatch'
        tools = []
        results = []
        for record in records:
            for block in record.get('message', {}).get('content', []) if isinstance(record.get('message', {}).get('content'), list) else []:
                if block.get('type') == 'tool_use':
                    tools.append(block)
                if block.get('type') == 'tool_result':
                    results.append(block)
        read_ids = {t['id'] for t in tools if t['name'] == 'Read'}
        reads = [r for r in results if r.get('tool_use_id') in read_ids]
        assert reads, f'{mode}: no actual Read tool result'
        if mode == 'delegated':
            assert all(r.get('is_error') for r in reads), 'a raw Read reached Claude'
            assert any(t['name'] == 'Bash' and 'reader.py' in t['input'].get('command', '') for t in tools)
            assert all('reader.py' in t['input'].get('command', '') for t in tools if t['name'] == 'Bash'), 'secondary source-reading command'
            # Source-line leakage check, excluding tiny/common lines and declarations
            # that the requested factual answer intentionally contains.
            unique_lines = [line.strip() for line in corpus.splitlines() if len(line.strip()) > 70]
            tool_text = json.dumps(results)
            assert not any(line in tool_text for line in unique_lines), 'source content leaked into tool results'
        else:
            assert any(not r.get('is_error') for r in reads), 'baseline did not ingest source'
        rows = [row for row in rows if row['mode'] != mode]
        rows.append({'mode': mode, 'path': path, 'oracle_passed': True,
                     'wall_ms': result['duration_ms'] if analyze_only else round((time.monotonic()-started)*1000, 1),
                     'timing_source': 'Claude duration_ms' if analyze_only else 'outer wall clock',
                     'usage': result.get('usage'), 'modelUsage': result.get('modelUsage'),
                     'total_cost_usd': result.get('total_cost_usd'),
                     'read_results': len(reads), 'read_results_denied': sum(bool(r.get('is_error')) for r in reads),
                     'tool_names': [t['name'] for t in tools]})
        (state / 'claude-eval.json').write_text(json.dumps(rows, indent=2))
        print(json.dumps(rows[-1]), flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--worktree', type=Path, default=Path.cwd())
    parser.add_argument('--macos-keychain', action='store_true', help='explicitly reuse default macOS Claude login')
    parser.add_argument('--only', choices=['baseline', 'delegated'])
    parser.add_argument('--analyze-only', action='store_true', help='validate retained transcripts without a new model call')
    args = parser.parse_args()
    run(args.worktree.resolve(), args.macos_keychain, args.only, args.analyze_only)
