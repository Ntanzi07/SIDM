package users

import (
	"context"
	"sort"
	"sync"

	"SIDM/internal/platform"
)

// Memory é o Repo em memória, para os testes.
type Memory struct {
	mu       sync.Mutex
	Users    map[string]User
	Sessions map[string]Session
}

func NewMemory() *Memory {
	return &Memory{Users: map[string]User{}, Sessions: map[string]Session{}}
}

func (m *Memory) CreateUser(ctx context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Users[u.Username]; ok {
		return platform.ErrDuplicate
	}
	for _, other := range m.Users {
		if u.Email != "" && other.Email == u.Email {
			return ErrEmailTaken
		}
	}
	m.Users[u.Username] = *u
	return nil
}

func (m *Memory) FindUser(ctx context.Context, username string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.Users[username]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &u, nil
}

func (m *Memory) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.Users {
		if u.Email != "" && u.Email == email {
			return &u, nil
		}
	}
	return nil, platform.ErrNotFound
}

func (m *Memory) ListUsers(ctx context.Context) ([]User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]User, 0, len(m.Users))
	for _, u := range m.Users {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Username < list[j].Username })
	return list, nil
}

func (m *Memory) ReplaceUser(ctx context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Users[u.Username]; !ok {
		return platform.ErrNotFound
	}
	m.Users[u.Username] = *u
	return nil
}

func (m *Memory) CountActiveAdmins(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, u := range m.Users {
		if u.IsAdmin() {
			n++
		}
	}
	return n, nil
}

func (m *Memory) CreateSession(ctx context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sessions[s.ID] = *s
	return nil
}

func (m *Memory) FindSession(ctx context.Context, id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.Sessions[id]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return &s, nil
}

func (m *Memory) DeleteSession(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Sessions, id)
	return nil
}

func (m *Memory) DeleteSessions(ctx context.Context, username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.Sessions {
		if s.Username == username {
			delete(m.Sessions, id)
		}
	}
	return nil
}
