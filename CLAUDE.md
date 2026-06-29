# CLAUDE.md

Guidance for working in this repo. Read alongside the Go engineering skill (this
project follows it: TDD, `testify/suite`, table-driven tests, `log/slog`,
conventional commits).

## What this is

`hrm` — a Go CLI that reads **live heart rate from a Whoop strap over Bluetooth LE**,
computes a stress level, shows it in a Bubble Tea TUI, and stores the time-series
locally so meetings/Jira can be correlated with stress.

## Hard constraints (don't relitigate)

- **BLE only, no Whoop API.** Whoop's REST API has *no* continuous heart rate — only
  daily data. Live BPM comes solely from the strap's BLE "Broadcast Heart Rate" mode
  (standard HR service `0x180D`, measurement char `0x2A37`). No OAuth anywhere.
- **macOS Bluetooth permission is a TCC issue, not a bug.** A CLI inherits Bluetooth
  permission from its terminal app; if denied, macOS kills it with `abort trap: 6`
  (no prompt). Fix is user-side: grant the terminal Bluetooth access or
  `tccutil reset Bluetooth`. See `make bluetooth-help` and the README. Embedding an
  Info.plist / app bundle does **not** help for terminal-run usage.
- **No git VCS stamping yet for binaries** — build with `-buildvcs=false` (baked into
  the Makefile). Harmless once history exists.

## Build / test / run

```bash
make build        # -> dist/hrm
make test         # go test ./...
make test-race    # race detector on pure packages only (see below)
make cover        # coverage profile + total
make cli-help     # build + print CLI help
```

`go test -race` **must exclude `internal/ble`**: it links macOS CoreBluetooth (cgo)
which aborts under the race detector's linker. That's why the pure HR-measurement
parser lives in `internal/heartrate` (no cgo) so it *can* be race-tested.

## Architecture

Domain logic is transport-free; cgo/UI/wiring stay at the edges.

```
cmd/hrm/            cobra commands: monitor, devices, calibrate, report, reset, offenders
internal/model/     shared types: Sample, Event, Zone (no deps)
internal/config/    data-dir resolution, Config load/save, stress tuning
internal/heartrate/ pure 0x2A37 parser (cgo-free, race-testable)
internal/ble/       scan/connect/subscribe via tinygo.org/x/bluetooth (cgo, macOS CoreBluetooth)
internal/stress/    HR-relative + trend model -> score/zone + debounced change events
internal/store/     JSONL persistence (append-only NDJSON)
internal/report/    single-day summary; owns tag-interval pairing (PairTags)
internal/insights/  cross-day analytics; `offenders` ranks people by meeting stress
internal/tui/        Bubble Tea dashboard + tag form (self-rendered sparkline/gauge, no chart lib)
```

The monitor pipeline (in `cmd/hrm/monitor.go`): `ble.Monitor` → channel → `stress.Engine`
→ `store` (persist sample + any change event) → channel of `tui.Frame` → TUI.

## Data model

Stored under `~/.heartrate-monitor/` (`HRM_DATA_DIR` / `--data-dir` override) as
**JSON Lines**, never a single growing array:

- `samples/YYYY-MM-DD.jsonl` — one `Sample` per line, partitioned by local date.
- `events.jsonl` — one file for all `Event`s (tags + stress changes).
- `config.json`, `hrm.log`.

**Tags are intervals.** An `EventTag` carries an `ID`, `Kind` (meeting/focus/…),
`Label` (title), optional `Person`/`Note`; a later `EventTagEnd` with the same `ID`
closes it. Unclosed tags are point markers. `report.PairTags` resolves them — reuse it,
don't reimplement pairing. `insights.Offenders` attributes each closed meeting to every
person listed and ranks by avg stress / HR / delta-vs-daily-baseline.

## Conventions specific to this repo

- Stress model is **HR-relative + trend** (heart-rate reserve + rate-of-change), tunable
  via `config.json`. Zone changes are debounced by `MinDwell` to avoid flapping.
- Keep new analytics in `internal/insights`; keep single-day rendering in `internal/report`.
- The TUI deliberately avoids a charting dependency — it renders its own Unicode sparkline
  and gauge. Don't add `ntcharts`/chart libs without reason.
- Resting-HR baseline comes from `hrm calibrate` (there's no API for it).

## Not yet built (roadmap)

Calendar (`.ics`/Google) auto-tagging, Jira correlation, HRV-based stress (RR-intervals
are captured opportunistically already), per-device provenance on samples.
