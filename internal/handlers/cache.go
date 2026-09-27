package handlers

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// urlCacheTTL bounds how long a stale entry can live. Update and Delete evict
// the key, but an eviction can fail or race a concurrent cache fill in Get.
const urlCacheTTL = time.Hour

func urlCacheKey(code string) string {
	return "url:" + code
}

// Redis is only a cache in front of Postgres, so none of these helpers fail the
// request: errors are logged and a failed read is treated as a miss.

// cachedURL returns the cached target URL for code and whether it was found.
func (h *Handler) cachedURL(ctx context.Context, code string) (string, bool) {
	target, err := h.rdb.Get(ctx, urlCacheKey(code)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("redis: get %s: %v", code, err)
		}
		return "", false
	}
	return target, true
}

func (h *Handler) cacheURL(ctx context.Context, code, target string) {
	if err := h.rdb.Set(ctx, urlCacheKey(code), target, urlCacheTTL).Err(); err != nil {
		log.Printf("redis: set %s: %v", code, err)
	}
}

func (h *Handler) uncacheURL(ctx context.Context, code string) {
	if err := h.rdb.Del(ctx, urlCacheKey(code)).Err(); err != nil {
		log.Printf("redis: del %s: %v", code, err)
	}
}
