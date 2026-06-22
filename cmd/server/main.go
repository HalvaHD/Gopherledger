package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopherledger/internal/config"
	"gopherledger/internal/handler"
	"gopherledger/internal/router"
	"gopherledger/internal/service"
	"gopherledger/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load error: %v", err)
	}
	config.MyConfiguration = cfg
	st := store.New()
	svc := service.New(st)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.StartAccrualWorker(ctx)

	h := handler.New(svc)
	r := router.New(h)
	addr := cfg.ServerHost + ":" + itoa(cfg.ServerPort)

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("Сервер запустился на %s", addr)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Ошибка сервера: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)

	signal.Notify(
		stop,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-stop

	log.Println("Получен сигнал завершения работы")

	ctx1, cancel1 := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel1()

	if err := server.Shutdown(ctx1); err != nil {
		log.Printf("Graceful shutdown провалилось: %v", err)
	}

	log.Println("Сервер остановлен")
}
func itoa(v int) string {
	return fmtInt(v)
}

func fmtInt(v int) string {
	return fmt.Sprintf("%d", v)
}
