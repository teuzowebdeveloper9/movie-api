package env

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func String(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return fallback
}

func Int(key string, fallback int) (int, error) {
	v := String(key, "")
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w", key, v, err)
	}
	return n, nil
}

func Bool(key string, fallback bool) (bool, error) {
	v := String(key, "")
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid %s=%q: %w", key, v, err)
	}
	return b, nil
}

func Duration(key string, fallback time.Duration) (time.Duration, error) {
	v := String(key, "")
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w", key, v, err)
	}
	return d, nil
}
