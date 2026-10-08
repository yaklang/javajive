"""The production source scheduler conserves tests even with stale cost hints."""

import math
import unittest

from tools.ci_scheduler.shard import ShardError, source_parent_shards


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


if __name__ == "__main__":
    unittest.main()
