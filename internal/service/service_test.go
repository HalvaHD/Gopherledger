package service

import (
	"context"
	"gopherledger/internal/domain"
	"os"
	"testing"

	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockRepo struct {
	mock.Mock
}

func (m *mockRepo) CreateUser(login, hash string) (*domain.User, error) {
	args := m.Called(login, hash)
	return args.Get(0).(*domain.User), args.Error(1)
}
func (m *mockRepo) GetUserByLogin(login string) (*domain.User, error) {
	args := m.Called(login)
	return args.Get(0).(*domain.User), args.Error(1)
}
func (m *mockRepo) CreateOrder(uid int64, number string) (*domain.Order, error) {
	args := m.Called(uid, number)
	return args.Get(0).(*domain.Order), args.Error(1)
}
func (m *mockRepo) GetUserOrders(uid int64) ([]domain.Order, error) {
	args := m.Called(uid)
	return args.Get(0).([]domain.Order), args.Error(1)
}
func (m *mockRepo) GetOrdersForProcessing() ([]domain.Order, error) {
	args := m.Called()
	return args.Get(0).([]domain.Order), args.Error(1)
}
func (m *mockRepo) UpdateOrderStatus(num, status string, accrual float64) error {
	args := m.Called(num, status, accrual)
	return args.Error(0)
}
func (m *mockRepo) GetBalance(uid int64) (domain.Balance, error) {
	args := m.Called(uid)
	return args.Get(0).(domain.Balance), args.Error(1)
}
func (m *mockRepo) Withdraw(uid int64, order string, sum float64) error {
	args := m.Called(uid, order, sum)
	return args.Error(0)
}
func (m *mockRepo) GetWithdrawals(uid int64) ([]domain.Withdrawal, error) {
	args := m.Called(uid)
	return args.Get(0).([]domain.Withdrawal), args.Error(1)
}
func (m *mockRepo) GetStatistics() (*domain.Statistics, error) {
	args := m.Called()
	return args.Get(0).(*domain.Statistics), args.Error(1)
}
func hash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}
func TestService_Register(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("CreateUser",
			"ivan",
			mock.AnythingOfType("string"),
		).Return(&domain.User{
			ID:    1,
			Login: "ivan",
		}, nil)
		svc := New(repo)
		token, err := svc.RegisterUser("ivan", "123")
		assert.NoError(t, err)
		assert.Nil(t, uuid.Validate(token))
		repo.AssertExpectations(t)
	})
	t.Run("duplicate user", func(t *testing.T) {
		repo := new(mockRepo)

		repo.On("CreateUser",
			"ivan",
			mock.Anything,
		).Return(&domain.User{}, domain.ErrUserExists)

		svc := New(repo)

		_, err := svc.RegisterUser("ivan", "123")

		assert.ErrorIs(t, err, domain.ErrUserExists)
	})
}

func TestService_Login(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("GetUserByLogin", "ivan").
			Return(&domain.User{
				ID:           10,
				Login:        "ivan",
				PasswordHash: hash("123"),
			}, nil)
		svc := New(repo)
		token, err := svc.LoginUser("ivan", "123")
		assert.NoError(t, err)
		assert.Nil(t, uuid.Validate(token))
	})
	t.Run("wrong password", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("GetUserByLogin", "ivan").
			Return(&domain.User{
				ID:           10,
				PasswordHash: "badHash",
			}, nil)
		svc := New(repo)
		_, err := svc.LoginUser("ivan", "123")

		assert.ErrorIs(t, err, domain.ErrInvalidPassword)
	})
	t.Run("user missing", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("GetUserByLogin", "ghost").
			Return(&domain.User{}, domain.ErrUserNotFound)
		svc := New(repo)
		_, err := svc.LoginUser("ghost", "123")
		assert.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

func TestService_CreateOrder(t *testing.T) {
	repo := new(mockRepo)
	svc := New(repo)
	repo.On("CreateOrder", int64(1), "79927398713").
		Return(&domain.Order{
			UserID: 1,
			Number: "79927398713",
			Status: domain.OrderStatusNew,
		}, nil)
	_, err := svc.CreateOrder(1, "79927398713")
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestService_CreateOrderErrors(t *testing.T) {
	cases := []error{
		domain.ErrInvalidOrder,
		domain.ErrOrderExists,
		domain.ErrOrderOwnedByUser,
	}
	for _, e := range cases {
		t.Run(e.Error(), func(t *testing.T) {
			repo := new(mockRepo)
			repo.On("CreateOrder", int64(2), "79927398713").
				Return(&domain.Order{}, e)
			svc := New(repo)
			_, err := svc.CreateOrder(2, "79927398713")
			assert.ErrorIs(t, err, e)
		})
	}
}

func TestService_Withdraw(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("GetBalance", int64(5)).
			Return(domain.Balance{Current: 100}, nil)
		repo.On("Withdraw", int64(5), "79927398713", 50.0).
			Return(nil)
		svc := New(repo)
		err := svc.Withdraw(5, "79927398713", 50)
		assert.NoError(t, err)
	})
	t.Run("no money", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("GetBalance", int64(5)).
			Return(domain.Balance{Current: 10}, nil)
		repo.On("Withdraw", int64(5), "79927398713", 50.0).
			Return(domain.ErrInsufficientFunds)
		svc := New(repo)
		err := svc.Withdraw(5, "79927398713", 50)
		assert.ErrorIs(t, err, domain.ErrInsufficientFunds)
	})
	t.Run("bad luhn", func(t *testing.T) {
		repo := new(mockRepo)
		svc := New(repo)
		repo.On("GetBalance", int64(5)).
			Return(domain.Balance{Current: 10}, nil)
		err := svc.Withdraw(5, "123", 50)
		assert.ErrorIs(t, err, domain.ErrInvalidOrder)
	})
}

func TestService_BalanceAndOrders(t *testing.T) {
	repo := new(mockRepo)
	repo.On("GetBalance", int64(1)).
		Return(domain.Balance{Current: 100}, nil)
	repo.On("GetUserOrders", int64(1)).
		Return([]domain.Order{{Number: "1"}}, nil)
	svc := New(repo)
	_, err := svc.GetBalance(1)
	assert.NoError(t, err)
	_, err = svc.GetUserOrders(1)
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestService_Stats(t *testing.T) {
	repo := new(mockRepo)
	repo.On("GetWithdrawals", int64(1)).
		Return([]domain.Withdrawal{{}}, nil)
	repo.On("GetStatistics").
		Return(&domain.Statistics{}, nil)
	svc := New(repo)
	_, err := svc.GetWithdrawals(1)
	assert.NoError(t, err)
	_, err = svc.GetStatistics()
	assert.NoError(t, err)
}

func TestService_ProcessPendingOrders(t *testing.T) {
	repo := new(mockRepo)
	repo.On("GetOrdersForProcessing").
		Return([]domain.Order{}, nil)
	svc := New(repo)
	svc.processAllPendingOrders(context.Background())
	repo.AssertExpectations(t)
}

func TestService_StartWorkerStops(t *testing.T) {
	repo := new(mockRepo)
	svc := New(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.StartAccrualWorker(ctx) // проверка на зависание
}

func TestService_ProcessOrdersError(t *testing.T) {
	repo := new(mockRepo)
	repo.On("GetOrdersForProcessing").
		Return([]domain.Order{}, os.ErrClosed)
	svc := New(repo)
	svc.processAllPendingOrders(context.Background())
	repo.AssertExpectations(t)
}
