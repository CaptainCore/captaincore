//
//    Generate rclone hashed passwords for .rclone.config
//    go run Scripts/Get/pw.go password
//
//		Pulled in from https://github.com/ncw/rclone/blob/master/fs/config.go
//


package main

import (
	"fmt"
	"io"
  "os"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"

)

func main() {
	fmt.Println( Obscure(os.Args[1])  )
}

// crypt internals
var (
	cryptKey = []byte{
		0x9c, 0x93, 0x5b, 0x48, 0x73, 0x0a, 0x55, 0x4d,
		0x6b, 0xfd, 0x7c, 0x63, 0xc8, 0x86, 0xa9, 0x2b,
		0xd3, 0x90, 0x19, 0x8e, 0xb8, 0x12, 0x8a, 0xfb,
		0xf4, 0xde, 0x16, 0x2b, 0x8b, 0x95, 0xf6, 0x38,
	}
	cryptBlock cipher.Block
	cryptRand  = rand.Reader
)

// crypt transforms in to out using iv under AES-CTR.
//
// in and out may be the same buffer.
//
// Note encryption and decryption are the same operation
func crypt(out, in, iv []byte) error {
	if cryptBlock == nil {
		var err error
		cryptBlock, err = aes.NewCipher(cryptKey)
		if err != nil {
			return err
		}
	}
	stream := cipher.NewCTR(cryptBlock, iv)
	stream.XORKeyStream(out, in)
	return nil
}


func Obscure(x string) (string) {
	plaintext := []byte(x)
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(cryptRand, iv); err != nil {
		// return "", errors.Wrap(err, "failed to read iv")
	}
	if err := crypt(ciphertext[aes.BlockSize:], plaintext, iv); err != nil {
		// return "", errors.Wrap(err, "encrypt failed")
	}
	return base64.RawURLEncoding.EncodeToString(ciphertext)
}
