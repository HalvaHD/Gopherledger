package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"gopherledger/internal/auth"
	"gopherledger/internal/config"
	"gopherledger/internal/handler"
)

// Auth проверяет хедер авторизации и добавляет айдишник в контексте
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := auth.ValidateToken(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), handler.CtxKeyUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Logging логгирует каждый запрос (гениально не правда ли?)
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)
		if config.MyConfiguration != nil &&
			config.MyConfiguration.LogLevel == "debug" {
			log.Printf(
				"%s %s -> %d (%s) ip=%s",
				r.Method,
				r.URL.Path,
				rec.status,
				duration,
				r.RemoteAddr,
			)
			return
		}
		log.Printf(
			"%s %s -> %d (%s)",
			r.Method,
			r.URL.Path,
			rec.status,
			duration,
		)
	})
}

// Recover- перехватывает панику, возвращая юзеру 500, при этом сам серваак не падает, а ошибка выводится в лог
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				http.Error(
					w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError,
				)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
