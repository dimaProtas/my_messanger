package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"my_messanger/internal/auth"
	"my_messanger/internal/chat"
	"my_messanger/internal/config"
	"my_messanger/internal/contacts"
	"my_messanger/internal/message"
	"my_messanger/internal/pkg/database"
	pkgkafka "my_messanger/internal/pkg/kafka"
	"my_messanger/internal/pkg/logger"
	"my_messanger/internal/pkg/redis"
	"my_messanger/internal/server"
	"my_messanger/internal/user"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	redisv8 "github.com/go-redis/redis/v8"
)

func main() {
	log := logger.New("info")
	slog.SetDefault(log)

	cfg := config.LoadConfig()

	db, err := database.NewPostgresDB(cfg)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	redisClient, err := redis.NewRedisClient(cfg)
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	kafkaProducer := pkgkafka.NewKafkaProducer(cfg)
	defer kafkaProducer.Close()

	connManager := server.NewConnectionManager()

	wsServer := server.NewWsServer(redisClient, cfg, connManager)

	authRepo := auth.NewRepository(db)
	authService := auth.NewService(authRepo, cfg, redisClient)
	authHandler := auth.NewHandler(authService)

	userRepo := user.NewRepository(db)
	userService := user.NewService(userRepo, log)
	userHandler := user.NewHandler(userService, log)

	contactsRepo := contacts.NewRepository(db)
	contactsService := contacts.NewService(contactsRepo, userRepo, log)
	contactsHandler := contacts.NewHandler(contactsService, log)

	chatRepo := chat.NewRepository(db)
	chatService := chat.NewService(chatRepo, log)
	chatHandler := chat.NewHandler(chatService, log)

	msgRepo := message.NewRepository(db)
	msgKafkaProducer := message.NewMessageProducer(kafkaProducer)
	msgPublisher := message.NewPublisher(redisClient, connManager, log)
	msgService := message.NewService(msgRepo, msgKafkaProducer, msgPublisher, chatRepo, log)
	msgHandler := message.NewHandler(msgService, log)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(server.RequestIDMiddleware)

	r.Get("/health", healthHandler(db, redisClient))
	r.Get("/ready", readyHandler())

	r.Route("/api/v1/auth", authHandler.RegisterRoutes)
	r.Get("/api/v1/ws", wsServer.ServeWs)

	r.Group(func(r chi.Router) {
		r.Use(server.AuthMiddleware(cfg, redisClient))
		userHandler.RegisterRoutes(r)
		contactsHandler.RegisterRoutes(r)

		r.Route("/chats", func(r chi.Router) {
			chatHandler.RegisterRoutes(r)
			msgHandler.RegisterRoutes(r)
		})
	})

	httpServer := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()

	msgConsumer := message.NewConsumer(cfg, msgService, log)
	go func() {
		slog.Info("starting kafka consumer")
		if err := msgConsumer.Start(consumerCtx); err != nil {
			slog.Error("kafka consumer error", "error", err)
		}
	}()

	go func() {
		slog.Info("starting http server", "port", cfg.ServerPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully...")

	consumerCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server forced to shutdown", "error", err)
	}

	if err := msgConsumer.Stop(); err != nil {
		slog.Error("failed to stop kafka consumer", "error", err)
	}

	slog.Info("server stopped")
}

func healthHandler(db interface{ Ping() error }, redisClient *redisv8.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy","reason":"database unreachable"}`))
			return
		}
		if _, err := redisClient.Ping(r.Context()).Result(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy","reason":"redis unreachable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}
}

func readyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	}
}
