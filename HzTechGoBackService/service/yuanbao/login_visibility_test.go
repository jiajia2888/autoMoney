package yuanbao

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoginOnlyOffersQRWhenImageIsReady(t *testing.T) {
	s := &Service{Ctx: context.Background(), Credentials: filepath.Join(t.TempDir(), "auth.json"), state: "starting"}
	c := s.Status()
	if c.State != "starting" || c.QRReady {
		t.Fatal("launch must not claim QR ready", c)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...)
	os.WriteFile(s.Credentials+".qr.png", png, 0600)
	c = s.Status()
	if c.State != "waiting" || !c.QRReady {
		t.Fatal("ready QR not offered", c)
	}
	raw, err := s.QR()
	if err != nil || len(raw) != len(png) {
		t.Fatal(err)
	}
	s.state = "saved"
	if s.Status().QRReady {
		t.Fatal("completed login must not expose stale QR")
	}
	if _, err = s.QR(); err == nil {
		t.Fatal("stale QR served")
	}
}
