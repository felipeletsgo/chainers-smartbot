package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ChainersAPIClient struct {
	BaseURL      string
	Token        string
	HTTPClient   *http.Client
	Headers      map[string]string
	headersMu    sync.RWMutex
	RequestCount int // Contador de requests para debug
	ErrorCount   int // Contador de erros
	csrfTokenValue string
	rawCookieString string
}

func NewChainersAPIClient(token string, csrfToken string, rawCookie string) *ChainersAPIClient {
	// Semente para aleatoriedade
	mrand.Seed(time.Now().UnixNano())

	// CookieJar para gerenciar sessão
	jar, _ := cookiejar.New(nil)

	client := &ChainersAPIClient{
		BaseURL: "https://chainers.io/api",
		Token:   token,
		csrfTokenValue: csrfToken,
		rawCookieString: rawCookie,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
			// Desabilita redirect automático para ter mais controle
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("stopped after 3 redirects")
				}
				return nil
			},
		},
		Headers: map[string]string{
			"Authorization":      fmt.Sprintf("Bearer %s", token),
			"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36",
			"Accept":             "application/json",
			"Referer":            "https://chainers.io/game",
			"Origin":             "https://chainers.io",
			"Sec-Ch-Ua":          `"Brave";v="149", "Chromium";v="149", "Not)A;Brand";v="24"`,
			"Sec-Ch-Ua-Mobile":   "?0",
			"Sec-Ch-Ua-Platform": `"Windows"`,
			"Sec-Fetch-Dest":     "empty",
			"Sec-Fetch-Mode":     "cors",
			"Sec-Fetch-Site":     "same-origin",
			"Sec-Gpc":            "1",
			"Priority":           "u=1, i",
		},
		RequestCount: 0,
		ErrorCount:   0,
	}
	return client
}

// InitSession faz uma requisição inicial e injeta um cookie CSRF sintético (Double Submit pattern)
func (c *ChainersAPIClient) InitSession() error {
	req, err := http.NewRequest("GET", "https://chainers.io/", nil)
	if err != nil {
		return err
	}
	
	req.Header.Set("User-Agent", c.Headers["User-Agent"])
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to init session: %w", err)
	}
	defer resp.Body.Close()

	u, _ := url.Parse("https://chainers.io")
	cookies := []*http.Cookie{}
	hasRealCsrf := false
	csrfFromRaw := ""

	if c.rawCookieString != "" {
		// Divide o raw cookie e adiciona ao jar
		parts := strings.Split(c.rawCookieString, "; ")
		for _, part := range parts {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				name := strings.TrimSpace(kv[0])
				val := strings.TrimSpace(kv[1])

				if name == "x-csrf" {
					hasRealCsrf = true
					if unescaped, err := url.QueryUnescape(val); err == nil {
						csrfFromRaw = unescaped
					} else {
						csrfFromRaw = val
					}
					// Se achou no raw, não usamos o sintético!
				}

				cookies = append(cookies, &http.Cookie{
					Name:   name,
					Value:  val,
					Domain: "chainers.io",
					Path:   "/",
				})
			}
		}
	}

	// Se tem x-csrf no Raw, usa ele. Se não, tenta o c.csrfTokenValue
	csrfJSON := ""
	if hasRealCsrf {
		csrfJSON = csrfFromRaw
	} else if c.csrfTokenValue != "" {
		csrfJSON = c.csrfTokenValue
		if strings.Contains(csrfJSON, "%") {
			if decoded, err := url.QueryUnescape(csrfJSON); err == nil {
				csrfJSON = decoded
			}
		}
	}

	// Se ele NÃO foi adicionado pelo Raw Cookie, a gente adiciona manualmente
	if !hasRealCsrf {
		cookieValue := strings.ReplaceAll(url.QueryEscape(csrfJSON), "%3A", ":")
		cookies = append(cookies, &http.Cookie{
			Name:   "x-csrf",
			Value:  cookieValue,
			Domain: "chainers.io",
			Path:   "/",
		})
	}

	// Decodifica o JWT para extrair userID e sessionID se precisar
	var userID, sessionID string
	parts := strings.Split(c.Token, ".")
	if len(parts) == 3 {
		if payload, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
			var claims map[string]interface{}
			if err := json.Unmarshal(payload, &claims); err == nil {
				if v, ok := claims["usersID"].(string); ok {
					userID = v
				}
				if v, ok := claims["sessionID"].(string); ok {
					sessionID = v
				}
			}
		}
	}

	if c.rawCookieString == "" {
		cookies = append(cookies, &http.Cookie{
			Name:   "accessToken",
			Value:  c.Token,
			Domain: "chainers.io",
			Path:   "/",
		})
		if userID != "" {
			cookies = append(cookies, &http.Cookie{
				Name:   "userID",
				Value:  userID,
				Domain: "chainers.io",
				Path:   "/",
			})
		}
		if sessionID != "" {
			cookies = append(cookies, &http.Cookie{
				Name:   "session_id",
				Value:  sessionID,
				Domain: "chainers.io",
				Path:   "/",
			})
		}
	}

	c.HTTPClient.Jar.SetCookies(u, cookies)
	
	// Atualiza os headers imediatamente com o novo token CSRF
	c.updateDynamicHeaders("GET")

	return nil
}

