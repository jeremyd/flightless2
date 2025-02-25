package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/ssh/terminal"
)

func Encrypt(passphrase, plaintext string) string {
	key, salt := DeriveKey(passphrase, nil)
	iv := make([]byte, 12)
	// http://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf
	// Section 8.2
	rand.Read(iv)
	b, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(b)
	data := aesgcm.Seal(nil, iv, []byte(plaintext), nil)
	return hex.EncodeToString(salt) + "-" + hex.EncodeToString(iv) + "-" + hex.EncodeToString(data)
}

func Decrypt(passphrase, ciphertext string) string {
	arr := strings.Split(ciphertext, "-")
	salt, _ := hex.DecodeString(arr[0])
	iv, _ := hex.DecodeString(arr[1])
	data, _ := hex.DecodeString(arr[2])
	key, _ := DeriveKey(passphrase, salt)
	b, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(b)
	data, _ = aesgcm.Open(nil, iv, data, nil)
	return string(data)
}

func DeriveKey(passphrase string, salt []byte) ([]byte, []byte) {
	if salt == nil {
		salt = make([]byte, 8)
		// http://www.ietf.org/rfc/rfc2898.txt
		// Salt.
		rand.Read(salt)
	}
	return pbkdf2.Key([]byte(passphrase), salt, 1000, 32, sha256.New), salt
}

func GetNewPwd() []byte {
	// Prompt the user to enter a password
	fmt.Println("Enter password")
	// We will use this to store the users input
	// Read the users input
	//pwd1, err := term.ReadPassword(syscall.Stdin)
	pwd1, err := terminal.ReadPassword(int(os.Stdin.Fd()))

	fmt.Println("Confirm password")
	pwd2, err2 := terminal.ReadPassword(int(os.Stdin.Fd()))
	//pwd2, err2 := term.ReadPassword(syscall.Stdin)
	if err != nil || err2 != nil {
		log.Println(err)
	}

	if bytes.Equal(pwd1, pwd2) == false {
		log.Println("Passwords do not match")
		os.Exit(1)
	}

	// Return the users input as a byte slice which will save us
	// from having to do this conversion later on
	return pwd1
}

func GetPwd() []byte {
	// Prompt the user to enter a password
	fmt.Println("Enter password")
	// We will use this to store the users input

	// Read the users input
	//	pwd1, err := term.ReadPassword(syscall.Stdin)
	pwd1, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		log.Println(err)
	}

	// Return the users input as a byte slice which will save us
	// from having to do this conversion later on
	return pwd1
}

func HashAndSalt(pwd []byte) string {

	// Use GenerateFromPassword to hash & salt pwd
	// MinCost is just an integer constant provided by the bcrypt
	// package along with DefaultCost & MaxCost.
	// The cost can be any value you want provided it isn't lower
	// than the MinCost (4)
	hash, err := bcrypt.GenerateFromPassword(pwd, bcrypt.MinCost)
	if err != nil {
		log.Println(err)
	}
	// GenerateFromPassword returns a byte slice so we need to
	// convert the bytes to a string and return it
	return string(hash)
}

func ComparePasswords(hashedPwd string, plainPwd []byte) bool {
	// Since we'll be getting the hashed password from the DB it
	// will be a string so we'll need to convert it to a byte slice
	byteHash := []byte(hashedPwd)
	err := bcrypt.CompareHashAndPassword(byteHash, plainPwd)
	if err != nil {
		log.Println(err)
		return false
	}

	return true
}
