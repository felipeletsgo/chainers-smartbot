# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```powershell
# Run directly (no external dependencies — pure stdlib)
go run .

# Build executable with version suffix
go build -o bot_v126.exe .

# Monitor logs in real-time (PowerShell)
Get-Content logs\smartbot_*.log -Wait -Tail 20

# Check if bot is running
tasklist | findstr bot

# Build check (no test framework — manual verification only)
go build -o bot_v126.exe .
```

## High-Level Architecture

Automation bot for Chainers.io (blockchain farming game). Maximizes Biopoints (BP) per minute via farming, crafting, and reward pool strategies.

### Core Components

**main.go** - Entry point with panic recovery and auto-restart
- `runBot()` loop up to `maxRestartAttempts` (10), restart delay scales with attempt count
- Graceful shutdown on Ctrl+C via signal channel

**bot_manager.go** - Central orchestration (`SmartBot` struct)
- Main loop in `Run()` — Phase 1 runs `SyncFarmState` + `CheckAndClaimCrafts` in parallel via `sync.WaitGroup`
- Inventory cache with `inventoryCacheTTL = 5s` (line 45); always call `InvalidateInventoryCache()` after harvest/plant/craft/merge
- Heartbeat updated at top of each cycle

**api_client.go** - HTTP client for Chainers.io API
- Exponential backoff retry (1s, 2s, 4s, 8s, max 30s)
- CSRF Double Submit pattern + cookie jar session management
- All endpoints return `map[string]interface{}` — use `getInt/getFloat/getString/getBool` helpers from `utils.go`

**farming_handler.go** - BPM optimization and planting logic
- BPM = `weight / growthMins`; highest BPM seeds matched to highest multiplier plots
- Plot multipliers: legendary 16×, epic 8×, rare 4×, uncommon 2×
- Animal food mapping: bird/chicken → `bird_food`; deer → `deer_food`; pig → `piggy_food`; cattle/cow → `cattle_food`
- Batch harvesting: collect all, then single delay

**pool_strategist.go** - Reward pool "sniping"
- Critical window = last 45s before pool closes; checks within 30min of critical or every 10min otherwise
- Deposits when `ratio >= historical_average * (1 + margin)`; history stored in `cfb_history.json`
- `minProfitMargin = 10.0` (line 48)

**crafting_handler.go** - Recipe crafting and seed merging
- Recipes loaded once at startup; protects important inventory items

**missions_handler.go** - Claimable mission detection
- Checks `GetUserEventsStatus` every 30min; result surfaced in Discord updates

**config_manager.go** - Configuration loading
- Load order: `config.txt` (required JWT) → `discord.txt` (optional webhook) → `bot_config.txt` (optional advanced settings)
- Auto-updates `x-csrf` in `bot_config.txt` when cookie jar refreshes it

**discord_notifier.go** - Discord webhook status updates
- Rate-limited by `UPDATE_INTERVAL`; forced send on pool critical events

**logger.go** - File-based logging
- Daily log at `logs/smartbot_YYYY-MM-DD.log`; heartbeat at `logs/heartbeat.txt`

### Data Flow

```
main.go → runBot() [panic recovery, auto-restart]
  └─> SmartBot.Run() [main loop]
        ├─ PARALLEL: SyncFarmState() + CheckAndClaimCrafts()
        ├─ CollectHarvestOnly()         → InvalidateInventoryCache if collected
        ├─ AnalyzeNeedsAndCraft()       → InvalidateInventoryCache if crafted
        ├─ ProcessSeedMerges()          → InvalidateInventoryCache if merged
        ├─ AnalyzeAndActConditional()   [pool]
        ├─ PlantPriorityOrBPM()         → InvalidateInventoryCache if planted
        ├─ SyncFarmState()              [only if collected or planted]
        └─ Calculate sleep until next event → send Discord update
```

### Sleep Calculation

- Items ready now → 1s
- Next harvest → max(1s, time_until_harvest)
- Pool critical window → max(5s, time_until_window)
- Default → max(30s, next_target) — capped at 10min if no events

### Configuration Files

| File | Required | Purpose |
|------|----------|---------|
| `config.txt` | Yes | JWT token (must contain `eyJ`) |
| `discord.txt` | No | Discord webhook URL |
| `bot_config.txt` | No | Advanced settings (KEY=VALUE, `#` comments) |
| `cfb_history.json` | Auto-created | Pool ratio history |

**`bot_config.txt` keys** (all optional, defaults shown):
```
UPDATE_INTERVAL=600       # Discord update interval (seconds)
MIN_PROFIT_MARGIN=10.0    # Pool deposit threshold (%)
AUTO_DEPOSIT=true
MIN_RESERVE=0             # Minimum BP to keep in inventory
MIN_FOOD_STOCK=2
PREFER_HIGH_RARITY=false
DEBUG_MODE=false
MAX_RETRIES=4
CSRF_TOKEN=...            # Auto-updated by bot
RAW_COOKIE=...            # Auto-updated by bot (x-csrf refreshed automatically)
```

### Important Constants

| File | Constant | Value | Purpose |
|------|----------|-------|---------|
| `main.go:13` | `maxRestartAttempts` | 10 | Max auto-restarts after crash |
| `main.go:14` | `restartDelay` | 5s | Base delay between restarts (scales with count) |
| `bot_manager.go:45` | `inventoryCacheTTL` | 5s | Inventory cache duration |
| `pool_strategist.go:48` | `minProfitMargin` | 10.0 | Min profit margin over historical average |

### Version Management

1. Update version banner in `main.go` (line ~20)
2. Update version string in `bot_manager.go` `Run()` log message
3. Build: `go build -o bot_vNNN.exe .`
4. Update `README.md` "What's New" and `CHANGELOG.md`

### Error Handling Philosophy

- **API errors**: Retry with exponential backoff, never stop
- **401 unauthorized**: Fatal (token expired — replace `config.txt` with new JWT from browser DevTools → Application → Cookies → `accessToken`)
- **429 rate limit**: Wait `5s * (attempt+1)`, retry
- **Panic**: Log stack trace, trigger auto-restart

### Modifying Farming Logic

Key methods in `farming_handler.go`:
- `AnalyzeSeedsBPM()` — builds BPM table
- `PlantPriorityOrBPM()` — main planting logic and seed-to-plot matching
- `CollectHarvestOnly()` — batch harvesting

### Modifying Pool Strategy

Key methods in `pool_strategist.go`:
- `AnalyzeAndActConditional()` — when to check pool
- `ShouldDeposit()` — deposit decision vs historical average
- `LoadCfbHistory()` / `SaveCfbHistory()` — ratio history persistence

### Operational Notes

- **Bot appears stuck** — Normal. Bot sleeps until next event. Check `logs/heartbeat.txt` for last activity timestamp.
- **Token expired** — 401 fatal. Replace `config.txt` with new JWT.
- **High error rate** — Check logs for `ERROR`/`PANIC` entries; API instability is expected.
- **Faster dev cycle** — Reduce `inventoryCacheTTL` in `bot_manager.go:45`.

