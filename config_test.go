package flume

import (
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	// store the actual default factory, so we don't modify it
	realPkgFactory := pkgFactory
	realDefaultConfigVars := DefaultConfigEnvVars
	origflume := os.Getenv("flume")
	origLogxi := os.Getenv("LOGXI")
	defer func() {
		pkgFactory = realPkgFactory
		DefaultConfigEnvVars = realDefaultConfigVars
		os.Setenv("flume", origflume)
		os.Setenv("LOGXI", origLogxi)
	}()

	pkgFactory = NewFactory()

	os.Setenv("LOGXI", `{"levels":"blue=DBG"}`)

	require.NoError(t, ConfigFromEnv())
	assert.NotNil(t, pkgFactory.loggers["blue"])

	// flume takes precedence
	pkgFactory = NewFactory()

	os.Setenv("flume", `{"levels":"yellow=DBG"}`)
	require.NoError(t, ConfigFromEnv())
	assert.Nil(t, pkgFactory.loggers["blue"])
	assert.NotNil(t, pkgFactory.loggers["yellow"])

	// custom args
	pkgFactory = NewFactory()

	v1 := uuid.New()
	os.Setenv(v1, `{"levels":"pink=DBG"}`)
	require.NoError(t, ConfigFromEnv(v1))
	assert.NotNil(t, pkgFactory.loggers["pink"])

	// bad value
	os.Setenv(v1, `{"levels":"pink=DBG"`)
	require.Error(t, ConfigFromEnv(v1))

}
