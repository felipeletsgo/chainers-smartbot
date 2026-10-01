package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type FarmingHandler struct {
	bot                  *SmartBot
	apiClient            *ChainersAPIClient
	plots                []map[string]interface{}
	bpmTable             map[string]float64
	vegMap               map[string]float64
	nextHarvestTimestamp float64
	bedDefinitions       map[string][]string // bedItemCode -> list of compatible seed types
	seedTypeCode         map[string]string   // seedCode -> seed type code
}

func NewFarmingHandler(bot *SmartBot) *FarmingHandler {
	return &FarmingHandler{
		bot:                  bot,
		apiClient:            bot.apiClient,
		plots:                make([]map[string]interface{}, 0),
		bpmTable:             make(map[string]float64),
		vegMap:               make(map[string]float64),
		nextHarvestTimestamp: 0,
		bedDefinitions:       make(map[string][]string),
		seedTypeCode:         make(map[string]string),
	}
}

func (f *FarmingHandler) AnalyzeSeedsBPM() {
	seedsDef, err := f.apiClient.GetSeedDefinitions()
	if err != nil || len(seedsDef) == 0 {
		fmt.Println("⚠️ Erro ao carregar definições de sementes")
		return
	}

	vegDef, err := f.apiClient.GetItemDefinitions()
	if err != nil {
		fmt.Println("⚠️ Erro ao carregar definições de vegetais")
	}

	for _, v := range vegDef {
		code := getString(v, "code")
		weight := getFloat(v, "rewardPoolBaseWeight")
		if code != "" {
			f.vegMap[code] = weight
		}
	}

	animalProducts := map[string][]string{
		"bird":    {"egg", "bird_egg"},
		"chicken": {"egg", "chicken_egg"},
		"pig":     {"truffle", "pig_truffle"},
		"piggy":   {"truffle"},
		"cattle":  {"milk", "cattle_milk"},
		"cow":     {"milk"},
		"sheep":   {"wool", "sheep_wool"},
		"deer":    {"fur", "leather", "deer_fur"},
	}

	count := 0
	for _, s := range seedsDef {
		seedCode := getString(s, "code")

		if typeObj, ok := s["type"].(map[string]interface{}); ok {
			f.seedTypeCode[seedCode] = getString(typeObj, "code")
		}

		targetBase := strings.Replace(strings.Replace(seedCode, "_seeds", "", -1), "_food", "", -1)

		weight := f.vegMap[targetBase]

		if weight == 0 {
			for animal, products := range animalProducts {
				if strings.Contains(targetBase, animal) {
					for _, prod := range products {
						possibleCode := strings.Replace(targetBase, animal, prod, -1)
						weight = f.vegMap[possibleCode]
						if weight == 0 {
							weight = f.vegMap[prod]
						}
						if weight > 0 {
							break
						}
					}
				}
				if weight > 0 {
					break
				}
			}
		}

		rawTimeSeconds := getFloat(s, "growthTimeSeconds")
		if rawTimeSeconds == 0 {
			rawTimeSeconds = getFloat(s, "growthTime")
			if rawTimeSeconds == 0 {
				rawTimeSeconds = 3600
			}
		}

		growthMins := rawTimeSeconds / 60.0
		if growthMins > 0 {
			f.bpmTable[seedCode] = weight / growthMins
			count++
		}
	}
	fmt.Printf("✅ BPM Calculado para %d sementes\n", count)
}

func (f *FarmingHandler) LoadBedDefinitions() {
	bedsDef, err := f.apiClient.GetBedDefinitions()
	if err != nil || len(bedsDef) == 0 {
		fmt.Println("⚠️ Erro ao carregar definições de canteiros")
		return
	}

	count := 0
	for _, b := range bedsDef {
		bedCode := getString(b, "code")
		if typeObj, ok := b["type"].(map[string]interface{}); ok {
			if compatRaw, ok := typeObj["compatibleFarmVegetablesTypesCodes"].([]interface{}); ok {
				var compat []string
				for _, c := range compatRaw {
					if strC, ok := c.(string); ok {
						compat = append(compat, strC)
					}
				}
				f.bedDefinitions[bedCode] = compat
				count++
			}
		}
	}
	fmt.Printf("✅ %d tipos de canteiros carregados\n", count)
}

