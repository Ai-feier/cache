package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	errKeyNotFound = errors.New("cache：键不存在")
	//errKeyExpired = errors.New("cache：键过期")
)

type MapCacheOption func(cache *MapCache)

type MapCache struct {
	data map[string]*item
	//data sync.Map
	mutex sync.RWMutex
	close chan struct{}
	// 删除回调方法
	onEvicted func(key string, val any)
	//onEvicted func(ctx context.Context, key string, val any)
	//onEvicteds []func(key string, val any)
}

func NewMapCache(interval time.Duration, opts ...MapCacheOption) *MapCache {
	res := &MapCache{
		data:      make(map[string]*item, 100),
		close:     make(chan struct{}),
		onEvicted: func(key string, val any) {},
	}

	for _, opt := range opts {
		opt(res)
	}

	go func() {
		ticker := time.NewTicker(interval)
		for {
			// 定时检测批量数据, 删除过期 K-V
			select {
			case t := <-ticker.C:
				res.mutex.Lock()
				i := 0
				for key, val := range res.data {
					if i > 10000 {
						break
					}
					if val.deadlineBefore(t) {
						res.delete(key)
					}
					i++
				}
				res.mutex.Unlock()
			case <-res.close:
				return
			}
		}
	}()

	return res
}

func BuildInMapCacheWithEvictedCallback(fn func(key string, val any)) MapCacheOption {
	return func(cache *MapCache) {
		cache.onEvicted = fn
	}
}

func (b *MapCache) Set(ctx context.Context, key string, val any, expiration time.Duration) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.set(key, val, expiration)
}

func (b *MapCache) set(key string, val any, expiration time.Duration) error {
	var dl time.Time
	if expiration > 0 {
		dl = time.Now().Add(expiration)
	}
	b.data[key] = &item{
		val:      val,
		deadline: dl,
	}
	return nil
}

func (b *MapCache) Get(ctx context.Context, key string) (any, error) {
	b.mutex.RLock()
	res, ok := b.data[key]
	b.mutex.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w, key: %s", errKeyNotFound, key)
	}
	now := time.Now()
	if res.deadlineBefore(now) {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		res, ok = b.data[key]
		if !ok {
			return nil, fmt.Errorf("%w, key: %s", errKeyNotFound, key)
		}
		if res.deadlineBefore(now) {
			b.delete(key)
			return nil, fmt.Errorf("%w, key: %s", errKeyNotFound, key)
		}
	}
	return res.val, nil
}

func (b *MapCache) Delete(ctx context.Context, key string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.delete(key)
	return nil
}

func (b *MapCache) LoadAndDelete(ctx context.Context, key string) (any, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	val, ok := b.data[key]
	if !ok {
		return nil, errKeyNotFound
	}
	b.delete(key)
	return val.val, nil
}

func (b *MapCache) delete(key string) {
	itm, ok := b.data[key]
	if !ok {
		return
	}
	delete(b.data, key)
	b.onEvicted(key, itm.val)
}

func (b *MapCache) Close() error {
	select {
	case b.close <- struct{}{}:
	default:
		return errors.New("重复关闭")
	}
	return nil
}

type item struct {
	val      any
	deadline time.Time
}

func (i *item) deadlineBefore(t time.Time) bool {
	return !i.deadline.IsZero() && i.deadline.Before(t)
}
