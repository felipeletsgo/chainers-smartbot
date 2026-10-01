package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Logger struct {
	mu           sync.Mutex
	logFile      *os.File
	logFilePath  string
	console      bool
	lastActivity time.Time
	heartbeatFile string
}

func NewLogger(logDir string, enableConsole bool) (*Logger, error) {
	// Cria diretório de logs se não existe
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Nome do arquivo com data atual
	dateStr := time.Now().Format("2006-01-02")
	logPath := filepath.Join(logDir, fmt.Sprintf("smartbot_%s.log", dateStr))

	// Abre arquivo em modo append
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := &Logger{
		logFile:      file,
		logFilePath:  logPath,
		console:      enableConsole,
		lastActivity: time.Now(),
		heartbeatFile: filepath.Join(logDir, "heartbeat.txt"),
	}

	// Inicializa heartbeat
	logger.updateHeartbeat()

	return logger, nil
}

func (l *Logger) updateHeartbeat() {
	l.mu.Lock()
	defer l.mu.Unlock()

	heartbeat := fmt.Sprintf("%d|%s", time.Now().Unix(), time.Now().Format("2006-01-02 15:04:05"))
	os.WriteFile(l.heartbeatFile, []byte(heartbeat), 0644)
	l.lastActivity = time.Now()
}

func (l *Logger) GetLastActivity() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastActivity
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

func (l *Logger) log(level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, message)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Escreve no arquivo
	if l.logFile != nil {
		l.logFile.WriteString(logLine)
	}

	// Escreve no console
	if l.console {
		fmt.Print(logLine)
	}

	// Atualiza heartbeat
	l.lastActivity = time.Now()

	// Flush forçado para garantir que logs não se percam em caso de crash
	if l.logFile != nil {
		l.logFile.Sync()
	}
}

func (l *Logger) Info(message string) {
	l.log("INFO", message)
}

func (l *Logger) Error(message string) {
	l.log("ERROR", message)
}

func (l *Logger) Warn(message string) {
	l.log("WARN", message)
}

func (l *Logger) Debug(message string) {
	l.log("DEBUG", message)
}

func (l *Logger) Fatal(message string) {
	l.log("FATAL", message)
	l.Close()
	os.Exit(1)
}

func (l *Logger) LogPanic(panicMsg interface{}, stackTrace string) {
	l.log("PANIC", fmt.Sprintf("Recovered from panic: %v\n%s", panicMsg, stackTrace))
}

