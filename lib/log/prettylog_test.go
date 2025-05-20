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

package log

import (
	"log/slog"
	"strings"
	"testing"
)

type captureStream struct {
	lines [][]byte
}

func (cs *captureStream) Write(bytes []byte) (int, error) {
	cs.lines = append(cs.lines, bytes)
	return len(bytes), nil
}

func Test_WritesToProvidedStream(t *testing.T) {
	t.Parallel()
	cs := &captureStream{}
	handler := New(&slog.HandlerOptions{Level: slog.LevelDebug}, WithDestinationWriter(cs), WithOutputEmptyAttrs())
	handler.WithAttrs(nil)
	handler.WithGroup("")
	logger := slog.New(handler)

	logger.Info("testing logger")
	if len(cs.lines) != 1 {
		t.Errorf("expected 1 lines logged, got: %d", len(cs.lines))
	}

	line := string(cs.lines[0])
	if !strings.Contains(line, "INFO: testing logger") {
		t.Errorf("expected `testing logger` but found `%s`", line)
	}
	if !strings.HasSuffix(line, "\n") {
		t.Errorf("exected line to be terminated with `\\n` but found `%s`", line[len(line)-1:])
	}

	// just check that we can make a line for each color
	logger.Debug("testing debug")
	logger.Info("testing info")
	logger.Log(t.Context(), slog.LevelWarn-1, "testing warn-1")
	logger.Warn("testing warn")
	logger.Error("testing error")
	logger.Log(t.Context(), slog.LevelError+1, "testing error+1")
	if len(cs.lines) != 7 {
		t.Errorf("expected 7 lines logged, got: %d", len(cs.lines))
	}
}

func Test_SkipEmptyAttributes(t *testing.T) {
	t.Parallel()
	cs := &captureStream{}
	handler := New(nil, WithDestinationWriter(cs))
	logger := slog.New(handler)

	logger.Info("testing logger")
	if len(cs.lines) != 1 {
		t.Errorf("expected 1 lines logged, got: %d", len(cs.lines))
	}

	line := string(cs.lines[0])
	if !strings.Contains(line, "INFO: testing logger") {
		t.Errorf("expected `testing logger` but found `%s`", line)
	}
	if !strings.HasSuffix(line, "\n") {
		t.Errorf("exected line to be terminated with `\\n` but found `%s`", line[len(line)-1:])
	}
}
