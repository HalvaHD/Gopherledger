// Пакет store реализует хранилище данных в памяти.
// Используйте отдельные мьютексы для независимых групп данных.
// Реализуйте этот пакет самостоятельно.
package store

import (
	"sort"
	"sync"
	"time"

	"gopherledger/internal/domain"
)

// Store хранит все данные приложения в памяти.
type Store struct {
	// users хранит пользователей по их ID
	users map[int64]*domain.User
	// usersByLogin хранит пользователей по логину - для быстрого поиска при авторизации
	usersByLogin map[string]*domain.User
	// orders хранит заказы по номеру заказа
	orders map[string]*domain.Order
	// balances хранит текущий баланс каждого пользователя по его ID
	balances map[int64]*domain.Balance
	// withdrawals хранит историю списаний для каждого пользователя по его ID
	withdrawals map[int64][]*domain.Withdrawal
	// nextID используется для генерации уникальных числовых ID
	nextID     int64
	userMu     sync.RWMutex
	orderMu    sync.RWMutex
	balanceMu  sync.RWMutex
	withdrawMu sync.RWMutex
	idMu       sync.Mutex
}

func (s *Store) next() int64 {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	s.nextID++
	return s.nextID
}

func New() *Store {
	return &Store{
		users:        make(map[int64]*domain.User),
		usersByLogin: make(map[string]*domain.User),
		orders:       make(map[string]*domain.Order),
		balances:     make(map[int64]*domain.Balance),
		withdrawals:  make(map[int64][]*domain.Withdrawal),
	}
}

// GetStatistics - получаем всю инфу по структуре, для экспорта
func (s *Store) GetStatistics() (*domain.Statistics, error) {
	s.userMu.RLock()
	defer s.userMu.RUnlock()
	stats := domain.Statistics{}
	stats.UserCount = len(s.users)
	stats.OrdersCount = len(s.orders)
	for _, o := range s.orders {
		switch o.Status {
		case domain.OrderStatusNew:
			stats.TypesOfOrders.NEW++
		case domain.OrderStatusProcessing:
			stats.TypesOfOrders.PROCESSING++
		case domain.OrderStatusProcessed:
			stats.TypesOfOrders.PROCESSED++
			stats.TotalAccrual += o.Accrual
		case domain.OrderStatusInvalid:
			stats.TypesOfOrders.INVALID++
		}
	}
	for _, list := range s.withdrawals {
		for _, w := range list {
			stats.TotalWithdraw += w.Sum
		}
	}
	return &stats, nil
}

// CreateUser - создаем нового юзера
func (s *Store) CreateUser(login, passwordHash string) (*domain.User, error) {
	s.userMu.Lock()
	defer s.userMu.Unlock()
	if _, ok := s.usersByLogin[login]; ok {
		return nil, domain.ErrUserExists
	}
	id := s.next()
	user := &domain.User{
		ID:           id,
		Login:        login,
		PasswordHash: passwordHash,
	}
	s.users[id] = user
	s.usersByLogin[login] = user
	s.balanceMu.Lock()
	s.balances[id] = &domain.Balance{}
	s.balanceMu.Unlock()
	return user, nil
}

// GetUserByLogin - получаем юзера по логину
func (s *Store) GetUserByLogin(login string) (*domain.User, error) {
	s.userMu.RLock()
	defer s.userMu.RUnlock()
	user, ok := s.usersByLogin[login]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

// CreateOrder - создает новый заказ юзера
func (s *Store) CreateOrder(userID int64, number string) (*domain.Order, error) {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	if existing, ok := s.orders[number]; ok {
		if existing.UserID == userID {
			return nil, domain.ErrOrderOwnedByUser
		}
		return nil, domain.ErrOrderExists
	}
	order := &domain.Order{
		ID:         s.next(),
		UserID:     userID,
		Number:     number,
		Status:     domain.OrderStatusNew,
		UploadedAt: time.Now(),
	}
	s.orders[number] = order
	return order, nil
}

// GetUserOrders - получает все заказы юзера, сортированные по новизне
func (s *Store) GetUserOrders(userID int64) ([]domain.Order, error) {
	s.orderMu.RLock()
	defer s.orderMu.RUnlock()
	var res []domain.Order
	for _, o := range s.orders {
		if o.UserID == userID {
			res = append(res, *o)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].UploadedAt.After(res[j].UploadedAt)
	})
	return res, nil
}

// GetOrdersForProcessing - получает все заказы новые и в обработке
func (s *Store) GetOrdersForProcessing() ([]domain.Order, error) {
	s.orderMu.RLock()
	defer s.orderMu.RUnlock()
	var res []domain.Order
	for _, o := range s.orders {
		if o.Status == domain.OrderStatusNew ||
			o.Status == domain.OrderStatusProcessing {
			res = append(res, *o)
		}
	}
	return res, nil
}

// UpdateOrderStatus - обновляет статус заказа, накидывает денег на баланс юзера, если accural > 0 и он обработан
func (s *Store) UpdateOrderStatus(number, status string, accrual float64) error {
	s.orderMu.Lock()
	order, ok := s.orders[number]
	if !ok {
		s.orderMu.Unlock()
		return nil
	}
	order.Status = status
	order.Accrual = accrual
	userID := order.UserID
	s.orderMu.Unlock()
	if status == domain.OrderStatusProcessed && accrual > 0 {
		s.balanceMu.Lock()
		b := s.balances[userID]
		b.Current += accrual
		s.balanceMu.Unlock()
	}
	return nil
}

// GetBalance - показывает баланс юзера
func (s *Store) GetBalance(userID int64) (domain.Balance, error) {
	s.balanceMu.RLock()
	defer s.balanceMu.RUnlock()
	b, ok := s.balances[userID]
	if !ok {
		return domain.Balance{}, nil
	}
	return *b, nil
}

// Withdraw - списывает деньги с баланса и фиксирует операцию, ошибка если денег не хватает
func (s *Store) Withdraw(userID int64, orderNumber string, sum float64) error {
	s.balanceMu.Lock()
	defer s.balanceMu.Unlock()
	balance, ok := s.balances[userID]
	if !ok || balance.Current < sum {
		return domain.ErrInsufficientFunds
	}
	balance.Current -= sum
	balance.Withdrawn += sum
	w := &domain.Withdrawal{
		ID:          s.next(),
		UserID:      userID,
		OrderNumber: orderNumber,
		Sum:         sum,
		ProcessedAt: time.Now(),
	}
	s.withdrawMu.Lock()
	s.withdrawals[userID] = append(s.withdrawals[userID], w)
	s.withdrawMu.Unlock()
	return nil
} // Операция атомарная
// GetWithdrawals - возвращает список трат юзера, сортированные по новизне
func (s *Store) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	s.withdrawMu.RLock()
	defer s.withdrawMu.RUnlock()
	list := s.withdrawals[userID]
	res := make([]domain.Withdrawal, len(list))
	for i, w := range list {
		res[i] = *w
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ProcessedAt.After(res[j].ProcessedAt)
	})
	return res, nil
}
