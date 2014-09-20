package gosecret

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var gosecretRegex, _ = regexp.Compile("\\[(gosecret\\|[^\\]]*)\\]")

func createRandomBytes(length int) []byte {
	random_bytes := make([]byte, length)
	rand.Read(random_bytes)
	return random_bytes
}

// Create a random 256-bit array suitable for use as an AES-256 cipher key.
func CreateKey() []byte {
	return createRandomBytes(32)
}

// Create a random initialization vector to use for encryption.  Each gosecret tag should have a different
// initialization vector.
func CreateIV() []byte {
	return createRandomBytes(12)
}

func createCipher(key []byte) (cipher.AEAD, error) {
	aes, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(aes)
	if err != nil {
		return nil, err
	}
	return aesgcm, nil
}

func encrypt(plaintext []byte, key []byte, iv []byte, ad []byte) ([]byte, error) {
	aesgcm, err := createCipher(key)
	if (err != nil) {
		return nil, err
	}
	return aesgcm.Seal(nil, iv, plaintext, ad), nil
}

func decrypt(ciphertext []byte, key []byte, iv []byte, ad []byte) ([]byte, error) {
	aesgcm, err := createCipher(key)
	if (err != nil) {
		return nil, err
	}

	return aesgcm.Open(nil, iv, ciphertext, ad)
}

// EncryptTags looks for any tagged data of the form [gosecret|authtext|plaintext] in the input content byte
// array and replaces each with an encrypted gosecret tag.  Note that the input content must be valid UTF-8.
// The second parameter is the name of the keyfile to use for encrypting all tags in the content, and the
// third parameter is the 256-bit key itself.
// EncryptTags returns a []byte with all unencrypted [gosecret] blocks replaced by encrypted gosecret tags.
func EncryptTags(content []byte, keyname string, key []byte) ([]byte, error) {

	if (!utf8.Valid(content)) {
		return nil, errors.New("File is not valid UTF-8")
	}

	match := gosecretRegex.Match(content)
	
	if (match) {
		matches := gosecretRegex.FindAllSubmatch(content, -1)
		for _, match := range matches {
			// The string we need is in the first capture group
			matchStr := string(match[1])
			parts := strings.Split(matchStr, "|")
			if (len(parts) > 3) {
				// Block is already encrypted.  Skipping.
				// TODO: Support optional reencryption.
			} else {
				iv := CreateIV()
				cipherText, err := encrypt([]byte(parts[2]), key, iv, []byte(parts[1]))
				if (err != nil) {
					return nil, err
				}

				replacement := fmt.Sprintf("[gosecret|%s|%s|%s|%s]",
					parts[1],
					base64.StdEncoding.EncodeToString(cipherText),
					base64.StdEncoding.EncodeToString(iv),
					keyname)

				content = bytes.Replace(content, match[0], []byte(replacement), 1)
			}

		}
	}

	return content, nil;
}

// DecryptTags looks for any tagged data of the form [gosecret|authtext|ciphertext|initvector|keyname] in the
// input content byte array and replaces each with a decrypted version of the ciphertext.  Note that the
// input content must be valid UTF-8.  The second parameter is the path to the directory in which keyfiles
// live.  For each |keyname| in a gosecret block, there must be a corresponding file of the same name in the
// keystore directory.
// DecryptTags returns a []byte with all [gosecret] blocks replaced by plaintext.
func DecryptTags(content []byte, keyroot string) ([]byte, error) {

	if (!utf8.Valid(content)) {
		return nil, errors.New("File is not valid UTF-8")
	}

	match := gosecretRegex.Match(content)

	if (match) {
		matches := gosecretRegex.FindAllSubmatch(content, -1)
		for _, match := range matches {
			// The string we need is in the first capture group
			matchStr := string(match[1])
			parts := strings.Split(matchStr, "|")
			if (len(parts) < 5) {
				// Block is not encrypted.  Skipping.
			} else {
				ct, err := base64.StdEncoding.DecodeString(parts[2])
				if (err != nil) {
					fmt.Println("Unable to decode ciphertext", parts[2], err)
					return nil, err
				}

				iv, err := base64.StdEncoding.DecodeString(parts[3])
				if (err != nil) {
					fmt.Println("Unable to decode IV", err)
					return nil, err
				}

				keyfile, err := ioutil.ReadFile(filepath.Join(keyroot, parts[4]))
				if (err != nil) {
					fmt.Println("Unable to read file for decryption", err)
					return nil, err
				}

				key, err := base64.StdEncoding.DecodeString(string(keyfile))
				if (err != nil) {
					fmt.Println("Unable to decode key", err)
					return nil, err
				}

				plainText, err := decrypt(ct, key, iv, []byte(parts[1]))
				if (err != nil) {
					return nil, err
				}

				content = bytes.Replace(content, match[0], []byte(plainText), 1)
			}
		}
	}

	return content, nil;
}
