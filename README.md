# SmartBot Chainers - Go Edition

Bot automatizado para o jogo **Chainers.io** desenvolvido em Go. O bot gerencia automaticamente atividades de farming, crafting e estratégias de pool com foco na **máxima otimização de recursos**.

## 🆕 What's New - Version V118 (Recovery Edition)

**Última atualização**: 2026-02-01

### Novas Funcionalidades:
- ✅ **Sistema de Logging em Arquivo** - Todos os logs salvos em `logs/smartbot_YYYY-MM-DD.log`
- ✅ **Panic Recovery com Stack Trace** - Captura panics e previne crashes silenciosos
- ✅ **Auto-Restart Inteligente** - Reinicia automaticamente até 10 vezes após crash
- ✅ **Heartbeat Tracking** - Monitoramento de última atividade em `logs/heartbeat.txt`

### Problema Resolvido:
O bot V117 parava inesperadamente sem deixar rastros. A V118 inclui sistemas completos de recuperação e monitoramento.

> 📖 **Veja o [CHANGELOG.md](CHANGELOG.md)** para detalhes completos de todas as melhorias.

---

## 🎮 Sobre o Chainers.io

Chainers é um jogo **free-to-start** multiplayer com economia criativa sustentável, onde você planta sementes, colhe produtos, alimenta animais e participa de **Reward Pools** para ganhar tokens reais (**$CFB, BNB, Polygon (POL), USDT**).

### ⏱️ Ciclo de Reward Pools

As Reward Pools operam em **ciclos de 4 horas** (14.400 segundos):
- Players depositam produtos (medidos em Biopoints/weight) na pool
- Ao final do ciclo, rewards são distribuídos proporcionalmente
- **3 tipos de pools**: CFB, IMATIC (Polygon), IBNB (BNB)
- **3 tiers por pool**: level_0, level_1, level_2
- Seu ganho = `(Seu Weight / Total Weight) × Prêmio do Bloco`

### Biopoints (BP) / Weight - A Moeda do Jogo

Tudo no jogo é medido em **Biopoints (BP)** ou **weight**:
- Cada produto (vegetal, carne, lã, etc.) tem um valor base em BP (`rewardPoolBaseWeight`)
- Os produtos são depositados nas **Reward Pools** para ganhar tokens
- Quanto maior seu weight total, maior sua fatia do prêmio
- **Dica pro**: Verifique o histórico da pool antes de depositar para maximizar ganhos

### Níveis de Jogador

Conforme você deposita nas pools, acumula pontos e sobe de nível:
- **Level 0** (Muddy Boots): 0 - 10.000 pontos
- **Level 1** (Seed Slinger): 10.001 - 10.000.000 pontos
- **Level 2** (Sprout Tamer): 10.000.001 - 10.000.000.000 pontos

Maiores níveis desbloqueiam pools com prêmios maiores!

### Canteiros e Multiplicadores

O jogo possui diferentes raridades de canteiros, cada um com um **multiplicador de produção**:

| Raridade | Multiplicador | Produção |
|----------|---------------|----------|
| **Common** | 1x | 1 unidade por plantio |
| **Uncommon** | 2x | 2 unidades por plantio |
| **Rare** | 4x | 4 unidades por plantio |
| **Epic** | 8x | 8 unidades por plantio |
| **Legendary** | 16x | 16 unidades por plantio |

**Estratégia**: Sempre plantar as melhores sementes nos melhores canteiros!

---

## 🤖 Como o Bot Funciona

### Métrica BPM (Biopoints Per Minute)

O bot usa uma métrica chave chamada **BPM** para decidir o que plantar:

```
BPM = Valor do Produto (BP) / Tempo de Crescimento (minutos)
```

**Exemplo**:
- Tomate: 60 BP, 60 minutos → 1.0 BPM
- Alface: 30 BP, 20 minutos → 1.5 BPM ✅ **Melhor opção!**

O bot calcula o BPM de todas as sementes e sempre planta a mais rentável primeiro!

### Ciclo de Operação do Bot

