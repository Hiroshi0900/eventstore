package keyresolver

import (
	"strings"
	"testing"

	es "github.com/Hiroshi0900/eventstore"
)

func TestDefaultKeyResolver(t *testing.T) {
	t.Run("NewDefault creates resolver with default config", func(t *testing.T) {
		// given
		// no setup required

		// when
		sut := NewDefault()

		// then
		if sut.GetShardCount() != 1 {
			t.Errorf("GetShardCount() = %d, want %d", sut.GetShardCount(), 1)
		}
	})

	t.Run("New with zero ShardCount defaults to 1", func(t *testing.T) {
		// given
		config := Config{ShardCount: 0}

		// when
		sut := New(config)

		// then
		if sut.GetShardCount() != 1 {
			t.Errorf("GetShardCount() = %d, want %d", sut.GetShardCount(), 1)
		}
	})

	t.Run("ResolvePartitionKey without sharding", func(t *testing.T) {
		// given
		sut := NewDefault()
		id := es.NewAggregateId("MemorialSetting", "abc123")

		// when
		pkey := sut.ResolvePartitionKey(id)

		// then
		expected := "MemorialSetting-0" // With ShardCount=1, shard ID is always 0
		if pkey != expected {
			t.Errorf("ResolvePartitionKey() = %q, want %q", pkey, expected)
		}
	})

	t.Run("ResolvePartitionKey with sharding", func(t *testing.T) {
		// given
		sut := New(Config{ShardCount: 4})
		id := es.NewAggregateId("MemorialSetting", "abc123")

		// when
		pkey := sut.ResolvePartitionKey(id)

		// then
		if !strings.HasPrefix(pkey, "MemorialSetting-") {
			t.Errorf("ResolvePartitionKey() = %q, should start with 'MemorialSetting-'", pkey)
		}
		suffix := pkey[len("MemorialSetting-"):]
		if suffix != "0" && suffix != "1" && suffix != "2" && suffix != "3" {
			t.Errorf("ResolvePartitionKey() = %q, shard ID should be 0-3", pkey)
		}
	})

	t.Run("ResolveSortKeyForEvent includes zero-padded sequence number", func(t *testing.T) {
		// given
		sut := NewDefault()
		id := es.NewAggregateId("MemorialSetting", "abc123")

		// when
		skey := sut.ResolveSortKeyForEvent(id, 42)

		// then
		expected := "MemorialSetting-abc123-00000000000000000042"
		if skey != expected {
			t.Errorf("ResolveSortKeyForEvent() = %q, want %q", skey, expected)
		}
	})

	t.Run("ResolveSortKeyForEvent handles large sequence numbers", func(t *testing.T) {
		// given
		sut := NewDefault()
		id := es.NewAggregateId("Test", "1")

		// when
		skey := sut.ResolveSortKeyForEvent(id, 12345678901234567890)

		// then
		if !strings.HasSuffix(skey, "12345678901234567890") {
			t.Errorf("ResolveSortKeyForEvent() = %q, should end with the sequence number", skey)
		}
	})

	t.Run("ResolveSortKeyForSnapshot uses 0 as suffix", func(t *testing.T) {
		// given
		sut := NewDefault()
		id := es.NewAggregateId("MemorialSetting", "abc123")

		// when
		skey := sut.ResolveSortKeyForSnapshot(id)

		// then
		expected := "MemorialSetting-abc123-0"
		if skey != expected {
			t.Errorf("ResolveSortKeyForSnapshot() = %q, want %q", skey, expected)
		}
	})

	t.Run("ResolveAggregateIDKey returns AsString format", func(t *testing.T) {
		// given
		sut := NewDefault()
		id := es.NewAggregateId("MemorialSetting", "abc123")

		// when
		aidKey := sut.ResolveAggregateIDKey(id)

		// then
		expected := "MemorialSetting-abc123"
		if aidKey != expected {
			t.Errorf("ResolveAggregateIDKey() = %q, want %q", aidKey, expected)
		}
	})

	t.Run("ResolveEventKeys returns all components", func(t *testing.T) {
		// given
		sut := NewDefault()
		id := es.NewAggregateId("MemorialSetting", "abc123")

		// when
		keys := sut.ResolveEventKeys(id, 5)

		// then
		if keys.PartitionKey != "MemorialSetting-0" {
			t.Errorf("PartitionKey = %q, want %q", keys.PartitionKey, "MemorialSetting-0")
		}
		if keys.SortKey != "MemorialSetting-abc123-00000000000000000005" {
			t.Errorf("SortKey = %q, want %q", keys.SortKey, "MemorialSetting-abc123-00000000000000000005")
		}
		if keys.AggregateIDKey != "MemorialSetting-abc123" {
			t.Errorf("AggregateIDKey = %q, want %q", keys.AggregateIDKey, "MemorialSetting-abc123")
		}
	})

	t.Run("ResolveSnapshotKeys returns all components", func(t *testing.T) {
		// given
		sut := NewDefault()
		id := es.NewAggregateId("MemorialSetting", "abc123")

		// when
		keys := sut.ResolveSnapshotKeys(id)

		// then
		if keys.PartitionKey != "MemorialSetting-0" {
			t.Errorf("PartitionKey = %q, want %q", keys.PartitionKey, "MemorialSetting-0")
		}
		if keys.SortKey != "MemorialSetting-abc123-0" {
			t.Errorf("SortKey = %q, want %q", keys.SortKey, "MemorialSetting-abc123-0")
		}
		if keys.AggregateIDKey != "MemorialSetting-abc123" {
			t.Errorf("AggregateIDKey = %q, want %q", keys.AggregateIDKey, "MemorialSetting-abc123")
		}
	})
}

