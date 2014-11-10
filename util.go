package gowrapp

import "os"

// GetenvDefault returns a value from the specified env var, with a fallback
func GetenvDefault(key string, def string) string {
	env := os.Getenv(key)
	if env == "" {
		return def
	}
	return env
}
