package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"files_archiver/internal/config"
	"files_archiver/internal/handlers"
	"files_archiver/internal/repository"
	"files_archiver/internal/service"
)

const (
	envPath = "../.env"
)

func main() {
	// Инициализация структурированного логгера
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Загрузка конфигурации
	cfg, err := config.Load(envPath)
	if err != nil {
		logger.Error("Ошибка загрузки конфигурации", "error", err)
		os.Exit(1)
	}

	// Инициализация репозитория
	repo := repository.NewInMemoryRepository()

	taskService := service.NewTaskService(repo, cfg, logger)

	taskHandler := handlers.NewTaskHandler(taskService, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", taskHandler.CreateTask)
	mux.HandleFunc("POST /tasks/{id}/links", taskHandler.AddLinks)
	mux.HandleFunc("GET /tasks/{id}/status", taskHandler.GetStatus)

}
