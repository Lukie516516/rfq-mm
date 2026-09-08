package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("DOTENV_TEST_KEY=fromfile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_KEY", "fromenv")
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DOTENV_TEST_KEY") != "fromenv" {
		t.Fatal("existing env must win")
	}
}
