package hosted

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

// Store encrypts OAuth state at rest. Keys are random IDs or SHA-256 token digests.
type Store struct {
	db   *bbolt.DB
	aead cipher.AEAD
}

var errMissing = errors.New("record not found")

func OpenStore(path, key string) (*Store, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("TOKEN_ENCRYPTION_KEY must be base64 for 32 bytes")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, aead: aead}
	if err := db.Update(func(tx *bbolt.Tx) error {
		for _, name := range []string{"clients", "pending", "codes", "access", "refresh", "ynab"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func digest(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (s *Store) encode(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, data, nil), nil
}

func (s *Store) decode(data []byte, v any) error {
	if len(data) < s.aead.NonceSize() {
		return errors.New("encrypted record is corrupt")
	}
	nonce := data[:s.aead.NonceSize()]
	plain, err := s.aead.Open(nil, nonce, data[s.aead.NonceSize():], nil)
	if err != nil {
		return fmt.Errorf("decrypting record: %w", err)
	}
	return json.Unmarshal(plain, v)
}

func (s *Store) put(bucket, key string, v any) error {
	data, err := s.encode(v)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error { return tx.Bucket([]byte(bucket)).Put([]byte(key), data) })
}

func (s *Store) get(bucket, key string, v any) error {
	return s.db.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket([]byte(bucket)).Get([]byte(key))
		if data == nil {
			return errMissing
		}
		return s.decode(data, v)
	})
}

func (s *Store) take(bucket, key string, v any) error {
	return s.takeIf(bucket, key, v, func() bool { return true })
}

func (s *Store) takeIf(bucket, key string, v any, valid func() bool) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		data := b.Get([]byte(key))
		if data == nil {
			return errMissing
		}
		if err := s.decode(data, v); err != nil {
			return err
		}
		if !valid() {
			return errMissing
		}
		return b.Delete([]byte(key))
	})
}

func (s *Store) delete(bucket, key string) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return tx.Bucket([]byte(bucket)).Delete([]byte(key)) })
}
