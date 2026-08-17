package server

import (
	"testing"

	"github.com/loom/loom/envconfig"
)

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOOM_MODELS", "")
	envconfig.ReloadServerConfig()
}
