package flume

import (
	"io"
	"strings"
)

// LogFuncWriter is a writer which writes to a logging function signature
// like that of testing.T.Log() and fmt/log.Println().
// It can be used to redirect slog output (or any other line-oriented logging
// output) to some other logging function.
//
//	SetOut(LogFuncWriter(fmt.Println, true))
//	SetOut(LogFuncWriter(t.Log, true))
func LogFuncWriter(l func(args ...any), trimSpace bool) io.Writer {
	return &logWriter{lf: l, trimSpace: trimSpace}
}

// LoggerFuncWriter is a writer which writes to functions with a signature like
// slog.Logger's logging functions, like slog.Logger.Info(), slog.Logger.Debug(), and slog.Logger.Error().
// It can be used to adapt libraries which expect a logging function to slog.Logger.
//
//	http.Server{
//	    ErrorLog: log.New(LoggerFuncWriter(flume.New("http").Error), "", 0),
//	}
func LoggerFuncWriter(l func(msg string, kvpairs ...any)) io.Writer {
	return &loggerWriter{lf: l}
}

type logWriter struct {
	lf        func(args ...any)
	trimSpace bool
}

// Write implements io.Writer
func (t *logWriter) Write(p []byte) (int, error) {
	s := string(p)
	if t.trimSpace {
		s = strings.TrimSpace(s)
	}

	t.lf(s)

	return len(p), nil
}

type loggerWriter struct {
	lf func(msg string, kvpairs ...any)
}

// Write implements io.Writer
func (t *loggerWriter) Write(p []byte) (int, error) {
	t.lf(string(p))
	return len(p), nil
}
