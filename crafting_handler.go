package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RecipeData struct {
	OfferID  string
	RecipeID string
	Inputs   []map[string]interface{}
}

type CraftingHandler struct {
	bot                  *SmartBot
	apiClient            *ChainersAPIClient
	recipesMap           map[string]RecipeData
	currentQueue         []map[string]interface{}
	protectedIngredients []string
	recipesLoaded        bool
}

func NewCraftingHandler(bot *SmartBot) *CraftingHandler {
	return &CraftingHandler{
		bot:           bot,
		apiClient:     bot.apiClient,
		recipesMap:    make(map[string]RecipeData),
		currentQueue:  make([]map[string]interface{}, 0),
		recipesLoaded: false,
		protectedIngredients: []string{
			"common_corn_seeds", "uncommon_corn_seeds", "rare_corn_seeds", "epic_corn_seeds",
			"common_corn", "uncommon_corn", "rare_corn", "epic_corn",
			"common_peas_seeds", "uncommon_peas_seeds", "rare_peas_seeds", "epic_peas_seeds",
			"common_peas", "uncommon_peas", "rare_peas", "epic_peas",
		},
	}
}

// ========== CARREGA RECEITAS APENAS UMA VEZ ==========
func (c *CraftingHandler) LoadRecipesOnce() {
	if c.recipesLoaded {
		return
	}

	c.bot.Log("📚 Carregando receitas de crafting...")
	rawOffers, err := c.apiClient.GetCraftingOffers()
	if err != nil {
		c.bot.Log("⚠️ Erro ao carregar receitas")
		return
	}

	for _, item := range rawOffers {
		itemCode := getString(item, "code")

		recipesList, ok := item["recipes"].([]interface{})
		if !ok || len(recipesList) == 0 {
			continue
		}

		recipe, ok := recipesList[0].(map[string]interface{})
		if !ok {
			continue
		}

		requiredItems, ok := recipe["requiredItems"].([]interface{})
		if !ok {
			requiredItems = []interface{}{}
		}

		inputs := make([]map[string]interface{}, 0)
		for _, req := range requiredItems {
			if reqMap, ok := req.(map[string]interface{}); ok {
				inputs = append(inputs, reqMap)
			}
		}

		c.recipesMap[itemCode] = RecipeData{
			OfferID:  getString(item, "id"),
			RecipeID: getString(recipe, "id"),
			Inputs:   inputs,
		}
	}

	c.recipesLoaded = true
	c.bot.Log(fmt.Sprintf("✅ %d receitas carregadas", len(c.recipesMap)))
}

// Retorna se o inventário mudou (para info/debug apenas)
func (c *CraftingHandler) CheckAndClaimCrafts() bool {
	crafts, err := c.apiClient.GetActiveCrafts()
	if err != nil {
		return false
	}

	c.currentQueue = crafts

	if len(c.currentQueue) == 0 {
		return false
	}

	queueCopy := make([]map[string]interface{}, len(c.currentQueue))
	copy(queueCopy, c.currentQueue)

	inventoryChanged := false

	for _, craft := range queueCopy {
		isClaimed := getBool(craft, "isClaimed")
		itemID := getString(craft, "id")
		name := getString(craft, "craftingOfferCode")
		if name == "" {
			name = "Item Desconhecido"
		}

		c.bot.Log(fmt.Sprintf("🕵️ Pendente: %s | Claimed: %v", name, isClaimed))

		if !isClaimed && itemID != "" {
			c.bot.Log(fmt.Sprintf("🔨 Coletando: %s", name))
			res, err := c.apiClient.ClaimCraft(itemID)

			if err == nil && getBool(res, "success") {
				c.bot.Log(fmt.Sprintf("✅ %s coletado!", name))
				c.removeFromQueue(itemID)
				inventoryChanged = true          // INVENTÁRIO MUDOU!
				c.bot.InvalidateInventoryCache() // Invalida IMEDIATAMENTE
			} else {
				errMsg := "Erro desconhecido"
				if err != nil {
					errMsg = err.Error()
				} else if errStr, ok := res["error"].(string); ok {
					errMsg = errStr
				}
				c.bot.Log(fmt.Sprintf("❌ Falha: %s", errMsg))
			}

			time.Sleep(200 * time.Millisecond)
		}
	}

	return inventoryChanged
}

