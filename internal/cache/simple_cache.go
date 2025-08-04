// internal/cache/simple_cache.go
package cache

import (
	"GoFacialEmulator/internal/models"
	"sync"
	"time"
)

type SimpleCache struct {
	devices    map[int]CachedDevice
	userCounts map[int]CachedCount
	mu         sync.RWMutex
}

type CachedDevice struct {
	Device    models.Device
	ExpiresAt time.Time
}

type CachedCount struct {
	Count     int
	ExpiresAt time.Time
}

func NewSimpleCache() *SimpleCache {
	return &SimpleCache{
		devices:    make(map[int]CachedDevice),
		userCounts: make(map[int]CachedCount),
	}
}

func (c *SimpleCache) GetDevice(id int) (models.Device, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cached, exists := c.devices[id]; exists && time.Now().Before(cached.ExpiresAt) {
		return cached.Device, true
	}
	return models.Device{}, false
}

func (c *SimpleCache) SetDevice(id int, device models.Device, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.devices[id] = CachedDevice{
		Device:    device,
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (c *SimpleCache) GetUserCount(deviceID int) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cached, exists := c.userCounts[deviceID]; exists && time.Now().Before(cached.ExpiresAt) {
		return cached.Count, true
	}
	return 0, false
}

func (c *SimpleCache) SetUserCount(deviceID, count int, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.userCounts[deviceID] = CachedCount{
		Count:     count,
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (c *SimpleCache) InvalidateDevice(id int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.devices, id)
	delete(c.userCounts, id)
}
