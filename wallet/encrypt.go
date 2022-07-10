package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"io/ioutil"
)

func encrypt_file(fn string, key []byte) (outfn string) {
	dat, er := ioutil.ReadFile(fn)
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

	if er = ioutil.WriteFile(outfn, gcm.Seal(nonce, nonce, dat, nil), 0600); er != nil {
		println(er.Error())
		cleanExit(1)
	}

	return
}

func decrypt_file(fn string, key []byte) (outfn string) {
	ciphertext, er := ioutil.ReadFile(fn)
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

	outfn = fn + ".enc"

	if er = ioutil.WriteFile(outfn, plaintext, 0600); er != nil {
		println(er.Error())
		cleanExit(1)
	}

	return
}
