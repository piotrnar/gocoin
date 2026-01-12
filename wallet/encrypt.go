package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"os"
	"strings"
)

func encrypt_file(fn string, key []byte) (outfn string) {
	dat, er := os.ReadFile(fn)
	if er != nil {
		println(er.Error())
		cleanExit(1)
	}

	cphr, er := aes.NewCipher(key)
	if er != nil {
		println(er.Error())
		cleanExit(1)
	}

	gcm, er := cipher.NewGCM(cphr)
	if er != nil {
		println(er.Error())
		cleanExit(1)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, er = io.ReadFull(rand.Reader, nonce); er != nil {
		println(er.Error())
		cleanExit(1)
	}

	outfn = fn + ".enc"

	if er = os.WriteFile(outfn, gcm.Seal(nonce, nonce, dat, nil), 0600); er != nil {
		println(er.Error())
		cleanExit(1)
	}

	return
}

func decrypt_file(fn string, key []byte) (outfn string) {
	ciphertext, er := os.ReadFile(fn)
	if er != nil {
		println(er.Error())
		cleanExit(1)
	}

	cphr, er := aes.NewCipher(key)
	if er != nil {
		println(er.Error())
		cleanExit(1)
	}

	gcmDecrypt, er := cipher.NewGCM(cphr)
	if er != nil {
		println(er.Error())
		cleanExit(1)
	}

	nonceSize := gcmDecrypt.NonceSize()
	if len(ciphertext) < nonceSize {
		println("ERROR: Encrypted message is shorter than the nonce size")
		cleanExit(1)
	}
	nonce, encryptedMessage := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, er := gcmDecrypt.Open(nil, nonce, encryptedMessage, nil)
	if er != nil {
		println(er.Error())
		cleanExit(1)
	}

	if strings.HasSuffix(fn, ".enc") {
		if len(fn) <= 4 {
			outfn = "out.tmp"
		} else {
			outfn = fn[:len(fn)-4]
		}
	} else {
		outfn = fn + ".dec"
	}

	if er = os.WriteFile(outfn, plaintext, 0600); er != nil {
		println(er.Error())
		cleanExit(1)
	}

	return
}
