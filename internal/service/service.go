// Пакет service содержит бизнес-логику приложения.
//
// Взаимодействие с хранилищем осуществляется через интерфейс.
// Определите этот интерфейс здесь, по месту использования.
package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"gopherledger/internal/auth"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"gopherledger/internal/config"
	"gopherledger/internal/domain"
)

// Stats - фиксированная структура для экспорта статистики
type Stats struct {
	TotalUsers     int
	TotalOrders    int
	ByStatus       map[string]int
	TotalAccrual   float64
	TotalWithdrawn float64
}

// Repository — то, что сервис ожидает от стора
type Repository interface {
	CreateUser(login, passwordHash string) (*domain.User, error)
	GetUserByLogin(login string) (*domain.User, error)
	CreateOrder(userID int64, number string) (*domain.Order, error)
	GetUserOrders(userID int64) ([]domain.Order, error)
	GetOrdersForProcessing() ([]domain.Order, error)
	UpdateOrderStatus(number, status string, accrual float64) error
	GetBalance(userID int64) (domain.Balance, error)
	Withdraw(userID int64, orderNumber string, sum float64) error
	GetWithdrawals(userID int64) ([]domain.Withdrawal, error)
	GetStatistics() (*domain.Statistics, error)
}

// Service реализует бизнес-логику приложения.
// processingOrders хранит номера заказов, которые сейчас обрабатываются воркером.
// Защитите конкурентный доступ к этому полю самостоятельно.
type Service struct {
	repo             Repository
	processingOrders map[string]bool
	mu               sync.Mutex
}

// New создаёт Service.
func New(repo Repository) *Service {
	return &Service{
		repo:             repo,
		processingOrders: make(map[string]bool),
	}
}

// RegisterUser регистрирует нового пользователя и возвращает токен аутентификации.
// Хешируйте пароль перед сохранением с помощью crypto/sha256.
func (s *Service) RegisterUser(login, password string) (string, error) {
	if strings.TrimSpace(login) == "" || strings.TrimSpace(password) == "" {
		return "", domain.ErrInvalidAuthInfo
	}
	passHash := hashPassword(password)
	user, err := s.repo.CreateUser(login, passHash)
	if err != nil {
		return "", err
	}
	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		return "", fmt.Errorf("ошибка создания токена %w", err)
	}
	return token, nil

}

// LoginUser проверяет учётные данные и возвращает токен аутентификации.
func (s *Service) LoginUser(login, password string) (string, error) {
	user, err := s.repo.GetUserByLogin(login)
	if err != nil {
		return "", err
	}
	if user.PasswordHash != hashPassword(password) {
		return "", domain.ErrInvalidPassword
	}
	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		return "", fmt.Errorf("ошибка создания токена: %w", err)
	}
	return token, nil
}

// CreateOrder проверяет номер заказа по алгоритму Луна и сохраняет заказ.
func (s *Service) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if !validateLuhn(number) {
		return &domain.Order{}, domain.ErrInvalidOrder
	}
	order, err := s.repo.CreateOrder(userID, number)
	if err != nil {
		return &domain.Order{}, err
	}
	return order, nil
}

// GetUserOrders возвращает все заказы пользователя.
func (s *Service) GetUserOrders(userID int64) ([]domain.Order, error) {
	return s.repo.GetUserOrders(userID)
}

// GetBalance возвращает текущий баланс пользователя.
func (s *Service) GetBalance(userID int64) (domain.Balance, error) {
	return s.repo.GetBalance(userID)
}

// Withdraw проверяет номер заказа по алгоритму Луна и списывает сумму с баланса.
func (s *Service) Withdraw(userID int64, orderNumber string, sum float64) error {
	if !validateLuhn(orderNumber) {
		return domain.ErrInvalidOrder
	}
	bal, err := s.repo.GetBalance(userID)
	if err != nil {
		return err
	}
	if bal.Current < sum {
		return domain.ErrInsufficientFunds
	}
	res := s.repo.Withdraw(userID, orderNumber, sum)
	return res
}

