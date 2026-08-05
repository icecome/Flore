package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func TestVerifyAssetSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	digest := []byte("dummy sha256 digest 32 bytes!!")
	validSig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, digest))

	t.Run("valid signature passes", func(t *testing.T) {
		asset := &Asset{FileName: "flore.zip", Signature: validSig}
		if err := verifyAssetSignatureWithKey(pub, asset, digest); err != nil {
			t.Fatalf("expected pass, got %v", err)
		}
	})

	t.Run("tampered digest fails", func(t *testing.T) {
		asset := &Asset{FileName: "flore.zip", Signature: validSig}
		tampered := append([]byte(nil), digest...)
		tampered[0] ^= 0xff
		if err := verifyAssetSignatureWithKey(pub, asset, tampered); err == nil {
			t.Fatal("expected failure for tampered digest")
		}
	})

	t.Run("wrong key fails", func(t *testing.T) {
		otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
		asset := &Asset{FileName: "flore.zip", Signature: validSig}
		if err := verifyAssetSignatureWithKey(otherPub, asset, digest); err == nil {
			t.Fatal("expected failure for wrong key")
		}
	})

	t.Run("missing signature fails", func(t *testing.T) {
		asset := &Asset{FileName: "flore.zip"}
		if err := verifyAssetSignatureWithKey(pub, asset, digest); err == nil {
			t.Fatal("expected failure for missing signature")
		}
	})

	t.Run("nil asset fails", func(t *testing.T) {
		if err := verifyAssetSignatureWithKey(pub, nil, digest); err == nil {
			t.Fatal("expected failure for nil asset")
		}
	})

	t.Run("invalid base64 fails", func(t *testing.T) {
		asset := &Asset{FileName: "flore.zip", Signature: "!!not-base64!!"}
		if err := verifyAssetSignatureWithKey(pub, asset, digest); err == nil {
			t.Fatal("expected failure for invalid base64")
		}
	})

	t.Run("embedded key is well-formed", func(t *testing.T) {
		if len(updatePublicKey) != ed25519.PublicKeySize {
			t.Fatalf("embedded public key size = %d, want %d", len(updatePublicKey), ed25519.PublicKeySize)
		}
	})

	t.Run("error mentions signature", func(t *testing.T) {
		err := verifyAssetSignatureWithKey(pub, &Asset{FileName: "x.zip"}, digest)
		if err == nil || !strings.Contains(err.Error(), "签名") {
			t.Fatalf("expected Chinese signature error, got %v", err)
		}
	})
}
