/*
Copyright 2025 Stoolap Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package fastmap

import (
	"iter"
)

// Pair contains a key-value pair
type Pair[V any] struct {
	Key   int64
	Value V
}

// Int64Map is a specialized map for int64 keys.
// It is specifically optimized for clustered keys, where keys are likely
// to be close to each other (e.g., sequential IDs, timestamp ranges).
type Int64Map[V any] struct {
	data       []Pair[V] // key-value pairs
	size       int       // current size
	growAt     int       // size threshold for growth
	mask       int       // power-of-2 mask
	hasZeroKey bool      // special case for zero key
	zeroVal    V         // value for zero key
}

// NewInt64Map creates a new Int64Map with given capacity
func NewInt64Map[V any](capacity int) *Int64Map[V] {
	// Ensure capacity is at least 8 and is a power of 2
	size := 8
	if capacity > 8 {
		capacity = int(float64(capacity) / 0.75)
		size = 1
		for size < capacity {
			size *= 2
		}
	}

	return &Int64Map[V]{
		data:   make([]Pair[V], size),
		mask:   size - 1,
		growAt: int(float64(size) * 0.75),
	}
}

// Has checks if key exists in the map
func (m *Int64Map[V]) Has(key int64) bool {
	// Nil map check
	if m == nil {
		return false
	}

	// Special case for zero key
	if key == 0 {
		return m.hasZeroKey
	}

	// Calculate primary hash and secondary hash
	// Primary hash is the same as in the standard map
	// Secondary hash is optimized for clustered keys
	idx := m.primaryIndex(key)

	// Direct hit check (most common case)
	if m.data[idx].Key == key {
		return true
	}

	// Empty slot check (early exit)
	if m.data[idx].Key == 0 {
		return false
	}

	// Collision: try secondary probing
	// This is optimized for clustered keys by using a different probing sequence
	secondary := m.secondaryIndex(key)
	probeDistance := 1

	for i := 0; i < len(m.data); i++ {
		// For clustered keys, use quadratic probing with the secondary hash
		// This creates less clustering than linear probing for sequential keys
		idx = (idx + probeDistance*secondary) & m.mask
		probeDistance++

		if m.data[idx].Key == key {
			return true
		}
		if m.data[idx].Key == 0 {
			return false
		}
	}

	return false
}

// Get retrieves a value by key
func (m *Int64Map[V]) Get(key int64) (V, bool) {
	// Nil map check
	if m == nil {
		var zero V
		return zero, false
	}

	// Special case for zero key
	if key == 0 {
		if m.hasZeroKey {
			return m.zeroVal, true
		}
		var zero V
		return zero, false
	}

	// Calculate primary and secondary indices
	idx := m.primaryIndex(key)

	// Direct hit check (most common case)
	if m.data[idx].Key == key {
		return m.data[idx].Value, true
	}

	// Empty slot check (early exit)
	if m.data[idx].Key == 0 {
		var zero V
		return zero, false
	}

	// Collision: try secondary probing
	secondary := m.secondaryIndex(key)
	probeDistance := 1

	for i := 0; i < len(m.data); i++ {
		// Quadratic probing with secondary hash
		idx = (idx + probeDistance*secondary) & m.mask
		probeDistance++

		if m.data[idx].Key == key {
			return m.data[idx].Value, true
		}
		if m.data[idx].Key == 0 {
			var zero V
			return zero, false
		}
	}

	// Should never happen in properly sized map
	var zero V
	return zero, false
}

// Put adds or updates key with value
func (m *Int64Map[V]) Put(key int64, val V) {
	// Special case for zero key
	if key == 0 {
		if !m.hasZeroKey {
			m.size++
		}
		m.zeroVal = val
		m.hasZeroKey = true
		return
	}

	// Growth check
	if m.size >= m.growAt {
		m.grow()
	}

	// Calculate primary and secondary indices
	idx := m.primaryIndex(key)

	// Fast path: empty slot
	if m.data[idx].Key == 0 {
		m.data[idx].Key = key
		m.data[idx].Value = val
		m.size++
		return
	}

	// Fast path: update existing key
	if m.data[idx].Key == key {
		m.data[idx].Value = val
		return
	}

	// Collision: try secondary probing
	secondary := m.secondaryIndex(key)
	probeDistance := 1

	for i := 0; i < len(m.data); i++ {
		// Quadratic probing with secondary hash
		idx = (idx + probeDistance*secondary) & m.mask
		probeDistance++

		if m.data[idx].Key == 0 {
			m.data[idx].Key = key
			m.data[idx].Value = val
			m.size++
			return
		}
		if m.data[idx].Key == key {
			m.data[idx].Value = val
			return
		}
	}

	// Should never happen in properly sized map, but just in case
	m.grow()
	m.Put(key, val)
}

// PutIfNotExists adds key-value pair only if key doesn't exist
func (m *Int64Map[V]) PutIfNotExists(key int64, val V) (V, bool) {
	// Special case for zero key
	if key == 0 {
		if m.hasZeroKey {
			return m.zeroVal, false
		}
		m.hasZeroKey = true
		m.zeroVal = val
		m.size++
		return val, true
	}

	// Growth check
	if m.size >= m.growAt {
		m.grow()
	}

	// Calculate primary and secondary indices
	idx := m.primaryIndex(key)

	// Fast path: empty slot
	if m.data[idx].Key == 0 {
		m.data[idx].Key = key
		m.data[idx].Value = val
		m.size++
		return val, true
	}

	// Check existing key
	if m.data[idx].Key == key {
		return m.data[idx].Value, false
	}

	// Collision: try secondary probing
	secondary := m.secondaryIndex(key)
	probeDistance := 1

	for i := 0; i < len(m.data); i++ {
		// Quadratic probing with secondary hash
		idx = (idx + probeDistance*secondary) & m.mask
		probeDistance++

		if m.data[idx].Key == 0 {
			m.data[idx].Key = key
			m.data[idx].Value = val
			m.size++
			return val, true
		}
		if m.data[idx].Key == key {
			return m.data[idx].Value, false
		}
	}

	// Should never happen in properly sized map, but just in case
	m.grow()
	return m.PutIfNotExists(key, val)
}

// Del deletes a key and its value
func (m *Int64Map[V]) Del(key int64) bool {
	// Nil map check
	if m == nil {
		return false
	}

	// Special case for zero key
	if key == 0 {
		if m.hasZeroKey {
			var zero V
			m.hasZeroKey = false
			m.zeroVal = zero
			m.size--
			return true
		}
		return false
	}

	// Calculate primary and secondary indices
	idx := m.primaryIndex(key)

	// Check if key exists at primary position
	if m.data[idx].Key == key {
		// Use tombstone approach for deletion
		// This is simpler and faster for workloads with minimal deletions
		var zero V
		m.data[idx].Key = 0
		m.data[idx].Value = zero
		m.size--

		// Fix the deletion by re-inserting affected keys
		m.fixChain(idx)
		return true
	}

	// Early exit if empty slot
	if m.data[idx].Key == 0 {
		return false
	}

	// Collision: try secondary probing
	secondary := m.secondaryIndex(key)
	probeDistance := 1

	for i := 0; i < len(m.data); i++ {
		// Quadratic probing with secondary hash
		idx = (idx + probeDistance*secondary) & m.mask
		probeDistance++

		if m.data[idx].Key == key {
			// Found key, mark as deleted and fix chain
			var zero V
			m.data[idx].Key = 0
			m.data[idx].Value = zero
			m.size--
			m.fixChain(idx)
			return true
		}
		if m.data[idx].Key == 0 {
			return false
		}
	}

	return false
}

// ForEach iterates through all key-value pairs
func (m *Int64Map[V]) ForEach(f func(int64, V) bool) {
	if m == nil {
		return
	}

	// Zero key first
	if m.hasZeroKey && !f(0, m.zeroVal) {
		return
	}

	// Then all other keys
	for _, p := range m.data {
		if p.Key != 0 && !f(p.Key, p.Value) {
			return
		}
	}
}

// All returns an iterator over key-value pairs
func (m *Int64Map[V]) All() iter.Seq2[int64, V] {
	return m.ForEach
}

// Keys returns an iterator over keys
func (m *Int64Map[V]) Keys() iter.Seq[int64] {
	return func(yield func(int64) bool) {
		if m == nil {
			return
		}

		// Zero key first
		if m.hasZeroKey && !yield(0) {
			return
		}

		// Then all other keys
		for _, p := range m.data {
			if p.Key != 0 && !yield(p.Key) {
				return
			}
		}
	}
}

// Values returns an iterator over values
func (m *Int64Map[V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		if m == nil {
			return
		}

		// Zero key's value first
		if m.hasZeroKey && !yield(m.zeroVal) {
			return
		}

		// Then all other values
		for _, p := range m.data {
			if p.Key != 0 && !yield(p.Value) {
				return
			}
		}
	}
}

// Clear removes all items from the map
func (m *Int64Map[V]) Clear() {
	if m == nil {
		return
	}

	// Reset zero key
	var zero V
	m.hasZeroKey = false
	m.zeroVal = zero

	// Clear all entries
	for i := range m.data {
		m.data[i] = Pair[V]{}
	}

	m.size = 0
}

// Len returns the number of elements in the map
func (m *Int64Map[V]) Len() int {
	if m == nil {
		return 0
	}
	return m.size
}

// Internal helper functions

// primaryIndex calculates the initial index for a key
func (m *Int64Map[V]) primaryIndex(key int64) int {
	// Primary hash function optimized for integer keys
	h := key * int64(0x9E3779B9)
	return int(h^(h>>16)) & m.mask
}

// secondaryIndex calculates a secondary index for quadratic probing
// This helps reduce clustering for sequential or clustered keys
func (m *Int64Map[V]) secondaryIndex(key int64) int {
	// Secondary hash function uses a different constant multiplier
	// This creates a different distribution pattern
	h := key * int64(0x85EBCA77)
	return int(h^(h>>13)) | 1 // Ensure it's odd for better distribution
}

// grow increases the size of the map and rehashes all entries
func (m *Int64Map[V]) grow() {
	// Use a more conservative growth factor for large maps
	// For small maps, double the size
	// For large maps (>1M entries), use 1.5x growth to reduce memory spikes
	oldLen := len(m.data)
	newLen := oldLen * 2

	// For very large maps, use a smaller growth factor to reduce memory pressure
	if oldLen >= 1_048_576 { // 1M entries
		newLen = oldLen + (oldLen / 2) // 1.5x growth for large maps
	}

	// Ensure new length is a power of 2 for efficient masking
	if newLen&(newLen-1) != 0 {
		// Round up to next power of 2
		newLen--
		newLen |= newLen >> 1
		newLen |= newLen >> 2
		newLen |= newLen >> 4
		newLen |= newLen >> 8
		newLen |= newLen >> 16
		newLen |= newLen >> 32
		newLen++
	}

	// Create new data array
	oldData := m.data
	m.data = make([]Pair[V], newLen)
	m.mask = newLen - 1
	m.growAt = int(float64(newLen) * 0.75)

	// Save zero key info (avoid re-entry which calls Put)
	hasZeroKey := m.hasZeroKey
	zeroVal := m.zeroVal

	// Reset size (will be incremented as we add items)
	m.size = 0
	m.hasZeroKey = false

	// Re-insert all non-zero keys directly (bypass Put to avoid redundant checks)
	// This optimization reduces function call overhead
	for i := range oldData {
		p := &oldData[i] // Use pointer to avoid copying the pair
		if p.Key != 0 {
			// Reuse the optimized insertion logic but avoid calling Put
			// to reduce function call overhead and extra checks
			h := m.primaryIndex(p.Key)
			// Try to insert at the primary position
			if m.data[h].Key == 0 {
				// Fast path: slot is empty
				m.data[h].Key = p.Key
				m.data[h].Value = p.Value
				m.size++
				continue
			}

			// Collision: use quadratic probing
			secondary := m.secondaryIndex(p.Key)
			probeDistance := 1

			for j := 0; j < len(m.data); j++ {
				// Quadratic probe with secondary hash
				h = (h + probeDistance*secondary) & m.mask
				probeDistance++

				if m.data[h].Key == 0 {
					m.data[h].Key = p.Key
					m.data[h].Value = p.Value
					m.size++
					break
				}
			}
		}
	}

	// Restore zero key if it existed
	if hasZeroKey {
		m.hasZeroKey = true
		m.zeroVal = zeroVal
		m.size++
	}

	// Help the garbage collector by explicitly clearing the reference to oldData
	// This isn't strictly necessary but can help reduce memory pressure
	// by allowing the GC to collect the old array sooner
	oldData = nil
}

// fixChain repairs the hash chain after deletion
// For our clustered use case with minimal deletions, we use a simple approach:
// 1. Collect all keys from the next segment that might be affected
// 2. Remove them from the table
// 3. Re-insert them
func (m *Int64Map[V]) fixChain(emptyIdx int) {
	// Collect potentially affected keys
	var keysToReinsert []Pair[V]
	var zero V

	// Maximum chain distance to check
	maxDistance := 16 // Max probe sequence length to check

	// Scan ahead for keys that might need to be repositioned
	i := 1
	for ; i <= maxDistance; i++ {
		nextIdx := (emptyIdx + i) & m.mask
		if m.data[nextIdx].Key == 0 {
			break // Found end of chain
		}

		// Check if this key would like to be earlier in the chain
		// For a key to be potentially affected, it must have its primary
		// position earlier than our current checking position
		primary := m.primaryIndex(m.data[nextIdx].Key)
		if isBetween(primary, emptyIdx, nextIdx) {
			keysToReinsert = append(keysToReinsert, m.data[nextIdx])
			m.data[nextIdx].Key = 0
			m.data[nextIdx].Value = zero
		}
	}

	// If we found few or no keys to reinsert, we're done
	if len(keysToReinsert) == 0 {
		return
	}

	// Re-insert the collected keys
	for _, p := range keysToReinsert {
		// Get primary index
		idx := m.primaryIndex(p.Key)

		// If the primary index is empty, use it
		if m.data[idx].Key == 0 {
			m.data[idx] = p
			continue
		}

		// Otherwise, probe to find a new slot
		secondary := m.secondaryIndex(p.Key)
		probeDistance := 1

		for j := 0; j < len(m.data); j++ {
			idx = (idx + probeDistance*secondary) & m.mask
			probeDistance++

			if m.data[idx].Key == 0 {
				m.data[idx] = p
				break
			}
		}
	}
}

// isBetween checks if idx is in the range [start, end] in a circular buffer
func isBetween(original, empty, current int) bool {
	if empty <= current {
		return original <= empty || original > current
	}
	return original <= empty && original > current
}
