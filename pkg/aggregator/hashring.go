package aggregator

import (
	"fmt"
	"hash/crc32"
	"sort"
	"sync"
)

// HashRing implements consistent hashing for distributing agents across
// aggregator shards. Each shard is represented as multiple virtual nodes
// on the ring to ensure even distribution.
type HashRing struct {
	mu           sync.RWMutex
	shards       map[string]int    // shard_id -> shard_index
	ring         []ringEntry
	virtualNodes int
}

type ringEntry struct {
	hash    uint32
	shardID string
}

// NewHashRing creates a consistent hash ring with the given virtual node count.
// Higher virtualNodes = more even distribution, typical: 150-300.
func NewHashRing(virtualNodes int) *HashRing {
	if virtualNodes == 0 {
		virtualNodes = 150
	}
	return &HashRing{
		shards:       make(map[string]int),
		virtualNodes: virtualNodes,
	}
}

// AddShard adds an aggregator shard to the ring.
func (hr *HashRing) AddShard(shardID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if _, exists := hr.shards[shardID]; exists {
		return
	}

	hr.shards[shardID] = len(hr.shards)

	for i := 0; i < hr.virtualNodes; i++ {
		vkey := fmt.Sprintf("%s#%d", shardID, i)
		hash := crc32.ChecksumIEEE([]byte(vkey))
		hr.ring = append(hr.ring, ringEntry{hash: hash, shardID: shardID})
	}

	sort.Slice(hr.ring, func(i, j int) bool {
		return hr.ring[i].hash < hr.ring[j].hash
	})
}

// RemoveShard removes an aggregator shard and redistributes its nodes.
func (hr *HashRing) RemoveShard(shardID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if _, exists := hr.shards[shardID]; !exists {
		return
	}

	delete(hr.shards, shardID)

	filtered := make([]ringEntry, 0, len(hr.ring)-hr.virtualNodes)
	for _, e := range hr.ring {
		if e.shardID != shardID {
			filtered = append(filtered, e)
		}
	}
	hr.ring = filtered
}

// GetShard returns the shard ID responsible for the given key (e.g., node_id).
func (hr *HashRing) GetShard(key string) string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.ring) == 0 {
		return ""
	}

	hash := crc32.ChecksumIEEE([]byte(key))

	idx := sort.Search(len(hr.ring), func(i int) bool {
		return hr.ring[i].hash >= hash
	})
	if idx >= len(hr.ring) {
		idx = 0
	}

	return hr.ring[idx].shardID
}

// GetShardCount returns the number of active shards.
func (hr *HashRing) GetShardCount() int {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return len(hr.shards)
}

// GetAllShards returns all registered shard IDs.
func (hr *HashRing) GetAllShards() []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	shards := make([]string, 0, len(hr.shards))
	for id := range hr.shards {
		shards = append(shards, id)
	}
	return shards
}

// GetDistribution returns how many virtual nodes each shard owns.
// Useful for verifying balanced distribution.
func (hr *HashRing) GetDistribution() map[string]int {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	dist := make(map[string]int)
	for _, e := range hr.ring {
		dist[e.shardID]++
	}
	return dist
}
