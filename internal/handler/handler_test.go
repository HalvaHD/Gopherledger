package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"gopherledger/internal/domain"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type fakeService struct {
	mock.Mock
}

func (f *fakeService) RegisterUser(login, password string) (string, error) {
	args := f.Called(login, password)
	return args.String(0), args.Error(1)
}
func (f *fakeService) LoginUser(login, password string) (string, error) {
	args := f.Called(login, password)
	return args.String(0), args.Error(1)
}
func (f *fakeService) CreateOrder(uid int64, num string) (*domain.Order, error) {
	args := f.Called(uid, num)
	return args.Get(0).(*domain.Order), args.Error(1)
}
func (f *fakeService) GetUserOrders(uid int64) ([]domain.Order, error) {
	args := f.Called(uid)
	return args.Get(0).([]domain.Order), args.Error(1)
}
func (f *fakeService) GetBalance(uid int64) (domain.Balance, error) {
	args := f.Called(uid)
	return args.Get(0).(domain.Balance), args.Error(1)
}
func (f *fakeService) Withdraw(uid int64, order string, sum float64) error {
	args := f.Called(uid, order, sum)
	return args.Error(0)
}
func (f *fakeService) GetWithdrawals(uid int64) ([]domain.Withdrawal, error) {
	args := f.Called(uid)
	return args.Get(0).([]domain.Withdrawal), args.Error(1)
}
func (f *fakeService) GetStatistics() (*domain.Statistics, error) {
	args := f.Called()
	return args.Get(0).(*domain.Statistics), args.Error(1)
}

func authReq(login, pass string) []byte {
	b, _ := json.Marshal(credentials{
		Login:    login,
		Password: pass,
	})
	return b
}
func withUser(req *http.Request, id int64) *http.Request {
	ctx := context.WithValue(req.Context(), CtxKeyUserID, id)
	return req.WithContext(ctx)
}
func orderReq(number string) []byte {
	b, _ := json.Marshal(struct {
		Number string `json:"number"`
	}{
		Number: number,
	})
	return b
}

func TestRegisterHandler(t *testing.T) {
	tests := []struct {
		name   string
		login  string
		pass   string
		status int
		mock   func(s *fakeService)
	}{
		{
			name:   "ok register",
			login:  "vasya",
			pass:   "123",
			status: http.StatusOK,
			mock: func(s *fakeService) {
				s.On("RegisterUser", "vasya", "123").
					Return("token123", nil)
			},
		},
		{
			name:   "duplicate user",
			login:  "vasya",
			pass:   "123",
			status: http.StatusConflict,
			mock: func(s *fakeService) {
				s.On("RegisterUser", "vasya", "123").
					Return("", domain.ErrUserExists)
			},
		},
		{
			name:   "empty fields",
			login:  "",
			pass:   "",
			status: http.StatusBadRequest,
			mock: func(s *fakeService) {
				s.On("RegisterUser", "", "").
					Return("", domain.ErrInvalidAuthInfo)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			svc := new(fakeService)
			tc.mock(svc)

			h := New(svc)

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/user/register",
				bytes.NewReader(authReq(tc.login, tc.pass)),
			)

			rec := httptest.NewRecorder()
			h.Register(rec, req)

			assert.Equal(t, tc.status, rec.Code)
			svc.AssertExpectations(t)
		})
	}
}

