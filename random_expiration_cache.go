package cache

import (
	"context"
	"math/rand"
	"time"
)

// RandomExpirationCache 解决雪崩场景(一大批 K-V 同时过期)
type RandomExpirationCache struct {
	Cache
}

func (w *RandomExpirationCache) Set(ctx context.Context, key string, val any, expiration time.Duration) error {
	if expiration > 0 {
		// 加上一个 [0,300)s 的偏移量
		offset := time.Duration(rand.Intn(300)) * time.Second
		expiration = expiration + offset
		return w.Cache.Set(ctx, key, val, expiration)
	}
	return w.Cache.Set(ctx, key, val, expiration)
}
