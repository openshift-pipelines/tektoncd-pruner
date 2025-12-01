package version

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGet verifies version information is populated correctly from runtime.
func TestGet(t *testing.T) {
	v := Get()

	assert.Equal(t, runtime.Version(), v.GoLang)
	assert.Equal(t, runtime.GOOS, v.Platform)
	assert.Equal(t, runtime.GOARCH, v.Arch)
	assert.NotNil(t, v.Version)
	assert.NotNil(t, v.GitCommit)
	assert.NotNil(t, v.BuildDate)
}

// TestGetConsistency ensures Get returns consistent values across calls.
func TestGetConsistency(t *testing.T) {
	v1 := Get()
	v2 := Get()

	assert.Equal(t, v1, v2)
}