func TestLoginHandler(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"success", nil, http.StatusOK},
		{"wrong password", domain.ErrInvalidPassword, http.StatusUnauthorized},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {

			svc := new(fakeService)
			svc.On("LoginUser", "ivan", "pass").
				Return("jwt", tt.err)

			h := New(svc)

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/user/login",
				bytes.NewReader(authReq("ivan", "pass")),
			)

			rec := httptest.NewRecorder()
			h.Login(rec, req)

			assert.Equal(t, tt.status, rec.Code)
		})
	}
}
func TestCreateOrderHandler(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"ok", nil, http.StatusAccepted},
		{"bad Luhn", domain.ErrInvalidOrder, http.StatusUnprocessableEntity},
		{"conflict", domain.ErrOrderExists, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := new(fakeService)
			svc.On("CreateOrder", int64(1), mock.Anything).
				Return(&domain.Order{}, tt.err)
			h := New(svc)
			body := orderReq("79927398713")
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/user/orders",
				bytes.NewReader(body),
			)
			req = withUser(req, 1)
			rec := httptest.NewRecorder()
			h.CreateOrder(rec, req)
			assert.Equal(t, tt.status, rec.Code)
		})
	}
}

func TestGetOrdersHandler(t *testing.T) {
	t.Run("has orders", func(t *testing.T) {
		svc := new(fakeService)
		svc.On("GetUserOrders", int64(5)).
			Return([]domain.Order{{Number: "1"}}, nil)
		h := New(svc)
		req := withUser(
			httptest.NewRequest(http.MethodGet, "/", nil),
			5,
		)
		rec := httptest.NewRecorder()
		h.GetOrders(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
	t.Run("no orders", func(t *testing.T) {
		svc := new(fakeService)
		svc.On("GetUserOrders", int64(5)).
			Return([]domain.Order(nil), nil)
		h := New(svc)
		req := withUser(
			httptest.NewRequest(http.MethodGet, "/", nil),
			5,
		)
		rec := httptest.NewRecorder()
		h.GetOrders(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestGetBalanceHandler(t *testing.T) {
	svc := new(fakeService)
	svc.On("GetBalance", int64(7)).
		Return(domain.Balance{Current: 100}, nil)
	h := New(svc)
	req := withUser(
		httptest.NewRequest(http.MethodGet, "/", nil),
		7,
	)
	rec := httptest.NewRecorder()
	h.GetBalance(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWithdrawHandler(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"ok", nil, http.StatusOK},
		{"not enough", domain.ErrInsufficientFunds, http.StatusPaymentRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := new(fakeService)
			svc.On("Withdraw", int64(3), "79927398713", 50.0).
				Return(tt.err)
			h := New(svc)
			w := domain.Withdrawal{
				OrderNumber: "79927398713",
				Sum:         50,
			}
			body, _ := json.Marshal(w)
			req := withUser(
				httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)),
				3,
			)
			rec := httptest.NewRecorder()
			h.Withdraw(rec, req)
			assert.Equal(t, tt.status, rec.Code)
		})
	}
}
func TestWithdrawalsHandler(t *testing.T) {
	t.Run("with data", func(t *testing.T) {
		svc := new(fakeService)
		svc.On("GetWithdrawals", int64(1)).
			Return([]domain.Withdrawal{{}}, nil)
		h := New(svc)
		req := withUser(
			httptest.NewRequest(http.MethodGet, "/", nil),
			1,
		)
		rec := httptest.NewRecorder()
		h.GetWithdrawals(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
	t.Run("empty", func(t *testing.T) {
		svc := new(fakeService)
		svc.On("GetWithdrawals", int64(1)).
			Return([]domain.Withdrawal(nil), nil)
		h := New(svc)
		req := withUser(
			httptest.NewRequest(http.MethodGet, "/", nil),
			1,
		)
		rec := httptest.NewRecorder()
		h.GetWithdrawals(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestExportStatsHandler(t *testing.T) {
	svc := new(fakeService)
	svc.On("GetStatistics").Return(&domain.Statistics{
		UserCount:     10,
		OrdersCount:   5,
		TotalAccrual:  20,
		TotalWithdraw: 3,
	}, nil)
	h := New(svc)
	req := httptest.NewRequest(http.MethodPost, "/stats", nil)
	rec := httptest.NewRecorder()
	h.ExportStats(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	_, err := os.Stat("stats.txt")
	assert.NoError(t, err)
	err1 := os.Remove("stats.txt")
	if err1 != nil {
		return
	}
}
