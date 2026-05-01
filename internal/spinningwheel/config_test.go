package spinningwheel

import "testing"

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Name != "Spinning Wheel" {
		t.Fatalf("expected plugin name, got %#v", config)
	}
	if config.Enabled != true {
		t.Fatalf("expected plugin enabled by default")
	}
	if config.DataDir == "" {
		t.Fatalf("expected default data dir")
	}
	if config.Addr != "127.0.0.1:8787" {
		t.Fatalf("expected default addr, got %q", config.Addr)
	}
	if config.StreamPollInterval <= 0 {
		t.Fatalf("expected positive stream poll interval")
	}
}

func TestConfigWithOverrides(t *testing.T) {
	config := DefaultConfig().
		WithDataDir("/tmp/verdandi").
		WithAddr("127.0.0.1:9898").
		WithEnabled(false)

	if config.DataDir != "/tmp/verdandi" {
		t.Fatalf("expected override data dir, got %q", config.DataDir)
	}
	if config.Addr != "127.0.0.1:9898" {
		t.Fatalf("expected override addr, got %q", config.Addr)
	}
	if config.Enabled {
		t.Fatalf("expected disabled config")
	}
}
