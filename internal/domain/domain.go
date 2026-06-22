// Пакет domain содержит все бизнес-типы и сентинел-ошибки приложения.
// Этот пакет предоставлен и изменять его не нужно.
package domain

import (
	"errors"
	"time"
)

// ---------------------------------------------------------------------------
// Сентинел-ошибки
// ---------------------------------------------------------------------------

var (
	ErrUserExists        = errors.New("пользователь уже существует")
	ErrUserNotFound      = errors.New("пользователь не найден")
	ErrInvalidPassword   = errors.New("неверный пароль")
	ErrOrderExists       = errors.New("заказ уже загружен другим пользователем")
	ErrOrderOwnedByUser  = errors.New("заказ уже загружен этим пользователем")
	ErrInsufficientFunds = errors.New("недостаточно баллов")
	ErrInvalidOrder      = errors.New("неверный номер заказа")
	ErrInvalidAuthInfo   = errors.New("неправильный формат регистрационных данных") //В оригинале не было ошибки для неправильного логина
)

// ---------------------------------------------------------------------------
// Модели
// ---------------------------------------------------------------------------

// User представляет зарегистрированного пользователя.
type User struct {
	ID           int64  `json:"-"`
	Login        string `json:"-"`
	PasswordHash string `json:"-"`
}

// Order представляет заказ, загруженный пользователем.
type Order struct {
	ID         int64     `json:"-"`
	UserID     int64     `json:"-"`
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    float64   `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// Balance представляет текущий баланс пользователя.
type Balance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

// Withdrawal представляет операцию списания баллов.
type Withdrawal struct {
	ID          int64     `json:"-"`
	UserID      int64     `json:"-"`
	OrderNumber string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

// Statistics дает фиксированный формат экспорта файлов
type Statistics struct {
	UserCount     int           `json:"user_count"`
	TotalAccrual  float64       `json:"total_accrual"`
	OrdersCount   int           `json:"orders_count"`
	TotalWithdraw float64       `json:"total_withdraw"`
	GeneratedAt   time.Time     `json:"generated_at"`
	TypesOfOrders TypesOfOrders `json:"orders_distribution"`
}

// TypesOfOrders фиксирует все типы статусов заказов
type TypesOfOrders struct {
	NEW        int `json:"new"`
	PROCESSING int `json:"processing"`
	PROCESSED  int `json:"processed"`
	INVALID    int `json:"invalid"`
}

// ---------------------------------------------------------------------------
// Константы статусов заказа
// ---------------------------------------------------------------------------

const (
	OrderStatusNew        = "NEW"
	OrderStatusProcessing = "PROCESSING"
	OrderStatusInvalid    = "INVALID"
	OrderStatusProcessed  = "PROCESSED"
)
