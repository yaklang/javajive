"""The production source scheduler conserves tests even with stale cost hints."""

import math
import unittest

from tools.ci_scheduler.shard import ShardError, source_parent_batches, source_parent_shards


class SourceDurationSharding(unittest.TestCase):
    def test_expensive_parents_do_not_accumulate_in_alphabetical_shards(self):
        names = [f"Test{i:03d}" for i in range(128)]
        hints = {name: (120 if i % 4 == 1 else 1) for i, name in enumerate(names)}
        lanes = source_parent_shards(names, hints, 16)
        self.assertEqual(sorted(n for lane in lanes for n in lane.items), names)
        self.assertEqual(len({n for lane in lanes for n in lane.items}), len(names))
        self.assertEqual(lanes, source_parent_shards(reversed(names), hints, 16))
        loads = [sum(lane.duration_ns for lane in lanes[i:i+4]) for i in range(0, 16, 4)]
        self.assertEqual(len(set(loads)), 1)
        self.assertLess(max(loads), sum(hints[n] * 1_000_000_000 for n in names[1::4]))

    def test_unknown_and_zero_time_tests_remain_mandatory(self):
        names = ["TestKnown", "TestNew", "TestZero", "TestOther"]
        lanes = source_parent_shards(names, {"TestKnown": 3, "TestZero": 0, "TestStale": 1000}, 2)
        observed = [n for lane in lanes for n in lane.items]
        self.assertCountEqual(observed, names)
        self.assertNotIn("TestStale", observed)
        self.assertTrue(all(lane.duration_ns > 0 for lane in lanes))

    def test_invalid_inventory_and_durations_fail_closed(self):
        with self.assertRaises(ShardError):
            source_parent_shards(["TestA", "TestA"], {}, 1)
        for duration in [-1, math.nan, math.inf, True, "3"]:
            with self.subTest(duration=duration), self.assertRaises(ShardError):
                source_parent_shards(["TestA"], {"TestA": duration}, 1)
        for names, count in [([], 1), (["TestA"], 0), (["TestA"], 2)]:
            with self.subTest(names=names, count=count), self.assertRaises(ShardError):
                source_parent_shards(names, {}, count)

    def test_batches_conserve_inventory_and_bound_cost_and_parent_count(self):
        names = [f"Test{i:03d}" for i in range(100)] + ["TestOversized", "TestUnknown"]
        hints = {name: (i % 9) for i, name in enumerate(names[:100])}
        hints.update(TestOversized=201, TestStale=9999)
        batches = source_parent_batches(names, hints, max_seconds=30, max_parents=7)
        self.assertEqual(batches, source_parent_batches(reversed(names), hints, max_seconds=30, max_parents=7))
        self.assertCountEqual([name for batch in batches for name in batch.items], names)
        self.assertTrue(all(0 < len(batch.items) <= 7 for batch in batches))
        for batch in batches:
            if batch.duration_ns > 30_000_000_000:
                self.assertEqual(batch.items, ["TestOversized"])
        self.assertEqual(batches[0].items, ["TestOversized"])

    def test_cost_drift_does_not_pin_the_rest_of_a_large_lane(self):
        names = [f"Test{i:02d}" for i in range(16)]
        hints = dict.fromkeys(names, 30)
        actual = hints | {names[0]: 150}
        def finish(groups):
            workers = [0] * 4
            for group in groups:
                index = min(range(4), key=lambda i: (workers[i], i))
                workers[index] += sum(actual[name] for name in group.items)
            return max(workers)
        lanes = source_parent_shards(names, hints, 4)
        batches = source_parent_batches(names, hints, max_seconds=30)
        self.assertEqual(finish(lanes), 240)
        self.assertEqual(finish(batches), 150)

    def test_batch_inputs_fail_closed(self):
        for names, hints in [([], {}), (["TestA", "TestA"], {}), (["TestA"], {"TestA": math.nan})]:
            with self.subTest(names=names), self.assertRaises(ShardError):
                source_parent_batches(names, hints)
        for limit in [0, -1, math.inf, math.nan, True, "3"]:
            with self.subTest(limit=limit), self.assertRaises(ShardError):
                source_parent_batches(["TestA"], {}, max_seconds=limit)
        for count in [0, -1, True, 1.5]:
            with self.subTest(count=count), self.assertRaises(ShardError):
                source_parent_batches(["TestA"], {}, max_parents=count)


if __name__ == "__main__":
    unittest.main()
