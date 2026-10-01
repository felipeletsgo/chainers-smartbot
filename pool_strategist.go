package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type PoolReport struct {
	PoolSize      int     `json:"pool_size"`
	RatioNow      float64 `json:"ratio_now"`
	RatioAvg      float64 `json:"ratio_avg"`
	TimeRemaining string  `json:"time_remaining"`
	Status        string  `json:"status"`
	LastAction    string  `json:"last_action"`
}

type HistoryEntry struct {
	Timestamp int64   `json:"timestamp"`
	BlockID   string  `json:"block_id"`
	Ratio     float64 `json:"ratio"`
	TotalBP   float64 `json:"total_bp"`
	Type      string  `json:"type"`
}

type PoolStrategist struct {
	bot              *SmartBot
	apiClient        *ChainersAPIClient
	historyFile      string
	minReserve       int
	autoDeposit      bool
	minProfitMargin  float64
	nextCriticalTime float64
	latestReport     PoolReport
	lastPoolCheck    time.Time
	poolCheckInterval time.Duration
	currentBlockID   string // rastreia bloco ativo para resetar LastAction a cada ciclo
}

func NewPoolStrategist(bot *SmartBot) *PoolStrategist {
	return &PoolStrategist{
		bot:              bot,
		apiClient:        bot.apiClient,
		historyFile:      "cfb_history.json",
		minReserve:       0,
		autoDeposit:      bot.config.AutoDeposit,
		minProfitMargin:  bot.config.MinProfitMargin,
		nextCriticalTime: 0,
		poolCheckInterval: 10 * time.Minute, // Checa no máximo a cada 10min
		currentBlockID:   "",
		latestReport: PoolReport{
			PoolSize:      0,
			RatioNow:      0,
			RatioAvg:      0,
			TimeRemaining: "--",
			Status:        "Iniciando",
			LastAction:    "Monitorando",
		},
	}
}

// NOVA FUNÇÃO: Verifica pool condicionalmente
func (p *PoolStrategist) AnalyzeAndActConditional() bool {
	nowUTC := time.Now().UTC()
	
	// OTIMIZAÇÃO: Só checa se:
	// 1. Faltam menos de 30 minutos para janela crítica
	// 2. OU passou mais de 10 minutos desde último check
	timeToCritical := p.nextCriticalTime - float64(nowUTC.Unix())
	timeSinceLastCheck := nowUTC.Sub(p.lastPoolCheck)
	
	shouldCheck := false
	
	if timeToCritical > -60 && timeToCritical < 1800 { // Faltam menos de 30 minutos, E não passou mais de 1 minuto do fechamento
		shouldCheck = true
	} else if timeSinceLastCheck > p.poolCheckInterval {
		shouldCheck = true
	}
	
	if !shouldCheck {
		// Calcula tempo visual com base no cache para não congelar o log
		remainCached := (p.nextCriticalTime + 45) - float64(nowUTC.Unix())
		if remainCached < 0 {
			remainCached = 0
		}
		hours := int(remainCached / 3600)
		minutes := int((int(remainCached) % 3600) / 60)

		p.bot.Log(fmt.Sprintf("🔄 Pool Cache: Ratio %.6f | Faltam: %dh %02dm (não consultou API)", 
			p.latestReport.RatioNow, hours, minutes))
		return false
	}
	
	// Atualiza timestamp do check
	p.lastPoolCheck = nowUTC
	
	// Executa análise normal
	return p.AnalyzeAndAct()
}

