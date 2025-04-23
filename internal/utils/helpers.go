package utils

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

func init() {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())
}

// RemoveAccentsFromString removes accents from a string
func RemoveAccentsFromString(s string) string {
	if s == "" {
		return ""
	}

	charsTable := map[rune]rune{
		'À': 'A', 'Á': 'A', 'Â': 'A', 'Ã': 'A', 'Ä': 'A', 'Å': 'A', 'Ç': 'C', 'È': 'E', 'É': 'E', 'Ê': 'E',
		'Ë': 'E', 'Ì': 'I', 'Í': 'I', 'Î': 'I', 'Ï': 'I', 'Ò': 'O', 'Ó': 'O', 'Ô': 'O', 'Õ': 'O', 'Ö': 'O',
		'Ù': 'U', 'Ú': 'U', 'Û': 'U', 'Ü': 'U', 'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
		'ç': 'c', 'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ð': 'o',
		'ñ': 'n', 'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
		'ý': 'y', 'ÿ': 'y', ' ': ' ',
	}

	result := []rune{}
	for _, c := range s {
		if c <= 128 {
			result = append(result, c)
		} else {
			if replacement, ok := charsTable[c]; ok {
				result = append(result, replacement)
			} else {
				result = append(result, '_')
			}
		}
	}

	return string(result)
}

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

// CheckOS returns the operating system type
func CheckOS() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "darwin":
		return "MacOS"
	default:
		return "Unknown"
	}
}

// GetLocalIPAddress returns the local IP address
func GetLocalIPAddress() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// EnsureDirectory ensures a directory exists, creating it if necessary
func EnsureDirectory(path string) error {
	if !FileExists(path) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

// Contains checks if a slice contains a value
func Contains[T comparable](slice []T, val T) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
