package tests

import (
	"net/http"
	"testing"

	"fergus.molloy.xyz/vfmp/internal/config"
)

func getTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../config.test.yml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	return cfg
}

func TestHealthCheck(t *testing.T) {
	cfg := getTestConfig(t)
	resp, err := http.Get("http://" + cfg.HTTPAddr + "/control/healthcheck")

	if err != nil {
		t.Fatalf("error getting healthcheck endpoint: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("service was not healthy: %d", resp.StatusCode)
	}
}
