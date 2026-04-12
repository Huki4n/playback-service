package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"service/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("CONFIG_NAME", "nonexistent")

	cfg := config.Load()

	assert.Equal(t, "service", cfg.ServiceName)
	assert.Equal(t, "8080", cfg.HTTPPort)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.True(t, cfg.TracingEnabled)
	assert.Equal(t, "localhost:4317", cfg.OTLPEndpoint)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("CONFIG_NAME", "nonexistent")
	t.Setenv("SERVICE_NAME", "test-svc")
	t.Setenv("HTTP_PORT", "9999")
	t.Setenv("LOG_LEVEL", "error")

	cfg := config.Load()

	assert.Equal(t, "test-svc", cfg.ServiceName)
	assert.Equal(t, "9999", cfg.HTTPPort)
	assert.Equal(t, "error", cfg.LogLevel)
}

func TestLoad_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`service_name: from-file
http_port: "3000"
`)
	require.NoError(t, os.WriteFile(dir+"/config.test.yaml", content, 0o644))

	t.Setenv("CONFIG_NAME", "config.test")
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cfg := config.Load()

	assert.Equal(t, "from-file", cfg.ServiceName)
	assert.Equal(t, "3000", cfg.HTTPPort)
}
