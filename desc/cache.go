package desc

import (
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// The global cache is used to store descriptors that wrap items in
// protoregistry.GlobalTypes and protoregistry.GlobalFiles. This prevents
// repeating work to re-wrap underlying global descriptors.
var cache = globalCache{cache: mapCache{}}

type descriptorCache interface {
	get(protoreflect.Descriptor) Descriptor
	put(protoreflect.Descriptor, Descriptor)
}

type globalCache struct {
	cacheMu sync.RWMutex
	cache   mapCache
}

func (c *globalCache) get(d protoreflect.Descriptor) Descriptor {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.cache.get(d)
}

func (c *globalCache) put(key protoreflect.Descriptor, val Descriptor) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cache.put(key, val)
}

func (c *globalCache) withLock(fn func(descriptorCache)) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	// Pass the underlying mapCache. We don't want fn to use
	// c.get or c.put sine we already have the lock. So those
	// methods would try to re-acquire and then deadlock!
	fn(c.cache)
}

type mapCache map[protoreflect.Descriptor]Descriptor

func (c mapCache) get(d protoreflect.Descriptor) Descriptor {
	return c[d]
}

func (c mapCache) put(key protoreflect.Descriptor, val Descriptor) {
	c[key] = val
}

type noopCache struct{}

func (noopCache) get(d protoreflect.Descriptor) Descriptor {
	return nil
}

func (noopCache) put(key protoreflect.Descriptor, val Descriptor) {
}
