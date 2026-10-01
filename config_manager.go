package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config contém todas as configurações do bot
type Config struct {
	// Autenticação
	Token     string
	CsrfToken string
	RawCookie string

	// Discord
	DiscordWebhook   string
	DiscordEnabled   bool
	UpdateInterval   int // segundos entre updates no Discord

	// Pool Strategy
	MinProfitMargin  float64 // Margem mínima de lucro para depositar (%)
	AutoDeposit      bool    // Se true, deposita automaticamente
	MinReserve       int     // BP mínimo para manter no inventário

	// Farming
	MinFoodStock     int     // Estoque mínimo de comida antes de craftar
	PreferHighRarity bool    // Se true, prioriza plantar sementes de alta raridade

	// Sistema
	DebugMode        bool    // Modo debug (mais logs)
	MaxRetries       int     // Máximo de tentativas em requests
}

// DefaultConfig retorna configuração padrão
func DefaultConfig() *Config {
	return &Config{
		DiscordEnabled:   false,
		UpdateInterval:   600, // 10 minutos
		MinProfitMargin:  10.0,
		AutoDeposit:      true,
		MinReserve:       0,
		MinFoodStock:     2,
		PreferHighRarity: false,
		DebugMode:        false,
		MaxRetries:       4,
	}
}

// LoadConfig carrega configuração de arquivo
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	// 1. Carrega token obrigatório
	token, err := loadToken()
	if err != nil {
		return nil, err
	}
	cfg.Token = token

	// 2. Tenta carregar webhook do Discord (opcional)
	webhook, err := loadWebhookOptional()
	if err == nil && webhook != "" {
		cfg.DiscordWebhook = webhook
		cfg.DiscordEnabled = true
	}

	// 3. Tenta carregar arquivo avançado de config (opcional)
	if _, err := os.Stat("bot_config.txt"); err == nil {
		loadAdvancedConfig(cfg)
	}

	return cfg, nil
}

func loadToken() (string, error) {
	file, err := os.Open("config.txt")
	if err != nil {
		return "", fmt.Errorf("config.txt não encontrado: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.Contains(line, "eyJ") {
			return line, nil
		}
	}

	return "", fmt.Errorf("token JWT não encontrado no config.txt")
}

func loadWebhookOptional() (string, error) {
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

func loadAdvancedConfig(cfg *Config) {
	file, err := os.Open("bot_config.txt")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Ignora comentários e linhas vazias
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch strings.ToUpper(key) {
		case "CSRF_TOKEN":
			cfg.CsrfToken = value
		case "RAW_COOKIE":
			cfg.RawCookie = value
		case "UPDATE_INTERVAL":
			if v, err := strconv.Atoi(value); err == nil && v > 0 {
				cfg.UpdateInterval = v
			}
		case "MIN_PROFIT_MARGIN":
			if v, err := strconv.ParseFloat(value, 64); err == nil && v >= 0 {
				cfg.MinProfitMargin = v
			}
		case "AUTO_DEPOSIT":
			cfg.AutoDeposit = strings.ToLower(value) == "true"
		case "MIN_RESERVE":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 {
				cfg.MinReserve = v
			}
		case "MIN_FOOD_STOCK":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 {
				cfg.MinFoodStock = v
			}
		case "PREFER_HIGH_RARITY":
			cfg.PreferHighRarity = strings.ToLower(value) == "true"
		case "DEBUG_MODE":
			cfg.DebugMode = strings.ToLower(value) == "true"
		case "MAX_RETRIES":
			if v, err := strconv.Atoi(value); err == nil && v > 0 {
				cfg.MaxRetries = v
			}
		}
	}
}

// UpdateRawCookieInConfig lê o bot_config.txt, atualiza a chave x-csrf dentro de RAW_COOKIE e reescreve
func UpdateRawCookieInConfig(jar http.CookieJar) {
	importUrl, _ := url.Parse("https://chainers.io")
	var newCsrf string
	for _, cookie := range jar.Cookies(importUrl) {
		if cookie.Name == "x-csrf" {
			newCsrf = cookie.Value // Já está url encoded (se não estiver, o próximo passo escapa)
			break
		}
	}
	if newCsrf == "" {
		return
	}

	// Lê todo o conteúdo de bot_config.txt
	content, err := os.ReadFile("bot_config.txt")
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	changed := false

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "RAW_COOKIE=") {
			// O cookie recebido do http.CookieJar já costuma vir url-encoded se o servidor mandou assim.
			// Ou às vezes vem "cru". Pra não ter erro, substituímos do jeito que recebemos, sem alterar as %22.
			parts := strings.Split(line, ";")
			for j, part := range parts {
				if strings.Contains(part, "x-csrf=") {
					parts[j] = " x-csrf=" + newCsrf
					if j == 0 {
						parts[j] = "RAW_COOKIE=x-csrf=" + newCsrf
					}
					changed = true
				}
			}
			if changed {
				lines[i] = strings.Join(parts, ";")
				break
			}
		}
	}

	if changed {
		os.WriteFile("bot_config.txt", []byte(strings.Join(lines, "\n")), 0644)
	}
}

// CreateSampleConfig cria arquivo de exemplo
func CreateSampleConfig() error {
	content := `# SmartBot Chainers - Configuração Avançada (OPCIONAL)
# Deixe este arquivo vazio ou delete-o para usar valores padrão

# Discord (segundos entre updates no Discord)
UPDATE_INTERVAL=600

# Pool Strategy
MIN_PROFIT_MARGIN=10.0   # Margem mínima de lucro (%)
AUTO_DEPOSIT=true         # Depositar automaticamente
MIN_RESERVE=0             # BP mínimo para manter

# Farming
MIN_FOOD_STOCK=2          # Estoque mínimo de comida
PREFER_HIGH_RARITY=false  # Priorizar sementes raras

# Sistema
DEBUG_MODE=false          # Modo debug (mais logs)
MAX_RETRIES=4             # Máximo de tentativas em requests
`

	return os.WriteFile("bot_config.txt.example", []byte(content), 0644)
}

// Print exibe a configuração atual
func (c *Config) Print() {
	fmt.Println("📋 Configuração Carregada:")
	fmt.Printf("  • Token: %s...%s\n", c.Token[:10], c.Token[len(c.Token)-10:])
	fmt.Printf("  • Discord: %v\n", c.DiscordEnabled)
	if c.DiscordEnabled {
		fmt.Printf("    - Update Interval: %ds\n", c.UpdateInterval)
	}
	fmt.Printf("  • Min Profit Margin: %.1f%%\n", c.MinProfitMargin)
	fmt.Printf("  • Auto Deposit: %v\n", c.AutoDeposit)
	fmt.Printf("  • Min Food Stock: %d\n", c.MinFoodStock)
	fmt.Printf("  • Debug Mode: %v\n", c.DebugMode)
	fmt.Println()
}
