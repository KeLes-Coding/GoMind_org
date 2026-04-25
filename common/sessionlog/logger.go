package sessionlog

import (
	"GopherAI/model"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultLogDir      = "logs/session_conversations"
	defaultMaxSessions = 100
)

type Config struct {
	Dir         string
	MaxSessions int
	BaseLogPath string
}

type messageLogEntry struct {
	Event            string              `json:"event"`
	LoggedAt         time.Time           `json:"logged_at"`
	MessageKey       string              `json:"message_key"`
	SessionID        string              `json:"session_id"`
	SessionVersion   int64               `json:"session_version"`
	UserName         string              `json:"username"`
	Role             string              `json:"role"`
	Content          string              `json:"content"`
	ReasoningContent string              `json:"reasoning_content,omitempty"`
	ResponseMeta     string              `json:"response_meta,omitempty"`
	Extra            string              `json:"extra,omitempty"`
	IsUser           bool                `json:"is_user"`
	Status           model.MessageStatus `json:"status"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

var (
	mu          sync.Mutex
	logDir      = defaultLogDir
	maxSessions = defaultMaxSessions
)

func Setup(cfg Config) {
	mu.Lock()
	defer mu.Unlock()

	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" && strings.TrimSpace(cfg.BaseLogPath) != "" {
		dir = filepath.Join(filepath.Dir(cfg.BaseLogPath), "session_conversations")
	}
	if dir == "" {
		dir = defaultLogDir
	}

	limit := cfg.MaxSessions
	if limit <= 0 {
		limit = defaultMaxSessions
	}

	logDir = dir
	maxSessions = limit
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Printf("sessionlog setup mkdir failed dir=%s err=%v", logDir, err)
	}
}

func RecordMessageBestEffort(message *model.Message) {
	if message == nil || strings.TrimSpace(message.SessionID) == "" {
		return
	}

	if err := recordMessage(message); err != nil {
		log.Printf("sessionlog record failed session_id=%s message_key=%s err=%v", message.SessionID, message.MessageKey, err)
	}
}

func recordMessage(message *model.Message) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(logDir, sanitizeSessionID(message.SessionID)+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	entry := messageLogEntry{
		Event:            "message_upsert",
		LoggedAt:         time.Now(),
		MessageKey:       message.MessageKey,
		SessionID:        message.SessionID,
		SessionVersion:   message.SessionVersion,
		UserName:         message.UserName,
		Role:             messageRole(message),
		Content:          message.Content,
		ReasoningContent: message.ReasoningContent,
		ResponseMeta:     message.ResponseMeta,
		Extra:            message.Extra,
		IsUser:           message.IsUser,
		Status:           message.Status,
		CreatedAt:        message.CreatedAt,
		UpdatedAt:        message.UpdatedAt,
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		return err
	}

	return pruneOldSessionLogsLocked()
}

func messageRole(message *model.Message) string {
	if message.IsUser {
		return "user"
	}
	return "assistant"
}

func sanitizeSessionID(sessionID string) string {
	var builder strings.Builder
	for _, r := range sessionID {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	safe := strings.Trim(builder.String(), "_")
	if safe == "" {
		return "unknown_session"
	}
	return safe
}

func pruneOldSessionLogsLocked() error {
	if maxSessions <= 0 {
		return nil
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}

	type sessionFile struct {
		name    string
		modTime time.Time
	}

	files := make([]sessionFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, sessionFile{name: entry.Name(), modTime: info.ModTime()})
	}
	if len(files) <= maxSessions {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	for _, file := range files[maxSessions:] {
		if err := os.Remove(filepath.Join(logDir, file.name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
