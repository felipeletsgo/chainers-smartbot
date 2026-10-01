package main

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

type SmartBot struct {
	token                string
	apiClient            *ChainersAPIClient
	farming              *FarmingHandler
	crafting             *CraftingHandler
	poolStrategy         *PoolStrategist
	missions             *MissionsHandler
	discord              *DiscordNotifier
	bedsStatus           map[string]map[string]interface{}
	priorityPlantTargets []string
	config               *Config
	logger               *Logger

	// CACHE INTELIGENTE - Invalida automaticamente
	cachedInventory    []map[string]interface{}
	inventoryCacheTime time.Time
	inventoryCacheTTL  time.Duration
}

func NewSmartBot() (*SmartBot, error) {
	return NewSmartBotWithLogger(nil)
}

func NewSmartBotWithLogger(logger *Logger) (*SmartBot, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	bot := &SmartBot{
		token:                cfg.Token,
		bedsStatus:           make(map[string]map[string]interface{}),
		priorityPlantTargets: make([]string, 0),
		inventoryCacheTTL:    5 * time.Second,
		logger:               logger,
		config:               cfg,
	}

	bot.apiClient = NewChainersAPIClient(cfg.Token, cfg.CsrfToken, cfg.RawCookie)
	bot.farming = NewFarmingHandler(bot)
	bot.crafting = NewCraftingHandler(bot)
	bot.poolStrategy = NewPoolStrategist(bot)
	bot.missions = NewMissionsHandler(bot)

	discord, err := NewDiscordNotifier(bot)
	if err == nil {
		bot.discord = discord
	}

	return bot, nil
}


func (b *SmartBot) Log(msg string) {
	if b.logger != nil {
		b.logger.Info(msg)
	} else {
		// Fallback para console se logger não disponível
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		fmt.Printf("%s - %s\n", timestamp, msg)
	}
}

func (b *SmartBot) RemovePriorityTarget(target string) {
	newTargets := make([]string, 0)
	for _, t := range b.priorityPlantTargets {
		if t != target {
			newTargets = append(newTargets, t)
		}
	}
	b.priorityPlantTargets = newTargets
}

// ============ CACHE INTELIGENTE DE INVENTÁRIO ============
func (b *SmartBot) GetCachedInventory() ([]map[string]interface{}, error) {
	if time.Since(b.inventoryCacheTime) < b.inventoryCacheTTL && b.cachedInventory != nil {
		return b.cachedInventory, nil
	}

	inventory, err := b.apiClient.GetUserInventory()
	if err != nil {
		return nil, err
	}

	b.cachedInventory = inventory
	b.inventoryCacheTime = time.Now()
	return inventory, nil
}

func (b *SmartBot) InvalidateInventoryCache() {
	b.cachedInventory = nil
	b.inventoryCacheTime = time.Time{}
}

