package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type DiscordNotifier struct {
	bot            *SmartBot
	webhookURL     string
	lastUpdateTime int64
	updateInterval int64
}

type DiscordEmbed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type EmbedFooter struct {
	Text string `json:"text"`
}

type DiscordPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []DiscordEmbed `json:"embeds,omitempty"`
}

// NewDiscordNotifier - Versão original (carrega de discord.txt)
func NewDiscordNotifier(bot *SmartBot) (*DiscordNotifier, error) {
	webhookURL, err := loadWebhook()
	if err != nil {
		return nil, err
	}
	
	return &DiscordNotifier{
		bot:            bot,
		webhookURL:     webhookURL,
		lastUpdateTime: 0,
		updateInterval: 600, // 10 minutos padrão
	}, nil
}

// NewDiscordNotifierWithConfig - Nova versão (usa Config)
func NewDiscordNotifierWithConfig(bot *SmartBot, cfg *Config) (*DiscordNotifier, error) {
	if !cfg.DiscordEnabled || cfg.DiscordWebhook == "" {
		return nil, fmt.Errorf("discord não habilitado")
	}
	
	return &DiscordNotifier{
		bot:            bot,
		webhookURL:     cfg.DiscordWebhook,
		lastUpdateTime: 0,
		updateInterval: int64(cfg.UpdateInterval),
	}, nil
}

func loadWebhook() (string, error) {
	file, err := os.Open("discord.txt")
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "http") {
			return line, nil
		}
	}
	
	return "", fmt.Errorf("webhook URL não encontrada")
}

func (d *DiscordNotifier) SendNotification(message string) {
	if d.webhookURL == "" {
		return
	}
	
	payload := DiscordPayload{
		Content: fmt.Sprintf("🤖 **SmartBot Aviso**\n%s", message),
	}
	
	d.send(payload)
}

func (d *DiscordNotifier) ShouldSend(force bool) bool {
	if d.webhookURL == "" {
		return false
	}
	currentTime := time.Now().Unix()
	if !force && (currentTime-d.lastUpdateTime < d.updateInterval) {
		return false
	}
	return true
}