func (f *FarmingHandler) isSeedCompatibleWithBed(seedCode string, bedItemCode string) bool {
	seedType, ok := f.seedTypeCode[seedCode]
	if !ok || seedType == "" {
		// Se não encontrou o tipo, assume fallback
		return !strings.Contains(bedItemCode, "coop") && !strings.Contains(bedItemCode, "bird") && 
		       !strings.Contains(bedItemCode, "pig") && !strings.Contains(bedItemCode, "cattle") && 
			   !strings.Contains(bedItemCode, "deer") && !strings.Contains(bedItemCode, "fuzzies") && 
			   !strings.Contains(bedItemCode, "peacock")
	}

	// Busca compatibilidades do canteiro
	compatTypes, ok := f.bedDefinitions[bedItemCode]
	if !ok {
		// Tenta limpar raridade se for canteiro genérico
		parts := strings.SplitN(bedItemCode, "_", 2)
		if len(parts) == 2 {
			baseCode := "common_" + parts[1]
			compatTypes, ok = f.bedDefinitions[baseCode]
		}
	}

	if !ok {
		return false
	}

	for _, ct := range compatTypes {
		if ct == seedType {
			return true
		}
	}
	return false
}

func (f *FarmingHandler) SyncFarmState() {
	data, err := f.apiClient.GetFarmState()
	if err == nil {
		f.plots = data
		f.updateBedsStatus()
	} else {
		fmt.Printf("⚠️ Erro ao sincronizar fazenda: %v\n", err)
	}
}

