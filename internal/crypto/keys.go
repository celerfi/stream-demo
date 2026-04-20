package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Keypair holds an Ed25519 public/private key pair
type Keypair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// GenerateKeypair creates a new random Ed25519 keypair
func GenerateKeypair() (*Keypair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair: %w", err)
	}
	return &Keypair{
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// PublicKeyHex returns the public key as a hex string
func (k *Keypair) PublicKeyHex() string {
	return hex.EncodeToString(k.PublicKey)
}

// Sign signs a message and returns the signature as a hex string
func (k *Keypair) Sign(message []byte) string {
	sig := ed25519.Sign(k.PrivateKey, message)
	return hex.EncodeToString(sig)
}

// Verify checks a hex-encoded signature against a message and hex-encoded public key
func Verify(pubKeyHex string, message []byte, sigHex string) (bool, error) {
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false, fmt.Errorf("invalid public key hex: %w", err)
	}

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false, fmt.Errorf("invalid signature hex: %w", err)
	}

	return ed25519.Verify(pubBytes, message, sigBytes), nil
}