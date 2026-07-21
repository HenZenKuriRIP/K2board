package server

import "K2board/internal/services"

// GetConfigVersion delegates to the services package.
func GetConfigVersion() int64 {
	return services.GetConfigVersion()
}

// BumpConfigVersion delegates to the services package.
func BumpConfigVersion() {
	services.BumpConfigVersion()
}
