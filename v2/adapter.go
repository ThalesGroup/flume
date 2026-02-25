package flume

import (
	"context"
	"log/slog"
)

// These types are to help transition interfaces which used to depend on flumev1 to slog.
// Let's say you had a public function which accepted a flumev1.Logger instance:
//
//     func SetLogger(l flumev1.Logger) {...}
//
// To break the dependency on flumev1 without breaking callers, you could re-define the arg
// as a locally defined interface, and offer a new alternate function that takes a *slog.Logger:
//
//    type Logger interface{
//       // just specify the logging methods you actually depend on
//       Info(string, ...any)
//     }
//
//    func SetLogger(l Logger) {...}
//    func SetSlogLogger(l *slog.Logger) {...}
//
// Internally, you should rewrite the code to write to slog's logging methods, like those which take
// a context.  To do that while remaining compatible with older flumev1.Loggers, you could internally define
// a logging interface which resembles slog, then adapt both the *slog.Logger and the flumev1.Logger to it:
//
//    type loggerI interface{
//      InfoContext(context.Context, string, ...any)
//    }
//    type adapter struct {
//       l flumev1.Logger
//    }
//    // implements loggerI
//    func (a *adapter) InfoContext(_ context.Context, msg string, args ...any) {
//			a.l.Info(msg, args...)
//    }
//
// *slog.Logger already implements loggerI, so no adaption necessary.  Just write your internal code against
// loggerI, which may be either a *slog.Logger or an *adapter.
//
// This file is a basic implementation of the boilerplate required:
//
// FlumeV1Logger: an interface which covers most of the flumev1.Logger interface.  Create a local type alias for it in your
//            package, and where ever your public API refers to flumev1.Logger, change the reference to this.
// SlogLogger: an interface which covers the most common parts of *slog.Logger's API.  Internally, anywhere you used to log
//             to flume/yugolog.Logger, re-write the code to write to this interface.  Prefer the XXXContext() variants where
//             ever you have a context in scope.  This interface should not appear in your public API surface.  It should only
//             be used internally.
// SlogAdapter: Wraps a flumev1.Logger instance, and implements the SlogLogger interface.
//
// Here's how your package might look:
//
//    // Create a type alias, so your API doesn't export types from this package.
//    type Logger = flume.FlumeV1Logger
//
//    // Unexported package variable holding the logger your code will log to.  Drop in
//    // replacement for *slog.Logger
//    var logger yugoslog.SlogLogger
//
//    // deprecated: Use Logger() instead
//    func Logging(l Logger) {
//      logger = yugoslog.NewSlogAdapter(l)
//    }
//
//    func Logger(l *slog.Logger) {
//      logger = l
//    }
//
// In the next major version of your package, remove Logger(), and replace yugoslog.SlogLogger with direct references to *slog.Logger.
//
// Note that this won't work if your package uses With()...it could be done by extending the FlumeV1Logger interface  and SlogLogger
// interfaces with a With() method, but would require wrapping an adapter around *slog.Loggers too.

// FlumeV1Logger describes the most commonly used parts of the flumev1 Logger API.  In most cases, references
// to flume/yugolog.Logger could be replaced with this interface without breaking callers.
type FlumeV1Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Error(msg string, args ...any)

	IsDebug() bool
	IsInfo() bool
}

// SlogLogger describes most of the commonly used parts of the *slog.Logger API.  *slog.Logger implements
// this interface natively.  The intent is make your code depend on this interface, instead of directly
// on *slog.Logger.  This will let you inject either a *slog.Logger or a *SlogAdapter.
type SlogLogger interface {
	Enabled(ctx context.Context, l slog.Level) bool
	Log(ctx context.Context, level slog.Level, msg string, args ...any)

	Debug(msg string, args ...any)
	DebugContext(ctx context.Context, msg string, args ...any)

	Info(msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	Warn(msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)

	Error(msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

// SlogAdapter wraps a FlumeV1Logger and implements SlogLogger.
// Because FlumeV1Logger has no Warn method, Warn and WarnContext
// are mapped to Info (preferring under-reporting over over-reporting).
type SlogAdapter struct {
	l FlumeV1Logger
}

// NewSlogAdapter create an adapter which implements SlogLogger, and translates those
// calls to an old flumev1/yugolog instance.
func NewSlogAdapter(l FlumeV1Logger) *SlogAdapter {
	return &SlogAdapter{l}
}

func (s *SlogAdapter) Enabled(_ context.Context, l slog.Level) bool {
	switch {
	case l <= slog.LevelDebug:
		return s.l.IsDebug()
	case l <= slog.LevelWarn:
		return s.l.IsInfo()
	default:
		return true
	}
}

func (s *SlogAdapter) Log(_ context.Context, level slog.Level, msg string, args ...any) {
	switch {
	case level <= slog.LevelDebug:
		s.l.Debug(msg, args...)
	case level <= slog.LevelWarn:
		s.l.Info(msg, args...)
	default:
		s.l.Error(msg, args...)
	}
}

func (s *SlogAdapter) Debug(msg string, args ...any) {
	s.l.Debug(msg, args...)
}

func (s *SlogAdapter) DebugContext(_ context.Context, msg string, args ...any) {
	s.l.Debug(msg, args...)
}

func (s *SlogAdapter) Info(msg string, args ...any) {
	s.l.Info(msg, args...)
}

func (s *SlogAdapter) InfoContext(_ context.Context, msg string, args ...any) {
	s.l.Info(msg, args...)
}

func (s *SlogAdapter) Warn(msg string, args ...any) {
	s.l.Info(msg, args...)
}

func (s *SlogAdapter) WarnContext(_ context.Context, msg string, args ...any) {
	s.l.Info(msg, args...)
}

func (s *SlogAdapter) Error(msg string, args ...any) {
	s.l.Error(msg, args...)
}

func (s *SlogAdapter) ErrorContext(_ context.Context, msg string, args ...any) {
	s.l.Error(msg, args...)
}
