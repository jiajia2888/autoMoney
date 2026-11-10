package wechat

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Identity struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}
type credential struct {
	Session    string    `json:"session"`
	Identity   Identity  `json:"identity"`
	VerifiedAt time.Time `json:"verified_at"`
}
type store struct{ path string }

var errStorage = errors.New("视频号本地会话读取或保存失败，请检查文件权限或重新扫码")

func atomicWrite(path string, data []byte) error {
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return errStorage
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".wechat-*")
	if err != nil {
		return errStorage
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if f.Chmod(0600) != nil {
		return errStorage
	}
	if _, err = f.Write(data); err != nil {
		return errStorage
	}
	if f.Sync() != nil {
		return errStorage
	}
	if f.Close() != nil {
		return errStorage
	}
	if os.Rename(f.Name(), path) != nil {
		return errStorage
	}
	return nil
}
func (s store) aead(create bool) (cipher.AEAD, error) {
	key, err := os.ReadFile(s.path + ".key")
	if errors.Is(err, os.ErrNotExist) && create {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, errStorage
		}
		if err = atomicWrite(s.path+".key", key); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, errStorage
	}
	if len(key) != 32 {
		return nil, errStorage
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errStorage
	}
	return cipher.NewGCM(block)
}
func (s store) save(c credential) error {
	a, err := s.aead(true)
	if err != nil {
		return err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return errStorage
	}
	data, err := json.Marshal(c)
	if err != nil {
		return errStorage
	}
	return atomicWrite(s.path, a.Seal(nonce, nonce, data, []byte("automoney-wechat-v1")))
}
func (s store) load() (*credential, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || len(raw) > 65536 {
		return nil, errStorage
	}
	a, err := s.aead(false)
	if err != nil {
		return nil, err
	}
	if len(raw) < a.NonceSize() {
		return nil, errStorage
	}
	data, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte("automoney-wechat-v1"))
	if err != nil {
		return nil, errStorage
	}
	var c credential
	if json.Unmarshal(data, &c) != nil || c.Session == "" || c.Identity.Username == "" {
		return nil, errStorage
	}
	return &c, nil
}
func (s store) clear() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errStorage
	}
	return nil
}
