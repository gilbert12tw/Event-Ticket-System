#!/usr/bin/env python3
"""Generate the PH2-22 Redis reservation gate experiment figures.

Reads the two JSON summaries emitted by `cmd/reservation_experiment` (one per
mode) and produces four PNGs under the chosen output directory:

    latency_percentiles.png   bar chart of p50 / p95 / p99 / max latency, off vs on
    throughput.png            bar chart of RPS + wall-clock duration
    outcomes.png              stacked bar of confirmed / waitlisted / error counts
    latency_cdf.png           CDF of per-request latency, both modes overlaid

The plots are designed to read at a glance: matched x-axes, consistent
colors (off = muted red, on = teal), and labels on each bar.
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

OFF_COLOR = "#c0504d"
ON_COLOR = "#3a8d8d"


def load(path: Path) -> dict:
    with path.open() as f:
        return json.load(f)


def plot_latency_percentiles(off: dict, on: dict, out: Path) -> None:
    metrics = ["p50", "p95", "p99", "max"]
    off_vals = [off["latency_ms"][m] for m in metrics]
    on_vals = [on["latency_ms"][m] for m in metrics]
    fig, ax = plt.subplots(figsize=(8, 5))
    width = 0.35
    x = range(len(metrics))
    ax.bar([i - width / 2 for i in x], off_vals, width, color=OFF_COLOR, label="BOOKING_PREADMISSION=off")
    ax.bar([i + width / 2 for i in x], on_vals, width, color=ON_COLOR, label="BOOKING_PREADMISSION=on")
    for i, (off_v, on_v) in enumerate(zip(off_vals, on_vals)):
        ax.text(i - width / 2, off_v, f"{off_v:.0f}", ha="center", va="bottom", fontsize=9)
        ax.text(i + width / 2, on_v, f"{on_v:.0f}", ha="center", va="bottom", fontsize=9)
    ax.set_xticks(list(x))
    ax.set_xticklabels([m.upper() for m in metrics])
    ax.set_ylabel("Booking latency (ms)")
    ax.set_title(
        f"Booking burst latency — {off['vus']} concurrent attempts, capacity={off['capacity']}"
    )
    ax.legend()
    ax.grid(axis="y", linestyle=":", alpha=0.4)
    fig.tight_layout()
    fig.savefig(out / "latency_percentiles.png", dpi=140)
    plt.close(fig)


def plot_throughput(off: dict, on: dict, out: Path) -> None:
    fig, (ax_rps, ax_wall) = plt.subplots(1, 2, figsize=(10, 4.5))
    ax_rps.bar(["off", "on"], [off["rps"], on["rps"]], color=[OFF_COLOR, ON_COLOR])
    for i, v in enumerate([off["rps"], on["rps"]]):
        ax_rps.text(i, v, f"{v:.0f}", ha="center", va="bottom", fontsize=10)
    ax_rps.set_ylabel("Bookings completed per second")
    ax_rps.set_title("Throughput")
    ax_rps.grid(axis="y", linestyle=":", alpha=0.4)

    ax_wall.bar(["off", "on"], [off["wall_clock_ms"], on["wall_clock_ms"]], color=[OFF_COLOR, ON_COLOR])
    for i, v in enumerate([off["wall_clock_ms"], on["wall_clock_ms"]]):
        ax_wall.text(i, v, f"{v:.0f} ms", ha="center", va="bottom", fontsize=10)
    ax_wall.set_ylabel("Wall-clock duration of burst (ms)")
    ax_wall.set_title("Burst duration")
    ax_wall.grid(axis="y", linestyle=":", alpha=0.4)

    fig.suptitle(
        f"PH2-22 Redis gate — {off['vus']} concurrent attempts on a capacity={off['capacity']} event",
        fontsize=11,
    )
    fig.tight_layout()
    fig.savefig(out / "throughput.png", dpi=140)
    plt.close(fig)


def plot_outcomes(off: dict, on: dict, out: Path) -> None:
    labels = ["off", "on"]
    confirmed = [off["outcomes"]["confirmed"], on["outcomes"]["confirmed"]]
    waitlisted = [off["outcomes"]["waitlisted"], on["outcomes"]["waitlisted"]]
    error = [off["outcomes"]["error"], on["outcomes"]["error"]]
    fig, ax = plt.subplots(figsize=(7, 4.5))
    ax.bar(labels, confirmed, color="#4f8c4a", label="confirmed")
    ax.bar(labels, waitlisted, bottom=confirmed, color="#cbb451", label="waitlisted")
    bottoms = [c + w for c, w in zip(confirmed, waitlisted)]
    ax.bar(labels, error, bottom=bottoms, color="#a83030", label="error")
    for i, (c, w, e) in enumerate(zip(confirmed, waitlisted, error)):
        ax.text(i, c / 2, f"{c} confirmed", ha="center", va="center", color="white", fontsize=9)
        ax.text(i, c + w / 2, f"{w} waitlisted", ha="center", va="center", color="black", fontsize=9)
        if e:
            ax.text(i, c + w + e / 2, f"{e} error", ha="center", va="center", color="white", fontsize=9)
    ax.set_ylabel("Bookings")
    ax.set_title(
        f"Outcome composition — capacity={off['capacity']} ⇒ confirmed must equal capacity, no oversell"
    )
    ax.legend(loc="upper right")
    fig.tight_layout()
    fig.savefig(out / "outcomes.png", dpi=140)
    plt.close(fig)


def plot_latency_cdf(off: dict, on: dict, out: Path) -> None:
    def cdf(values: list[float]) -> tuple[list[float], list[float]]:
        s = sorted(values)
        n = len(s)
        return s, [(i + 1) / n for i in range(n)]

    fig, ax = plt.subplots(figsize=(8, 5))
    off_x, off_y = cdf(off["latency_ms"]["all_ms"])
    on_x, on_y = cdf(on["latency_ms"]["all_ms"])
    ax.plot(off_x, off_y, color=OFF_COLOR, linewidth=2, label="BOOKING_PREADMISSION=off")
    ax.plot(on_x, on_y, color=ON_COLOR, linewidth=2, label="BOOKING_PREADMISSION=on")
    ax.axhline(0.95, color="grey", linestyle=":", linewidth=1)
    ax.axhline(0.99, color="grey", linestyle=":", linewidth=1)
    ax.text(ax.get_xlim()[1] * 0.98, 0.955, "p95", color="grey", fontsize=8, ha="right")
    ax.text(ax.get_xlim()[1] * 0.98, 0.992, "p99", color="grey", fontsize=8, ha="right")
    ax.set_xlabel("Booking latency (ms)")
    ax.set_ylabel("Cumulative fraction of requests")
    ax.set_title(
        f"Latency CDF — {off['vus']} concurrent attempts, capacity={off['capacity']}"
    )
    ax.legend(loc="lower right")
    ax.grid(linestyle=":", alpha=0.4)
    fig.tight_layout()
    fig.savefig(out / "latency_cdf.png", dpi=140)
    plt.close(fig)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--off", required=True, type=Path, help="JSON summary for gate=off run")
    parser.add_argument("--on", required=True, type=Path, help="JSON summary for gate=on run")
    parser.add_argument("--out", required=True, type=Path, help="output directory for PNGs")
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)

    off = load(args.off)
    on = load(args.on)
    if off["mode"] != "off" or on["mode"] != "on":
        print(f"error: expected mode=off/on; got off={off['mode']!r} on={on['mode']!r}", file=sys.stderr)
        return 2

    plot_latency_percentiles(off, on, args.out)
    plot_throughput(off, on, args.out)
    plot_outcomes(off, on, args.out)
    plot_latency_cdf(off, on, args.out)
    print(f"wrote: {args.out}/latency_percentiles.png")
    print(f"wrote: {args.out}/throughput.png")
    print(f"wrote: {args.out}/outcomes.png")
    print(f"wrote: {args.out}/latency_cdf.png")
    return 0


if __name__ == "__main__":
    sys.exit(main())
