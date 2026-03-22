package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/example/lowcode-realtime/internal/broker"
	"github.com/example/lowcode-realtime/internal/gateway"
)

func main() {
	// Load .env from the current working directory (same as `go run` / binary cwd).
	// Does not override variables already set in the process environment.
	if err := godotenv.Load(); err != nil {
		log.Printf("env: no .env file loaded (%v) — using OS environment only", err)
	} else {
		log.Printf("env: loaded .env from working directory")
	}

	addr := envOr("SERVER_ADDR", ":8080")
	redisAddr := envOr("REDIS_ADDR", "127.0.0.1:6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := envOrInt("REDIS_DB", 0)
	broadcastMQType := envOr("BROADCAST_MQ_TYPE", "redis")
	broadcastMQAddr := envOr("BROADCAST_MQ_ADDR", redisAddr)
	broadcastMQPassword := envOr("BROADCAST_MQ_PASSWORD", "")
	broadcastMQDB := envOr("BROADCAST_MQ_DB", "0")
	presenceTTLSeconds := envOrInt("PRESENCE_TTL_SECONDS", 60)

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	mb, err := broker.NewMessageBroker(broker.BrokerConfig{
		BroadcastType: broadcastMQType,
		BroadcastCfg: map[string]string{
			"addr":     broadcastMQAddr,
			"password": broadcastMQPassword,
			"db":       broadcastMQDB,
		},
		PresenceCfg: broker.RedisPubSubConfig{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		},
	})
	if err != nil {
		log.Fatalf("init message broker failed: %v", err)
	}
	defer mb.Close()

	var jwtAuth *gateway.JWTAuth
	if sec := os.Getenv("JWT_SECRET"); sec != "" {
		jwtAuth = &gateway.JWTAuth{
			Secret:    []byte(sec),
			UserClaim: envOr("JWT_USER_CLAIM", "sub"),
		}
	}

	s := gateway.NewServer(mb, rdb, time.Duration(presenceTTLSeconds)*time.Second, jwtAuth)

	router := gin.Default()
	corsOrigins := gateway.ParseCORSOrigins(envOr("CORS_ALLOWED_ORIGINS", "*"))
	router.Use(gateway.CORSMiddleware(corsOrigins))
	s.RegisterRoutes(router)

	log.Printf("realtime server listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return def
}
