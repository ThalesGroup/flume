package flume

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	// store the actual default factory, so we don't modify it
	realPkgFactory := pkgFactory
	realDefaultConfigVars := DefaultConfigEnvVars
	origflume := os.Getenv("FLUME")
	defer func() {
		pkgFactory = realPkgFactory
		DefaultConfigEnvVars = realDefaultConfigVars
		os.Setenv("FLUME", origflume)
	}()

	pkgFactory = NewFactory()

	// flume takes precedence
	pkgFactory = NewFactory()

	os.Setenv("FLUME", `{"levels":"yellow=DBG"}`)
	require.NoError(t, ConfigFromEnv())
	assert.Nil(t, pkgFactory.loggers["blue"])
	assert.NotNil(t, pkgFactory.loggers["yellow"])

	// custom args
	pkgFactory = NewFactory()

	v1 := "big_random_env_var_name_asdfasdfasdfasdfasdfasdfasd"
	os.Unsetenv(v1)
	defer os.Unsetenv(v1)
	os.Setenv(v1, `{"levels":"pink=DBG"}`)
	require.NoError(t, ConfigFromEnv(v1))
	assert.NotNil(t, pkgFactory.loggers["pink"])

	// bad value
	os.Setenv(v1, `{"levels":"pink=DBG"`)
	require.Error(t, ConfigFromEnv(v1))

}
