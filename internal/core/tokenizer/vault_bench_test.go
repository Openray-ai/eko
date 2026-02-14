package tokenizer

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// --- helpers ---

// benchSessionIDs pre-generates n valid session IDs to exclude generation cost from timings.
func benchSessionIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = GenerateSessionID()
	}
	return ids
}

// newBenchVaultManager creates a VaultManager suitable for benchmarks (long TTL, no interference from cleanup).
func newBenchVaultManager() *VaultManager {
	return NewVaultManager(time.Hour)
}

// --- GetOrCreate benchmarks ---

func BenchmarkVaultManager_GetOrCreate_New(b *testing.B) {
	b.ReportAllocs()
	ids := benchSessionIDs(b.N)
	vm := NewVaultManager(time.Hour, WithMaxVaults(b.N+1))
	defer vm.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vm.GetOrCreate(ids[i])
	}
}

func BenchmarkVaultManager_GetOrCreate_CacheHit(b *testing.B) {
	b.ReportAllocs()
	vm := newBenchVaultManager()
	defer vm.Stop()

	id := GenerateSessionID()
	_, _ = vm.GetOrCreate(id)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vm.GetOrCreate(id)
	}
}

func BenchmarkVaultManager_GetOrCreate_Scale(b *testing.B) {
	for _, vaultCount := range []int{10, 100, 1000, 5000, 10000} {
		b.Run(fmt.Sprintf("vaults_%d", vaultCount), func(b *testing.B) {
			b.ReportAllocs()
			vm := NewVaultManager(time.Hour, WithMaxVaults(vaultCount+1))
			defer vm.Stop()

			// Pre-fill with vaultCount vaults.
			ids := benchSessionIDs(vaultCount)
			for _, id := range ids {
				_, _ = vm.GetOrCreate(id)
			}

			// Benchmark lookup of an existing vault.
			target := ids[vaultCount/2]
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = vm.GetOrCreate(target)
			}
		})
	}
}

func BenchmarkVaultManager_GetOrCreate_Concurrent(b *testing.B) {
	for _, goroutines := range []int{1, 4, 16, 64, 256} {
		b.Run(fmt.Sprintf("goroutines_%d", goroutines), func(b *testing.B) {
			b.ReportAllocs()
			vm := newBenchVaultManager()
			defer vm.Stop()

			id := GenerateSessionID()
			_, _ = vm.GetOrCreate(id)

			b.SetParallelism(goroutines)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = vm.GetOrCreate(id)
				}
			})
		})
	}
}

// --- Cleanup benchmarks ---

func BenchmarkVaultManager_Cleanup(b *testing.B) {
	for _, tc := range []struct {
		vaults      int
		expiredPct  int
	}{
		{100, 0},
		{100, 25},
		{100, 50},
		{100, 100},
		{1000, 0},
		{1000, 50},
		{1000, 100},
		{5000, 50},
		{10000, 50},
	} {
		name := fmt.Sprintf("vaults_%d_expired_%d%%", tc.vaults, tc.expiredPct)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				vm := NewVaultManager(time.Hour, WithMaxVaults(tc.vaults+1))

				ids := benchSessionIDs(tc.vaults)
				for _, id := range ids {
					_, _ = vm.GetOrCreate(id)
				}

				// Expire the requested percentage of vaults.
				expireCount := tc.vaults * tc.expiredPct / 100
				now := time.Now()
				vm.mu.Lock()
				idx := 0
				for _, vault := range vm.vaults {
					if idx >= expireCount {
						break
					}
					vault.mu.Lock()
					vault.expiresAt = now.Add(-time.Second)
					vault.mu.Unlock()
					idx++
				}
				vm.mu.Unlock()

				b.StartTimer()
				vm.Cleanup()
				b.StopTimer()
				vm.Stop()
			}
		})
	}
}

// --- Vault.Store / Vault.GetToken benchmarks ---

func BenchmarkVault_Store(b *testing.B) {
	b.ReportAllocs()
	vm := newBenchVaultManager()
	defer vm.Stop()

	vault, _ := vm.GetOrCreate(GenerateSessionID())

	originals := make([]string, b.N)
	tokens := make([]string, b.N)
	for i := range originals {
		originals[i] = fmt.Sprintf("original-%d@example.com", i)
		tokens[i] = fmt.Sprintf("token-%d@example.com", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vault.Store(originals[i], tokens[i])
	}
}

func BenchmarkVault_GetToken(b *testing.B) {
	b.ReportAllocs()
	vm := newBenchVaultManager()
	defer vm.Stop()

	vault, _ := vm.GetOrCreate(GenerateSessionID())
	const preload = 1000
	keys := make([]string, preload)
	for i := 0; i < preload; i++ {
		key := fmt.Sprintf("original-%d@example.com", i)
		keys[i] = key
		_ = vault.Store(key, fmt.Sprintf("token-%d@example.com", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vault.GetToken(keys[i%preload])
	}
}

// --- Vault.ReverseTokens benchmarks ---

func BenchmarkVault_ReverseTokens(b *testing.B) {
	for _, tokenCount := range []int{1, 10, 100, 1000, 10000} {
		b.Run(fmt.Sprintf("tokens_%d", tokenCount), func(b *testing.B) {
			b.ReportAllocs()
			vm := newBenchVaultManager()
			defer vm.Stop()

			vault, _ := vm.GetOrCreate(GenerateSessionID())
			for i := 0; i < tokenCount; i++ {
				_ = vault.Store(
					fmt.Sprintf("orig-%d@example.com", i),
					fmt.Sprintf("tok-%d@example.com", i),
				)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reverse := vault.ReverseTokens()
				runtime.KeepAlive(reverse)
			}
		})
	}
}

// --- Vault.StoreSubTokens benchmark ---

func BenchmarkVault_StoreSubTokens(b *testing.B) {
	b.ReportAllocs()
	vm := newBenchVaultManager()
	defer vm.Stop()

	vault, _ := vm.GetOrCreate(GenerateSessionID())

	var counter atomic.Uint64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := counter.Add(1)
		subs := map[string]string{
			fmt.Sprintf("examp%d.eac", c): fmt.Sprintf("gmail%d.com", c),
			fmt.Sprintf("eac%d", c):       fmt.Sprintf("com%d", c),
			fmt.Sprintf("usa%d", c):       fmt.Sprintf("eko%d", c),
			fmt.Sprintf("examp%d", c):     fmt.Sprintf("gmail%d", c),
		}
		vault.StoreSubTokens(subs)
	}
}
