package btc

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"testing"
)

func Test_TaggedHash(t *testing.T) {
	// Test _TaggedHash function
	tag := "TestTag"
	expectedHash := sha256.New()
	expectedHash.Write([]byte(tag))
	tagHash := expectedHash.Sum(nil)
	expectedHash.Reset()
	expectedHash.Write(tagHash)
	expectedHash.Write(tagHash)

	actualHash := _TaggedHash(tag)

	if !bytes.Equal(actualHash.Sum(nil), expectedHash.Sum(nil)) {
		t.Errorf("Expected hash: %x, got: %x", expectedHash.Sum(nil), actualHash.Sum(nil))
	}
}

func Test_Hasher(t *testing.T) {
	// Test Hasher function
	for i, tag := range hash_tags {
		expectedHash := _TaggedHash(tag)
		expectedBytes, _ := expectedHash.(encoding.BinaryMarshaler).MarshalBinary()
		actualHash := Hasher(i)

		actualBytes, err := actualHash.(encoding.BinaryMarshaler).MarshalBinary()
		if err != nil {
			t.Errorf("Error marshaling binary: %v", err)
			continue
		}

		if !bytes.Equal(actualBytes, expectedBytes) {
			t.Errorf("Expected hash: %x, got: %x", expectedBytes, actualBytes)
		}
	}
}

func Test_ShaHash(t *testing.T) {
	// Test ShaHash function
	data := []byte("test data")
	expectedHash := sha256.New()
	expectedHash.Write(data)
	tmp := expectedHash.Sum(nil)
	expectedHash.Reset()
	expectedHash.Write(tmp)
	expectedHashSum := expectedHash.Sum(nil)

	actualHash := make([]byte, 32)
	ShaHash(data, actualHash)

	if !bytes.Equal(actualHash, expectedHashSum) {
		t.Errorf("Expected hash: %x, got: %x", expectedHashSum, actualHash)
	}
}

func Test_Sha2Sum(t *testing.T) {
	// Test Sha2Sum function
	data := []byte("test data")
	expectedHash := sha256.New()
	expectedHash.Write(data)
	tmp := expectedHash.Sum(nil)
	expectedHash.Reset()
	expectedHash.Write(tmp)
	expectedHashSum := expectedHash.Sum(nil)

	actualHash := Sha2Sum(data)

	if !bytes.Equal(actualHash[:], expectedHashSum) {
		t.Errorf("Expected hash: %x, got: %x", expectedHashSum, actualHash)
	}
}
