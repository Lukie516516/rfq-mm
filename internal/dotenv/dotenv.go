package dotenv

import (
	"bufio"
	"os"
	"strings"
)

// Load sets env vars from a KEY=VALUE file. Existing env wins.
// Unknown or missing files are ignored so local runs stay optional.
func Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, set := os.LookupEnv(k); set {
			continue
		}
		_ = os.Setenv(k, strings.TrimSpace(v))
	}
	return sc.Err()
}