func (p *PoolStrategist) AnalyzeAndAct() bool {
	shouldNotifyDiscord := false
	
	poolList, err := p.apiClient.GetPoolStatus()
	if err != nil || len(poolList) == 0 {
		p.bot.Log("🔍 Pool CFB não localizada na API.")
		return false
	}
	
	var active map[string]interface{}
	for _, pool := range poolList {
		currency := getString(pool, "currency")
		rewardsCode := getString(pool, "rewardsPoolCode")
		if strings.Contains(currency, "CFB") || strings.Contains(rewardsCode, "CFB") {
			active = pool
			break
		}
	}
	
	if active == nil {
		p.bot.Log("🔍 Nenhum bloco ativo de CFB encontrado na API.")
		return false
	}
	
	blockID := getString(active, "id")
	if blockID == "" {
		blockID = getString(active, "_id")
	}

	// Reset LastAction quando um novo bloco de pool é detectado
	if blockID != "" && blockID != p.currentBlockID {
		p.currentBlockID = blockID
		p.latestReport.LastAction = "Monitorando"
	}
	
	endDateStr := getString(active, "endDate")
	endDateStr = strings.Replace(endDateStr, "Z", "+00:00", -1)
	
	endDt, err := time.Parse(time.RFC3339, endDateStr)
	if err != nil {
		endDt = time.Now().UTC().Add(1 * time.Hour)
	}

	nowUTC := time.Now().UTC()
	remain := endDt.Sub(nowUTC).Seconds()

	p.nextCriticalTime = float64(endDt.Unix()) - 45

	rewardPool := getFloat(active, "blockPayoutAmount") / 1_000_000_000

	var totalBP float64
	candidateKeys := []string{
		"totalVegetablesWeight",
		"totalWeight",
		"totalDepositedWeight",
		"depositedWeight",
		"totalBP",
		"totalVegetables",
		"farmWeight",
		"weight",
	}
	for _, key := range candidateKeys {
		val := getFloat(active, key)
		if val > 0 {
			totalBP = val
			p.bot.Log(fmt.Sprintf("✅ Pool size field found: '%s' = %.0f", key, totalBP))
			break
		}
	}
	if totalBP == 0 {
		for k, v := range active {
			if f, ok := v.(float64); ok && f > 1000 {
				totalBP = f
				p.bot.Log(fmt.Sprintf("⚠️ Fallback: using field '%s' = %.0f as totalBP", k, totalBP))
				break
			}
		}
	}
	if totalBP == 0 {
		p.bot.Log("❌ totalBP remains 0 — pool size will be inaccurate")
		totalBP = 1.0
	}

	ratio := rewardPool / totalBP
	avg := p.getHistoricalAverage()
	
	p.latestReport.PoolSize = int(totalBP)
	p.latestReport.RatioNow = ratio
	p.latestReport.RatioAvg = avg
	
	if remain > 0 {
		p.latestReport.Status = "Aberta"
	} else {
		p.latestReport.Status = "Fechada"
	}
	
	hours := int(remain / 3600)
	minutes := int((int(remain) % 3600) / 60)
	p.latestReport.TimeRemaining = fmt.Sprintf("%dh %02dm", hours, minutes)
	
	msg := fmt.Sprintf("🏊 Pool: Ratio %.6f | BP: %d | Faltam: %s", ratio, int(totalBP), p.latestReport.TimeRemaining)
	
	if remain <= 45 {
		p.bot.Log(fmt.Sprintf("🎯 JANELA CRÍTICA: %ds restantes! %s", int(remain), msg))
		shouldNotifyDiscord = true
	} else {
		p.bot.Log(msg)
	}
	
	if remain > 45 {
		return false
	}
	
	if remain >= -1 && remain <= 45 {
		if blockID != "" {
			p.saveSniperSnapshot(blockID, ratio, totalBP)
		}

		if avg == 0 {
			p.bot.Log(fmt.Sprintf("⏳ SNIPER: Coletando primeira métrica para média histórica (Ratio: %.6f). Nenhum depósito será feito nesta pool.", ratio))
			p.latestReport.LastAction = "Aprendendo Média"
			shouldNotifyDiscord = true
		} else {
			target := avg * (1 + (p.minProfitMargin / 100))
			if ratio >= target {
				p.bot.Log(fmt.Sprintf("✅ SNIPER: Ratio %.6f rentável (Meta: %.6f).", ratio, target))
				if p.processDeposit(blockID) {
					p.latestReport.LastAction = "DEPOSITADO"
					shouldNotifyDiscord = true
				}
			} else if p.latestReport.LastAction == "Monitorando" {
				p.bot.Log(fmt.Sprintf("⛔ Sniper Abortado: Ratio %.6f abaixo da meta (%.6f).", ratio, target))
				p.latestReport.LastAction = "Abortado"
				shouldNotifyDiscord = true
			}
		}
	}
	
	return shouldNotifyDiscord
}

func (p *PoolStrategist) processDeposit(blockID string) bool {
	if blockID == "" {
		return false
	}
	
	inventory, err := p.apiClient.GetUserInventory()
	if err != nil {
		return false
	}
	
	depositPayload := make([]map[string]interface{}, 0)
	
	for _, i := range inventory {
		itemType := getString(i, "itemType")
		count := getInt(i, "count")
		if itemType == "farmVegetables" && count > 0 {
			toDeposit := count - p.minReserve
			if toDeposit <= 0 {
				continue
			}
			depositPayload = append(depositPayload, map[string]interface{}{
				"itemCode": getString(i, "itemCode"),
				"count":    toDeposit,
			})
		}
	}
	
	if len(depositPayload) > 0 && p.autoDeposit {
		p.bot.Log(fmt.Sprintf("📤 Enviando %d tipos de vegetais para o bloco %s...", len(depositPayload), blockID))
		res, err := p.apiClient.DepositToPool(blockID, depositPayload)
		return err == nil && getBool(res, "success")
	}
	
	return false
}

func (p *PoolStrategist) getHistoricalAverage() float64 {
	if _, err := os.Stat(p.historyFile); os.IsNotExist(err) {
		return 0
	}
	
	file, err := os.ReadFile(p.historyFile)
	if err != nil {
		return 0
	}
	
	var data []HistoryEntry
	if err := json.Unmarshal(file, &data); err != nil {
		return 0
	}
	
	validRatios := make([]float64, 0)
	for _, d := range data {
		if d.Type == "sniper_decision" && d.BlockID != "" && d.Ratio > 0 {
			validRatios = append(validRatios, d.Ratio)
		}
	}
	
	if len(validRatios) > 30 {
		validRatios = validRatios[len(validRatios)-30:]
	}
	
	if len(validRatios) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, r := range validRatios {
		sum += r
	}
	
	return sum / float64(len(validRatios))
}

func (p *PoolStrategist) saveSniperSnapshot(blockID string, ratio, totalBP float64) {
	if blockID == "" {
		return
	}
	
	data := make([]HistoryEntry, 0)
	
	if file, err := os.ReadFile(p.historyFile); err == nil {
		json.Unmarshal(file, &data)
	}
	
	for _, d := range data {
		if d.BlockID == blockID {
			return
		}
	}
	
	data = append(data, HistoryEntry{
		Timestamp: time.Now().Unix(),
		BlockID:   blockID,
		Ratio:     ratio,
		TotalBP:   totalBP,
		Type:      "sniper_decision",
	})
	
	if len(data) > 100 {
		data = data[len(data)-100:]
	}
	
	jsonData, err := json.MarshalIndent(data, "", "    ")
	if err == nil {
		os.WriteFile(p.historyFile, jsonData, 0644)
		p.bot.Log(fmt.Sprintf("💾 Snapshot Sniper salvo (Bloco: %s)", blockID))
	}
}

func (p *PoolStrategist) GetLatestReport() PoolReport {
	return p.latestReport
}

func (p *PoolStrategist) GetNextCriticalTime() float64 {
	return p.nextCriticalTime
}