func (c *CraftingHandler) removeFromQueue(itemID string) {
	newQueue := make([]map[string]interface{}, 0)
	for _, craft := range c.currentQueue {
		if getString(craft, "id") != itemID {
			newQueue = append(newQueue, craft)
		}
	}
	c.currentQueue = newQueue
}

// Retorna TRUE se iniciou craft (inventário mudou)
func (c *CraftingHandler) AnalyzeNeedsAndCraft() bool {
	if len(c.currentQueue) > 0 {
		item := c.currentQueue[0]
		name := getString(item, "craftingOfferCode")
		if name == "" {
			name = "Item"
		}
		c.bot.Log(fmt.Sprintf("⏳ Fila ocupada (%s)", name))
		return false
	}

	c.bot.priorityPlantTargets = []string{}

	activeAnimals := make(map[string]bool)

	debug := c.bot.config.DebugMode
	if debug {
		c.bot.Log(fmt.Sprintf("🏠 [DEBUG] Verificando %d beds de fazenda", len(c.bot.bedsStatus)))
	}
	for _, info := range c.bot.bedsStatus {
		isCoop, _ := info["is_coop"].(bool)
		if !isCoop {
			continue
		}
		name := getString(info, "type_name")
		if debug {
			c.bot.Log(fmt.Sprintf("   🐓 [DEBUG] Coop: type_name='%s'", name))
		}
		if strings.Contains(name, "bird") || strings.Contains(name, "chicken") {
			activeAnimals["bird_food"] = true
		} else if strings.Contains(name, "deer") {
			activeAnimals["deer_food"] = true
		} else if strings.Contains(name, "pig") {
			activeAnimals["piggy_food"] = true
		} else if strings.Contains(name, "cattle") || strings.Contains(name, "cow") {
			activeAnimals["cattle_food"] = true
		} else if strings.Contains(name, "fuzz") {
			activeAnimals["fuzzies_food"] = true
		} else if strings.Contains(name, "peacock") {
			activeAnimals["peacock_food"] = true
		} else if debug {
			c.bot.Log(fmt.Sprintf("      ⚠️ [DEBUG] Tipo não reconhecido: %s", name))
		}
	}

	if debug {
		c.bot.Log(fmt.Sprintf("🎯 [DEBUG] Animais ativos: %v", activeAnimals))
	}

	if len(activeAnimals) == 0 {
		return false
	}

	// USA CACHE (será fresco se acabou de coletar)
	inventory, err := c.bot.GetCachedInventory()
	if err != nil {
		return false
	}

	anyActionTaken := false

	for baseName := range activeAnimals {
		totalStock := 0
		for _, i := range inventory {
			itemCode := getString(i, "itemCode")
			if strings.Contains(itemCode, baseName) {
				count := getInt(i, "count")
				if count == 0 {
					count = getInt(i, "amount")
				}
				totalStock += count
			}
		}

		if c.bot.config.DebugMode {
			c.bot.Log(fmt.Sprintf("🔢 [DEBUG] %s: estoque=%d (%d itens no inventário)", baseName, totalStock, len(inventory)))
		}

		if totalStock >= c.bot.config.MinFoodStock {
			continue
		}

		c.bot.Log(fmt.Sprintf("📉 %s: Estoque Baixo (%d)", baseName, totalStock))
		crafted := c.resolveBestTier(baseName, inventory)
		if crafted {
			anyActionTaken = true
		}
	}

	return anyActionTaken
}

