package utils

import "os"

func EnvOrDefault(env string, fallback string) string {
	value, ok := os.LookupEnv(env)

	if !ok {
		return fallback
	}

	return value
}
