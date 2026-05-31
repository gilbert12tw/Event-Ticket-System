#!/usr/bin/env python3
"""Guard Phase 3 demo-ha operations to the scheduled Asia/Taipei windows."""

from __future__ import annotations

import argparse
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from zoneinfo import ZoneInfo


TAIPEI = ZoneInfo("Asia/Taipei")


@dataclass(frozen=True)
class Window:
    name: str
    start: datetime
    end: datetime


WINDOWS = [
    Window(
        "Demo 1 deploy/test",
        datetime(2026, 6, 3, 0, 0, tzinfo=TAIPEI),
        datetime(2026, 6, 4, 0, 0, tzinfo=TAIPEI),
    ),
    Window(
        "Demo 1",
        datetime(2026, 6, 4, 19, 0, tzinfo=TAIPEI),
        datetime(2026, 6, 4, 21, 0, tzinfo=TAIPEI),
    ),
    Window(
        "Demo 2 deploy/test",
        datetime(2026, 6, 10, 0, 0, tzinfo=TAIPEI),
        datetime(2026, 6, 11, 0, 0, tzinfo=TAIPEI),
    ),
    Window(
        "Demo 2",
        datetime(2026, 6, 11, 19, 0, tzinfo=TAIPEI),
        datetime(2026, 6, 11, 21, 0, tzinfo=TAIPEI),
    ),
]


def parse_now(value: str | None) -> datetime:
    if value:
        parsed = datetime.fromisoformat(value)
        if parsed.tzinfo is None:
            return parsed.replace(tzinfo=TAIPEI)
        return parsed.astimezone(TAIPEI)
    return datetime.now(timezone.utc).astimezone(TAIPEI)


def format_windows() -> str:
    return ", ".join(
        f"{window.name}: {window.start.isoformat()} to {window.end.isoformat()}"
        for window in WINDOWS
    )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Refuse demo-ha actions outside the scheduled Asia/Taipei windows."
    )
    parser.add_argument(
        "--mode",
        required=True,
        choices=["dev-single-az", "demo-ha", "post-demo"],
        help="Requested Phase 3 deployment mode.",
    )
    parser.add_argument(
        "--now-taipei",
        default=None,
        help="Override current time for tests, for example 2026-06-03T10:00:00.",
    )
    parser.add_argument(
        "--allow-outside-window",
        action="store_true",
        help="Allow demo-ha outside scheduled windows for an explicit rehearsal.",
    )
    args = parser.parse_args()

    if args.mode != "demo-ha":
        print(f"not-required|{args.mode}")
        return 0

    now = parse_now(args.now_taipei)
    for window in WINDOWS:
        if window.start <= now < window.end:
            print(f"inside|{window.name}|{now.isoformat()}")
            return 0

    windows = format_windows()
    if args.allow_outside_window:
        print(f"override||{now.isoformat()}|{windows}")
        return 0

    print(f"outside||{now.isoformat()}|{windows}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
