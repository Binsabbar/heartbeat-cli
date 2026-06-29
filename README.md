# hrm — Live Whoop Heart-Rate & Stress Monitor

A Go CLI that reads your heart rate **live from your Whoop strap over Bluetooth LE**,
shows it in an interactive terminal dashboard, computes a **stress level**, flags when
your stress **changes zone**, and stores the time-series locally so you can correlate
spikes with meetings and Jira tickets — to learn what triggers you at work.

## Why Bluetooth (and not the Whoop API)

Whoop's official REST API only exposes **daily** data (recovery, HRV, resting HR, day
strain) — it has **no continuous/intraday heart rate**. The only way to get live BPM is
the strap's **"Broadcast Heart Rate"** feature, which makes it advertise as a standard
BLE Heart Rate Monitor (service `0x180D`, characteristic `0x2A37`). This tool reads that.

## Prerequisites

1. **Enable broadcast** on the strap: Whoop app → your device → **Broadcast Heart Rate**.
   The strap broadcasts to **one** listener at a time, so disconnect other apps.
2. **macOS Bluetooth permission (important).** A command-line tool inherits Bluetooth
   permission from the **terminal app** it runs in. Grant your terminal (Warp / iTerm /
   Terminal) access under **System Settings → Privacy & Security → Bluetooth**, then re-run.

   If `hrm devices` dies instantly with **`abort trap: 6`**, the terminal hasn't been
   granted (macOS aborts instead of prompting). Fix:
   - If your terminal **is** in that Bluetooth list → enable it and re-run.
   - If it is **not** listed (nothing to toggle), force a fresh prompt:
     ```bash
     tccutil reset Bluetooth      # resets the Bluetooth privacy list for all apps
     hrm devices                  # now click "Allow" on the prompt
     ```
   - If your terminal still never prompts, run `hrm` once from Apple's **Terminal.app** or
     **iTerm2**, grant the prompt, then use any granted terminal. (`make bluetooth-help`
     prints these steps.)

## Install / build

```bash
go build -o hrm ./cmd/hrm    # add -buildvcs=false if building before the first git commit
```

## Usage

```bash
hrm devices                  # scan for nearby HR devices
hrm devices --save WHOOP     # pin your strap to config (match by name or id)
hrm calibrate                # sit still ~2 min to learn your resting HR
hrm monitor                  # live dashboard (default)
hrm monitor --print          # stream readings to stdout (debug, no TUI)
hrm report                   # summarise today; --date YYYY-MM-DD for another day
```

In the dashboard: **`t`** tag the current moment (e.g. `standup`, `JIRA-123`), **`s`**
start/stop a named session, **`q`** quit.

Global flags: `--data-dir`, `--device`, `--resting-hr`, `--max-hr`.

## Stress model (HR-relative + trend)

There is no Whoop "stress" metric in any feed, so `hrm` computes its own, personalised to
your heart rate:

```
ewma  = exponentially-weighted moving average of raw BPM   (suppresses jitter)
pHRR  = clamp((ewma - restingHR) / (maxHR - restingHR), 0, 1)   # heart-rate reserve
slope = max(0, d(ewma)/dt) normalised by SlopeFullBpmPerSec      # acute rise
score = clamp(100 * (HRRWeight*pHRR + SlopeWeight*slope), 0, 100)
```

The score maps to zones **calm / mild / elevated / high**. A new zone must persist for
`MinDwell` (default 15s) before a **stress-change event** fires, so it doesn't flap on
noise. All weights, thresholds, and `MinDwell` live in `config.json` and are tunable.

Set your baseline with `hrm calibrate` (or `--resting-hr` / `--max-hr`). `maxHR` defaults
to a generic 190; for best accuracy set it from a known max or `220 − age`.

## Data layout

Stored under `~/.heartrate-monitor/` (override with `--data-dir` or `$HRM_DATA_DIR`) as
**JSON Lines** (NDJSON) — append-only, crash-resilient, and streamable, which a single
growing JSON array is not:

```
config.json                  # baselines, saved device, stress tuning
samples/2026-06-29.jsonl     # one HR sample per line, partitioned by local date
events.jsonl                 # tags, session boundaries, stress changes
hrm.log                      # app log
```

## Development

```bash
go test ./...                # full suite (testify suites, table-driven)
go test -race ./internal/stress/ ./internal/store/ ./internal/report/ ./internal/heartrate/
```

Note: the `internal/ble` package links macOS CoreBluetooth (cgo); its parsing logic lives
in the dependency-free `internal/heartrate` package so it can be tested under `-race`
without linking the platform Bluetooth stack.

## Roadmap

- Auto-correlate with Google Calendar / `.ics` meetings.
- Pull Jira activity (issues touched, status changes) around stress spikes.
- HRV-based stress (RR-intervals are already captured opportunistically when broadcast).
```