func (c *ChainersAPIClient) generateTokenID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "9b25" + hex.EncodeToString(b)
}

func (c *ChainersAPIClient) updateDynamicHeaders(method string) {
	c.headersMu.Lock()
	defer c.headersMu.Unlock()

	c.Headers["X-Request-Token-Id"] = c.generateTokenID()

	// Busca CSRF do CookieJar
	u, _ := url.Parse(c.BaseURL)
	cookies := c.HTTPClient.Jar.Cookies(u)
	for _, cookie := range cookies {
		if cookie.Name == "x-csrf" {
			unescaped, _ := url.QueryUnescape(cookie.Value)
			c.Headers["X-Csrf"] = unescaped
			break
		}
	}

	if method == "GET" {
		delete(c.Headers, "Content-Type")
	} else if method == "POST" {
		c.Headers["Content-Type"] = "application/json"
	}
}

// get com retry exponencial
func (c *ChainersAPIClient) get(endpoint string, retries int) (map[string]interface{}, error) {
	fullURL := c.BaseURL + endpoint

	for attempt := 0; attempt < retries; attempt++ {
		c.updateDynamicHeaders("GET")
		c.RequestCount++

		req, err := http.NewRequest("GET", fullURL, nil)
		if err != nil {
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		c.headersMu.RLock()
		for k, v := range c.Headers {
			req.Header.Set(k, v)
		}
		c.headersMu.RUnlock()

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			c.ErrorCount++
			backoff := time.Duration(1<<uint(attempt)) * time.Second // Exponencial: 1s, 2s, 4s, 8s
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			time.Sleep(backoff)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			continue
		}

		// Status codes
		switch resp.StatusCode {
		case 200:
			var result map[string]interface{}
			if err := json.Unmarshal(body, &result); err != nil {
				return nil, fmt.Errorf("failed to unmarshal: %w", err)
			}
			return result, nil

		case 401:
			return nil, fmt.Errorf("unauthorized - token inválido ou expirado")

		case 429:
			// Rate limit - aumenta tempo de espera
			waitTime := time.Duration(5*(attempt+1)) * time.Second
			fmt.Printf("⚠️  Rate Limit (429). Aguardando %v...\n", waitTime)
			time.Sleep(waitTime)
			continue

		case 500, 502, 503, 504:
			// Server errors - retry
			time.Sleep(time.Duration(2*(attempt+1)) * time.Second)
			continue

		default:
			// Outros erros
			fmt.Printf("⚠️  HTTP %d: %s\n", resp.StatusCode, string(body))
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}

	return nil, fmt.Errorf("max retries (%d) exceeded for %s", retries, endpoint)
}

// post com melhor handling e retry para CSRF
func (c *ChainersAPIClient) post(endpoint string, payload interface{}) (map[string]interface{}, error) {
	fullURL := c.BaseURL + endpoint
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	var result map[string]interface{}
	var lastErr error

	for attempt := 0; attempt < 4; attempt++ {
		c.updateDynamicHeaders("POST")
		c.RequestCount++

		req, err := http.NewRequest("POST", fullURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}

		c.headersMu.RLock()
		for k, v := range c.Headers {
			req.Header.Set(k, v)
		}
		c.headersMu.RUnlock()

		// Delay randômico humanizado
		time.Sleep(time.Duration(300+mrand.Intn(400)) * time.Millisecond)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			c.ErrorCount++
			lastErr = fmt.Errorf("request failed: %w", err)
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			time.Sleep(backoff)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = fmt.Errorf("failed to unmarshal response: %w", err)
			continue
		}

		if resp.StatusCode != 200 && resp.StatusCode != 201 {
			if resp.StatusCode == 403 {
				fmt.Printf("⚠️  POST %s retornou 403. O Token CSRF é inválido ou expirou. Atualize RAW_COOKIE no bot_config.txt!\n", endpoint)
				return result, fmt.Errorf("HTTP 403: CSRF token inválido. Atualize bot_config.txt")
			}

			// Se for erro de servidor ou rate limit, aplica backoff
			if resp.StatusCode >= 500 || resp.StatusCode == 429 {
				fmt.Printf("⚠️  POST %s returned %d. Aplicando backoff...\n", endpoint, resp.StatusCode)
				backoff := time.Duration(1<<uint(attempt)) * time.Second
				if resp.StatusCode == 429 {
					backoff = time.Duration(5*(attempt+1)) * time.Second
				}
				time.Sleep(backoff)
				continue
			}

			fmt.Printf("⚠️  POST %s returned %d: %s\n", endpoint, resp.StatusCode, string(body))
			return result, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Atualiza o cookie no config APENAS se o servidor mandou um NOVO x-csrf via Set-Cookie
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "x-csrf" {
				UpdateRawCookieInConfig(c.HTTPClient.Jar)
				break
			}
		}

		return result, nil
	}

	return result, lastErr
}

