package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config содержит настройки приложения
type Config struct {
	ServerPort            int           `yaml:"port"`
	ServerTimeout         time.Duration `yaml:"timeout"`
	FileMaxSize           int64         `yaml:"max_size"`
	FileAllowedExtensions []string      `yaml:"allowed_extensions"`
}

// Load загружает конфигурацию из .env файла
func Load(path string) (*Config, error) {
	cfg := &Config{}

	// Чтение .env файла
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла .env: %w", err)
	}

	// Парсинг строк .env
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "SERVER_PORT":
			port, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("ошибка парсинга SERVER_PORT: %w", err)
			}
			if port <= 0 {
				return nil, fmt.Errorf("SERVER_PORT должен быть положительным числом")
			}
			cfg.ServerPort = port
		case "SERVER_TIMEOUT":
			timeout, err := time.ParseDuration(value)
			if err != nil {
				return nil, fmt.Errorf("ошибка парсинга SERVER_TIMEOUT: %w", err)
			}
			if timeout <= 0 {
				return nil, fmt.Errorf("SERVER_TIMEOUT должен быть положительным")
			}
			cfg.ServerTimeout = timeout
		case "FILE_MAX_SIZE":
			maxSize, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("ошибка парсинга FILE_MAX_SIZE: %w", err)
			}
			if maxSize <= 0 {
				return nil, fmt.Errorf("FILE_MAX_SIZE должен быть положительным")
			}
			cfg.FileMaxSize = maxSize
		case "FILE_ALLOWED_EXTENSIONS":
			extensions := strings.Split(strings.Trim(value, "[]"), ",")
			for i, ext := range extensions {
				ext = strings.TrimSpace(ext)
				if ext == "" {
					return nil, fmt.Errorf("FILE_ALLOWED_EXTENSIONS содержит пустое значение")
				}
				extensions[i] = ext
			}
			if len(extensions) == 0 {
				return nil, fmt.Errorf("FILE_ALLOWED_EXTENSIONS не может быть пустым")
			}
			cfg.FileAllowedExtensions = extensions
		}
	}

	// Проверка, что все обязательные поля заполнены
	if cfg.ServerPort == 0 {
		return nil, fmt.Errorf("SERVER_PORT не указан")
	}
	if cfg.ServerTimeout == 0 {
		return nil, fmt.Errorf("SERVER_TIMEOUT не указан")
	}
	if cfg.FileMaxSize == 0 {
		return nil, fmt.Errorf("FILE_MAX_SIZE не указан")
	}
	if len(cfg.FileAllowedExtensions) == 0 {
		return nil, fmt.Errorf("FILE_ALLOWED_EXTENSIONS не указан")
	}

	return cfg, nil
}
