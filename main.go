package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
)

const (
	maxRestartAttempts = 10
	restartDelay       = 5 * time.Second
)

func main() {
	// Banner
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║   SMARTBOT CHAINERS V126 - GO EDITION   ║")
	fmt.Println("║     Otimizado & Ultra Rápido + Recovery  ║")
	fmt.Println("╚═══════════════════════════════════════════╝")
	fmt.Println()

	// Verifica pré-requisitos
	if _, err := os.Stat("config.txt"); os.IsNotExist(err) {
		fmt.Println("❌ Erro Crítico: Arquivo 'config.txt' não encontrado!")
		fmt.Println("Crie um arquivo config.txt e cole seu Token dentro.")
		os.Exit(1)
	}

	// Inicializa logger
	logger, err := NewLogger("logs", true)
	if err != nil {
		fmt.Printf("⚠️  Não foi possível inicializar logger de arquivo: %v\n", err)
		fmt.Println("Continuando apenas com log no console...")
		logger = &Logger{console: true}
	}
	defer logger.Close()

	logger.Info(">>> SmartBot Chainers V125 (Go Optimized + Recovery) Iniciado <<<")

	// Loop de auto-restart
	restartCount := 0
	for restartCount < maxRestartAttempts {
		if runBot(logger) {
			// Saída normal (Ctrl+C ou parada intencional)
			logger.Info("Bot encerrado normalmente pelo usuário.")
			break
		}

		// Saída por crash
		restartCount++
		if restartCount < maxRestartAttempts {
			waitTime := restartDelay * time.Duration(restartCount)
			logger.Warn(fmt.Sprintf("Bot crashed! Reiniciando em %v (tentativa %d/%d)...",
				waitTime, restartCount, maxRestartAttempts))
			time.Sleep(waitTime)
			logger.Info("─────────────────────────────────────────")
			logger.Info(fmt.Sprintf("Reiniciando bot... (tentativa %d)", restartCount))
		} else {
			logger.Fatal(fmt.Sprintf("Bot atingiu limite de %d reinícios. Abortando.", maxRestartAttempts))
		}
	}
}

// runBot executa o bot e retorna true se foi encerrado normalmente, false se houve crash
func runBot(logger *Logger) (normalExit bool) {
	// Recupera panics
	defer func() {
		if r := recover(); r != nil {
			stackTrace := string(debug.Stack())
			logger.LogPanic(r, stackTrace)
			normalExit = false
		}
	}()

	// Inicializa o bot
	bot, err := NewSmartBotWithLogger(logger)
	if err != nil {
		logger.Fatal(fmt.Sprintf("❌ Erro ao inicializar bot: %v", err))
		return false
	}

	// Setup para capturar Ctrl+C graceful
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Canal para erros fatais
	errChan := make(chan error, 1)

	// Goroutine para executar o bot
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic: %v", r)
			}
		}()
		if err := bot.Run(); err != nil {
			errChan <- err
		}
	}()

	// Aguarda sinal de interrupção ou erro fatal
	select {
	case <-sigChan:
		logger.Info("🛑 Bot interrompido pelo usuário.")
		logger.Info("💾 Salvando estado...")
		// Cleanup se necessário
		return true // Saída normal
	case err := <-errChan:
		logger.Error(fmt.Sprintf("❌ Erro Fatal: %v", err))
		return false // Saída por crash
	}
}

