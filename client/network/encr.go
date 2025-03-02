package network

import (
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

type aesData struct {
	cipher.Block
	cipher.AEAD
}

func (c *OneConnection) Encrypt(plaintext []byte) ([]byte, error) {
	if c.aesData == nil {
		return nil, fmt.Errorf("c.aesData is nil")
	}
	nonce := make([]byte, c.aesData.AEAD.NonceSize())
	if _, er := io.ReadFull(rand.Reader, nonce); er != nil {
		return nil, er
	}
	return c.aesData.AEAD.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *OneConnection) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := c.aesData.AEAD.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := c.aesData.AEAD.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
