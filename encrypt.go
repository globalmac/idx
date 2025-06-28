package idx

import (
	"archive/tar"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"os"
)

const (
	saltSize   = 16
	nonceSize  = 12
	keySize    = 32
	pbkdf2Iter = 100_000
)

// iPBKDF2 реализует кастомный PBKDF2-HMAC-SHA256
func iPBKDF2(password, salt []byte, iter, keyLen int) []byte {
	var dk []byte
	var blockNum uint32 = 1
	hLen := sha256.Size
	for len(dk) < keyLen {

		h := hmac.New(sha256.New, password)
		h.Write(salt)
		var b [4]byte
		binaryBigEndianPutUint32(b[:], blockNum)
		h.Write(b[:])
		u := h.Sum(nil)

		t := make([]byte, hLen)
		copy(t, u)

		for i := 1; i < iter; i++ {
			h = hmac.New(sha256.New, password)
			h.Write(u)
			u = h.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		dk = append(dk, t...)
		blockNum++
	}
	return dk[:keyLen]
}

// binaryBigEndianPutUint32 записывает uint32 в буфер в порядке возрастания
func binaryBigEndianPutUint32(buf []byte, v uint32) {
	buf[0] = byte(v >> 24)
	buf[1] = byte(v >> 16)
	buf[2] = byte(v >> 8)
	buf[3] = byte(v)
}

func EncryptDB(inputFile, archiveFile, password string) error {

	out, err := os.Create(archiveFile)
	if err != nil {
		return err
	}
	defer out.Close()

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	key := iPBKDF2([]byte(password), salt, pbkdf2Iter, keySize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	_, err = cipher.NewGCM(block)
	if err != nil {
		return err
	}

	if _, err := out.Write(salt); err != nil {
		return err
	}
	if _, err := out.Write(nonce); err != nil {
		return err
	}

	// Заводим pipe и goroutine
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		gzw := gzip.NewWriter(pw)
		tw := tar.NewWriter(gzw)

		info, err := os.Stat(inputFile)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		header.Name = "database.bin"
		if err := tw.WriteHeader(header); err != nil {
			pw.CloseWithError(err)
			return
		}

		in, err := os.Open(inputFile)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		defer in.Close()

		if _, err := io.Copy(tw, in); err != nil {
			pw.CloseWithError(err)
			return
		}
		tw.Close()
		gzw.Close()
	}()

	// Чтение tar.gz потоковая передача, шифрование и запись в файл
	buf := make([]byte, 64*1024)
	stream := cipher.StreamWriter{
		S: cipher.NewCTR(block, append(nonce, make([]byte, block.BlockSize()-len(nonce))...)),
		W: out,
	}
	if _, err := io.CopyBuffer(stream, pr, buf); err != nil {
		return err
	}
	return nil
}

func DecryptDB(archiveFile, outputFile, password string) error {
	in, err := os.Open(archiveFile)
	if err != nil {
		return err
	}
	defer in.Close()

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(in, salt); err != nil {
		return err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(in, nonce); err != nil {
		return err
	}
	key := iPBKDF2([]byte(password), salt, pbkdf2Iter, keySize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	ctr := cipher.NewCTR(block, append(nonce, make([]byte, block.BlockSize()-len(nonce))...))
	stream := cipher.StreamReader{
		S: ctr,
		R: in,
	}

	gzr, err := gzip.NewReader(stream)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		out, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	return nil
}
