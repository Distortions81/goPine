#!/usr/bin/env python3
"""Regression budgets for the PineTime BLE ELF, not a whole-program stack proof.

Read actual DWARF frame sizes and the linked task-stack table. Check selected
core paths with 2 KiB reserved for deeper calls, interrupts and runtime work.
Indirect calls/recursion prevent TinyGo from proving a complete upper bound;
hardware testing is still required. No firmware is modified or uploaded.
"""
import argparse
import json
from pathlib import Path
import re
import struct
import subprocess

ROOT = "runtime.run$1$gowrapper"
RUN = "main.run"
LOOP = "(*main.watchLoop).run"
TICK = "(*main.watchUI).tickTimers"
FLUSH = "(*main.settingsPersistence).flush"
SAVE = "(*github.com/Distortions81/goPine/internal/checkpoint.Journal).save"
RESERVE = 2048


def parse_symbols(text):
    symbols = {}
    for line in text.splitlines():
        match = re.fullmatch(r"([0-9a-fA-F]+) ([A-Za-z]) (.+)", line.strip())
        if match:
            symbols[match[3]] = int(match[1], 16)
    return symbols


def parse_frames(text):
    frames, address = {}, None
    for line in text.splitlines():
        match = re.search(r"\bFDE .*\bpc=([0-9a-fA-F]+)\.\.\.([0-9a-fA-F]+)", line)
        if match:
            address = int(match[1], 16)
            frames[address] = 0
        elif re.search(r"\bCIE\s*$", line):
            address = None
        elif address is not None:
            match = re.search(r"DW_CFA_def_cfa_offset: \+?(\d+)", line)
            if match and frames[address] is not None:
                frames[address] = max(frames[address], int(match[1]))
            # Fail closed for frames our simple SP-relative parser cannot bound.
            if re.search(r"DW_CFA_(def_cfa_expression|def_cfa_register|def_cfa_sf|def_cfa_offset_sf)", line):
                frames[address] = None
    return frames


def stack_table(text):
    data = bytearray()
    for line in text.splitlines():
        match = re.match(r"\s*[0-9a-fA-F]+\s+((?:[0-9a-fA-F]{8}\s+)+)", line)
        if match:
            data.extend(bytes.fromhex(match[1]))
    if len(data) != 4:
        raise ValueError("Expected one task-stack entry; new tasks require a budget review")
    return struct.unpack("<I", data)[0]


def audit(symbols, by_address, task_stack):
    def frame(name, required=True):
        if name not in symbols:
            if required:
                raise ValueError(f"Required function missing: {name}")
            return 0  # Inlined optional helpers are covered by their caller frame.
        size = by_address.get(symbols[name])
        if size is None:
            raise ValueError(f"Missing or unsupported DWARF frame: {name}")
        return size

    sizes = {name: frame(name) for name in (ROOT, RUN, LOOP, TICK, FLUSH, SAVE)}
    callbacks = [name for name in symbols if re.fullmatch(r"main\.(?:openApplication|run)\$\d+", name)]
    if not callbacks:
        raise ValueError("Application callbacks missing; review changed call paths")
    # Deliberately retain a callback frame even when the compiler tail-calls it.
    callback = max(frame(name) for name in callbacks)
    base = sizes[ROOT] + sizes[RUN]
    live = base + sizes[LOOP]
    snapshot = frame("(*main.watchUI).runtimeSnapshot", False)
    calendar = frame("main.calendarMillis", False)
    paths = {
        "initialization": base + frame("main.openApplication", False) + frame("main.buildClockTime"),
        "timer": live + sizes[TICK] + frame("(*main.watchUI).loadRuntime", False) + calendar,
        "settings_save": live + callback + sizes[FLUSH] + max(sizes[SAVE], snapshot + calendar),
        "input": live + frame("(*main.watchUI).handle", False) + callback,
    }
    heap = symbols["_heap_end"] - symbols["_heap_start"]
    errors = []
    if task_stack != 8192:
        errors.append("Task stack changed from 8192 bytes; review heap and stack budgets together")
    if heap < 40 * 1024:
        errors.append(f"Heap region shrank below 40 KiB: {heap} bytes")
    if sizes[RUN] > 256:
        errors.append(f"Long-lived main.run frame exceeds 256 bytes: {sizes[RUN]}")
    # These dependencies previously retained tens of KiB of unrelated code.
    # Keep the embedded formatting and image-checksum paths deliberately small.
    if any(name.startswith(("fmt.", "(*fmt.")) for name in symbols):
        errors.append("Generic fmt linked into firmware; use the integer UI/error helpers")
    if any("crypto/internal/fips140" in name for name in symbols):
        errors.append("FIPS module linked into firmware; use the image SHA-256 subset")
    for name, size in paths.items():
        if size + RESERVE > task_stack:
            errors.append(f"{name}: core frames {size} + reserve {RESERVE} > stack {task_stack}")
    return {
        "task_stack_bytes": task_stack,
        "ram_reserved_bytes": symbols["_heap_start"] - 0x20000000,
        "heap_region_bytes": heap,
        "display_strip_bytes": 2880,
        "deeper_call_reserve_bytes": RESERVE,
        "frames_bytes": sizes,
        "callback_frame_budget_bytes": callback,
        "core_path_frames_bytes": paths,
        "errors": errors,
        "limitation": "Selected-path regression budgets, not full call-graph bounds or measured free heap.",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("elf", type=Path)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()

    def output(tool, *options):
        return subprocess.check_output([tool, *options, str(args.elf)], text=True)

    symbols = parse_symbols(output("llvm-nm", "--numeric-sort"))
    frames = parse_frames(output("llvm-dwarfdump", "--debug-frame"))
    stack = stack_table(output("llvm-objdump", "-s", "-j", ".tinygo_stacksizes"))
    result = audit(symbols, frames, stack)
    result["elf"] = str(args.elf)
    if args.report:
        args.report.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
    return bool(result["errors"])


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"Resource check failed: {error}")
