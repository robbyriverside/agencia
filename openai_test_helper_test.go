package agencia

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

var (
	openAICheckOnce sync.Once
	openAICheckErr  error
	openAIReady     bool
)

func requireAPI(t *testing.T) {
	t.Helper()
	openAICheckOnce.Do(func() {
		if skip := os.Getenv("SKIP_OPENAI_TESTS"); skip != "" {
			openAICheckErr = fmt.Errorf("SKIP_OPENAI_TESTS set: %s", skip)
			return
		}
		_ = godotenv.Load()
		if key := os.Getenv("OPENAI_API_KEY"); strings.TrimSpace(key) == "" {
			openAICheckErr = errors.New("OPENAI_API_KEY not configured")
			return
		}
		if force := os.Getenv("FORCE_OPENAI_TESTS"); strings.EqualFold(force, "1") || strings.EqualFold(force, "true") {
			openAIReady = true
			return
		}
		base := os.Getenv("OPENAI_API_BASE")
		if strings.TrimSpace(base) == "" {
			base = "https://api.openai.com/v1"
		}
		u, err := url.Parse(base)
		if err != nil {
			openAICheckErr = fmt.Errorf("invalid OPENAI_API_BASE: %w", err)
			return
		}
		host := u.Host
		if host == "" {
			host = base
		}
		if !strings.Contains(host, ":") {
			if u.Scheme == "http" {
				host += ":80"
			} else {
				host += ":443"
			}
		}
		conn, err := net.DialTimeout("tcp", host, 3*time.Second)
		if err != nil {
			openAICheckErr = fmt.Errorf("cannot reach %s: %w", host, err)
			return
		}
		_ = conn.Close()
		openAIReady = true
	})
	if !openAIReady {
		t.Skipf("skipping OpenAI-dependent test: %v", openAICheckErr)
	}
}
