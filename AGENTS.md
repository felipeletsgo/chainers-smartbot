# AGENTS.md

## Build & Run

```bash
# Run directly (no dependencies beyond stdlib)
go run .

# Build executable (version suffix recommended)
go build -o bot_v118.exe .

# Windows: check if running
tasklist | findstr bot

# Monitor logs (PowerShell)
Get-Content logs\smartbot_*.log -Wait -Tail 20
```

## Architecture Flow

```
main.go → runBot() with panic recovery & auto-restart (10 attempts)
  └─> bot_manager.Run() loop:
        SyncFarm → Collect → Craft/Claim → Merge → PoolAnalyze → Plant → Sleep
```

Handler responsibilities:
- `farming_handler.go` - BPM calculation, batch harvest, planting strategy
- `crafting_handler.go` - recipes, protection rules, queue processing  
- `pool_strategist.go` - critical window (last 45s), history-based threshold
- `api_client.go` - exponential backoff (1s/2s/4s/8s/30s max), CSRF & cookies

## Critical Constants

- `inventoryCacheTTL = 5s` (`bot_manager.go:45`) — cache duration; always `InvalidateInventoryCache()` after inventory-modifying operations (harvest/plant/craft/merge)
- Pool critical window = 45 seconds before cycle end (`pool_strategist.go:140`)
- `minProfitMargin = 10.0%` (`pool_strategist.go:48`) — deposit threshold multiplier vs historical average
- `maxRestartAttempts = 10` (`main.go:13`) — auto-restart limit after crashes

## Repo-Specific Conventions

- No external dependencies (`go.mod` only stdlib). No `go mod tidy` needed.
- API responses are `map[string]interface{}` — use `getInt/getFloat/getString` helpers from `utils.go` for safe access.
- All logs written to `logs/smartbot_YYYY-MM-DD.log` + `logs/heartbeat.txt` (updated each cycle).
- Config loading order: `config.txt` (required JWT) → `discord.txt` (optional webhook) → `bot_config.txt` (optional advanced settings).
- Advanced config keys (all in `bot_config.txt`): `UPDATE_INTERVAL` (seconds, default 600), `MIN_PROFIT_MARGIN` (%, default 10.0), `AUTO_DEPOSIT` (true/false), `MIN_RESERVE` (BP, default 0), `MIN_FOOD_STOCK` (default 2), `PREFER_HIGH_RARITY` (false), `DEBUG_MODE` (false), `MAX_RETRIES` (4).
- Build executables include version suffix (e.g., `bot_v118.exe`).

## Verification & Testing

No test framework present. Manual verification:
1. `go run .` with valid `config.txt` (JWT starting with `eyJ`)
2. Check first 3 cycles appear in logs; no PANIC/FATAL entries
3. Verify heartbeat timestamp updates every cycle

## Pre-commit Quick Check

- Build succeeds: `go build -o bot_v118.exe .`
- Start/stop gracefully (Ctrl+C) without panic

## Operational Gotchas

- Bot "stuck" = sleeping until next event. Check `logs/heartbeat.txt` to confirm liveness.
- Token expires → 401 fatal. Replace entire `config.txt` contents with new JWT from browser DevTools (Application → Cookies → `chainers.io` → `token`).
- Cache invalidation is manual. Features that modify inventory must call `bot.InvalidateInventoryCache()` immediately after the operation (harvest/plant/craft/merge).
- Pool check rate-limited: only checks within 30min of critical window or every `poolCheckInterval` (10min) otherwise (`pool_strategist.go:74-78`).
- Sleep minimums: harvest path → 1s; pool critical → 5s; default → 30s (`bot_manager.go:261-267`).
- Effective BPM = base seed BPM × plot multiplier (legendary 16x, epic 8x, rare 4x, uncommon 2x) (`farming_handler.go:414-440`).
- Animal food mapping: bird/chicken → `bird_food`; deer → `deer_food`; pig → `piggy_food`; cattle/cow → `cattle_food` (`farming_handler.go:509-518`).
- Main loop runs SyncFarm + CraftClaim in parallel via WaitGroup, everything else sequential (`bot_manager.go:136-146`).

