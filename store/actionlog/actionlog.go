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

package actionlog

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

const (
	workQueueMaxLength = 1024
	insertDeadline     = 10 * time.Second
)

type Logger struct {
	work                chan imsdb.AddActionLogParams
	imsDBQ              *store.DBQ
	actionLogEnabled    bool
	synchronousForTests bool

	// dropped counts the rows discarded because the queue was full, for the
	// worker to report once it's keeping up again.
	dropped atomic.Int64
}

func NewLogger(
	ctx context.Context,
	imsDBQ *store.DBQ,
	actionLogEnabled bool,
	synchronousForTests bool,
) *Logger {
	logger := &Logger{
		work:                make(chan imsdb.AddActionLogParams, workQueueMaxLength),
		imsDBQ:              imsDBQ,
		actionLogEnabled:    actionLogEnabled,
		synchronousForTests: synchronousForTests,
	}
	go logger.startWorker(ctx)
	return logger
}

// Log queues a row to be written by the worker goroutine. This is called from
// request handlers, so the send must never block: the queue fills up exactly
// when the database is struggling, and blocking here would stall every request
// in flight and turn a slow database into a hung server. A dropped audit row is
// the cheaper loss.
func (l *Logger) Log(ctx context.Context, record imsdb.AddActionLogParams) {
	if l.actionLogEnabled {
		if l.synchronousForTests {
			l.writeRow(ctx, record)
			return
		}
		select {
		case l.work <- record:
		default:
			l.dropped.Add(1)
		}
	}
}

func (l *Logger) Close() {}

func (l *Logger) startWorker(ctx context.Context) {
	for row := range l.work {
		l.writeRow(ctx, row)
		// Report drops from the worker rather than from Log, so that a full
		// queue produces one line per drained row instead of one per request.
		if dropped := l.dropped.Swap(0); dropped > 0 {
			slog.Warn("action log queue was full; rows were dropped", "count", dropped)
		}
	}
	slog.Info("actionlog.Logger worker finished")
}

func (l *Logger) writeRow(ctx context.Context, row imsdb.AddActionLogParams) {
	// We don't use loggerCtx here, since it gets cancelled soon after SIGINT.
	// We use a different context, so that there's still a chance to write a final
	// row before the server quits.
	ctx, cancel := context.WithTimeout(ctx, insertDeadline)
	defer cancel()
	_, err := l.imsDBQ.AddActionLog(ctx, l.imsDBQ, row)
	if err != nil {
		slog.Error("failed to add action log to db", "error", err)
	}
}
