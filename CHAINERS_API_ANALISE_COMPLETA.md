# Análise Completa das APIs do Chainers

## Resumo dos Arquivos Lidos

Todos os 5 arquivos foram lidos COMPLETAMENTE:

| Arquivo | Tamanho | Linhas | Responses | Conteúdo |
|---------|---------|--------|-----------|----------|
| api1.txt | 122,350 chars | 390 linhas | 9 | Configurações, Pools, User Balance |
| api2.txt | 213,630 chars | 553 linhas | 13 | Inventário, Crafting, Depósitos na Pool |
| api3.txt | 108,269 chars | 455 linhas | 10 | Jardins, Crafting, Controles |
| api4.txt | 171,756 chars | 79 linhas | 2 | Fertilizantes, Crafting Offers |
| api5.txt | 266,008 chars | 42 linhas | 1 | Vegetables (rewardPoolBaseWeight) |

**TOTAL: 682,013 caracteres lidos completamente**

---

## Dados Críticos Extraídos

### 1. Todos os 170 Produtos com Valores em BP (rewardPoolBaseWeight)

#### Produtos de Menor Valor (BP 1-10)
- common_strawberry: 1 BP
- common_peas: 1 BP
- uncommon_strawberry: 2 BP
- uncommon_peas: 2 BP
- rare_strawberry: 3 BP
- rare_peas: 3 BP
- common_corn: 2 BP
- uncommon_corn: 3 BP
- epic_peas: 4 BP
- rare_corn: 4 BP

#### Produtos de Maior Valor (BP 1000+)
- uncommon_jack_truffle: 10000 BP
- rare_truffle: 5500 BP
- common_jack_truffle: 9000 BP
- uncommon_truffle: 4755 BP
- legendary_jack_pumpkin: 7360 BP
- epic_jack_pumpkin: 5210 BP
- common_truffle: 4100 BP
- legendary_pineapple: 4510 BP
- epic_pineapple: 2000 BP
- rare_jack_pumpkin: 3060 BP

#### Produtos Mais Valiosos por Categoria

**Milho:**
- common_corn: 2 BP
- legendary_corn: 9 BP (4.5x mais)

**Berinjela:**
- common_eggplant: 8 BP
- legendary_eggplant: 129 BP (16x mais!)

**Melancia:**
- common_watermelon: 51 BP
- legendary_watermelon: 825 BP (16x mais!)

**Lótus Negro:**
- common_black_lotus: 400 BP
- legendary_black_lotus: 10050 BP (25x mais!)

### 2. Reward Pools - Configuração Completa

#### Tipos de Pools e Prêmios por Bloco (4 horas)

**Pool CFB (Chainer Fidelity Bonds):**
- Level 0: 120 CFB por bloco
- Level 1: 1100 CFB por bloco
- Level 2: 1800 CFB por bloco

**Pool IMATIC (Polygon):**
- Level 0: 8 POL por bloco
- Level 1: 100 POL por bloco
- Level 2: 160 POL por bloco

**Pool IBNB (BNB):**
- Level 0: 0.015 BNB por bloco
- Level 1: 0.017 BNB por bloco
- Level 2: 0.0315 BNB por bloco

### 3. Sistema de Níveis de Jogador

- **Level 0** (Muddy Boots): 0 - 10.000 pontos
- **Level 1** (Seed Slinger): 10.001 - 10.000.000 pontos
- **Level 2** (Sprout Tamer): 10.000.001 - 10.000.000.000 pontos

### 4. APIs Mapeadas (Endpoints Completos)

#### Farm Data APIs
- GET `/api/farm/data/seeds` - Definições de sementes (com growthTime)
- GET `/api/farm/data/vegetables` - Definições de vegetais (com rewardPoolBaseWeight)
- GET `/api/farm/data/fertilizers` - Definições de fertilizantes
- GET `/api/farm/data/devices` - Definições de dispositivos (phytolamps)

#### Farm User APIs
- GET `/api/farm/user/gardens` - Estado da fazenda (canteiros, plantações)
- GET `/api/farm/user/inventory` - Inventário completo do usuário
- GET `/api/farm/reward-pools/user-block-vegetables` - Vegetais depositados na pool

#### Farm Control APIs
- POST `/api/farm/control/collect-harvest` - Colher produtos prontos
- POST `/api/farm/control/plant-seed` - Plantar sementes

#### Reward Pools APIs
- GET `/api/farm/reward-pools/general-pools-info` - Informações das pools ativas
- GET `/api/farm/reward-pools/config` - Configuração das pools
- GET `/api/farm/reward-pools/user-level-status` - Nível atual do jogador
- GET `/api/farm/reward-pools/active-blocks-data` - Dados dos blocos ativos
- POST `/api/farm/reward-pools/add-vegetables-to-block` - Depositar na pool

#### Crafting APIs
- GET `/api/main/crafting/groups` - Grupos de crafting (all, farmBeds, farmSeeds, etc.)
- GET `/api/main/crafting/offers` - Ofertas de crafting disponíveis
- GET `/api/main/crafting/user-pending-offers` - Crafts em andamento
- POST `/api/main/crafting/craft` - Iniciar crafting
- POST `/api/main/crafting/claim` - Claimar craft concluído

#### General APIs
- GET `/api/main/general/currencies-config` - Configuração de moedas
- GET `/api/main/user/balance` - Saldo do usuário
- GET `/api/main/user/profile-data` - Perfil do usuário

---

## Como o Bot Usa Essas Informações

### Cálculo de BPM (Biopoints Per Minute)

```go
// farming_handler.go:83-91
rawTimeSeconds := getFloat(s, "growthTimeSeconds")
if rawTimeSeconds == 0 {
    rawTimeSeconds = getFloat(s, "growthTime")
    if rawTimeSeconds == 0 { rawTimeSeconds = 3600 }  // Padrão: 1 hora
}

growthMins := rawTimeSeconds / 60.0
f.bpmTable[seedCode] = weight / growthMins
```

### Exemplo Prático de Cálculo

**Legendary Eggplant em Canteiro Legendary 16x:**
- BP: 129
- Tempo: 60 min (assumindo)
- BPM base: 129 / 60 = 2.15 BPM/min
- BPM com multiplicador 16x: 2.15 × 16 = **34.4 BPM**

**Common Corn em Canteiro Common 1x:**
- BP: 2
- Tempo: 60 min (assumindo)
- BPM base: 2 / 60 = 0.033 BPM/min
- BPM com multiplicador 1x: 0.033 × 1 = **0.033 BPM**

**Diferença: Legendary eggplant é 1042x mais rentável que common corn!**

---

## Estratégia Otimizada do Bot

1. **Matching Otimizado**: Maior BPM → Maior multiplicador de canteiro
2. **Pool Sniper**: Deposita quando ratio > média histórica + margem de segurança
3. **Cache Inteligente**: Inventário com TTL de 10s para reduzir calls à API
4. **Sono Inteligente**: Bot dorme até próximo evento (colheita ou pool crítica)

---

## Headers de Autenticação

```
Authorization: Bearer <token_jwt>
X-Csrf: <token_csrf_json>
X-Request-Token-Id: <token_aleatorio_hex>
Content-Type: application/json (para POST)
```

