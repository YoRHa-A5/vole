package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"net/http"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if it exists (does nothing if missing)
	godotenv.Load()

	listenAddr := getEnv("LISTEN_ADDR", ":9090")
	readTimeout := getDurationEnv("READ_TIMEOUT", 30*time.Second)
	connectTimeout := getDurationEnv("CONNECT_TIMEOUT", 15*time.Second)
	maxBodySize := getIntEnv("MAX_BODY_SIZE", 10*1024*1024) // 10 MB default
	userAgent := getEnv("USER_AGENT", "vole/1.0")

	client := newExtractClient(connectTimeout, readTimeout, int64(maxBodySize), userAgent)
	h := newHandler(client)

	log.Printf("vole listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, h); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func getIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
			return n
		}
	}
	return defaultVal
}
