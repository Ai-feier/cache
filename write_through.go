package cache

import (
	"context"
	"log"
	"time"
)

type WriteThroughCache struct {
	Cache
	// 用户提供写数据库方法
	StoreFunc func(ctx context.Context, key string, val any) error
}

// Set 先写数据库, 在更新缓存
func (w *WriteThroughCache) Set(ctx context.Context, key string, val any, expiration time.Duration) error {
	err := w.StoreFunc(ctx, key, val)
	if err != nil {
		return err
	}
	return w.Cache.Set(ctx, key, val, expiration)
}

// SetV2 先写数据库后, 异步写缓存
func (w *WriteThroughCache) SetV2(ctx context.Context, key string, val any, expiration time.Duration) error {
	err := w.StoreFunc(ctx, key, val)
	go func() {
		er := w.Cache.Set(ctx, key, val, expiration)
		if er != nil {
			log.Fatalln(er)
		}
	}()
	return err
}

// SetV3 全异步写数据库和缓存, 出错不好处理
func (w *WriteThroughCache) SetV3(ctx context.Context, key string, val any, expiration time.Duration) error {
	go func() {
		err := w.StoreFunc(ctx, key, val)
		if err != nil {
			log.Fatalln(err)
		}
		if err = w.Cache.Set(ctx, key, val, expiration); err != nil {
			log.Fatalln(err)
		}
	}()
	return nil
}

type WriteThroughCacheV1[T any] struct {
	Cache
	StoreFunc func(ctx context.Context, key string, val T) error
}

func (w *WriteThroughCacheV1[T]) Set(ctx context.Context, key string, val any, expiration time.Duration) error {
	err := w.StoreFunc(ctx, key, val.(T))
	if err != nil {
		return err
	}
	return w.Cache.Set(ctx, key, val, expiration)
}
