package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

type Config struct {
	AddrEnv  string `yaml:"addr_env"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

func NewClient(cfg Config) (*Client, error) {
	addr := "localhost:6379"
	if cfg.AddrEnv != "" {
		if envAddr := os.Getenv(cfg.AddrEnv); envAddr != "" {
			addr = envAddr
		}
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

func (c *Client) SetHeartbeat(ctx context.Context, nodeID string, data map[string]interface{}) error {
	key := fmt.Sprintf("flowpulse:heartbeat:%s", nodeID)
	val, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, val, 30*time.Second).Err()
}

func (c *Client) GetHeartbeat(ctx context.Context, nodeID string) (map[string]interface{}, error) {
	key := fmt.Sprintf("flowpulse:heartbeat:%s", nodeID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Client) GetActiveAgents(ctx context.Context) ([]string, error) {
	keys, err := c.rdb.Keys(ctx, "flowpulse:heartbeat:*").Result()
	if err != nil {
		return nil, err
	}
	prefix := "flowpulse:heartbeat:"
	nodeIDs := make([]string, 0, len(keys))
	for _, key := range keys {
		nodeIDs = append(nodeIDs, key[len(prefix):])
	}
	return nodeIDs, nil
}

func (c *Client) CacheFlowCount(ctx context.Context, tenantID string, count uint64) error {
	key := fmt.Sprintf("flowpulse:flowcount:%s", tenantID)
	return c.rdb.Set(ctx, key, count, 10*time.Second).Err()
}

func (c *Client) GetFlowCount(ctx context.Context, tenantID string) (uint64, error) {
	key := fmt.Sprintf("flowpulse:flowcount:%s", tenantID)
	return c.rdb.Get(ctx, key).Uint64()
}

func (c *Client) RateLimit(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	fullKey := fmt.Sprintf("flowpulse:ratelimit:%s", key)
	now := time.Now().UnixMilli()

	pipe := c.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, fullKey, "-inf", fmt.Sprintf("%d", now-window.Milliseconds()))
	pipe.ZCard(ctx, fullKey)
	pipe.ZAdd(ctx, fullKey, redis.Z{Score: float64(now), Member: now})
	pipe.Expire(ctx, fullKey, window)

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	count := cmds[1].(*redis.IntCmd).Val()
	return count < limit, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}