func (f *FarmingHandler) updateBedsStatus() {
	earliestFinishTs := math.Inf(1)
	nowUTC := time.Now().UTC()
	f.bot.bedsStatus = make(map[string]map[string]interface{})

	total, busy, ready := 0, 0, 0

	if len(f.plots) > 0 {
		for _, garden := range f.plots {
			gardenID := getString(garden, "userGardensID")
			placedBeds, ok := garden["placedBeds"].([]interface{})
			if !ok {
				continue
			}

			for _, bedRaw := range placedBeds {
				bed, ok := bedRaw.(map[string]interface{})
				if !ok {
					continue
				}

				total++
				bedID := getString(bed, "userBedsID")
				itemCode := strings.ToLower(getString(bed, "itemCode"))

				multiplier := 1
				if strings.Contains(itemCode, "legendary") {
					multiplier = 16
				} else if strings.Contains(itemCode, "epic") {
					multiplier = 8
				} else if strings.Contains(itemCode, "rare") {
					multiplier = 4
				} else if strings.Contains(itemCode, "uncommon") {
					multiplier = 2
				}

				isCoop := strings.Contains(itemCode, "coop") || strings.Contains(itemCode, "chicken") ||
					strings.Contains(itemCode, "pig") || strings.Contains(itemCode, "cattle") ||
					strings.Contains(itemCode, "deer") || strings.Contains(itemCode, "bird") ||
					strings.Contains(itemCode, "fuzzy") || strings.Contains(itemCode, "fuzzie") ||
					strings.Contains(itemCode, "peacock")

				production, hasProduction := bed["plantedSeed"].(map[string]interface{})

				if hasProduction {
					busy++
					name := getString(production, "seedCode")
					if name == "" {
						name = "Produção"
					}
					farmingID := getString(production, "userFarmingID")
					isReady := false
					harvestTime := 0.0

					dateGrowth := getString(production, "dateGrowth")
					if dateGrowth != "" {
						cleanDate := strings.Replace(dateGrowth, "Z", "+00:00", -1)
						finishDt, err := time.Parse(time.RFC3339, cleanDate)
						if err == nil {
							harvestTime = float64(finishDt.Unix())
							diff := finishDt.Sub(nowUTC).Seconds()
							if diff <= 0 {
								isReady = true
							} else if harvestTime < earliestFinishTs {
								earliestFinishTs = harvestTime
							}
						} else {
							f.bot.Log(fmt.Sprintf("⚠️ Data inválida em dateGrowth '%s' — ignorando bed %s", dateGrowth, bedID))
						}
					} else {
						// FALLBACK: Se não tem dateGrowth, estima baseado no BPM
						baseBPM := f.bpmTable[name]
						if baseBPM > 0 {
							// Calcula tempo de crescimento em segundos
							// BPM = BP / minutos, então minutos = BP / BPM
							// Mas não temos BP direto, usamos a tabela vegMap
							vegCode := strings.Replace(name, "_seeds", "", -1)
							vegCode = strings.Replace(vegCode, "_food", "", -1)
							bp := f.vegMap[vegCode]
							if bp > 0 {
								growthMinutes := bp / baseBPM
								growthSeconds := growthMinutes * 60
								harvestTime = float64(nowUTC.Unix()) + growthSeconds

								if harvestTime < earliestFinishTs {
									earliestFinishTs = harvestTime
								}
							}
						}
					}

					if isReady {
						ready++
					}

					f.bot.bedsStatus[bedID] = map[string]interface{}{
						"garden_id": gardenID, "bed_id": bedID, "farming_id": farmingID,
						"plant_name": name, "is_ready": isReady, "type_name": itemCode,
						"plot_type_raw": itemCode, "is_coop": isCoop, "multiplier": multiplier,
						"harvest_time": harvestTime, // Timestamp de colheita
					}
				} else {
					f.bot.bedsStatus[bedID] = map[string]interface{}{
						"garden_id": gardenID, "bed_id": bedID, "farming_id": nil,
						"is_ready": false, "is_empty": true, "can_plant_seed": !isCoop,
						"is_coop": isCoop, "type_name": itemCode,
						"plot_type_raw": itemCode, "multiplier": multiplier,
					}
				}
			}
		}
	}

	if !math.IsInf(earliestFinishTs, 1) {
		f.nextHarvestTimestamp = earliestFinishTs
	} else {
		f.nextHarvestTimestamp = 0
	}

	f.bot.Log(fmt.Sprintf("🌾 Fazenda: %d Slots | %d Plantados | %d Prontos", total, busy, ready))
}