// GetWithdrawals возвращает историю списаний пользователя.
func (s *Service) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	return s.repo.GetWithdrawals(userID)
}
func (s *Service) GetStatistics() (*domain.Statistics, error) {
	statistics, err := s.repo.GetStatistics()
	if err != nil {
		return statistics, err
	}
	return statistics, nil
}

// validateLuhn проверяет контрольную сумму номера заказа по алгоритму Луна.
func validateLuhn(number string) bool {
	if len(number) == 0 {
		return false
	}
	sum := 0
	double := false
	for i := len(number) - 1; i >= 0; i-- {
		ch := number[i]
		if ch < '0' || ch > '9' {
			return false
		}
		digit := int(ch - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}

// hashPassword возвращает hex-строку SHA-256 от пароля
func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", h)
}

// ---------------------------------------------------------------------------
// Воркер начислений
// ---------------------------------------------------------------------------

// StartAccrualWorker запускает фоновый цикл, который каждые 3 секунды
// передаёт необработанные заказы в processAllPendingOrders.
// Останавливается при отмене ctx.
func (s *Service) StartAccrualWorker(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processAllPendingOrders(ctx)
		}
	}
}

// processAllPendingOrders получает заказы для обработки и запускает горутины.
// Реализуйте самостоятельно.
func (s *Service) processAllPendingOrders(ctx context.Context) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load error: %v", err)
	}
	orders, err := s.repo.GetOrdersForProcessing()
	if err != nil {
		log.Printf("воркер: ошибка получения заказов: %v", err)
		return
	}
	g, gctx := errgroup.WithContext(ctx) // группа горутинок
	g.SetLimit(cfg.WorkerCount)          // Лимит на число горутин одновременно, тут лимит по файлу из ямлика
	for _, o := range orders {
		number := o.Number
		s.mu.Lock()
		if s.processingOrders[number] {
			s.mu.Unlock()
			continue
		}
		s.processingOrders[number] = true
		s.mu.Unlock()
		num := number
		g.Go(func() error {
			defer func() {
				s.mu.Lock()
				delete(s.processingOrders, num)
				s.mu.Unlock()
			}()
			s.processOrder(gctx, num)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		log.Printf("воркер: ошибка группы: %v", err)
	}
}

// processOrder обрабатывает один заказ. Реализуйте самостоятельно.
// Используйте вспомогательные функции ниже для генерации случайных значений.
func (s *Service) processOrder(ctx context.Context, number string) {
	if err := s.repo.UpdateOrderStatus(number, domain.OrderStatusProcessing, 0); err != nil {
		log.Printf("воркер: UpdateOrderStatus PROCESSING %s: %v", number, err)
		return
	}
	// select реализует грейсфул шатдаун
	select {
	case <-ctx.Done():
		return
	case <-time.After(randomDelay()):
	}
	if isInvalid() {
		if err := s.repo.UpdateOrderStatus(number, domain.OrderStatusInvalid, 0); err != nil {
			log.Printf("воркер: UpdateOrderStatus INVALID %s: %v", number, err)
		}
		return
	}
	accrual := randomAccrual()
	if err := s.repo.UpdateOrderStatus(number, domain.OrderStatusProcessed, accrual); err != nil {
		log.Printf("воркер: UpdateOrderStatus PROCESSED %s: %v", number, err)
	}
}

// ---------------------------------------------------------------------------
// Вспомогательные функции - предоставлены
// ---------------------------------------------------------------------------

// randomAccrual возвращает случайное начисление от 10 до 500 баллов.
func randomAccrual() float64 {
	return float64(rand.Intn(491) + 10)
}

// randomDelay возвращает случайную задержку от 2 до 6 секунд.
func randomDelay() time.Duration {
	return time.Duration(rand.Intn(5)+2) * time.Second
}

// isInvalid возвращает true примерно в 10% случаев.
func isInvalid() bool {
	return rand.Intn(10) == 0
}
