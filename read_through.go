package cache

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/singleflight"
	"log"
	"time"
)

var (
	ErrFailedToRefreshCache = errors.New("刷新缓存失败")
)

// ReadThroughCache
// Expiration 是你的过期时间
type ReadThroughCache struct {
	Cache
	// 从数据库中加载数据
	LoadFunc   func(ctx context.Context, key string) (any, error)
	Expiration time.Duration
	//loadFunc func(ctx context.Context, key string) (any, error)
	//LoadFunc func(key string) (any, error)
	//logFunc func()
	// 对于高并发场景, 仅允许单一goruntine操作
	g singleflight.Group
}

func (r *ReadThroughCache) Get(ctx context.Context, key string) (any, error) {
	val, err := r.Cache.Get(ctx, key)
	if errors.Is(err, errKeyNotFound) {
		// 全程单 goroutine, 有损性能但确保存在的数据返回, 对于缓存穿透场景(数据库中不存在)不佳
		val, err = r.LoadFunc(ctx, key)
		if err == nil {
			er := r.Cache.Set(ctx, key, val, r.Expiration)
			if er != nil {
				return val, fmt.Errorf("%w, 原因：%s", ErrFailedToRefreshCache, er.Error())
			}
		}
	}
	return val, err
}

func (r *ReadThroughCache) GetV1(ctx context.Context, key string) (any, error) {
	val, err := r.Cache.Get(ctx, key)
	if errors.Is(err, errKeyNotFound) {
		// 直接将缓存中的数据返回, 性能优先
		// 异步从数据库加载, 在写入缓存
		go func() {
			val, err = r.LoadFunc(ctx, key)
			if err == nil {
				er := r.Cache.Set(ctx, key, val, r.Expiration)
				if er != nil {
					log.Fatalln(er)
				}
			}
		}()
	}
	return val, err
}

func (r *ReadThroughCache) GetV2(ctx context.Context, key string) (any, error) {
	val, err := r.Cache.Get(ctx, key)
	if errors.Is(err, errKeyNotFound) {
		// 缓存中不存在数据就从数据库中读取
		// 后异步将数据更新到缓存
		val, err = r.LoadFunc(ctx, key)
		if err == nil {
			go func() {
				er := r.Cache.Set(ctx, key, val, r.Expiration)
				if er != nil {
					log.Fatalln(er)
				}
			}()
		}
	}
	return val, err
}

// GetV3 对于高并发场景, 可使用singleflight进行优化
func (r *ReadThroughCache) GetV3(ctx context.Context, key string) (any, error) {
	val, err := r.Cache.Get(ctx, key)
	if err == errKeyNotFound {
		val, err, _ = r.g.Do(key, func() (interface{}, error) {
			v, er := r.LoadFunc(ctx, key)
			if er == nil {
				er = r.Cache.Set(ctx, key, val, r.Expiration)
				if er != nil {
					return v, fmt.Errorf("%w, 原因：%s", ErrFailedToRefreshCache, er.Error())
				}
			}
			return v, er
		})
	}
	return val, err
}

type ReadThroughCacheV1[T any] struct {
	Cache
	LoadFunc   func(ctx context.Context, key string) (T, error)
	Expiration time.Duration
	//loadFunc func(ctx context.Context, key string) (any, error)
	//LoadFunc func(key string) (any, error)
	//logFunc func()
	g singleflight.Group
}

func (r *ReadThroughCacheV1[T]) Get(ctx context.Context, key string) (T, error) {
	val, err := r.Cache.Get(ctx, key)
	if errors.Is(err, errKeyNotFound) {
		val, err = r.LoadFunc(ctx, key)
		if err == nil {
			er := r.Cache.Set(ctx, key, val, r.Expiration)
			if er != nil {
				return val.(T), fmt.Errorf("%w, 原因：%s", ErrFailedToRefreshCache, er.Error())
			}
		}
	}
	return val.(T), err
}
