package cache

import (
	"context"
	"errors"
	"fmt"
)

type BloomFilterCache struct {
	ReadThroughCache
}

func NewBloomFilterCache(cache Cache, bf BloomFilter,
	loadFunc func(ctx context.Context, key string) (any, error)) *BloomFilterCache {
	return &BloomFilterCache{
		ReadThroughCache: ReadThroughCache{
			Cache: cache,
			LoadFunc: func(ctx context.Context, key string) (any, error) {
				// 如果布隆过滤器不存在就不查询数据库
				if !bf.HasKey(ctx, key) {
					return nil, errKeyNotFound
				}
				return loadFunc(ctx, key)
			},
		},
	}
}

type BloomFilterCacheV1 struct {
	ReadThroughCache
	Bf BloomFilter
}

func (r *BloomFilterCacheV1) Get(ctx context.Context, key string) (any, error) {
	val, err := r.Cache.Get(ctx, key)
	if errors.Is(err, errKeyNotFound) && r.Bf.HasKey(ctx, key) {
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

// BloomFilter 需提供布隆过滤器接口实现
type BloomFilter interface {
	HasKey(ctx context.Context, key string) bool
}
