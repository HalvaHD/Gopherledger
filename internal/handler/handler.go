package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gopherledger/internal/domain"
)

// Service интерфрейс который хранит все методы
type Service interface {
	RegisterUser(login, password string) (string, error)
	LoginUser(login, password string) (string, error)

	CreateOrder(userID int64, number string) (*domain.Order, error)
	GetUserOrders(userID int64) ([]domain.Order, error)

	GetBalance(userID int64) (domain.Balance, error)
	Withdraw(userID int64, orderNumber string, sum float64) error
	GetWithdrawals(userID int64) ([]domain.Withdrawal, error)

	GetStatistics() (*domain.Statistics, error)
}
type Handler struct {
	svc Service
}

func New(svc Service) *Handler {
	return &Handler{svc: svc}
}

// errorResponse фиксирует структтуру ошибки
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, userMsg string, internalErr error) {
	if internalErr != nil {
		log.Printf("handler error [%s]: %v", code, internalErr)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Code:    code,
		Message: userMsg,
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON error: %v", err)
	}
}

// credentials фиксирует стурктуру логин пароль
type credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// createOrderRequest структура для джисон формата номера заказа (все по RESTику)
type createOrderRequest struct {
	Number string `json:"number"`
}

// Register базовый хендлер регистрации, чекает логин пароль, есть ли такой юезр уже в бд, если нет, то
// создает токен и записывает логин пароль в бд
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Ошибка какая то с закрытием тела: %v", err)
		}
	}(r.Body)
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "некорректный JSON", err)
		return
	}
	token, err := h.svc.RegisterUser(req.Login, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUserExists):
			writeError(w, http.StatusConflict, "USER_EXISTS", "пользователь уже существует", err)
		case errors.Is(err, domain.ErrInvalidAuthInfo):
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неправильный формат регистрационных данных", err)
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка", err)
		}
		return
	}
	w.Header().Set("Authorization", token)
	w.WriteHeader(http.StatusOK)
}

// Login хендлер для логина, проверяет логин пароль и если ок, то создает еще раз токен, старый удаляет на всякий
// и токен, который он дал, нужно для будущих запросов в Header отправлять POSTMAN
// Authorization : token
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Ошибка какая то с закрытием тела: %v", err)
		}
	}(r.Body)
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "некорректный JSON", err)
		return
	}
	token, err := h.svc.LoginUser(req.Login, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidPassword) ||
			errors.Is(err, domain.ErrUserNotFound) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "неверные учетные данные", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка", err)
		return
	}
	w.Header().Set("Authorization", token)
	w.WriteHeader(http.StatusOK)
}

// CreateOrder - хендлер создания заказа, принимает в хедеры токен и просит в тело запроса номер заказа по Луну
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Ошибка какая то с закрытием тела: %v", err)
		}
	}(r.Body)
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "NO_CONTEXT", "ошибка авторизации", nil)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не удалось прочитать тело запроса", err)
		return
	}
	number := strings.TrimSpace(string(body))
	if number == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "номер заказа не может быть пустым", nil)
		return
	}

	_, err = h.svc.CreateOrder(userID, number)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidOrder):
			writeError(w, http.StatusUnprocessableEntity, "INVALID_ORDER", "номер заказа неверный", err)
		case errors.Is(err, domain.ErrOrderExists):
			writeError(w, http.StatusConflict, "ORDER_EXISTS", "заказ уже загружен другим пользователем", err)
		case errors.Is(err, domain.ErrOrderOwnedByUser):
			w.WriteHeader(http.StatusOK)
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка", err)
		}
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// GetOrders - хендлер получения списка заказов, согласно формату в ридмишке
func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "NO_CONTEXT", "ошибка авторизации", nil)
		return
	}
	orders, err := h.svc.GetUserOrders(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка", err)
		return
	}
	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// GetBalance - хендлер проверки баланса у юзера
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "NO_CONTEXT", "ошибка авторизации", nil)
		return
	}
	balance, err := h.svc.GetBalance(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка", err)
		return
	}
	writeJSON(w, http.StatusOK, balance)
}

// Withdraw - хендлер списания денег с баланса
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Ошибка какая то с закрытием тела: %v", err)
		}
	}(r.Body)
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "NO_CONTEXT", "ошибка авторизации", nil)
		return
	}
	var req domain.Withdrawal
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}
	err := h.svc.Withdraw(userID, req.OrderNumber, req.Sum)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidOrder):
			writeError(w, http.StatusUnprocessableEntity, "INVALID_ORDER", "номер заказа неверный", err)
		case errors.Is(err, domain.ErrInsufficientFunds):
			writeError(w, http.StatusPaymentRequired, "NO_FUNDS", "недостаточно средств", err)
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка", err)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GetWithdrawals показывает историю списаний
func (h *Handler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "NO_CONTEXT", "ошибка авторизации", nil)
		return
	}

	list, err := h.svc.GetWithdrawals(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка", err)
		return
	}

	if len(list) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

// ExportStats - экспортирует данные в красивый txt файлик в фиксированном формате
func (h *Handler) ExportStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetStatistics()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось собрать статистику", err)
		return
	}
	file, err := os.Create("stats.txt")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "FILE_ERROR", "не удалось создать файл", err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)
	writer := bufio.NewWriter(file)
	report := fmt.Sprintf(
		`Total users: %d
		Total orders: %d
		  NEW: %d
		  PROCESSING: %d
		  PROCESSED: %d
		  INVALID: %d
		Total accrual: %.2f
		Total withdraw: %.2f
		Generation Time: %s
		`,
		stats.UserCount,
		stats.OrdersCount,
		stats.TypesOfOrders.NEW,
		stats.TypesOfOrders.PROCESSING,
		stats.TypesOfOrders.PROCESSED,
		stats.TypesOfOrders.INVALID,
		stats.TotalAccrual,
		stats.TotalWithdraw,
		time.Now().Format(time.RFC3339),
	)
	if _, err := writer.WriteString(report); err != nil {
		writeError(w, http.StatusInternalServerError, "FILE_WRITE", "ошибка записи файла", err)
		return
	}
	if err := writer.Flush(); err != nil {
		writeError(w, http.StatusInternalServerError, "FILE_WRITE", "ошибка записи файла", err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type contextKey string

const CtxKeyUserID contextKey = "userID"

func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(CtxKeyUserID).(int64)
	return id, ok
}
