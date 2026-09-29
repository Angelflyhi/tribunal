package db

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"log"
)

func (d *DB) GenerateOrLoadKeyPair() (ed25519.PrivateKey, ed25519.PublicKey) {
	_, err := d.Exec(`CREATE TABLE IF NOT EXISTS system_keys (
		id TEXT PRIMARY KEY,
		private_key TEXT NOT NULL,
		public_key TEXT NOT NULL
	)`)
	if err != nil {
		log.Fatalf("Failed to create system_keys table: %v", err)
	}

	var privBase64, pubBase64 string
	err = d.QueryRow("SELECT private_key, public_key FROM system_keys WHERE id = 'default_ed25519'").Scan(&privBase64, &pubBase64)
	if err == sql.ErrNoRows {
		// Generate new key
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatalf("Failed to generate Ed25519 keypair: %v", err)
		}
		
		privBase64 = base64.StdEncoding.EncodeToString(priv)
		pubBase64 = base64.StdEncoding.EncodeToString(pub)
		
		_, err = d.Exec("INSERT INTO system_keys (id, private_key, public_key) VALUES ('default_ed25519', ?, ?)", privBase64, pubBase64)
		if err != nil {
			log.Fatalf("Failed to store Ed25519 keypair: %v", err)
		}
		
		return priv, pub
	} else if err != nil {
		log.Fatalf("Failed to load Ed25519 keypair: %v", err)
	}

	priv, _ := base64.StdEncoding.DecodeString(privBase64)
	pub, _ := base64.StdEncoding.DecodeString(pubBase64)
	
	return ed25519.PrivateKey(priv), ed25519.PublicKey(pub)
}
