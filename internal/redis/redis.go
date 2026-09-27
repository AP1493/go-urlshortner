package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
}

// envOr returns the trimmed value of key, or fallback when it is unset. The
// fallbacks match a Redis running on the host, so `go run ./cmd` works without
// extra config; compose overrides REDIS_HOST with the service name.
func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func InitRedis() (Redis, error) {
	host := envOr("REDIS_HOST", "localhost")

	useTLS, err := strconv.ParseBool(envOr("REDIS_TLS", "false"))
	if err != nil {
		return Redis{}, fmt.Errorf("invalid REDIS_TLS %q: %w", os.Getenv("REDIS_TLS"), err)
	}

	opts := &redis.Options{
		Addr:     net.JoinHostPort(host, envOr("REDIS_PORT", "6379")),
		Password: os.Getenv("REDIS_PASSWORD"),

		// Redis is only a cache, so when it is down a request should give up
		// quickly and fall back to Postgres. The defaults retry 4 commands x 5
		// dials, which stalls every redirect for seconds.
		DialTimeout:   500 * time.Millisecond,
		DialerRetries: 1,
		MaxRetries:    1,
		ReadTimeout:   500 * time.Millisecond,
	}

	// ElastiCache with in-transit encryption only accepts TLS. Its certificate
	// is checked against the system roots (ca-certificates in the image).
	if useTLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
	}

	client := redis.NewClient(opts)

	// NewClient connects lazily, so ping once to fail at startup rather than on
	// the first request when the address or password is wrong.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return Redis{}, fmt.Errorf("failed to connect redis: %w", err)
	}

	return Redis{client: client}, nil
}

func (r *Redis) Client() *redis.Client {
	return r.client
}

func (r *Redis) Close() error {
	return r.client.Close()
}
