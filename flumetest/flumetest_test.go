package flumetest

import (
	"github.com/gemalto/flume"
	"testing"
)

func init() {
	MustSetDefaults()
}

func TestStart(t *testing.T) {
	defer Start(t)()

	var log = flume.New("TestStart")
	log.Info("Hi", "color", "red", "size", 5, "multilinevalue")
}
