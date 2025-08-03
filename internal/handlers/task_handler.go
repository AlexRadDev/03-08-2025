package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"files_archiver/internal/service"
)

// TaskHandler обрабатывает HTTP-запросы для задач
type TaskHandler struct {
	service *service.TaskService
	logger  *slog.Logger
}

// NewTaskHandler создает новый обработчик задач
func NewTaskHandler(service *service.TaskService, logger *slog.Logger) *TaskHandler {
	return &TaskHandler{service: service, logger: logger}
}

// CreateTask создает новую задачу
func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := h.service.CreateTask()
	if err != nil {
		if strings.Contains(err.Error(), "сервер занят") {
			respondWithError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		h.logger.Error("Ошибка создания задачи", "error", err)
		respondWithError(w, http.StatusInternalServerError, "Ошибка создания задачи")
		return
	}

	respondWithJSON(w, http.StatusCreated, map[string]int64{"Задача номер: ": taskID})
}

// AddLinks добавляет ссылки к задаче
func (h *TaskHandler) AddLinks(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		respondWithError(w, http.StatusBadRequest, "Неверный формат URL")
		return
	}
	taskID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Неверный формат ID задачи")
		return
	}

	var req struct {
		URLs []string `json:"urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Неверный формат запроса")
		return
	}

	// Валидация URL
	for _, u := range req.URLs {
		if _, err := url.ParseRequestURI(u); err != nil {
			respondWithError(w, http.StatusBadRequest, "Неверный формат URL")
			return
		}
	}

	if err := h.service.AddLinks(taskID, req.URLs); err != nil {
		switch {
		case errors.Is(err, service.ErrTaskNotFound):
			respondWithError(w, http.StatusNotFound, "Задача не найдена")
		case errors.Is(err, service.ErrTooManyLinks):
			respondWithError(w, http.StatusBadRequest, "В эту задачу уже добавлено 3 ссылки")
		default:
			h.logger.Error("Ошибка добавления ссылок", "error", err)
			respondWithError(w, http.StatusInternalServerError, "Ошибка обработки задачи")
		}
		return
	}

	respondWithJSON(w, http.StatusAccepted, map[string]string{"message": "Ссылки добавлены, обработка начата"})
}

// GetStatus возвращает статус задачи
func (h *TaskHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		respondWithError(w, http.StatusBadRequest, "Неверный формат URL")
		return
	}
	taskID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Неверный формат ID задачи")
		return
	}

	status, err := h.service.GetStatus(taskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			respondWithError(w, http.StatusNotFound, "Задача не найдена")
			return
		}
		h.logger.Error("Ошибка получения статуса", "error", err)
		respondWithError(w, http.StatusInternalServerError, "Ошибка получения статуса")
		return
	}

	respondWithJSON(w, http.StatusOK, status)
}

// respondWithJSON отправляет JSON-ответ
func respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// respondWithError отправляет ошибку в JSON-формате
func respondWithError(w http.ResponseWriter, status int, message string) {
	respondWithJSON(w, status, map[string]string{"error": message})
}
