package service

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"files_archiver/internal/config"
	"files_archiver/internal/repository"
)

// адрес файла .env
const (
	zipPath       = "../archives"
	folderPath    = "archives/"
	folderPathImg = "../downloads"
)

var (
	ErrTaskNotFound = errors.New("задача не найдена")
	ErrTooManyLinks = errors.New("превышено максимальное количество ссылок (максимум 3)")
	ErrInvalidFile  = errors.New("недопустимый тип файла")
	ErrFileTooLarge = errors.New("файл слишком большой")
	ErrServerBusy   = errors.New("сервер занят: максимум 3 активные задачи")
)

// TaskService управляет задачами скачивания и архивации
type TaskService struct {
	repo               *repository.InMemoryRepository
	cfg                *config.Config
	logger             *slog.Logger
	taskCounter        int64
	activeTasksCounter int64
	filesCounter       int64
	urlsByTask         map[int64][]string // Репозиторий ссылок для каждой задачи
	urlsMu             sync.RWMutex
}

// NewTaskService создает новый сервис задач
func NewTaskService(repo *repository.InMemoryRepository, cfg *config.Config, logger *slog.Logger) *TaskService {
	return &TaskService{
		repo:               repo,
		cfg:                cfg,
		logger:             logger,
		urlsByTask:         make(map[int64][]string),
		taskCounter:        0,
		filesCounter:       0,
		activeTasksCounter: 0,
	}
}

// CreateTask создает новую задачу
func (s *TaskService) CreateTask() (int64, error) {
	if atomic.LoadInt64(&s.activeTasksCounter) >= 3 {
		return 0, ErrServerBusy
	}
	atomic.AddInt64(&s.activeTasksCounter, 1)

	taskID := atomic.AddInt64(&s.taskCounter, 1)

	zipName := ""
	if taskID < 10 {
		zipName = "Task_" + "0" + strconv.FormatInt(taskID, 10)
	} else {
		zipName = "Task_" + strconv.FormatInt(taskID, 10)
	}
	archivePath := zipPath + "/" + zipName

	task := repository.Task{
		ID:          taskID,
		Status:      "создали",
		ArchivePath: archivePath,
	}

	if err := s.repo.SaveTask(task); err != nil {
		return 0, err
	}

	s.urlsMu.Lock()
	s.urlsByTask[taskID] = []string{}
	s.urlsMu.Unlock()

	return taskID, nil
}

// AddLinks добавляет ссылку к задаче и запускает скачивание, если ссылок меньше 3
func (s *TaskService) AddLinks(taskID int64, urls []string) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return ErrTaskNotFound
	}

	s.urlsMu.RLock()
	currentURLs := s.urlsByTask[taskID]
	s.urlsMu.RUnlock()

	if len(currentURLs) >= 3 {
		return ErrTooManyLinks
	}

	if len(currentURLs)+len(urls) > 3 {
		return ErrTooManyLinks
	}

	s.urlsMu.Lock()
	s.urlsByTask[taskID] = append(s.urlsByTask[taskID], urls...)
	task.Status = "в процессе"
	if err := s.repo.SaveTask(task); err != nil {
		s.urlsMu.Unlock()
		return err
	}
	s.urlsMu.Unlock()

	return nil
}

// GetStatus возвращает статус задачи
func (s *TaskService) GetStatus(taskID int64) (map[string]string, error) {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return nil, ErrTaskNotFound
	}

	s.urlsMu.RLock()
	urlCount := len(s.urlsByTask[taskID])
	s.urlsMu.RUnlock()

	result := map[string]string{
		"status": "в процессе",
	}

	if urlCount >= 3 {

		var files []string

		s.urlsMu.RLock()

		for _, u := range s.urlsByTask[taskID] {
			filePath, err := s.downloadFile(u, taskID)
			if err != nil {
				s.logger.Warn("Ошибка скачивания файла при создании архива", "url", u, "error", err)
				continue
			}
			files = append(files, filePath)
		}
		s.urlsMu.RUnlock()

		if len(files) == 0 {
			task.Status = "неуспешно"
			if err := s.repo.SaveTask(task); err != nil {
				s.logger.Error("Ошибка обновления статуса задачи", "task_id", taskID, "error", err)
			}
			return map[string]string{"status": "failed"}, nil
		}

		archivePath, err := s.createZipArchive(taskID, files)
		if err != nil {
			s.logger.Error("Ошибка создания архива", "task_id", taskID, "error", err)
			task.Status = "неуспешно"
			if err := s.repo.SaveTask(task); err != nil {
				s.logger.Error("Ошибка обновления статуса задачи", "task_id", taskID, "error", err)
			}
			return map[string]string{"status": "неуспешно"}, nil
		}

		err = deleteFiles(files)
		if err != nil {
			s.logger.Error("Ошибка удаления файлов", "task_id", taskID, "error", err)
		}

		task.Status = "завершена"

		atomic.AddInt64(&s.activeTasksCounter, -1)

		task.ArchivePath = archivePath
		if err := s.repo.SaveTask(task); err != nil {
			s.logger.Error("Ошибка обновления задачи", "task_id", taskID, "error", err)
		}
		result["status"] = "завершена"
		result["archive_url"] = fmt.Sprintf("http://localhost:%d/%s", s.cfg.ServerPort, task.ArchivePath)
	}

	return result, nil
}

