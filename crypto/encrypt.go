// Adopted from github.com/hashicorp/memberlist with minor changes
// Only AES 128 bit encryption in GCM mode is supported now.

package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

const (
	cipherSize     = 1
	nonceSize      = 12
	tagSize        = 16
	maxPadOverhead = 16
	blockSize      = aes.BlockSize
)

//Cipher represents the encryption algorithm
type Cipher uint8

const (
	// Aes128Gcm AES 128 bit in GCM mode
	Aes128Gcm Cipher = 1
)

// encryptedLength is used to compute the buffer size needed
// for a message of given length
func encryptedLength(msgSize int) int {
	return cipherSize + nonceSize + msgSize + tagSize
}

// EncryptPayload is used to encrypt a message with a given key.
// We make use of AES-128 in GCM mode. dst buffer will have cipher,
// nonce, ciphertext and tag
func EncryptPayload(c Cipher, key []byte, msg []byte, data []byte, dst *bytes.Buffer) error {
	// Only AES 128 bits in GCM mode is supported now.
	if c != Aes128Gcm {
		return fmt.Errorf("invalid cipher for encryption")
	}

	// Get the AES block cipher
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	// Get the GCM cipher mode
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return err
	}

	// Grow the buffer to make room for everything
	offset := dst.Len()
	dst.Grow(encryptedLength(len(msg)))

	// Write the cipher value
	dst.WriteByte(byte(c))

	// Add a random nonce
	io.CopyN(dst, rand.Reader, nonceSize)
	afterNonce := dst.Len()

	// Encrypt message using GCM
	slice := dst.Bytes()[offset:]
	nonce := slice[cipherSize : cipherSize+nonceSize]

	out := gcm.Seal(nil, nonce, msg, data)
	// Truncate the plaintext, and write the cipher text
	dst.Truncate(afterNonce)
	dst.Write(out)
	return nil
}

// decryptMessage performs the actual decryption of ciphertext. This is in its
// own function to allow it to be called on all keys easily.
func decryptMessage(key, msg, data []byte) ([]byte, error) {
	// Get the AES block cipher
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Get the GCM cipher mode
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}

	// Decrypt the message
	nonce := msg[cipherSize : cipherSize+nonceSize]
	ciphertext := msg[cipherSize+nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, data)
	if err != nil {
		return nil, err
	}

	// Success!
	return plain, nil
}

// DecryptPayload is used to decrypt a message with a given key,
// and verify it's contents. returned slice has the plaintext data
func DecryptPayload(c Cipher, keys [][]byte, msg, data []byte) ([]byte, error) {
	// Ensure we have at least one byte
	if len(msg) == 0 {
		return nil, fmt.Errorf("cannot decrypt empty payload")
	}
	// Check if the cipher is supported
	msgCipher := Cipher(msg[0])
	if msgCipher != c {
		return nil, fmt.Errorf("unsupported encryption cipher %d", msg[0])
	}

	// Ensure the length is sane
	if len(msg) < encryptedLength(0) {
		return nil, fmt.Errorf("payload is too small to decrypt: %d", len(msg))
	}

	for _, key := range keys {
		plain, err := decryptMessage(key, msg, data)
		if err == nil {
			return plain, nil
		}
	}

	return nil, fmt.Errorf("no installed keys could decrypt the message")
}
