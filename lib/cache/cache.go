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
	"errors"
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

// New creates a new InMemory cache. The ttl indicates how long a cached value is good for, and the refresher
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
	this.dataPtr.Store(&dataAndTime[T]{
		time: time.UnixMilli(epochMs),
	})
	return this
}

func (im *InMemory[T]) Get(ctx context.Context) (*T, error) {
	if !im.initialized {
		return nil, errors.New("cache not initialized. Use the `New` function to create a cache")
	}
	if err := im.maybeRefresh(ctx); err != nil {
		return nil, fmt.Errorf("[maybeRefresh]: %w", err)
	}
	dataPtr := im.dataPtr.Load()
	return &dataPtr.data, nil
}

func (im *InMemory[T]) shouldRefresh() bool {
	expiresAt := im.dataPtr.Load().time.Add(im.ttl)
	return time.Now().After(expiresAt)
}

func (im *InMemory[T]) maybeRefresh(ctx context.Context) error {
	if !im.shouldRefresh() {
		return nil
	}
	im.writeMu.Lock()
	defer im.writeMu.Unlock()
	if !im.shouldRefresh() {
		return nil
	}
	newVal, err := im.refresher(ctx)
	if err != nil {
		return fmt.Errorf("[refresher]: %w", err)
	}
	im.dataPtr.Store(&dataAndTime[T]{
		data: newVal,
		time: time.Now(),
	})
	return nil
}
