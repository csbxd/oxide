"""Exercise batch scheduling and cancellation without Rust or Go subprocesses."""

from collections import Counter
from concurrent.futures import ThreadPoolExecutor
import json
import errno
from pathlib import Path
import tempfile
from types import SimpleNamespace
import threading
import time
import unittest
from unittest.mock import Mock, patch

import run
from run import CommandCancelled, CommandControl, InfrastructureFailure, Runner


def runner_fixture(jobs=1, batch_size=2):
    runner = Runner.__new__(Runner)
    runner.args = SimpleNamespace(jobs=jobs, batch_size=batch_size, command="promote",
                                  filter=None, select=None, build_timeout=5, timeout=5)
    runner.state_lock = threading.RLock()
    runner.control = CommandControl()
    runner.env = {"CARGO_TARGET_DIR": "/test/shared-cargo", "PRESERVED": "yes"}
    runner.cache = Path("/test/cache")
    runner.triple = "aarch64-unknown-linux-gnu"
    runner.work = Path("/test/work")
    runner.basefile = Path("/test/baseline.json")
    runner.report_path = Path("/test/report.json")
    runner.base = None
    runner.context = {}
    runner.started = time.time()
    runner.cases = {}
    runner.results = {}
    runner.details = {}
    runner.write_report = Mock()
    runner.prepare_worker_caches = Mock(return_value=[Path(f"/test/worker-{index}") for index in range(jobs)])
    return runner


def add_case(runner, name, suite="coretests", ignored=False):
    key = f"{suite}/{name}"
    runner.cases[key] = {"suite": suite, "name": name, "ignored": ignored}
    runner.results[key] = "ignored" if ignored else "not_run"
    return key


