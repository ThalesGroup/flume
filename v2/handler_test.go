package flume

import (
	"github.com/stretchr/testify/assert"
	"log/slog"
	"os"
	"runtime"
	"testing"
)

func TestDynamicHandler(t *testing.T) {

	dynHandler := &handler{newHandlerState(&slog.LevelVar{}, slog.NewJSONHandler(os.Stdout, nil), nil, "")}
	logger := slog.New(dynHandler)

	logger.Info("Hi")

	doit(t, logger, dynHandler)

	runtime.GC()
	runtime.GC()

	assert.Empty(t, dynHandler.children)

}

func doit(t *testing.T, logger *slog.Logger, dynHandler *handler) {
	child := logger.WithGroup("colors").With("blue", true)
	child.Info("There")
	dynHandler.setDelegateHandler(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("Hi again")
	child.Info("There")

	assert.Len(t, dynHandler.children, 1)
}
