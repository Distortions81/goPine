#!/usr/bin/env python3
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("resources", Path(__file__).with_name("check-resources.py"))
resources = importlib.util.module_from_spec(spec)
spec.loader.exec_module(resources)


class ResourceChecks(unittest.TestCase):
    def fixture(self):
        names = [resources.ROOT, resources.RUN, resources.LOOP, resources.TICK,
                 resources.FLUSH, resources.SAVE, "main.buildClockTime",
                 "main.openApplication$3", "main.openApplication"]
        symbols = {name: 0x8000 + 32 * i for i, name in enumerate(names)}
        frames = dict(zip(symbols.values(), [48, 104, 2256, 1088, 1032, 1328, 752, 664, 1976]))
        symbols.update(_heap_start=0x20005AC0, _heap_end=0x20010000)
        return symbols, frames

    def test_budgets(self):
        symbols, frames = self.fixture()
        result = resources.audit(symbols, frames, 8192)
        self.assertEqual(result["errors"], [])
        self.assertEqual(result["core_path_frames_bytes"]["settings_save"], 5432)
        self.assertEqual(result["heap_region_bytes"], 42304)

    def test_old_nested_frame_fails(self):
        symbols, frames = self.fixture()
        frames[symbols[resources.RUN]] = 4272
        result = resources.audit(symbols, frames, 8192)
        self.assertTrue(any("main.run" in error for error in result["errors"]))
        self.assertTrue(any("settings_save" in error for error in result["errors"]))

    def test_stack_increase_is_not_a_silent_fix(self):
        symbols, frames = self.fixture()
        self.assertTrue(resources.audit(symbols, frames, 16384)["errors"])

    def test_heap_shrink_fails(self):
        symbols, frames = self.fixture()
        symbols["_heap_start"] += 4096
        self.assertTrue(resources.audit(symbols, frames, 8192)["errors"])

    def test_missing_debug_frame_fails_closed(self):
        symbols, frames = self.fixture()
        del frames[symbols[resources.LOOP]]
        with self.assertRaises(ValueError):
            resources.audit(symbols, frames, 8192)

    def test_dwarf_and_symbols(self):
        frames = resources.parse_frames("""
0000 CIE
  DW_CFA_def_cfa: SP +0
0010 FDE cie=0000 pc=00008020...00008040
  DW_CFA_def_cfa_offset: +36
  DW_CFA_def_cfa_offset: +104
  DW_CFA_def_cfa_offset: +0
0030 FDE cie=0000 pc=00008040...00008080
  DW_CFA_def_cfa_expression: unknown
0050 FDE cie=0000 pc=00008080...00008084
""")
        self.assertEqual(frames, {0x8020: 104, 0x8040: None, 0x8080: 0})
        self.assertEqual(resources.parse_symbols("00008020 t main.run\n20010000 A _heap_end\n"),
                         {"main.run": 0x8020, "_heap_end": 0x20010000})

    def test_stack_table_is_little_endian_and_single_task(self):
        self.assertEqual(resources.stack_table(" 56850 00200000                             . ..\n"), 8192)
        for dump in ("", " 56850 00200000 00200000    . .. . ..\n"):
            with self.assertRaises(ValueError):
                resources.stack_table(dump)


if __name__ == "__main__":
    unittest.main()