class BatchSchedulingTests(unittest.TestCase):
    def test_default_worker_checks_each_case_once_and_preserves_suite_batches(self):
        runner = runner_fixture()
        first = add_case(runner, "first")
        rejected = add_case(runner, "second")
        third = add_case(runner, "third")
        alloc = add_case(runner, "first", "alloctests")
        ignored = add_case(runner, "ignored", ignored=True)
        add_case(runner, "unselected")
        runner.native_check = Mock(side_effect=lambda key: key != rejected)
        runner.batch = Mock()

        runner.run_batches({first, rejected, third, alloc, ignored})

        self.assertCountEqual([call.args[0] for call in runner.native_check.call_args_list],
                              [first, rejected, third, alloc])
        batches = [(call.args[0], call.args[1]) for call in runner.batch.call_args_list if call.args[1]]
        self.assertEqual(batches, [("coretests", [first]), ("coretests", [third]), ("alloctests", [alloc])])
        runner.prepare_worker_caches.assert_not_called()
        self.assertEqual(runner.env, {"CARGO_TARGET_DIR": "/test/shared-cargo", "PRESERVED": "yes"})

    def test_parallel_workers_overlap_and_keep_their_own_cargo_targets(self):
        runner = runner_fixture(jobs=2, batch_size=1)
        selected = {add_case(runner, f"case{index}") for index in range(4)}
        barrier = threading.Barrier(2, timeout=4)
        lock = threading.Lock()
        native = []
        batches = []
        targets = {}
        active = set()

        def native_check(key):
            with lock:
                native.append(key)
            return True

        def batch(suite, ids, cargo_target=None):
            identity = threading.get_ident()
            with lock:
                self.assertIsNotNone(cargo_target)
                self.assertNotIn(cargo_target, active)
                self.assertEqual(targets.setdefault(identity, cargo_target), cargo_target)
                active.add(cargo_target)
                batches.extend(ids)
            barrier.wait()
            barrier.wait()
            with lock:
                active.remove(cargo_target)

        runner.native_check, runner.batch = native_check, batch
        before = runner.env.copy()
        runner.run_batches(selected)

        runner.prepare_worker_caches.assert_called_once_with()
        self.assertEqual(Counter(native), Counter({key: 1 for key in selected}))
        self.assertEqual(Counter(batches), Counter({key: 1 for key in selected}))
        self.assertEqual(len(targets), 2)
        self.assertEqual(set(targets.values()), {Path("/test/worker-0"), Path("/test/worker-1")})
        self.assertFalse(active)
        self.assertEqual(runner.env, before)

    def test_no_tasks_does_not_prepare_worker_caches(self):
        runner = runner_fixture(jobs=2)
        ignored = add_case(runner, "ignored", ignored=True)
        runner.native_check, runner.batch = Mock(), Mock()
        runner.run_batches({ignored})
        runner.native_check.assert_not_called()
        runner.batch.assert_not_called()
        runner.prepare_worker_caches.assert_not_called()

    def test_worker_failure_and_interrupt_cancel_then_join_other_workers(self):
        for error in [RuntimeError("worker failed"), KeyboardInterrupt()]:
            with self.subTest(error=type(error).__name__):
                runner = runner_fixture(jobs=2, batch_size=1)
                first = add_case(runner, "first")
                second = add_case(runner, "second")
                both_started = threading.Barrier(2, timeout=4)
                other_finished = threading.Event()
                runner.native_check = lambda key: True

                def batch(suite, ids, cargo_target=None):
                    both_started.wait()
                    if ids == [first]:
                        raise error
                    try:
                        self.assertTrue(runner.control.cancelled.wait(4), "failed worker did not cancel peers")
                        runner.control.check()
                    finally:
                        other_finished.set()

                runner.batch = batch
                with self.assertRaises(type(error)):
                    runner.run_batches({first, second})
                self.assertTrue(other_finished.is_set(), "scheduler returned before the other worker exited")
                with self.assertRaises(CommandCancelled):
                    runner.control.check()

    def test_native_exception_cancels_before_any_batch(self):
        runner = runner_fixture()
        case = add_case(runner, "case")
        runner.native_check = Mock(side_effect=RuntimeError("native setup failed"))
        runner.batch = Mock()
        with self.assertRaisesRegex(RuntimeError, "native setup failed"):
            runner.run_batches({case})
        runner.batch.assert_not_called()
        with self.assertRaises(CommandCancelled):
            runner.control.check()

    def test_original_resource_failure_wins_even_when_peer_cancellation_is_enqueued_first(self):
        for jobs in (2, 3):
            for report_full in (False, True):
                with self.subTest(jobs=jobs, report_full=report_full), tempfile.TemporaryDirectory() as temporary:
                    runner = runner_fixture(jobs=jobs, batch_size=1)
                    first = add_case(runner, "first")
                    second = add_case(runner, "second")
                    log = Path(temporary) / "go-build.log"
                    log.write_text("write compiled object: no space left on device\n")
                    both_started = threading.Barrier(2, timeout=4)
                    peer_enqueued = threading.Event()
                    identities = {}
                    real_cancel = runner.control.cancel
                    real_check = runner.control.check
                    main_checks = 0
                    main_identity = threading.get_ident()

                    def ordered_cancel():
                        real_cancel()
                        identity = threading.get_ident()
                        if identity == identities.get("peer"):
                            # worker() appends its exception before cancel().
                            peer_enqueued.set()
                        elif identity == identities.get("source"):
                            self.assertTrue(peer_enqueued.wait(4), "peer cancellation was not enqueued")

                    def ordered_check():
                        nonlocal main_checks
                        if threading.get_ident() == main_identity:
                            main_checks += 1
                            if jobs == 3 and main_checks == 3:
                                # Exercise the startup cancellation handler as
                                # well as the ordinary post-join error path.
                                self.assertTrue(peer_enqueued.wait(4))
                        real_check()

                    def batch(suite, ids, cargo_target=None):
                        role = "source" if ids == [first] else "peer"
                        identities[role] = threading.get_ident()
                        both_started.wait()
                        if role == "source":
                            run.check_infrastructure(log, "go-build", runner.control)
                        self.assertTrue(runner.control.cancelled.wait(4))
                        runner.control.check()

                    runner.control.cancel = ordered_cancel
                    runner.control.check = ordered_check
                    runner.native_check = lambda key: True
                    runner.batch = batch
                    if report_full:
                        runner.write_report.side_effect = OSError(errno.ENOSPC, "report disk full")
                    with self.assertRaises(InfrastructureFailure) as raised:
                        runner.run_batches({first, second})
                    self.assertTrue(peer_enqueued.is_set())
                    self.assertEqual(raised.exception.stage, "go-build")
                    self.assertEqual(raised.exception.log, log)
                    self.assertIn("no space left on device", str(raised.exception))
                    if report_full:
                        self.assertTrue(any("report disk full" in note for note in raised.exception.__notes__))