// Retorna TRUE se iniciou merge (inventário mudou)
func (c *CraftingHandler) ProcessSeedMerges() bool {
	if len(c.currentQueue) > 0 {
		return false
	}

	// USA CACHE
	inventory, err := c.bot.GetCachedInventory()
	if err != nil {
		return false
	}

	type mergeCandidate struct {
		code  string
		data  RecipeData
		score int
	}

	mergeCandidates := make([]mergeCandidate, 0)

	for code, data := range c.recipesMap {
		if strings.Contains(code, "_seeds") && len(data.Inputs) > 0 {
			firstInput := getString(data.Inputs[0], "itemCode")
			if strings.Contains(firstInput, "_seeds") {
				score := c.rarityScore(code)
				mergeCandidates = append(mergeCandidates, mergeCandidate{code, data, score})
			}
		}
	}

	sort.Slice(mergeCandidates, func(i, j int) bool {
		return mergeCandidates[i].score > mergeCandidates[j].score
	})

	for _, candidate := range mergeCandidates {
		canCraft, _ := c.checkIngredients(candidate.data.Inputs, inventory, c.bot.config.MinFoodStock)

		if canCraft {
			c.bot.Log(fmt.Sprintf("🧬 Merge: %s", candidate.code))
			res, err := c.apiClient.StartCraft(candidate.data.OfferID, candidate.data.RecipeID)

			if err == nil && getBool(res, "success") {
				c.bot.Log(fmt.Sprintf("✅ Merge Iniciado: %s", candidate.code))
				c.currentQueue = append(c.currentQueue, map[string]interface{}{
					"craftingOfferCode": candidate.code,
					"isClaimed":         false,
				})
				time.Sleep(200 * time.Millisecond)
				return true // INVENTÁRIO MUDOU!
			}
		}
	}

	return false
}

func (c *CraftingHandler) rarityScore(code string) int {
	if strings.Contains(code, "legendary") {
		return 4
	}
	if strings.Contains(code, "epic") {
		return 3
	}
	if strings.Contains(code, "rare") {
		return 2
	}
	if strings.Contains(code, "uncommon") {
		return 1
	}
	return 0
}