func (f *FarmingHandler) GetFormattedReportText() (string, float64, float64) {
	totalBPM := 0.0

	type bedInfo struct {
		id   string
		data map[string]interface{}
	}
	beds := make([]bedInfo, 0, len(f.bot.bedsStatus))
	for id, data := range f.bot.bedsStatus {
		beds = append(beds, bedInfo{id, data})
	}

	sort.Slice(beds, func(i, j int) bool { return beds[i].id < beds[j].id })

	plantCounts := make(map[string]int)

	for _, bed := range beds {
		info := bed.data
		if info["farming_id"] != nil || getBool(info, "is_coop") {
			rawType := getString(info, "plot_type_raw")
			pName := "🟩" // Common
			if strings.Contains(rawType, "legendary") {
				pName = "🟨" // Legend
			} else if strings.Contains(rawType, "epic") {
				pName = "🟪" // Epic
			} else if strings.Contains(rawType, "rare") {
				pName = "🟦" // Rare
			} else if strings.Contains(rawType, "uncommon") {
				pName = "🟩" // Uncom
			}

			name := strings.ToUpper(strings.Replace(getString(info, "plant_name"), "_seeds", "", -1))
			if name == "" {
				name = strings.ToUpper(getString(info, "type_name"))
			}

			// Emojis dinâmicos
			emoji := "🌱"
			nameLower := strings.ToLower(name)
			if strings.Contains(nameLower, "cattle") || strings.Contains(nameLower, "cow") {
				emoji = "🐄"
			} else if strings.Contains(nameLower, "pig") {
				emoji = "🐖"
			} else if strings.Contains(nameLower, "deer") {
				emoji = "🦌"
			} else if strings.Contains(nameLower, "bird") || strings.Contains(nameLower, "chicken") {
				emoji = "🐔"
			} else if strings.Contains(nameLower, "peacock") {
				emoji = "🦚"
			} else if strings.Contains(nameLower, "fuzzy") || strings.Contains(nameLower, "fuzzie") {
				emoji = "🐹"
			} else if strings.Contains(nameLower, "strawberry") {
				emoji = "🍓"
			} else if strings.Contains(nameLower, "corn") {
				emoji = "🌽"
			} else if strings.Contains(nameLower, "carrot") {
				emoji = "🥕"
			} else if strings.Contains(nameLower, "tomato") {
				emoji = "🍅"
			} else if strings.Contains(nameLower, "eggplant") {
				emoji = "🍆"
			} else if strings.Contains(nameLower, "melon") || strings.Contains(nameLower, "watermelon") {
				emoji = "🍉"
			} else if strings.Contains(nameLower, "pineapple") {
				emoji = "🍍"
			} else if strings.Contains(nameLower, "potato") {
				emoji = "🥔"
			} else if strings.Contains(nameLower, "pumpkin") {
				emoji = "🎃"
			} else if strings.Contains(nameLower, "cabbage") {
				emoji = "🥬"
			} else if strings.Contains(nameLower, "radish") || strings.Contains(nameLower, "daikon") {
				emoji = "🧅"
			} else if strings.Contains(nameLower, "wheat") {
				emoji = "🌾"
			} else if strings.Contains(nameLower, "lavender") {
				emoji = "🪻"
			}

			plantName := getString(info, "plant_name")
			baseBPM := f.bpmTable[plantName]
			multiplier := getInt(info, "multiplier")
			finalBPM := baseBPM * float64(multiplier)
			totalBPM += finalBPM

			multTag := fmt.Sprintf("`%dx`", multiplier)
			key := fmt.Sprintf("%s %s %s %s", pName, multTag, emoji, name)
			plantCounts[key]++
		}
	}

	visualBPM := math.Round(totalBPM*10) / 10

	var lines []string

	type kv struct {
		Key   string
		Value int
	}
	var ss []kv
	for k, v := range plantCounts {
		ss = append(ss, kv{k, v})
	}
	// Sort by count descending
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})

	for _, kv := range ss {
		lines = append(lines, fmt.Sprintf("**%dx** %s", kv.Value, kv.Key))
	}

	if len(lines) == 0 {
		lines = append(lines, "Nenhuma plantação ativa.")
	}

	return strings.Join(lines, "\n"), visualBPM, visualBPM * 1440
}

