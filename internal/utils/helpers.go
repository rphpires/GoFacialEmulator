package utils

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

// GenerateMacAddress generates a random MAC address
func GenerateMacAddress() string {
	// The first byte of a MAC address must be even and can't be 0 or 255
	firstByte := rand.Intn(127)*2 + 2

	// Generate the next 5 bytes randomly
	macBytes := []int{firstByte}
	for i := 0; i < 5; i++ {
		macBytes = append(macBytes, rand.Intn(256))
	}

	// Format the MAC address in a readable format
	macAddress := make([]string, len(macBytes))
	for i, b := range macBytes {
		macAddress[i] = fmt.Sprintf("%02x", b)
	}

	return strings.Join(macAddress, ":")
}

// RandomAccessNotDone returns true with a 20% probability
func RandomAccessNotDone() bool {
	return rand.Intn(100) < 20
}

// IsPortAvailable verifica se uma porta está disponível para uso
func IsPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// WaitForPortToBecomeAvailable aguarda até que uma porta esteja disponível
func WaitForPortToBecomeAvailable(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if IsPortAvailable(port) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for port %d to become available", port)
}
