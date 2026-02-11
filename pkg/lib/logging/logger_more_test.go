package logging

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
)

type sink struct {
	logr.LogSink
	infos  int
	errors int
}

func (s *sink) Init(logr.RuntimeInfo) {}
func (s *sink) Enabled(_ int) bool    { return true }
func (s *sink) Info(_ int, _ string, _ ...interface{}) {
	s.infos++
}
func (s *sink) Error(_ error, _ string, _ ...interface{}) {
	s.errors++
}
func (s *sink) WithValues(_ ...interface{}) logr.LogSink { return s }
func (s *sink) WithName(_ string) logr.LogSink           { return s }

func TestSettings_allowed_RespectsLevel(t *testing.T) {
	old := Settings
	defer func() { Settings = old }()
	Settings.Level = 1
	if !Settings.allowed(1) {
		t.Fatalf("expected allowed")
	}
	if Settings.allowed(2) {
		t.Fatalf("expected not allowed")
	}
}

func TestLogger_Error_Nil_NoLog(t *testing.T) {
	s := &sink{}
	l := &Logger{Real: logr.New(s)}
	l.Error(nil, "msg")
	if s.infos != 0 && s.errors != 0 {
		t.Fatalf("expected no logs")
	}
}

func TestLogger_Error_UnwrapsWrappedErrorAndLogsError(t *testing.T) {
	old := Settings
	defer func() { Settings = old }()
	Settings.Level = 10

	s := &sink{}
	l := &Logger{Real: logr.New(s)}
	err := errors.New("root")
	wrapped := liberr.Wrap(err)
	l.Error(wrapped, "msg", "k", "v")
	// Wrapped error logs via Info() path in Logger.Error
	if s.infos == 0 {
		t.Fatalf("expected info log")
	}
}

func TestLevelLoggerImpl_Info_RespectsSettingsAllowed(t *testing.T) {
	old := Settings
	defer func() { Settings = old }()
	Settings.Level = 0
	s := &sink{}
	ll := &levelLoggerImpl{real: logr.New(s), level: 1}
	ll.Info("msg")
	if s.infos != 0 {
		t.Fatalf("expected no info")
	}
	Settings.Level = 2
	ll.Info("msg")
	if s.infos == 0 {
		t.Fatalf("expected info")
	}
}
