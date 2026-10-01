package main

import (
	"fmt"
	"time"
)

type MissionsHandler struct {
	bot           *SmartBot
	lastCheck     time.Time
	checkInterval time.Duration
	hasMissions   bool
}

func NewMissionsHandler(bot *SmartBot) *MissionsHandler {
	return &MissionsHandler{
		bot:           bot,
		checkInterval: 30 * time.Minute, // Check missions every 30 minutes
		hasMissions:   false,
	}
}

// CheckMissionsAvailable checks if any missions are complete and ready to claim.
// Returns true if any mission is available.
func (h *MissionsHandler) CheckMissionsAvailable(force bool) bool {
	if !force && time.Since(h.lastCheck) < h.checkInterval {
		return h.hasMissions
	}

	h.lastCheck = time.Now()
	if h.bot.logger != nil {
		h.bot.logger.Info("🔍 [Missions] Verificando missões disponíveis...")
	}

	eventsStatus, err := h.bot.apiClient.GetUserEventsStatus()
	if err != nil {
		if h.bot.logger != nil {
			h.bot.logger.Error(fmt.Sprintf("❌ [Missions] Falha ao verificar missões: %v", err))
		}
		return false
	}

	hasMissions := false

	for _, eventObj := range eventsStatus {
		event, ok := eventObj.(map[string]interface{})
		if !ok {
			continue
		}

		parentCode := getString(event, "parentCode")
		countToClaim := getInt(event, "countToClaim")

		// If there are rewards to claim in this event category
		if countToClaim > 0 && parentCode != "" {
			hasMissions = true
			if h.bot.logger != nil {
				h.bot.logger.Info(fmt.Sprintf("🎁 [Missions] %d missão(ões) disponíveis na categoria '%s'!", countToClaim, parentCode))
			}
		}
	}

	if !hasMissions {
		if h.bot.logger != nil {
			h.bot.logger.Info("ℹ️ [Missions] Nenhuma missão disponível para resgatar.")
		}
	}

	h.hasMissions = hasMissions
	return hasMissions
}

