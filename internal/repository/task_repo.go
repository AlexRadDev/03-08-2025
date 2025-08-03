package repository

import (
	"errors"
	"sync"
)

// Task представляет задачу скачивания и архивации
type Task struct {
	ID          int64
	Status      string
	URLs        []string
	ArchivePath string // ссылка на созданный zip архив
}

// InMemoryRepository хранит задачи в памяти
type InMemoryRepository struct {
	Tasks map[int64]Task
	mu    sync.RWMutex
}

// NewInMemoryRepository создает новый репозиторий
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		Tasks: make(map[int64]Task),
	}
}

// SaveTask сохраняет задачу
func (r *InMemoryRepository) SaveTask(task Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Tasks[task.ID] = task

	return nil
}

// GetTask получает задачу по ID
func (r *InMemoryRepository) GetTask(id int64) (Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.Tasks[id]
	if !exists {
		return Task{}, errors.New("задача не найдена")
	}

	return task, nil
}