// Helper para extrair listas de diferentes estruturas de resposta
func extractItems(result map[string]interface{}) []map[string]interface{} {
	if result == nil {
		return []map[string]interface{}{}
	}

	// Caso 1: result["items"] na raiz
	if items, ok := result["items"].([]interface{}); ok {
		return convertToMapSlice(items)
	}

	// Caso 2: result["data"]
	if data, ok := result["data"]; ok {
		// Subcaso 2a: result["data"]["items"]
		if dataMap, ok := data.(map[string]interface{}); ok {
			if items, ok := dataMap["items"].([]interface{}); ok {
				return convertToMapSlice(items)
			}
		}
		// Subcaso 2b: result["data"] é lista direta
		if dataList, ok := data.([]interface{}); ok {
			return convertToMapSlice(dataList)
		}
	}

	return []map[string]interface{}{}
}

func convertToMapSlice(input []interface{}) []map[string]interface{} {
	output := make([]map[string]interface{}, 0, len(input))
	for _, item := range input {
		if m, ok := item.(map[string]interface{}); ok {
			output = append(output, m)
		}
	}
	return output
}

// ==================== MÉTODOS DE NEGÓCIO ====================

func (c *ChainersAPIClient) GetSeedDefinitions() ([]map[string]interface{}, error) {
	result, err := c.get("/farm/data/seeds", 4)
	return extractItems(result), err
}

func (c *ChainersAPIClient) GetItemDefinitions() ([]map[string]interface{}, error) {
	result, err := c.get("/farm/data/vegetables", 4)
	return extractItems(result), err
}

func (c *ChainersAPIClient) GetBedDefinitions() ([]map[string]interface{}, error) {
	result, err := c.get("/farm/data/beds", 4)
	return extractItems(result), err
}

func (c *ChainersAPIClient) GetFarmState() ([]map[string]interface{}, error) {
	result, err := c.get("/farm/user/gardens", 4)
	return extractItems(result), err
}

func (c *ChainersAPIClient) GetUserInventory() ([]map[string]interface{}, error) {
	result, err := c.get("/farm/user/inventory?sort=lastUpdated&itemType=all&sortDirection=-1&skip=0&limit=0", 4)
	return extractItems(result), err
}

func (c *ChainersAPIClient) CollectHarvest(farmingID string) (map[string]interface{}, error) {
	return c.post("/farm/control/collect-harvest", map[string]interface{}{
		"userFarmingID": farmingID,
	})
}