func TestShardDistribution(t *testing.T) {
	t.Run("Different IDs distribute across shards", func(t *testing.T) {
		// given
		sut := New(Config{ShardCount: 4})
		ids := []string{"id1", "id2", "id3", "id4", "id5", "id6", "id7", "id8", "id9", "id10"}

		// when
		shardCounts := make(map[string]int)
		for _, idValue := range ids {
			id := es.NewAggregateId("Test", idValue)
			pkey := sut.ResolvePartitionKey(id)
			shardCounts[pkey]++
		}

		// then
		if len(shardCounts) == 1 {
			t.Error("All IDs ended up in the same shard, expected distribution")
		}
	})

	t.Run("Same ID always maps to same shard", func(t *testing.T) {
		// given
		sut := New(Config{ShardCount: 4})
		id := es.NewAggregateId("Test", "consistent-id")

		// when
		pkey1 := sut.ResolvePartitionKey(id)
		pkey2 := sut.ResolvePartitionKey(id)
		pkey3 := sut.ResolvePartitionKey(id)

		// then
		if pkey1 != pkey2 || pkey2 != pkey3 {
			t.Errorf("Same ID should always map to same partition key: %q, %q, %q", pkey1, pkey2, pkey3)
		}
	})
}

// TestComputeShardID_Boundary kills the CONDITIONALS_BOUNDARY mutant on L74
// (ShardCount <= 1 vs < 1): with ShardCount=1 shard must always be 0;
// with ShardCount=2 non-zero shards must be possible.
func TestComputeShardID_Boundary(t *testing.T) {
	t.Run("ShardCount=1 always returns shard 0 (no sharding)", func(t *testing.T) {
		// given
		// resolver with ShardCount=1 (the boundary value)
		sut := New(Config{ShardCount: 1})
		testIDs := []string{"aaa", "bbb", "ccc", "12345", "xyz"}

		// when
		// resolve partition keys for multiple IDs

		// then
		for _, v := range testIDs {
			id := es.NewAggregateId("Test", v)
			pkey := sut.ResolvePartitionKey(id)
			// with ShardCount=1, computeShardID must return 0
			expected := "Test-0"
			if pkey != expected {
				t.Errorf("ShardCount=1: ResolvePartitionKey(%q) = %q, want %q", v, pkey, expected)
			}
		}
	})

	t.Run("ShardCount=2 can return shard 1 (sharding is active)", func(t *testing.T) {
		// given
		// resolver with ShardCount=2 (just above boundary)
		sut := New(Config{ShardCount: 2})

		// when
		// try many IDs until we find one that hashes to shard 1
		foundShard1 := false
		for i := 0; i < 100; i++ {
			id := es.NewAggregateId("Test", string(rune('a'+i%26))+string(rune('a'+(i/26)%26)))
			pkey := sut.ResolvePartitionKey(id)
			if pkey == "Test-1" {
				foundShard1 = true
				break
			}
		}

		// then
		if !foundShard1 {
			t.Error("ShardCount=2 should distribute some IDs to shard 1, but none found")
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	t.Run("DefaultConfig returns expected values", func(t *testing.T) {
		// given
		// no setup required

		// when
		config := DefaultConfig()

		// then
		if config.ShardCount != 1 {
			t.Errorf("ShardCount = %d, want %d", config.ShardCount, 1)
		}
	})
}
