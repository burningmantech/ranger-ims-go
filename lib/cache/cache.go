//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// epochMs is some time long in the past.
const epochMs = 519850800000

type InMemory[T any] struct {
	dataPtr     atomic.Pointer[dataAndTime[T]]
	ttl         time.Duration
	refresher   func(context.Context) (T, error)
	writeMu     sync.Mutex
	initialized bool
}

type dataAndTime[T any] struct {
	data T
	time time.Time
}

// New creates a new InMemory cache. The ttl indicates how long a cached value is valid, and the refresher
// function is what fetches a new value for the cache when a refresh is needed.
func New[T any](
	ttl time.Duration,
	refresher func(context.Context) (T, error),
) *InMemory[T] {
	this := &InMemory[T]{
		dataPtr:     atomic.Pointer[dataAndTime[T]]{},
		ttl:         ttl,
		refresher:   refresher,
		initialized: true,
	}
	this.dataPtr.Store(emptyVal[T]())
	return this
}

func (im *InMemory[T]) Get(ctx context.Context) (*T, error) {
	if !im.initialized {
		panic("cache not initialized")
	}
	val, err := im.maybeRefreshAndGet(ctx)
	if err != nil {
		return nil, fmt.Errorf("[maybeRefreshAndGet]: %w", err)
	}
	return val, nil
}

func emptyVal[T any]() *dataAndTime[T] {
	return &dataAndTime[T]{
		time: time.UnixMilli(epochMs),
	}
}

func (im *InMemory[T]) Invalidate() {
	if !im.initialized {
		panic("cache not initialized")
	}
	im.writeMu.Lock()
	defer im.writeMu.Unlock()
	im.dataPtr.Store(emptyVal[T]())
}

func stillValid(creation time.Time, ttl time.Duration) bool {
	expiresAt := creation.Add(ttl)
	return time.Now().Before(expiresAt)
}

func (im *InMemory[T]) maybeRefreshAndGet(ctx context.Context) (*T, error) {
	// if it's still valid, return it
	v := im.dataPtr.Load()
	if stillValid(v.time, im.ttl) {
		return &v.data, nil
	}
	// otherwise get the write lock
	im.writeMu.Lock()
	defer im.writeMu.Unlock()
	// check again if the value is valid, because another caller might have refreshed it
	v = im.dataPtr.Load()
	if stillValid(v.time, im.ttl) {
		return &v.data, nil
	}
	// get a refreshed value and store it
	newVal, err := im.refresher(ctx)
	if err != nil {
		return new(T), fmt.Errorf("[refresher]: %w", err)
	}
	im.dataPtr.Store(
		&dataAndTime[T]{
			data: newVal,
			time: time.Now(),
		},
	)
	return &newVal, nil
}
