package utils

import (
	"sync"
)

var (
	httpClientFactoryInstance *HTTPClientFactory
	httpClientFactoryOnce     sync.Once
)

// GetHTTPClientFactory retorna a instância singleton da factory
func GetHTTPClientFactory() *HTTPClientFactory {
	httpClientFactoryOnce.Do(func() {
		httpClientFactoryInstance = NewHTTPClientFactory()
	})
	return httpClientFactoryInstance
}

// ResetHTTPClientFactory reseta o singleton (útil para testes)
func ResetHTTPClientFactory() {
	httpClientFactoryInstance = nil
	httpClientFactoryOnce = sync.Once{}
}