func (d *DiscordNotifier) SendStatusUpdate(stats map[string]interface{}, force bool) {
	if !d.ShouldSend(force) {
		return
	}
	
	currentTime := time.Now().Unix()
	
	plantListText := getString(stats, "planted_list")
	
	// Pool info
	poolData, ok := stats["pool_data"].(PoolReport)
	if !ok {
		poolData = PoolReport{}
	}
	
	timeLeft := poolData.TimeRemaining
	statusAction := poolData.LastAction
	yieldVal := poolData.RatioNow
	avgVal := poolData.RatioAvg
	
	diffText := ""
	if avgVal > 0 {
		diffPct := ((yieldVal - avgVal) / avgVal) * 100
		if diffPct >= 0 {
			diffText = fmt.Sprintf("📈 +%.1f%%", diffPct)
		} else {
			diffText = fmt.Sprintf("📉 %.1f%%", diffPct)
		}
	} else {
		diffText = "⏳ Aprendendo"
	}
	
	nextHarvests := getString(stats, "next_harvests")
	cycleTime := getString(stats, "cycle_time")
	hasMissions := false
	if hm, ok := stats["has_missions"].(bool); ok {
		hasMissions = hm
	}
	
	// Request stats
	reqCount := getInt(stats, "request_count")
	errCount := getInt(stats, "error_count")
	successRate := 100.0
	if reqCount > 0 {
		successRate = float64(reqCount-errCount) / float64(reqCount) * 100
	}

	sleepReason := getString(stats, "sleep_reason")
	wakeTime := getString(stats, "wake_time")

	// Traduz motivo do sono
	reasonEmoji := "💤"
	reasonText := sleepReason
	switch {
	case strings.HasPrefix(sleepReason, "harvest"):
		reasonEmoji = "🌾"
		reasonText = "Colheita"
	case sleepReason == "pool_critical":
		reasonEmoji = "🏊"
		reasonText = "Pool Crítico"
	case sleepReason == "pool_check":
		reasonEmoji = "🔍"
		reasonText = "Checar Pool"
	case sleepReason == "default_update":
		reasonEmoji = "⏰"
		reasonText = "Atualização"
	}

	// ──────── Embed Color ────────
	embedColor := 3447003 // Blue default
	if statusAction == "DEPOSITADO" {
		embedColor = 3066993 // Green
	} else if statusAction == "Aprendendo Média" {
		embedColor = 15105570 // Orange
	} else if strings.Contains(statusAction, "Abortado") {
		embedColor = 15158332 // Red
	}

	// ──────── Description: Mapa da Fazenda ────────
	desc := ""
	if plantListText != "" && plantListText != "Nenhuma" {
		desc = plantListText
	} else {
		desc = "_Nenhuma semente plantada no momento._"
	}

	// ──────── BPM / Rendimento ────────
	totalStr := getString(stats, "total_bpm")
	dailyStr := getString(stats, "daily_bpm")

	// ──────── Inventário ────────
	invBP := getString(stats, "inventory_bp")
	invCFB := getString(stats, "inventory_cfb")

	// ──────── Pool ────────
	poolText := fmt.Sprintf(
		"🎯 Yield: `%.6f`\n📊 Média: `%.6f` (%s)\n⏳ Fim em: `%s`\n🔔 Status: `%s`",
		yieldVal, avgVal, diffText, timeLeft, statusAction,
	)

	// ──────── Missões ────────
	missionField := "❌ Nenhuma pendente"
	if hasMissions {
		missionField = "✅ **DISPONÍVEL** — resgate pelo navegador!"
	}

	// ──────── Status do Bot ────────
	healthEmoji := "🟢"
	if successRate < 99 {
		healthEmoji = "🟡"
	}
	if successRate < 95 {
		healthEmoji = "🔴"
	}
	botStatus := fmt.Sprintf(
		"%s API: `%.1f%%` (%d req / %d err)\n⏱️ Ciclo: `%s` | %s Próximo: `%s` → `%s`",
		healthEmoji, successRate, reqCount, errCount,
		cycleTime, reasonEmoji, reasonText, wakeTime,
	)

	// ──────── Construção do Embed ────────
	embed := DiscordEmbed{
		Title:       "🚜 SmartBot — Painel da Fazenda",
		Color:       embedColor,
		Description: desc,
		Fields: []EmbedField{
			{
				Name:   "⚡ BPM Atual",
				Value:  fmt.Sprintf("`%s` BPM", totalStr),
				Inline: true,
			},
			{
				Name:   "🗓️ BP/Dia",
				Value:  fmt.Sprintf("`%s`", dailyStr),
				Inline: true,
			},
			{
				Name:   "🌾 Próximas Colheitas",
				Value:  fmt.Sprintf("`%s`", nextHarvests),
				Inline: true,
			},
			{
				Name:   "🎒 Inventário",
				Value:  fmt.Sprintf("🌱 `%s` BP\n💰 `≈ %s` CFB", invBP, invCFB),
				Inline: true,
			},
			{
				Name:   "🎯 Missões",
				Value:  missionField,
				Inline: true,
			},
			{
				Name:   "\u200b", // Spacer
				Value:  "\u200b",
				Inline: true,
			},
			{
				Name:   "🏊 Pool de CFB",
				Value:  poolText,
				Inline: false,
			},
			{
				Name:   "🤖 Status do Bot",
				Value:  botStatus,
				Inline: false,
			},
		},
		Footer: &EmbedFooter{
			Text: fmt.Sprintf("SmartBot V126 Go | %s", time.Now().Format("02/01/2006 15:04:05")),
		},
	}
	
	content := ""
	if hasMissions {
		content = "🎯 **Missões/recompensas disponíveis para resgate!**"
	}
	
	payload := DiscordPayload{
		Content: content,
		Embeds:  []DiscordEmbed{embed},
	}
	
	d.send(payload)
	d.lastUpdateTime = currentTime
}

func (d *DiscordNotifier) send(payload DiscordPayload) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return
	}
	
	req, err := http.NewRequest("POST", d.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 10 * time.Second}
	_, err = client.Do(req)
	if err != nil {
		// Silenciosamente ignora erros do Discord
		return
	}
}