```
1. SINCRONIZAÇÃO → Busca estado da fazenda
2. COLETA → Colhe produtos prontos
3. CRAFTING → Cria receitas e funde sementes
4. POOL → Analisa Reward Pools (apenas se necessário)
5. PLANTIO → Planta sementes otimizadas
6. RELATÓRIO → Envia status e calcula próximo ciclo
7. SONO → Aguarda até a próxima colheita/pool crítica
```

---

## 🌾 Sistema de Farming Inteligente

### Como Funciona (farming_handler.go)

O bot analisa todas as sementes disponíveis e calcula o BPM de cada uma:

```go
// farming_handler.go:91
f.bpmTable[seedCode] = weight / growthMins
```

**Otimizações implementadas**:

1. **Matching Otimizado** (farming_handler.go:343-345)
   ```go
   // Ordena canteiros vazios por multiplicador (maior primeiro)
   sort.Slice(emptyPlots, func(i, j int) bool {
       return getInt(emptyPlots[i], "multiplier") > getInt(emptyPlots[j], "multiplier")
   })
   ```

2. **Estratégia de Plantio** (farming_handler.go:294-380)
   - Melhor semente → Melhor canteiro (Legendary 16x)
   - Segunda melhor → Segundo melhor (Epic 8x)
   - E assim sucessivamente...

3. **Alimentação de Animais** (farming_handler.go:383-431)
   - Detecta coops/galinheiros/pocilgas vazios
   - Alimenta com a melhor comida disponível
   - Prioriza comida de maior raridade

4. **Colheita em Batch** (farming_handler.go:254-282)
   - Colhe TUDO de uma vez (sem delay entre itens)
   - Delay único APENAS após terminar todas as colheitas
   - Reduz drasticamente o tempo total de operação

### Multiplicadores no Código (farming_handler.go:129-133)

```go
multiplier := 1
if strings.Contains(itemCode, "legendary") { multiplier = 16 } else
if strings.Contains(itemCode, "epic") { multiplier = 8 } else
if strings.Contains(itemCode, "rare") { multiplier = 4 } else
if strings.Contains(itemCode, "uncommon") { multiplier = 2 }
```

---

## 💰 Estratégia de Reward Pool (CFB)

### Como Funciona o Sistema de Pool

As Reward Pools funcionam assim:

1. Os players depositam produtos (medidos em BP) na pool
2. A pool tem um prêmio fixo (ex: 1000 $CFB)
3. Seu ganho = `(Seus BP / Total BP da Pool) × Prêmio`
4. **Ratio** = `Prêmio / Total BP` (quanto maior, melhor!)

**Exemplo**:
- Prêmio: 1,000 $CFB
- Total BP na pool: 1,000,000
- Ratio: 0.001 $CFB por BP
- Se você depositar 10,000 BP → ganha 10 $CFB

### Sniper Automático (pool_strategist.go)

O bot implementa uma estratégia de **"sniper"** para maximizar ganhos:

1. **Monitoramento Contínuo** (pool_strategist.go:62-92)
   - Checa a pool apenas quando necessário (otimização!)
   - Cache de 10 minutos entre verificações normais
   - Verifica a cada 30s durante janela crítica

2. **Janela Crítica** (pool_strategist.go:140-176)
   - Os últimos 45 segundos antes do fechamento da pool
   - Momento de maior oportunidade para sniper

3. **Decisão Inteligente** (pool_strategist.go:178-202)
   ```go
   target := avg * (1 + (p.minProfitMargin / 100))
   if ratio >= target && currentAction != "DEPOSITADO" {
       // Deposita tudo!
   }
   ```

4. **Histórico de Ratios** (pool_strategist.go:239-275)
   - Mantém histórico em `cfb_history.json`
   - Calcula média dos últimos 30 ciclos
   - Usa a média como referência para decidir se é lucrativo

### Otimização de Cache (pool_strategist.go:62-92)

```go
if timeToCritical < 1800 { // < 30 minutos
    shouldCheck = true
} else if timeSinceLastCheck > p.poolCheckInterval {
    shouldCheck = true
}
```

Essa lógica evita chamadas desnecessárias à API, economizando rate limits!

---

## ⚒️ Sistema de Crafting

O bot automatiza o crafting de receitas para criar itens mais valiosos:

