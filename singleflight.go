package cache

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/singleflight"
	"time"
)

// 使用装饰器模式添加 singleflight 特性
type SingleflightCacheV1 struct {
	ReadThroughCache
}

func NewSingleflightCache(cache Cache, 
	loadFunc func(ctx context.Context, key string) (any, error), 
	expiration time.Duration) *SingleflightCacheV1 {
	
	g := &singleflight.Group{}
	return &SingleflightCacheV1{
		ReadThroughCache: ReadThroughCache{
			LoadFunc: func(ctx context.Context, key string) (any, error) {
				v, err, _ := g.Do(key, func() (interface{}, error) {
					return loadFunc(ctx, key)
				})
				return v, err
			},
			Cache: cache,
			Expiration: expiration,
		},
	}
}

type SingleflightCacheV2 struct {
	ReadThroughCache
	g singleflight.Group
}

func (s *SingleflightCacheV2) Get(ctx context.Context, key string) (any, error) {
	val, err := s.ReadThroughCache.Get(ctx, key)
	if errors.Is(err, errKeyNotFound) {
		val, err, _ = s.g.Do(key, func() (interface{}, error) {
			v, er := s.ReadThroughCache.LoadFunc(ctx, key)
			if er != nil {
				return nil, er
			}
			// 同步写入缓存, 可考虑异步
			er = s.Set(ctx, key, v, s.Expiration)
			if er != nil {
				return v, fmt.Errorf("%w, 原因：%s", ErrFailedToRefreshCache, er.Error())
			}
			return v, nil
		})
	}
	return val, err
}