// Функция deleteFiles удаляет все файлы, указанные в слайсе путей.
func deleteFiles(files []string) error {
	for _, filePath := range files {
		if _, err := os.Stat(filePath); err == nil {
			err := os.Remove(filePath)
			if err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// downloadFile скачивает файл по URL
func (s *TaskService) downloadFile(url string, taskID int64) (string, error) {
	ext := strings.ToLower(filepath.Ext(url))
	isAllowed := false
	for _, allowed := range s.cfg.FileAllowedExtensions {
		if ext == "."+allowed {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		start := time.Now()
		req, err := http.NewRequest(http.MethodHead, url, nil)
		if err != nil {
			s.logger.Warn("Ошибка создания HEAD-запроса", "url", url, "error", err)
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			s.logger.Warn("Ошибка выполнения HEAD-запроса", "url", url, "error", err, "duration", time.Since(start))
			return "", err
		}
		defer resp.Body.Close()

		s.logger.Info("HEAD-запрос выполнен", "url", url, "duration", time.Since(start))
		contentType := resp.Header.Get("Content-Type")
		for _, allowed := range s.cfg.FileAllowedExtensions {
			if strings.Contains(contentType, allowed) {
				isAllowed = true
				ext = "." + allowed
				break
			}
		}
	}

	if !isAllowed {
		return "", ErrInvalidFile
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.ContentLength > s.cfg.FileMaxSize {
		return "", ErrFileTooLarge
	}

	preffileName := ""

	if taskID < 10 {
		preffileName = "Task_" + "0" + strconv.FormatInt(taskID, 10) + "_" + strconv.FormatInt(atomic.AddInt64(&s.filesCounter, 1), 10)
	} else {
		preffileName = "Task_" + strconv.Itoa(int(taskID)) + "_" + strconv.FormatInt(atomic.AddInt64(&s.filesCounter, 1), 10)
	}

	fileName := fmt.Sprintf("%s/%s%s", folderPathImg, preffileName, ext)
	if err := os.MkdirAll(folderPathImg, 0755); err != nil {
		return "", err
	}

	file, err := os.Create(fileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}

	return fileName, nil
}

// createZipArchive создает ZIP-архив из скачанных файлов
func (s *TaskService) createZipArchive(taskID int64, files []string) (string, error) {
	archiveName := fmt.Sprintf("%s.zip", s.repo.Tasks[taskID].ArchivePath)

	if err := os.MkdirAll(zipPath, 0755); err != nil {
		return "", err
	}

	archive, err := os.Create(archiveName)
	if err != nil {
		return "", err
	}
	defer archive.Close()

	zipWriter := zip.NewWriter(archive)
	defer zipWriter.Close()

	for _, filePath := range files {
		file, err := os.Open(filePath)
		if err != nil {
			s.logger.Warn("Ошибка открытия файла для архивации", "file", filePath, "error", err)
			continue
		}
		defer file.Close()

		writer, err := zipWriter.Create(filepath.Base(filePath))
		if err != nil {
			return "", err
		}

		_, err = io.Copy(writer, file)
		if err != nil {
			return "", err
		}
	}

	zipName := ""
	if taskID < 10 {
		zipName = "Task_" + "0" + strconv.FormatInt(taskID, 10)
	} else {
		zipName = "Task_" + strconv.FormatInt(taskID, 10)
	}

	zipPathEnd := folderPath + zipName + ".zip"

	return zipPathEnd, nil
}
