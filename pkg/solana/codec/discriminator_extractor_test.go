package codec

import (
	"encoding/base64"
	mathrand "math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzExtractorHappyPath(f *testing.F) {
	// Seed with valid base64 discriminators
	seeds := []struct {
		Data string
	}{
		{"SGVsbG8gV29ybGQh"}, // Hello world!
		{"AAAAAAAAAAAA"},     // Zero bytes
		{"////////////"},     // Max value bytes
		{"QUJDREVGR0hJSktM"}, // ABCDEFGHIJKL
	}

	for _, seed := range seeds {
		f.Add(seed.Data)
	}

	extractor := NewDiscriminatorExtractor()
	f.Fuzz(func(t *testing.T, testString string) {
		// Extractor doesn't validate padding, newlines, or tabs
		if len(testString) < 12 ||
			strings.Contains(testString, "\n") ||
			strings.Contains(testString, "\r") ||
			strings.Contains(testString, "\t") ||
			strings.HasSuffix(testString, "=") ||
			strings.HasSuffix(testString, "==") {
			return
		}

		stdDecoded, err := base64.StdEncoding.DecodeString(testString)
		if err == nil {
			require.Equal(t, stdDecoded[:8], extractor.Extract(testString))
		}
	})
}

func TestDiscriminatorExtractorBase64Indexes(t *testing.T) {
	extractor := NewDiscriminatorExtractor()
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for i, c := range base64Chars {
		if extractor.b64Index[c] != byte(i) {
			t.Errorf("incorrect index for character %q: expected %d, got %d", c, i, extractor.b64Index[c])
		}
	}
}

func TestExtractor_Extract_ShortInput(t *testing.T) {
	extractor := NewDiscriminatorExtractor()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for short input, but none occurred")
		}
	}()

	// Attempt with 11-character string (needs at least 12)
	extractor.Extract("short_input")
}

// Custom extractor is around 40% faster than using stdlib
func BenchmarkDiscriminatorExtraction(b *testing.B) {
	generateDiscriminatorDecodeTestData := func(numTestEntries int) []string {
		// corresponds to a 12 character base64 encoded string
		entrySize := int64(8)
		var testData []string
		// Create seeded random source
		r := mathrand.New(mathrand.NewSource(entrySize))
		for range numTestEntries {
			data := make([]byte, entrySize)
			_, _ = r.Read(data)

			testData = append(testData, base64.StdEncoding.EncodeToString(data))
		}

		return testData
	}

	b.Run("Standard lib Base64", func(b *testing.B) {
		testData := generateDiscriminatorDecodeTestData(b.N)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = base64.StdEncoding.DecodeString(testData[i])
		}
	})

	b.Run("CustomExtractor", func(b *testing.B) {
		testData := generateDiscriminatorDecodeTestData(b.N)
		extractor := NewDiscriminatorExtractor()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			extractor.Extract(testData[i])
		}
	})
}
