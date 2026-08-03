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

package errorlog

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

const (
	workQueueMaxLength = 1024
	insertDeadline     = 10 * time.Second

	// A single pathological error mustn't be allowed to write an enormous row.
	maxMessageLength = 4096
	maxStackLength   = 8192
)

type Logger struct {
	work                chan imsdb.AddErrorLogParams
	imsDBQ              *store.DBQ
	errorLogEnabled     bool
	synchronousForTests bool
}

func NewLogger(
	ctx context.Context,
	imsDBQ *store.DBQ,
	errorLogEnabled bool,
	synchronousForTests bool,
) *Logger {
	logger := &Logger{
		work:                make(chan imsdb.AddErrorLogParams, workQueueMaxLength),
		imsDBQ:              imsDBQ,
		errorLogEnabled:     errorLogEnabled,
		synchronousForTests: synchronousForTests,
	}
	go logger.startWorker(ctx)
	return logger
}

func (l *Logger) Log(ctx context.Context, record imsdb.AddErrorLogParams) {
	if l.errorLogEnabled {
		record.ResponseMessage = truncate(record.ResponseMessage, maxMessageLength)
		record.InternalError = truncate(record.InternalError, maxMessageLength)
		record.StackTrace = truncate(record.StackTrace, maxStackLength)
		if l.synchronousForTests {
			l.writeRow(ctx, record)
		} else {
			l.work <- record
		}
	}
}

func (l *Logger) Close() {}

func (l *Logger) startWorker(ctx context.Context) {
	for row := range l.work {
		l.writeRow(ctx, row)
	}
	slog.Info("errorlog.Logger worker finished")
}

func (l *Logger) writeRow(ctx context.Context, row imsdb.AddErrorLogParams) {
	// We don't use loggerCtx here, since it gets canceled soon after SIGINT.
	// We use a different context, so that there's still a chance to write a final
	// row before the server quits.
	ctx, cancel := context.WithTimeout(ctx, insertDeadline)
	defer cancel()
	_, err := l.imsDBQ.AddErrorLog(ctx, l.imsDBQ, row)
	if err != nil {
		slog.Error("failed to add error log to db", "error", err)
	}
}

func truncate(s sql.NullString, maxLength int) sql.NullString {
	return conv.StringToSql(&s.String, maxLength)
}
