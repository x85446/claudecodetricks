#!/usr/bin/env python3
# TESTMASTER: id=indigobird-authority-thin-skill tier=fast parallel=yes
"""Exercise both skill forwarding clients against the real binary in private stores."""
import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[3]
BINARY = ROOT / 'bin/iterate-run'


def run_client(client, store, *args, expected=0, payload=None, env=None):
    command = [str(client)]
    if client == BINARY:
        command.append('conductor')
    command += [*args, '--store', str(store), '--json']
    process = subprocess.run(command, input=None if payload is None else json.dumps(payload),
                             text=True, capture_output=True, timeout=15, env=env)
    assert process.returncode == expected, (command, process.returncode, process.stdout, process.stderr)
    result = json.loads(process.stdout)
    assert result['ok'] == (expected == 0), result
    return result


def main():
    assert BINARY.is_file(), 'make bin/iterate-run first'
    with tempfile.TemporaryDirectory(prefix='conductor-skill-') as work:
        folder = Path(work)
        env = dict(os.environ, PATH=str(ROOT / 'bin') + os.pathsep + os.environ['PATH'])
        for harness in ('skills', 'codex-skills'):
            wrapper = ROOT / harness / 'iterate-conductor/scripts/control.sh'
            store = folder / harness / 'store'
            project = folder / harness / 'filemaster'
            project.mkdir(parents=True)
            read = lambda *a, **kw: run_client(wrapper, store, *a, env=env, **kw)
            direct = lambda *a, **kw: run_client(BINARY, store, *a, env=env, **kw)
            initial = read('config', 'show')
            assert initial == direct('config', 'show')
            created = read('project', 'create', '--path', str(project), '--revision', str(initial['revision']))
            project_id = created['data']['id']
            assert created['data']['enabled'] is False
            assert not list(project.iterdir()), 'registration created repository files'
            cfg = read('config', 'show')['data']
            sub = next(s for s in cfg['subscriptions'] if s['adapter'] == 'codex')
            sequence_id = sub['sequences'][0]['id']
            claude = next(s for s in cfg['subscriptions'] if s['adapter'] == 'claude')
            claude_sequence = claude['sequences'][0]['id']
            unrelated = copy.deepcopy(cfg['subscriptions'][-1])
            recurrence = dict(days=[1, 2, 3, 4, 5], start='22:00', end='06:00', timezone='America/Chicago', every_weeks=1)
            cfg['time_windows'] = [dict(id='weeknights', name='Weeknights', enabled=True, revision=1,
                                       start_mode='weekly', recurrence=recurrence)]
            sub['overdrive_windows'] = [dict(id=ident, name=name, enabled=False, revision=1, start_mode='weekly',
                recurrence=dict(days=[day], start=start, end='00:00', timezone='America/Chicago', every_weeks=1),
                end_mode=end, target_used_percent=target, **extra)
                for ident, name, day, start, target, end, extra in [
                    ('saturday', 'Before Sunday reset', 6, '00:00', 100, 'next_reset', {}),
                    ('monday', 'Before planned Tuesday reset', 1, '00:00', 100, 'explicit', dict(end_day=2, end_time='00:00')),
                    ('thursday', 'Thursday capacity target', 4, '16:00', 60, 'next_reset', {})]]
            claude['overdrive_windows'] = [dict(id='claude-thursday', name='Before Claude reset', enabled=False, revision=1,
                start_mode='weekly', recurrence=dict(days=[4], start='00:00', end='00:00', timezone='America/Chicago', every_weeks=1),
                end_mode='next_reset', target_used_percent=100)]
            base = dict(id='filemaster-overdrive', name='Follow Codex overdrive', project_id=project_id, enabled=True,
                        priority=10, mode='overdrive', participants=[sequence_id, claude_sequence], entries=[],
                        honor_engine_overdrive=True, only_during_overdrive=True,
                        run_limit='drain_queue', on_window_end='finish_checkpoint')
            cfg['automations'] = [base]
            revision = cfg['revision']
            read('config', 'validate', '--file', '-', payload=cfg)
            read('config', 'apply', '--file', '-', '--revision', str(revision), '--dry-run', payload=cfg)
            assert read('config', 'show')['revision'] == revision
            saved = read('config', 'apply', '--file', '-', '--revision', str(revision), payload=cfg)
            assert saved['data'] == direct('config', 'show')['data']
            cfg = saved['data']
            assert cfg['subscriptions'][-1] == unrelated
            assert not cfg['enabled'] and not cfg['projects'][0]['enabled']
            before = (store / 'config.json').read_bytes()
            # Reapplying the same conversational draft uses the existing stable
            # identities; the atomic API must not append duplicate definitions.
            repeated = read('config', 'apply', '--file', '-', '--revision', str(cfg['revision']), payload=cfg)['data']
            assert [a['id'] for a in repeated['automations']] == ['filemaster-overdrive']
            assert len(next(s for s in repeated['subscriptions'] if s['id'] == sub['id'])['overdrive_windows']) == 3
            assert len(next(s for s in repeated['subscriptions'] if s['id'] == claude['id'])['overdrive_windows']) == 1
            assert repeated['subscriptions'][-1] == unrelated
            before = (store / 'config.json').read_bytes()
            read('config', 'apply', '--file', '-', '--revision', str(revision), payload=cfg, expected=3)
            assert (store / 'config.json').read_bytes() == before
            # Every schedule behavior round-trips through both wrappers without dispatch.
            for limit in ('one_plan', 'drain_queue'):
                for ending in ('finish_checkpoint', 'finish_plan', 'interrupt'):
                    current = read('config', 'show')['data']
                    rule = copy.deepcopy(base)
                    rule.update(id='scheduled', name='Weeknight work', mode='scheduled', participants=[], window_id='weeknights',
                                honor_engine_overdrive=False, only_during_overdrive=False, run_limit=limit, on_window_end=ending,
                                entries=[dict(id='first', sequence_id=sequence_id, max_band='green'),
                                         dict(id='second', sequence_id=sequence_id, max_band='yellow'),
                                         dict(id='third', sequence_id=sequence_id, max_band='red')])
                    current['automations'] = [base, rule]
                    read('config', 'apply', '--file', '-', '--revision', str(current['revision']), payload=current)
                    assert direct('automation', 'show', 'scheduled')['data']['on_window_end'] == ending
                    assert direct('automation', 'show', 'scheduled')['data']['run_limit'] == limit
            # Named policy edits preserve references and unrelated definitions.
            current = read('config', 'show')['data']
            policy = copy.deepcopy(current['power_bands'][0])
            policy.update(id='weekend', name='Weekend budget', short_green=25)
            current['power_bands'].append(policy)
            next(s for s in current['subscriptions'] if s['id'] == sub['id'])['power_band_id'] = 'weekend'
            read('config', 'apply', '--file', '-', '--revision', str(current['revision']), payload=current)
            assert direct('subscription', 'show', sub['id'])['data']['power_band_id'] == 'weekend'
            assert direct('power-band', 'show', 'weekend')['data']['short_green'] == 25
            for action in ('status', 'explain'):
                a, b = read(action, '--project', project_id)['data'], direct(action, '--project', project_id)['data']
                a.pop('at', None); b.pop('at', None)
                assert a == b, (a, b)
            before = (store / 'config.json').read_bytes()
            sample = dict(project_id=project_id, now='2026-10-16T12:00:00Z', queue_available=True, project_busy=False)
            assert read('simulate', '--file', '-', payload=sample)['data'] == direct('simulate', '--file', '-', payload=sample)['data']
            assert before == (store / 'config.json').read_bytes()
            read('tick', '--project', project_id, expected=4)
            assert not list(project.iterdir()), 'gated tick touched project'
            # A real project start launches only the binary worker; global disabled
            # configuration and an empty fixture queue prevent any AI invocation.
            current = read('config', 'show')['data']
            started = read('start', '--project', project_id, '--revision', str(current['revision']))
            assert started['data']['pid'] > 0
            current = read('config', 'show')['data']
            assert current['projects'][0]['enabled'] and not current['enabled']
            read('stop', '--project', project_id, '--revision', str(current['revision']))
            # Check the worker completion before deleting its private state.
            import time
            log = store / 'worker.log'
            for _ in range(100):
                if log.exists() and log.stat().st_size:
                    break
                time.sleep(0.01)
            assert log.exists() and log.stat().st_size, 'binary worker did not finish gated tick'
            bad = read('not-a-command', expected=2)
            assert bad['error'], bad
            missing = subprocess.run([str(wrapper), 'status'], env=dict(env, PATH='/nonexistent'), capture_output=True, text=True)
            assert missing.returncode == 127 and 'not found' in missing.stderr
            print(harness + ': binary forwarding, revisions, idempotent IDs, schedules, budgets, overdrive, lifecycle and failure propagation pass')


if __name__ == '__main__':
    main()