func (c *CraftingHandler) resolveBestTier(baseName string, inventory []map[string]interface{}) bool {
	tiers := []string{"legendary", "epic", "rare", "uncommon", "common"}

	// FASE 1: IDENTIFICAR O MELHOR TIER POSSÍVEL
	var bestViableTier string
	var bestViableData RecipeData
	var bestViableMissing []map[string]interface{}

	for _, tier := range tiers {
		targetItem := tier + "_" + baseName
		data, exists := c.recipesMap[targetItem]
		if !exists {
			c.bot.Log(fmt.Sprintf("   [%s] Receita não existe", targetItem))
			continue
		}

		canCraft, missing := c.checkIngredients(data.Inputs, inventory, 0)

		if c.bot.config.DebugMode {
			ingredients := make([]string, 0)
			for _, ing := range data.Inputs {
				ingredients = append(ingredients, getString(ing, "itemCode"))
			}
			c.bot.Log(fmt.Sprintf("🔧 [DEBUG] %s: ingredientes=%v", targetItem, ingredients))
		}

		if canCraft {
			// Encontrou tier que pode craftar AGORA
			bestViableTier = tier
			bestViableData = data
			break
		} else {
			if c.bot.config.DebugMode {
				missingCodes := make([]string, 0)
				for _, m := range missing {
					missingCodes = append(missingCodes, getString(m, "code"))
				}
				c.bot.Log(fmt.Sprintf("   [DEBUG] [%s] Faltando: %v", targetItem, missingCodes))
			}

			// Verifica quais ingredientes faltantes têm sementes disponíveis (qualquer quantidade)
			seedsToPlant := make([]string, 0)
			for _, miss := range missing {
				vegCode := getString(miss, "code")
				need := getInt(miss, "amount")
				if need == 0 {
					need = 1
				}
				seedCode := vegCode
				if !strings.Contains(vegCode, "seeds") {
					seedCode = vegCode + "_seeds"
				}
				// Checa se há pelo menos uma semente disponível no inventário
				avail := 0
				for _, inv := range inventory {
					if getString(inv, "itemCode") == seedCode {
						avail = getInt(inv, "count")
						if avail == 0 {
							avail = getInt(inv, "amount")
						}
						break
					}
				}
				if avail > 0 {
					seedsToPlant = append(seedsToPlant, seedCode)
				}
			}

			// Se houver alguma semente disponível para os ingredientes faltantes, e ainda não temos um tier escolhido,
			// marcamos este tier para plantio e saímos (não craftamos ainda)
			if len(seedsToPlant) > 0 && bestViableTier == "" {
				bestViableTier = tier
				bestViableData = data
				bestViableMissing = missing
				c.bot.priorityPlantTargets = seedsToPlant
				c.bot.Log(fmt.Sprintf("🚜 Plantando para %s: %v", targetItem, seedsToPlant))
				return false
			}
			// Caso contrário, continua para o próximo tier (sem plantar ainda)
		}
	}

	// Não encontrou nenhum tier viável
	if bestViableTier == "" {
		c.bot.Log(fmt.Sprintf("❌ Sem recursos para craftar %s (nenhum tier viável)", baseName))
		return false
	}

	// FASE 2: EXECUTAR MELHOR OPÇÃO
	targetItem := bestViableTier + "_" + baseName
	canCraft, _ := c.checkIngredients(bestViableData.Inputs, inventory, 0)

	if canCraft {
		// Pode craftar AGORA
		c.bot.Log(fmt.Sprintf("🍳 Craftando %s: %s", bestViableTier, targetItem))
		res, err := c.apiClient.StartCraft(bestViableData.OfferID, bestViableData.RecipeID)

		if err == nil && getBool(res, "success") {
			c.bot.Log(fmt.Sprintf("✅ Craft Iniciado: %s", targetItem))
			c.currentQueue = append(c.currentQueue, map[string]interface{}{
				"craftingOfferCode": targetItem,
				"isClaimed":         false,
			})
			return true // INVENTÁRIO MUDOU!
		} else {
			errMsg := "Erro desconhecido"
			if err != nil {
				errMsg = err.Error()
			} else if errStr, ok := res["error"].(string); ok {
				errMsg = errStr
			}
			c.bot.Log(fmt.Sprintf("❌ Erro ao craftar: %s", errMsg))
		}
	} else {
		// Precisa plantar ingredientes — verifica completude
		neededMap := make(map[string]int)
		for _, miss := range bestViableMissing {
			vegCode := getString(miss, "code")
			need := getInt(miss, "amount")
			if need == 0 {
				need = 1
			}
			seedCode := vegCode
			if !strings.Contains(vegCode, "seeds") {
				seedCode = vegCode + "_seeds"
			}
			neededMap[seedCode] = need
		}
		// Checa disponibilidade
		allAvailable := true
		for seedCode, need := range neededMap {
			avail := 0
			for _, inv := range inventory {
				if getString(inv, "itemCode") == seedCode {
					avail = getInt(inv, "count")
					if avail == 0 {
						avail = getInt(inv, "amount")
					}
					break
				}
			}
			if avail < need {
				allAvailable = false
				break
			}
		}
		if allAvailable {
			finalSeeds := make([]string, 0)
			for code, qty := range neededMap {
				for i := 0; i < qty; i++ {
					finalSeeds = append(finalSeeds, code)
				}
			}
			c.bot.priorityPlantTargets = finalSeeds
			c.bot.Log(fmt.Sprintf("🚜 Plantando %v para %s", finalSeeds, targetItem))
			return false
		} else {
			c.bot.Log(fmt.Sprintf("⏸️ Receita incompleta para %s — aguardando colheita futura", targetItem))
		}
	}

	return false
}

func (c *CraftingHandler) checkIngredients(requiredItems []map[string]interface{}, inventory []map[string]interface{}, reserve int) (bool, []map[string]interface{}) {
	missing := make([]map[string]interface{}, 0)
	possible := true

	for _, req := range requiredItems {
		code := getString(req, "itemCode")
		amt := getInt(req, "count")
		if amt == 0 {
			amt = 1
		}

		have := 0
		for _, i := range inventory {
			if getString(i, "itemCode") == code {
				have = getInt(i, "count")
				if have == 0 {
					have = getInt(i, "amount")
				}
				break
			}
		}

		if have < amt + reserve {
			possible = false
			missing = append(missing, map[string]interface{}{
				"code":   code,
				"amount": (amt + reserve) - have,
			})
		}
	}

	return possible, missing
}