- Carrega todas as receitas disponíveis
- Calcula ingredientes necessários
- Protege ingredientes importantes (não usa tudo)
- Sistema de fila para processar múltiplos crafts
- Fundi sementes (merge) para obter sementes de maior nível

---

## ⚡ Cache Inteligente

O bot implementa um sistema de cache para reduzir chamadas API (bot_manager.go:89-107):

```go
func (b *SmartBot) GetCachedInventory() ([]map[string]interface{}, error) {
    if time.Since(b.inventoryCacheTime) < b.inventoryCacheTTL {
        return b.cachedInventory, nil // Retorna cache
    }
    // Busca da API se cache expirou
}
```

**Invalidação Automática**:
- Cache expira após 10 segundos
- Invalida imediatamente após coleta/plantio/crafting
- Garante dados sempre atualizados sem desperdiçar requests

---

## 🛠️ Pré-requisitos

- **Go 1.25.5** ou superior
- **Token de acesso** Chainers.io (JWT)
- **Webhook do Discord** (opcional, para notificações)

## 📦 Instalação

1. Clone o repositório:
```bash
git clone <repository-url>
cd smartbot-go
```

2. Configure o arquivo `config.txt`:
   - Cole seu token JWT do Chainers.io
   - O token deve começar com `"eyJ"`

## ⚙️ Configuração

### Configuração Básica
Edite o arquivo `config.txt` com seu token de autenticação.

### Discord (Opcional)
Crie um arquivo `discord.txt` com o webhook URL do seu servidor Discord para receber notificações.

### Configurações Avançadas

Edite `pool_strategist.go:48` para ajustar a margem mínima de lucro:
```go
minProfitMargin: 10.0, // 10% acima da média histórica
```

## 🚀 Como Executar

Execute o bot com:

```bash
go run .
```

Ou compile e execute:

```bash
go build -o smartbot
./smartbot
```

## 📊 Logs e Monitoramento

O bot V118 possui sistema completo de logging em arquivo:

### Arquivos de Log

**Log Principal**: `logs/smartbot_YYYY-MM-DD.log`
```
[2026-02-01 18:47:12] [INFO] 🌾 Fazenda: 14 Slots | 12 Plantados | 0 Prontos
[2026-02-01 18:47:12] [INFO] 💰 BPM: 107.1 | Próximas: 1m, 8m, 14m | Pool: 123m
[2026-02-01 18:47:12] [INFO] Requests: 19 | Erros: 0 | Taxa de Sucesso: 100.0%
```

**Heartbeat**: `logs/heartbeat.txt`
```
1769982680|2026-02-01 18:51:20
```

### Monitoramento em Tempo Real

```cmd
# Windows CMD
type logs\smartbot_2026-02-01.log

# Windows PowerShell
Get-Content logs\smartbot_*.log -Wait -Tail 20
```

**Legenda**:
- `BPM`: Biopoints por minuto (produção total da fazenda)
- `Próximas`: Tempo até as próximas colheitas
- `Pool`: Tempo até janela crítica da pool
- `Motivo`: O que vai despertar o bot
- `Ciclo`: Tempo de processamento

### Níveis de Log
- **INFO**: Operações normais
- **WARN**: Avisos (ex: retry de API)
- **ERROR**: Erros que não causaram crash
- **PANIC**: Panics recuperados (com stack trace)
- **FATAL**: Erros fatais que encerram o bot

## 🎯 Estratégias Implementadas

### 1. Maximização de BPM
- Sempre plantar a semente de maior BPM no canteiro de maior multiplicador
- Aproveitar ao máximo os multiplicadores 16x, 8x, 4x, 2x dos canteiros
- **O BPM é baseado no produto final** (legumes, ovos, leite, etc.) dividido pelo tempo
- Itens **raros podem ter BPM maior** que itens lendários - depende do valor do produto

### 2. Pool Sniper
- Depositar apenas quando ratio é favorável (acima da média + margem)
- Usar histórico para prever melhores momentos
- Evitar depositar em pools saturadas
- Monitorar janela crítica (últimos 45 segundos do ciclo de 4h)

