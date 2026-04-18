package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"

	"user-authentication/app/services"
	"user-authentication/config"
	"user-authentication/lib/db"
	"user-authentication/lib/web/middlewares"
	v1 "user-authentication/routes/v1"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		_ = godotenv.Load(".env")
	}

	cfg := config.Get()

	db.Connect()

	services.InitProvider(context.Background())

	router := httprouter.New()
	v1.Init(router)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
		IdleTimeout:  60 * time.Second,
		Handler:      middlewares.WithCORS(router),
	}

	go func() {
		log.Printf("server listening on http://localhost:%s (provider: %s)", cfg.Port, cfg.Provider)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-done

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Println("server stopped")
}
