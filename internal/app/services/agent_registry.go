package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chmouel/lazyworktree/internal/models"
	"github.com/chmouel/lazyworktree/internal/utils"
)

const (
	agentRegistrySchemaVersion = "lazyworktree-agent-registry-v1"
	agentRecentThreshold       = 10 * time.Minute
	agentRegistryLockTimeout   = 2 * time.Second
)

type sessionRegistryPayload struct {
	SchemaVersion string               `json:"schema_version"`
	Sessions      []agentSessionRecord `json:"sessions"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

type agentSessionRecord struct {
	Session *models.AgentSession `json:"session"`
}

// SessionRegistryStore persists agent session metadata across restarts.
type SessionRegistryStore interface {
	Load() (map[string]*models.AgentSession, error)
	Save(sessions []*models.AgentSession) error
}

type fileSessionRegistryStore struct {
	path string
	mu   sync.Mutex
}

func newFileSessionRegistryStore() SessionRegistryStore {
	return &fileSessionRegistryStore{path: agentSessionRegistryPath()}
}

func newFileSessionRegistryStoreWithPath(path string) SessionRegistryStore {
	return &fileSessionRegistryStore{path: path}
}

// NewTestSessionRegistryStore builds a registry store rooted at an explicit path for tests.
func NewTestSessionRegistryStore(path string) SessionRegistryStore {
	return newFileSessionRegistryStoreWithPath(path)
}

func (s *fileSessionRegistryStore) Load() (map[string]*models.AgentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path) // #nosec G304 -- registry path is app-controlled.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]*models.AgentSession{}, nil
		}
		return nil, err
	}

	var payload sessionRegistryPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	sessions := make(map[string]*models.AgentSession, len(payload.Sessions))
	for _, record := range payload.Sessions {
		if record.Session == nil {
			continue
		}
		key := strings.TrimSpace(record.Session.SessionKey)
		if key == "" {
			key = agentSessionKey(record.Session)
		}
		record.Session.SessionKey = key
		sessions[key] = cloneAgentSession(record.Session)
	}
	return sessions, nil
}

func (s *fileSessionRegistryStore) Save(sessions []*models.AgentSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), utils.DefaultDirPerms); err != nil {
		return err
	}
	if unlock, err := acquireAgentRegistryLock(s.path + ".lock"); err == nil {
		defer unlock()
	} else {
		return err
	}

	records := make([]agentSessionRecord, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		copied := cloneAgentSession(session)
		records = append(records, agentSessionRecord{Session: copied})
	}

	payload := sessionRegistryPayload{
		SchemaVersion: agentRegistrySchemaVersion,
		Sessions:      records,
		UpdatedAt:     time.Now().UTC(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return writeAtomically(s.path, data)
}

func agentSessionRegistryPath() string {
	if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "lazyworktree", "agent-sessions", "registry.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "lazyworktree", "agent-sessions", "registry.json")
	}
	return filepath.Join(home, ".local", "share", "lazyworktree", "agent-sessions", "registry.json")
}

func acquireAgentRegistryLock(path string) (func(), error) {
	deadline := time.Now().Add(agentRegistryLockTimeout)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, defaultFilePerms) //nolint:gosec // Lock path is app-controlled.
		if err == nil {
			_ = f.Close()
			return func() {
				_ = os.Remove(path)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func writeAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(defaultFilePerms); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func agentSessionKey(session *models.AgentSession) string {
	if session == nil {
		return ""
	}
	if strings.TrimSpace(session.JSONLPath) != "" {
		return string(session.Agent) + ":" + filepath.Clean(session.JSONLPath)
	}
	parts := []string{
		string(session.Agent),
		strings.TrimSpace(session.ID),
		filepath.Clean(strings.TrimSpace(session.CWD)),
	}
	return strings.Join(parts, ":")
}

func sessionObservationTime(session *models.AgentSession) time.Time {
	if session == nil {
		return time.Time{}
	}
	if !session.LastObservedAt.IsZero() && session.LastObservedAt.After(session.LastActivity) {
		return session.LastObservedAt
	}
	return session.LastActivity
}

func deriveAgentSessionTitle(session *models.AgentSession) string {
	if session == nil {
		return ""
	}
	if strings.TrimSpace(session.TaskLabel) != "" {
		return session.TaskLabel
	}
	if strings.TrimSpace(session.DisplayName) != "" {
		return session.DisplayName
	}
	if session.Agent == models.AgentKindPi {
		return "pi session"
	}
	return "Claude session"
}
