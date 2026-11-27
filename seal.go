package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/scrypt"
)

// Sealed layout: salt | nonce | AES-256-GCM ciphertext.
const sealSaltLen = 16

func seal(secret string, plain []byte) ([]byte, error) {
	salt := make([]byte, sealSaltLen)
	rand.Read(salt)
	aead, err := sealCipher(secret, salt)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	rand.Read(nonce)
	out := append(salt, nonce...)
	return aead.Seal(out, nonce, plain, nil), nil
}

func unseal(secret string, sealed []byte) ([]byte, error) {
	if len(sealed) < sealSaltLen {
		return nil, errors.New("sealed data too short")
	}
	aead, err := sealCipher(secret, sealed[:sealSaltLen])
	if err != nil {
		return nil, err
	}
	rest := sealed[sealSaltLen:]
	if len(rest) < aead.NonceSize() {
		return nil, errors.New("sealed data too short")
	}
	return aead.Open(nil, rest[:aead.NonceSize()], rest[aead.NonceSize():], nil)
}

func sealCipher(secret string, salt []byte) (cipher.AEAD, error) {
	// Parameters as recommended for interactive logins in the scrypt package docs.
	key, err := scrypt.Key([]byte(secret), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