func (b *SmartBot) Run() error {
	b.Log(">>> SmartBot Chainers V126 (Go Optimized + Recovery) Iniciado <<<")

	// INICIALIZAÇÃO (Uma única vez)
	if err := b.apiClient.InitSession(); err != nil {
		b.Log(fmt.Sprintf("⚠️ Aviso: Falha ao inicializar sessão CSRF: %v", err))
	}
	b.farming.AnalyzeSeedsBPM()
	b.farming.LoadBedDefinitions()
	b.crafting.LoadRecipesOnce()

	for {
		// Atualiza heartbeat no início de cada ciclo
		if b.logger != nil {
			b.logger.updateHeartbeat()
		}
		cycleStart := time.Now()

		// === FASE 1: SINCRONIZAÇÃO + FASE 3: CRAFTING CLAIM (PARALELO) ===
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			b.farming.SyncFarmState()
			wg.Done()
		}()
		go func() {
			b.crafting.CheckAndClaimCrafts()
			wg.Done()
		}()
		wg.Wait()

		// Estabilização pós-claims
		time.Sleep(200 * time.Millisecond)

		// === FASE 2: COLETA ===
		collected := b.farming.CollectHarvestOnly()
		if collected {
			// CRÍTICO: Inventário mudou, invalida cache
			b.InvalidateInventoryCache()
		}

		// === FASE 3 (continuação): CRAFTING ANÁLISE E MERGE ===
		crafted := b.crafting.AnalyzeNeedsAndCraft()
		if crafted {
			b.InvalidateInventoryCache()
		}

		merged := b.crafting.ProcessSeedMerges()
		if merged {
			b.InvalidateInventoryCache()
		}

		// === FASE 4: POOL STRATEGY (CONDICIONAL) ===
		poolCriticalEvent := b.poolStrategy.AnalyzeAndActConditional()

		// === FASE 5: PLANTIO ===
		planted := b.farming.PlantPriorityOrBPM()
		if planted {
			b.InvalidateInventoryCache()
		}

		// === FASE 6: SINCRONIZAÇÃO FINAL (APENAS SE NECESSÁRIO) ===
		// Atualiza estado da fazenda APÓS plantio/coleta para calcular sono correto
		if planted {
			b.farming.SyncFarmState()
		}

		// === FASE 7: RELATÓRIOS ===
		reportText, totalBPM, dailyYield := b.farming.GetFormattedReportText()

		// Busca inventário FINAL
		inventory, _ := b.GetCachedInventory()
		inventoryVal := b.farming.CalculateInventoryBPMValue(inventory)

		poolData := b.poolStrategy.GetLatestReport()
		estCFB := inventoryVal * poolData.RatioNow

		// === CÁLCULO DE SONO (COM DADOS ATUALIZADOS) ===
		curr := float64(time.Now().Unix())
		nextHarvest := b.farming.GetNextHarvestTime()
		nextPool := b.poolStrategy.GetNextCriticalTime()

		// DEBUG: Verifica se há itens prontos
		readyCount := 0
		for _, bed := range b.bedsStatus {
			if getBool(bed, "is_ready") && bed["farming_id"] != nil {
				readyCount++
			}
		}

		targets := make([]float64, 0)
		targetReasons := make([]string, 0) // DEBUG

		if readyCount > 0 {
			// CRÍTICO: Se tem item pronto, acorda em 1s (MAX BPM)
			b.Log(fmt.Sprintf("⚠️ ATENÇÃO: %d itens prontos! Acordando em 1s...", readyCount))
			targets = append(targets, curr+1)
			targetReasons = append(targetReasons, "items_ready")
		} else {
			// SEMPRE adiciona nextHarvest como target
			// Se nextHarvest <= curr, significa que colheita já passou ou é AGORA
			if nextHarvest > curr {
				targets = append(targets, nextHarvest)
				targetReasons = append(targetReasons, fmt.Sprintf("harvest_%.0fm", (nextHarvest-curr)/60))
			} else {
				// nextHarvest <= curr: colheita passou ou é AGORA, acorda em 1s
				targets = append(targets, curr+1)
				targetReasons = append(targetReasons, "harvest_now")
				b.Log(fmt.Sprintf("🔔 Colheita imediata detectada (nextHarvest=%.0f <= curr=%.0f)", nextHarvest, curr))
			}
		}

		// CRÍTICO: Verifica janela da pool
		poolCriticalEnd := nextPool + 45 // O momento exato em que o bloco fecha
		if poolCriticalEnd > curr {
			poolTime := nextPool - 45 // Acorda 45s ANTES de nextPool (ou seja, 90s antes de fechar)
			if poolTime > curr {
				targets = append(targets, poolTime)
				targetReasons = append(targetReasons, fmt.Sprintf("pool_%.0fm", (poolTime-curr)/60))
			} else {
				// Pool em janela crítica AGORA (faltam 90s ou menos)
				targets = append(targets, curr+10)
				targetReasons = append(targetReasons, "pool_critical")
			}
		}

		if len(targets) == 0 {
			targets = append(targets, curr+600)
			targetReasons = append(targetReasons, "default_10m")
		}

		// Escolhe o menor tempo (mais próximo)
		target := minFloat(targets)
		var chosenReason string
		minIdx := 0
		for i, t := range targets {
			if t == target {
				minIdx = i
				break
			}
		}
		chosenReason = targetReasons[minIdx]

		var sleepSec float64
		// Delay mínimo de 1s para colheitas (MAX BPM)
		if strings.HasPrefix(chosenReason, "harvest") {
			sleepSec = math.Max(1, target-curr)
		} else if chosenReason == "pool_critical" {
			sleepSec = math.Max(5, target-curr)
		} else {
			sleepSec = math.Max(30, target-curr)
		}

		wakeTime := time.Unix(int64(curr+sleepSec), 0).Format("15:04:05")

		// Log de performance
		cycleTime := time.Since(cycleStart).Seconds()

		// NOVO: Mostra próximas 3 colheitas
		nextHarvests := b.farming.GetNextHarvestsPreview(3)
		harvestPreview := ""
		if len(nextHarvests) > 0 {
			for i, h := range nextHarvests {
				mins := int((h.time - curr) / 60)
				if mins < 0 {
					mins = 0
				}
				if i > 0 {
					harvestPreview += ", "
				}
				harvestPreview += fmt.Sprintf("%dm", mins)
			}
		} else {
			harvestPreview = "nenhuma"
		}

		// DEBUG: Informações completas
		poolTimeRemain := int((nextPool - curr) / 60)

		b.Log(fmt.Sprintf("💰 BPM: %.1f | Próximas: %s | Pool: %dm | Motivo: %s | Ciclo: %.1fs | Até %s",
			totalBPM, harvestPreview, poolTimeRemain, chosenReason, cycleTime, wakeTime))
		b.Log(b.apiClient.GetStats())

		// Discord
		if b.discord != nil && b.discord.ShouldSend(poolCriticalEvent) {
			// Verifica missões APENAS se o Discord for enviar agora
			hasMissionsAvailable := b.missions.CheckMissionsAvailable(true)
			
			stats := map[string]interface{}{
				"planted_list":   reportText,
				"total_bpm":      fmt.Sprintf("%.1f", totalBPM),
				"daily_bpm":      formatNumber(dailyYield),
				"inventory_bp":   formatNumber(inventoryVal),
				"inventory_cfb":  fmt.Sprintf("%.2f", estCFB),
				"pool_data":      poolData,
				"next_harvests":  harvestPreview,
				"pool_time":      fmt.Sprintf("%dm", poolTimeRemain),
				"cycle_time":     fmt.Sprintf("%.1fs", cycleTime),
				"has_missions":   hasMissionsAvailable,
				"request_count":  b.apiClient.RequestCount,
				"error_count":    b.apiClient.ErrorCount,
				"sleep_reason":   chosenReason,
				"wake_time":      wakeTime,
			}
			b.discord.SendStatusUpdate(stats, poolCriticalEvent)
		}

		time.Sleep(time.Duration(sleepSec) * time.Second)
	}
}

func minFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