### 3. Eficiência de Ciclo
- Bot dorme até próximo evento importante (colheita ou pool)
- Reduz consumo de CPU e chamadas API
- Acorda 45s antes da janela crítica da pool
- **Check the timer** - analisa pool sizes no histórico antes de depositar

### 4. Proteção de Inventário
- Manter stock mínimo de comida para animais
- Não depositar tudo na pool (configurável)
- Priorizar crafts que geram mais valor
- **Usar beds eficientemente** - escolher crops otimizados para geração de BioPoints

## 💡 Dicas da Comunidade

Baseado em análises de players experientes:

1. **Maximize BioPoints por Minuto (BPM)**
   - Plante crops de crescimento rápido com bom valor em BP
   - Combine com canteiros de maior multiplicador (16x, 8x, 4x, 2x)
   - **Não assuma que lendário = melhor BPM** - verifique o valor do produto final

2. **Pool Strategy**
   - Deposite nas janelas de 45s antes do fechamento
   - Monitore o histórico de ratios para identificar momentos ótimos
   - Use calculadoras da comunidade para prever retornos

3. **Tier System**
   - Alcance tiers mais altos depositando mais BP
   - Tiers maiores = recompensas proporcionalmente maiores
   - Concentre depósitos em um único ciclo em vez de espalhar

4. **Eficiência de Espaço**
   - Use cada canteiro para maximizar produção de BP
   - Preencha todos os slots antes do ciclo da pool fechar
   - Planeje colheitas para coincidir com janelas de depósito

## 📁 Estrutura do Projeto

```
smartbot-go/
├── main.go                 # Ponto de entrada e loop principal
├── logger.go               # [V118] Sistema de logging em arquivo
├── api_client.go           # Cliente HTTP para API Chainers.io
├── bot_manager.go          # Gerenciador principal e cache
├── farming_handler.go      # Lógica de farming e BPM
├── crafting_handler.go     # Lógica de crafting e merge
├── pool_strategist.go      # Estratégia de pool sniper
├── discord_notifier.go     # Integração com Discord
├── config_manager.go       # Gerenciamento de configurações
├── utils.go                # Funções utilitárias
├── go.mod                  # Módulo Go (sem dependências externas)
├── README.md               # Documentação principal
├── CHANGELOG.md            # [V118] Histórico de mudanças e troubleshooting
├── CHAINERS_API_ANALISE_COMPLETA.md  # Análise detalhada das APIs
├── config.txt              # Token de autenticação
├── discord.txt             # Webhook do Discord (opcional)
├── cfb_history.json        # Histórico de ratios da pool
├── bot_v118.exe            # [V118] Executável compilado
├── bot.exe                 # Executável versão anterior
└── logs/                   # [V118] Diretório de logs
    ├── smartbot_YYYY-MM-DD.log  # Log diário
    └── heartbeat.txt            # Última atividade
```

## 🔧 Troubleshooting

### Bot Parou de Funcionar?

Se o bot parar inesperadamente, siga estes passos:

1. **Verificar o log de erros**
   ```cmd
   type logs\smartbot_*.log | findstr ERROR PANIC FATAL
   ```

2. **Verificar heartbeat**
   ```cmd
   type logs\heartbeat.txt
   ```
   - Se timestamp > 5 minutos no passado, bot pode estar travado

3. **Verificar se processo está rodando**
   ```cmd
   tasklist | findstr bot
   ```

4. **Auto-restart**
   - O bot V118 reinicia automaticamente até 10 vezes após crash
   - Cada restart espera tempo crescente (5s, 10s, 15s...)

