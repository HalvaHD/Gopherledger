// Пакет auth отвечает за генерацию и проверку токенов аутентификации.
package auth

import (
	"errors"
	"sync"

	"github.com/google/uuid"
)

// ErrInvalidToken возвращается, если токен не найден или недействителен.
var ErrInvalidToken = errors.New("недействительный токен")

type tokenStorage struct {
	mu sync.RWMutex

	byToken map[string]int64
	byUser  map[int64]string
}

var storage = &tokenStorage{
	byToken: make(map[string]int64),
	byUser:  make(map[int64]string),
}

// GenerateToken Создание токена через обычную юиайдишку
func GenerateToken(userID int64) (string, error) {
	token := uuid.New().String()
	storage.mu.Lock()
	defer storage.mu.Unlock()

	if old, ok := storage.byUser[userID]; ok {
		delete(storage.byToken, old)
	}

	storage.byToken[token] = userID
	storage.byUser[userID] = token

	return token, nil
}

// ValidateToken Здесь проверка токена на существование в "БД"
func ValidateToken(token string) (int64, error) {
	storage.mu.RLock()
	defer storage.mu.RUnlock()

	userID, ok := storage.byToken[token]
	if !ok {
		return 0, ErrInvalidToken
	}
	if current := storage.byUser[userID]; current != token {
		return 0, ErrInvalidToken
	}

	return userID, nil
}