class PromotionCancellationTests(unittest.TestCase):
    def test_exception_interrupt_and_prior_cancellation_never_promote(self):
        for outcome in [RuntimeError("batch failed"), KeyboardInterrupt(), "cancel"]:
            with self.subTest(outcome=outcome):
                runner = runner_fixture()
                case = add_case(runner, "case")
                runner.results[case] = "passed"
                runner.prepare, runner.discover, runner.command = Mock(), Mock(), Mock()
                runner.run_batches = Mock(side_effect=(lambda selected: runner.control.cancel())
                                          if outcome == "cancel" else outcome)
                error = CommandCancelled if outcome == "cancel" else type(outcome)
                with patch.object(run, "promote_baseline") as promote, patch.object(run, "save") as save:
                    with self.assertRaises(error):
                        runner.run()
                    promote.assert_not_called()
                    save.assert_not_called()


class ParallelArtifactsTests(unittest.TestCase):
    def test_concurrent_records_and_real_reports_retain_all_results(self):
        runner = runner_fixture(jobs=4)
        del runner.write_report
        with tempfile.TemporaryDirectory() as temporary:
            runner.work = Path(temporary)
            runner.report_path = runner.work / "latest.json"
            keys = [[add_case(runner, f"worker{worker}-case{index}") for index in range(8)]
                    for worker in range(4)]
            statuses = ["passed", "failed", "native_failed", "timeout"]
            round_start = threading.Barrier(4, timeout=4)

            def report_worker(worker):
                for key in keys[worker]:
                    round_start.wait()
                    runner.record(key, statuses[worker], f"detail for {key}")
                    runner.write_report()
                    # Concurrent readers must always receive complete JSON and
                    # matching counts, even while another worker publishes.
                    current = json.loads(runner.report_path.read_text())
                    self.assertEqual(current["counts"], dict(Counter(current["results"].values())))

            with ThreadPoolExecutor(max_workers=4) as executor:
                futures = [executor.submit(report_worker, worker) for worker in range(4)]
                for future in futures:
                    future.result(timeout=5)
            final = json.loads(runner.report_path.read_text())
            expected = {key: statuses[worker] for worker in range(4) for key in keys[worker]}
            self.assertEqual(final["results"], expected)
            self.assertEqual(final["details"], {key: f"detail for {key}" for key in expected})
            self.assertEqual(final["counts"], {status: 8 for status in statuses})
            self.assertEqual(final["discovered"], sorted(expected))
            self.assertEqual(final["cases"], runner.cases)
            self.assertEqual(final["limits"]["jobs"], 4)

    def test_real_worker_cache_copies_have_independent_writable_files(self):
        runner = runner_fixture(jobs=2)
        del runner.prepare_worker_caches
        with tempfile.TemporaryDirectory() as temporary:
            runner.cache = Path(temporary) / "cache"
            runner.work = Path(temporary) / "work"
            runner.work.mkdir()
            source = runner.cache / "cargo-target"
            fixtures = {"debug/deps/libfixture.rlib": "compiled artifact",
                        "debug/.fingerprint/fixture/stamp": "dependency fingerprint"}
            for name, contents in fixtures.items():
                path = source / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(contents)
            before = runner.env.copy()

            targets = runner.prepare_worker_caches()

            self.assertEqual(len(targets), 2)
            self.assertNotEqual(targets[0], targets[1])
            for name, contents in fixtures.items():
                files = [source / name, targets[0] / name, targets[1] / name]
                self.assertEqual(len({(path.stat().st_dev, path.stat().st_ino) for path in files}), 3)
                self.assertEqual([path.read_text() for path in files], [contents] * 3)
                files[1].write_text("worker zero changed this file")
                self.assertEqual(files[0].read_text(), contents)
                self.assertEqual(files[2].read_text(), contents)
            self.assertEqual(runner.env, before)


class CommandCancellationTests(unittest.TestCase):
    def test_cancel_kills_active_processes_and_prevents_new_starts(self):
        control = CommandControl()
        finished, active = Mock(pid=1001), Mock(pid=1002)
        with patch.object(run.subprocess, "Popen", side_effect=[finished, active]) as popen, \
                patch.object(run, "kill_process_group") as kill:
            first = control.start(["first"], start_new_session=True)
            second = control.start(["second"], start_new_session=True)
            control.finished(first)
            control.cancel()
            kill.assert_called_once_with(second)
            with self.assertRaises(CommandCancelled):
                control.start(["should-not-start"], start_new_session=True)
            self.assertEqual(popen.call_count, 2)
            control.finished(second)


if __name__ == "__main__":
    unittest.main()