func (c *ChainersAPIClient) PlantSeed(gardenID, bedID, seedID string) (map[string]interface{}, error) {
	return c.post("/farm/control/plant-seed", map[string]interface{}{
		"userGardensID": gardenID,
		"userBedsID":    bedID,
		"seedID":        seedID,
	})
}

func (c *ChainersAPIClient) GetCraftingOffers() ([]map[string]interface{}, error) {
	allOffers := make([]map[string]interface{}, 0)
	limit := 100
	skip := 0

	for {
		res, err := c.get(fmt.Sprintf("/main/crafting/offers?limit=%d&skip=%d&groupCode=all&withRequiredItemsSorting=false", limit, skip), 4)
		if err != nil {
			break
		}

		items := extractItems(res)
		if len(items) == 0 {
			break
		}

		allOffers = append(allOffers, items...)

		if len(items) < limit {
			break
		}
		skip += limit

		// Pequeno delay para não sobrecarregar API em paginação
		time.Sleep(200 * time.Millisecond)
	}

	return allOffers, nil
}

func (c *ChainersAPIClient) GetActiveCrafts() ([]map[string]interface{}, error) {
	result, err := c.get("/main/crafting/user-pending-offers?skip=0&limit=50&groupCode=all", 4)
	return extractItems(result), err
}

func (c *ChainersAPIClient) StartCraft(offerID, recipeID string) (map[string]interface{}, error) {
	return c.post("/main/crafting/craft", map[string]interface{}{
		"craftingOffersID":  offerID,
		"craftingRecipesID": recipeID,
		"count":             1,
	})
}

func (c *ChainersAPIClient) ClaimCraft(queueID string) (map[string]interface{}, error) {
	return c.post("/main/crafting/claim", map[string]interface{}{
		"usersCraftingOffersIDs": []string{queueID},
	})
}

func (c *ChainersAPIClient) GetPoolStatus() ([]map[string]interface{}, error) {
	result, err := c.get("/farm/reward-pools/active-blocks-data", 4)
	if err != nil {
		return nil, err
	}

	// Tenta extrair de 'data' (lista direta)
	if data, ok := result["data"].([]interface{}); ok {
		return convertToMapSlice(data), nil
	}

	// Tenta extrair de 'items' (se usarem)
	if items, ok := result["items"].([]interface{}); ok {
		return convertToMapSlice(items), nil
	}

	// Fallback: se result é um mapa único com info da pool
	if len(result) > 0 {
		return []map[string]interface{}{result}, nil
	}

	return []map[string]interface{}{}, nil
}

func (c *ChainersAPIClient) DepositToPool(blockID string, items []map[string]interface{}) (map[string]interface{}, error) {
	return c.post("/farm/reward-pools/add-vegetables-to-block", map[string]interface{}{
		"rewardsPoolsBlocksID": blockID,
		"vegetables":           items,
	})
}

// ==========================================
// MISSIONS & REWARDS
// ==========================================

func (c *ChainersAPIClient) GetUserEventsStatus() ([]interface{}, error) {
	resp, err := c.get("/missions/user/user-events-status?location=hub", 3)
	if err != nil {
		return nil, err
	}
	if data, ok := resp["data"].([]interface{}); ok {
		return data, nil
	}
	return nil, fmt.Errorf("invalid data format for user-events-status")
}

func (c *ChainersAPIClient) GetTasksProgress(parentCode string) ([]interface{}, error) {
	url := fmt.Sprintf("/missions/user/tasks-progress?parentCode=%s", parentCode)
	resp, err := c.get(url, 3)
	if err != nil {
		return nil, err
	}
	if data, ok := resp["data"].([]interface{}); ok {
		return data, nil
	}
	return nil, fmt.Errorf("invalid data format for tasks-progress")
}

func (c *ChainersAPIClient) ClaimMissionReward(tasksID string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"tasksID": tasksID,
	}
	return c.post("/missions/control/claim-reward", payload)
}

// GetStats retorna estatísticas do cliente
func (c *ChainersAPIClient) GetStats() string {
	successRate := 100.0
	if c.RequestCount > 0 {
		successRate = float64(c.RequestCount-c.ErrorCount) / float64(c.RequestCount) * 100
	}
	return fmt.Sprintf("Requests: %d | Erros: %d | Taxa de Sucesso: %.1f%%",
		c.RequestCount, c.ErrorCount, successRate)
}