> 📖 **Para troubleshooting detalhado, veja [CHANGELOG.md - Checklist de Troubleshooting](CHANGELOG.md#-checklist-de-troubleshooting)**

### Comportamento Esperado

- **Bot "travado"**: Normal! Bot dorme até próximo evento (economiza CPU/API)
- **Logs sem updates**: Verifique heartbeat - bot pode estar dormindo
- **100% sucesso**: Normal! Sistema de retry resolve a maioria dos erros

### Erros Comuns

| Erro | Causa | Solução |
|------|-------|---------|
| `unauthorized - token inválido` | Token expirado | Atualizar `config.txt` |
| `Rate Limit (429)` | Muitas requests | Bot aguarda automaticamente |
| `max retries exceeded` | API instável | Bot tenta novamente após backoff |

---

## 🔒 Segurança e Rate Limits

- **Exponential Backoff** (api_client.go:115): 1s, 2s, 4s, 8s em caso de erro
- **Rate Limit Handling** (api_client.go:142-147): Aguarda automaticamente ao receber 429
- **CSRF Protection** (api_client.go:73-91): Gera token `X-Request-Token-Id` e `X-Csrf` dinâmicos
- **Cookie Management** (api_client.go:31): Mantém sessão automaticamente

## 📡 APIs Principais Utilizadas

| Endpoint | Método | Descrição |
|----------|--------|-----------|
| `/api/farm/data/seeds` | GET | Obter definições de sementes |
| `/api/farm/data/vegetables` | GET | Obter definições de vegetais (valores BP) |
| `/api/farm/user/gardens` | GET | Obter estado da fazenda |
| `/api/farm/user/inventory` | GET | Obter inventário do usuário |
| `/api/farm/control/collect-harvest` | POST | Colher produtos |
| `/api/farm/control/plant-seed` | POST | Plantar sementes |
| `/api/farm/reward-pools/general-pools-info` | GET | Obter informações das pools |
| `/api/farm/reward-pools/add-vegetables-to-block` | POST | Depositar na pool |
| `/api/main/crafting/offers` | GET | Obter receitas de crafting |
| `/api/main/crafting/craft` | POST | Criar item |
| `/api/main/crafting/claim` | POST | Claimar craft concluído |

### Headers Obrigatórios

- `Authorization: Bearer <token>`
- `X-Csrf: <token_csrf>`
- `X-Request-Token-Id: <token_aleatorio>`
- `Content-Type: application/json` (para POST)

## 📚 Referências e Recursos da Comunidade

### Documentação Oficial
- [Chainers.io Official Site](https://chainers.io/)
- [Chainers Web3 Farm](https://chainers.io/web3-farm)
- [Official Documentation](https://docs.chainers.io/chainers-docs)

### Guias e Estratégias
- [Chainers Farm Rework - Medium](https://medium.com/@chainersgame/chainers-farm-rework-3a19e6d64705)
- [Chainers' Reward Pool Strategy - Medium](https://medium.com/@aasgibnev/chainers-reward-pool-revolutionizing-farming-strategy-and-player-rewards-755c7ef041b6)
- [Patch Notes 2.1](https://medium.com/@chainersgame/patch-notes-2-1-366eb021de16)

### Redes Sociais
- [Reward Pools Guide - Twitter](https://x.com/ChainersGame/status/2013612166331461781)
- [BioPoints Farming Tips - Twitter](https://x.com/ChainersGame/status/1988679593189097507)
- [Chainers NFT - Telegram](https://t.me/s/ChainersNFT)

### Calculadoras e Ferramentas da Comunidade
- [Chainer's BioPoint Calculator - YouTube](https://www.youtube.com/watch?v=qK6Yp3JFsxI)
- [Chainers Helper Calculator - YouTube](https://www.youtube.com/watch?v=VlMosBULUy0)
- [Discover Most Profitable Pool - YouTube](https://www.youtube.com/watch?v=bSlp5cyp1kQ)

### Vídeos Tutoriais
- [How to Play Chainers and Profit in 2026](https://www.youtube.com/watch?v=y5QxzQyrN_U)
- [Early Days BIOPOINTS Explained](https://www.youtube.com/watch?v=AH12v41z1iw)
- [Free Crypto Game Farm Crops & USDT](https://www.youtube.com/watch?v=-vTO2ZBooeQ)

## ⚠️ Aviso Legal

Este é um bot automatizado para um jogo blockchain. O uso deste bot pode violar os termos de serviço do jogo Chainers.io. Use por sua conta e risco.

## 📝 Licença

Consulte o arquivo LICENSE para obter informações sobre licença.

## 🤝 Contribuindo

Contribuições são bem-vindas! Sinta-se à vontade para abrir issues e pull requests.