// Retorna TRUE se coletou algo (inventário mudou)
func (f *FarmingHandler) CollectHarvestOnly() bool {
	collected := false
	readyItems := make([]map[string]interface{}, 0)
	for _, v := range f.bot.bedsStatus {
		if getBool(v, "is_ready") && v["farming_id"] != nil {
			readyItems = append(readyItems, v)
		}
	}

	if len(readyItems) > 0 {
		f.bot.Log(fmt.Sprintf("🚜 Colhendo %d itens", len(readyItems)))

		// OTIMIZAÇÃO: Colhe tudo SEM delay entre itens
		for _, info := range readyItems {
			res, err := f.apiClient.CollectHarvest(getString(info, "farming_id"))
			if err == nil && getBool(res, "success") {
				collected = true
			} else if err != nil {
				// VERIFICA ERRO 400 "not growth yet" - RETRY com 1s delay
				errStr := err.Error()
				if strings.Contains(errStr, "not growth yet") || strings.Contains(errStr, "400") {
					f.bot.Log(fmt.Sprintf("⏳ Erro 400, aguardando 1s para retry..."))
					time.Sleep(1 * time.Second)

					// TENTA NOVAMENTE
					res2, err2 := f.apiClient.CollectHarvest(getString(info, "farming_id"))
					if err2 == nil && getBool(res2, "success") {
						f.bot.Log(fmt.Sprintf("✅ Retry bem-sucedido!"))
						collected = true
					} else {
						f.bot.Log(fmt.Sprintf("❌ Retry falhou: %v", err2))
					}
				}
			}
		}

		// Delay ÚNICO após coletar tudo
		if collected {
			time.Sleep(200 * time.Millisecond)
		}
	}

	return collected // RETORNA TRUE SE INVENTÁRIO MUDOU
}

