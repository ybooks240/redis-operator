package utils

import (
	"fmt"
	"sort"
	"strings"
)

func LabelsForRedis(name string) map[string]string {
	return map[string]string{
		"app":                          "redisInstance",
		"redis.github.com/instance":    name,
		"app.kubernetes.io/name":       "redisInstance",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/component":  "redisInstance",
		"app.kubernetes.io/managed-by": "redis-operator",
	}
}

// GenerateRedisConfig generates Redis configuration from a map
func GenerateRedisConfig(config map[string]string) string {
	var configLines []string

	// Default Redis configuration
	defaultConfig := map[string]string{
		"bind":                        "0.0.0.0",
		"port":                        "6379",
		"dir":                         "/data",
		"appendonly":                  "yes",
		"appendfsync":                 "everysec",
		"save":                        "900 1 300 10 60 10000",
		"maxmemory-policy":            "allkeys-lru",
		"tcp-keepalive":               "300",
		"timeout":                     "0",
		"tcp-backlog":                 "511",
		"databases":                   "16",
		"stop-writes-on-bgsave-error": "yes",
		"rdbcompression":              "yes",
		"rdbchecksum":                 "yes",
		"dbfilename":                  "dump.rdb",
	}

	// Merge default config with user config
	finalConfig := make(map[string]string)
	for k, v := range defaultConfig {
		finalConfig[k] = v
	}
	for k, v := range config {
		finalConfig[k] = v
	}

	// Generate config lines in sorted order to ensure consistency
	// Sort keys to make the output deterministic
	keys := make([]string, 0, len(finalConfig))
	for key := range finalConfig {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Generate config lines in sorted key order
	for _, key := range keys {
		configLines = append(configLines, fmt.Sprintf("%s %s", key, finalConfig[key]))
	}

	return strings.Join(configLines, "\n")
}
