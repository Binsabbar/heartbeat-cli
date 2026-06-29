# hrm — Live Heart-Rate & Stress Monitor

A Go CLI that reads your heart rate **live from any standard Bluetooth LE heart-rate
monitor**, shows it in an interactive terminal dashboard, computes a **stress level**,
flags when your stress **changes zone**, and stores the time-series locally so you can
correlate spikes with meetings and Jira tickets — to learn what triggers you at work.

It works with any device exposing the standard BLE Heart Rate service (`0x180D`,
characteristic `0x2A37`) — a chest strap, an optical arm band, or a WHOOP strap put into
its **Broadcast Heart Rate** mode.

## Why Bluetooth (and not a vendor API)

Many wearable cloud APIs only expose **daily** aggregates (recovery, HRV, resting HR) and
have **no continuous/intraday heart rate**. Bluetooth LE is the vendor-neutral way to get
live BPM: any compliant strap advertises the standard Heart Rate service, and this tool
reads it directly — no account, no cloud, no API keys.

## Prerequisites

1. **Put your strap in broadcast/HR-monitor mode** so it advertises the BLE Heart Rate
   service. (On a WHOOP strap this is the in-app *Broadcast Heart Rate* toggle.) Most
   straps broadcast to **one** listener at a time, so disconnect other apps first.
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
hrm devices --save <name|id> # pin your strap to config (e.g. --save WHOOP)
hrm calibrate                # sit still ~2 min to learn your resting HR
hrm monitor                  # live dashboard (default)
hrm monitor --print          # stream readings to stdout (debug, no TUI)
hrm report                   # summarise today; --date YYYY-MM-DD for another day
hrm offenders                # rank people whose meetings raise your HR/stress
hrm reset                    # delete recorded data (--all also clears config; -f skips prompt)
```

### Finding your triggers — `hrm offenders`

Ranks the people whose tagged meetings coincide with your highest heart rate and stress,
attributing each meeting to every person listed on it:

```bash
hrm offenders                       # by avg stress, all time
hrm offenders --by delta            # by stress elevation vs your daily baseline
hrm offenders --by hr               # by avg heart rate
hrm offenders --days 30 --limit 10  # last 30 days, top 10
hrm offenders --kind focus          # analyse a different tag kind
```

```
#  PERSON    MEETINGS  TIME   AVG HR  AVG STRESS  PEAK  Δ VS DAY
1  Mohammed  3         1h30m  109     64          82    +14
2  Saleh     1         30m    95      40          40    -19
```

`Δ VS DAY` is how far a person's meetings sit above your average stress that day — a better
"this person specifically winds me up" signal than raw average (which can just reflect a busy
day). Only **closed** meetings with a named person are counted. Aliases: `triggers`, `people`.

In the dashboard:
- **`t`** open a tag. A small form captures **kind** (meeting / focus / break / interrupt),
  **title**, optional **person**, and optional **note** (Tab/Enter moves between fields, Esc
  cancels). The tag stays *open* as an interval.
- **`e`** close an open tag, recording its end time and duration. If several are open, press
  the listed number to pick one. A tag you never close stays a point marker.
- **`q`** quit.

`hrm report` pairs each tag with its close to show durations (e.g.
`09:30–10:02 (32m) [meeting] Sprint planning · Sarah · JIRA-451`), so you can line meetings
up against your stress timeline.

Global flags: `--data-dir`, `--device`, `--resting-hr`, `--max-hr`.

## Stress model (HR-relative + trend)

The BLE Heart Rate profile carries no "stress" metric, so `hrm` computes its own,
personalised to your heart rate:

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
events.jsonl                 # tags (open/close intervals) and stress changes
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

## Disclaimer

This is an independent, unofficial tool. It is **not affiliated with, endorsed by, or
sponsored by WHOOP, Inc.** or any other wearable vendor. It uses only the open, standard
Bluetooth LE Heart Rate profile and stores all data locally on your machine. Product names
are trademarks of their respective owners and are used only to describe compatibility.

Not a medical device — the heart-rate and stress figures are informational only and must
not be used for any medical or diagnostic purpose.