func (f *FarmingHandler) getRarityScore(itemCode string) int {
	code := strings.ToLower(itemCode)
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

// Retorna TRUE se plantou algo (inventário mudou)
func (f *FarmingHandler) PlantPriorityOrBPM() bool {
	// USA CACHE (será fresco após coleta/craft)
	inventory, err := f.bot.GetCachedInventory()
	if err != nil || len(inventory) == 0 {
		return false
	}

	fed := f.feedAnimals(inventory)

	// Coleta TODAS as sementes do inventário (prioritárias e normais juntas)
	allSeeds := make([]map[string]interface{}, 0)

	for _, i := range inventory {
		itemCode := getString(i, "itemCode")
		if strings.Contains(itemCode, "food") {
			continue
		}

		itemType := getString(i, "itemType")
		count := getInt(i, "count")

		if (itemType == "farmSeeds" || strings.Contains(itemCode, "seed")) && count > 0 {
			baseBPM := f.bpmTable[itemCode]
			isPriority := false
			for _, target := range f.bot.priorityPlantTargets {
				if itemCode == target {
					isPriority = true
					break
				}
			}

			seedData := map[string]interface{}{
				"itemID": getString(i, "itemID"), "id": getString(i, "id"),
				"itemCode": itemCode, "count": count,
				"is_priority": isPriority, "base_bpm": baseBPM,
			}
			allSeeds = append(allSeeds, seedData)
		}
	}

	// Coleta plots vazios (apenas os que aceitam sementes, não coops)
	emptyPlots := make([]map[string]interface{}, 0)
	for _, p := range f.bot.bedsStatus {
		if getBool(p, "is_empty") && getBool(p, "can_plant_seed") {
			emptyPlots = append(emptyPlots, p)
		}
	}

	planted := false
	plantedSeeds := make([]string, 0)

	// ═══════════════════════════════════════════════════════════════
	// ESTRATÉGIA UNIFICADA:
	// - Todas as sementes competem igualmente por effectiveBPM
	//   (baseBPM × multiplicador da plot)
	// - Legendary daikon (alto BPM + ingrediente de food) vai para
	//   a melhor plot possível, exatamente como qualquer semente
	// - Prioridade = "garanta que será plantada", NÃO "plante na pior plot"
	// - Quando as plots restantes ficam escassas (≤ prioridades pendentes),
	//   somente sementes prioritárias podem ser plantadas nessa rodada
	// ═══════════════════════════════════════════════════════════════

	for len(allSeeds) > 0 && len(emptyPlots) > 0 {
		// Conta quantas sementes prioritárias ainda precisam de plot
		unplantedPriority := 0
		for _, seed := range allSeeds {
			if getBool(seed, "is_priority") && getInt(seed, "count") > 0 {
				// Verifica se pelo menos 1 plot é compatível
				seedCode := getString(seed, "itemCode")
				for _, plot := range emptyPlots {
					if f.isSeedCompatibleWithBed(seedCode, getString(plot, "type_name")) {
						unplantedPriority++
						break
					}
				}
			}
		}

		// Se plots restantes ≤ prioridades pendentes, só prioritárias podem plantar
		priorityOnly := len(emptyPlots) <= unplantedPriority && unplantedPriority > 0

		bestSeedIdx := -1
		bestPlotIdx := -1
		maxEffectiveBPM := -1.0

		for seedIdx, seed := range allSeeds {
			if getInt(seed, "count") <= 0 {
				continue
			}

			isPriority := getBool(seed, "is_priority")

			// Se estamos em modo "só prioritárias", pula normais
			if priorityOnly && !isPriority {
				continue
			}

			baseBPM := getFloat(seed, "base_bpm")
			seedCode := getString(seed, "itemCode")

			for plotIdx, plot := range emptyPlots {
				bedItemCode := getString(plot, "type_name")
				if !f.isSeedCompatibleWithBed(seedCode, bedItemCode) {
					continue
				}

				multiplier := getInt(plot, "multiplier")
				effectiveBPM := baseBPM * float64(multiplier)

				if f.bot.config.PreferHighRarity {
					rarityScore := f.getRarityScore(seedCode)
					if rarityScore > 0 {
						effectiveBPM += float64(rarityScore) * 100000.0
					}
				}

				if effectiveBPM > maxEffectiveBPM {
					maxEffectiveBPM = effectiveBPM
					bestSeedIdx = seedIdx
					bestPlotIdx = plotIdx
				}
			}
		}

		if bestSeedIdx == -1 || bestPlotIdx == -1 {
			break
		}

		// Planta a melhor combinação encontrada
		seed := allSeeds[bestSeedIdx]
		plot := emptyPlots[bestPlotIdx]

		seedID := getString(seed, "itemID")
		if seedID == "" {
			seedID = getString(seed, "id")
		}
		seedCode := getString(seed, "itemCode")
		multiplier := getInt(plot, "multiplier")
		isPriority := getBool(seed, "is_priority")

		res, err := f.apiClient.PlantSeed(getString(plot, "garden_id"), getString(plot, "bed_id"), seedID)
		if err == nil && getBool(res, "success") {
			count := getInt(seed, "count") - 1
			seed["count"] = count
			if isPriority {
				f.bot.RemovePriorityTarget(seedCode)
			}
			if count <= 0 {
				allSeeds = append(allSeeds[:bestSeedIdx], allSeeds[bestSeedIdx+1:]...)
			}
			planted = true

			cleanName := strings.Replace(seedCode, "_seeds", "", -1)
			tag := fmt.Sprintf("%dx", multiplier)
			if isPriority {
				tag = fmt.Sprintf("P/%dx", multiplier)
			}
			plantedSeeds = append(plantedSeeds, fmt.Sprintf("%s(%s)", cleanName, tag))

			emptyPlots = append(emptyPlots[:bestPlotIdx], emptyPlots[bestPlotIdx+1:]...)
			time.Sleep(200 * time.Millisecond)
		} else {
			emptyPlots = append(emptyPlots[:bestPlotIdx], emptyPlots[bestPlotIdx+1:]...)
		}
	}

	// Log do que foi plantado
	if len(plantedSeeds) > 0 {
		f.bot.Log(fmt.Sprintf("🌱 Plantado: %s", strings.Join(plantedSeeds, ", ")))
	}

	return fed || planted // RETORNA TRUE SE INVENTÁRIO MUDOU
}

// Retorna TRUE se alimentou animais (inventário mudou)
func (f *FarmingHandler) feedAnimals(inventory []map[string]interface{}) bool {
	emptyCoops := make([]map[string]interface{}, 0)
	for _, info := range f.bot.bedsStatus {
		if getBool(info, "is_empty") && getBool(info, "is_coop") {
			emptyCoops = append(emptyCoops, info)
		}
	}
	if len(emptyCoops) == 0 {
		return false
	}

	fed := false

	for _, coop := range emptyCoops {
		coopType := getString(coop, "type_name")
		var foodType string
		if strings.Contains(coopType, "bird") || strings.Contains(coopType, "chicken") {
			foodType = "bird_food"
		} else if strings.Contains(coopType, "deer") {
			foodType = "deer_food"
		} else if strings.Contains(coopType, "pig") {
			foodType = "piggy_food"
		} else if strings.Contains(coopType, "cattle") || strings.Contains(coopType, "cow") {
			foodType = "cattle_food"
		} else if strings.Contains(coopType, "fuzz") {
			foodType = "fuzzies_food"
		} else if strings.Contains(coopType, "peacock") {
			foodType = "peacock_food"
		}

		if foodType == "" {
			continue
		}

		foods := make([]map[string]interface{}, 0)
		for _, i := range inventory {
			itemCode := strings.ToLower(getString(i, "itemCode"))
			if strings.Contains(itemCode, foodType) && getInt(i, "count") > 0 {
				foods = append(foods, i)
			}
		}

		sort.Slice(foods, func(i, j int) bool {
			return f.getRarityScore(getString(foods[i], "itemCode")) > f.getRarityScore(getString(foods[j], "itemCode"))
		})

		if len(foods) > 0 {
			var food map[string]interface{}
			for _, fItem := range foods {
				if f.isSeedCompatibleWithBed(getString(fItem, "itemCode"), coopType) {
					food = fItem
					break
				}
			}
			if food == nil {
				food = foods[0] // fallback se não achar explicitamente compatível
			}

			foodID := getString(food, "itemID")
			if foodID == "" {
				foodID = getString(food, "id")
			}
			f.bot.Log(fmt.Sprintf("🖖 Alimentando %s", coopType))
			res, err := f.apiClient.PlantSeed(getString(coop, "garden_id"), getString(coop, "bed_id"), foodID)
			if err == nil && getBool(res, "success") {
				fed = true
				// Decrementa contagem local para evitar usar o mesmo item em outro coop
				food["count"] = getInt(food, "count") - 1
			}
		}
	}

	return fed // RETORNA TRUE SE INVENTÁRIO MUDOU
}

func (f *FarmingHandler) GetNextHarvestTime() float64 {
	return f.nextHarvestTimestamp
}

// Nova função: Retorna próximas N colheitas
type HarvestPreview struct {
	time float64
	name string
}

func (f *FarmingHandler) GetNextHarvestsPreview(count int) []HarvestPreview {
	nowUTC := time.Now().UTC()
	nowTs := float64(nowUTC.Unix())

	harvests := make([]HarvestPreview, 0)

	// Coleta todos os timestamps de colheita
	for _, info := range f.bot.bedsStatus {
		if info["farming_id"] == nil {
			continue
		}

		plantName := getString(info, "plant_name")
		isReady := getBool(info, "is_ready")

		if isReady {
			// Item já está pronto
			harvests = append(harvests, HarvestPreview{nowTs, plantName})
		} else {
			// Pega o timestamp de colheita
			harvestTime := getFloat(info, "harvest_time")
			if harvestTime > nowTs {
				harvests = append(harvests, HarvestPreview{harvestTime, plantName})
			}
		}
	}

	// Ordena por tempo
	sort.Slice(harvests, func(i, j int) bool {
		return harvests[i].time < harvests[j].time
	})

	// Retorna apenas os primeiros N
	if len(harvests) > count {
		harvests = harvests[:count]
	}

	return harvests
}

func (f *FarmingHandler) CalculateInventoryBPMValue(inventory []map[string]interface{}) float64 {
	if inventory == nil {
		return 0
	}
	total := 0.0
	for _, item := range inventory {
		total += float64(getInt(item, "count")) * f.vegMap[getString(item, "itemCode")]
	}
	return total
}


