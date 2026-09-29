package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
)

func main() {
	bundlePath := flag.String("bundle", "", "Path to results-bundle.zip")
	flag.Parse()

	if *bundlePath == "" {
		log.Fatalf("Usage: verify-bundle -bundle <path-to-zip>")
	}

	zr, err := zip.OpenReader(*bundlePath)
	if err != nil {
		log.Fatalf("Failed to open bundle: %v", err)
	}
	defer zr.Close()

	var resultsHash, anchorHash string
	var manifest struct {
		ResultsHash string `json:"results_hash"`
		AnchorHash  string `json:"anchor_hash"`
		Issuer      string `json:"issuer"`
		Version     string `json:"version"`
		Signature   string `json:"signature"`
		PublicKey   string `json:"public_key"`
	}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			log.Fatal(err)
		}
		
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Fatal(err)
		}

		if f.Name == "results.json" {
			h := sha256.Sum256(data)
			resultsHash = hex.EncodeToString(h[:])
		} else if f.Name == "audit-anchor.json" {
			h := sha256.Sum256(data)
			anchorHash = hex.EncodeToString(h[:])
		} else if f.Name == "manifest.json" {
			if err := json.Unmarshal(data, &manifest); err != nil {
				log.Fatalf("Failed to parse manifest: %v", err)
			}
		}
	}

	if manifest.ResultsHash == "" || manifest.AnchorHash == "" || manifest.PublicKey == "" {
		log.Fatalf("Invalid or missing manifest.json")
	}

	fmt.Println("🔍 Checking Results Hash...")
	if resultsHash != manifest.ResultsHash {
		log.Fatalf("❌ results.json hash mismatch!")
	}
	fmt.Println("✅ Results Hash matches")

	fmt.Println("🔍 Checking Audit Anchor Hash...")
	if anchorHash != manifest.AnchorHash {
		log.Fatalf("❌ audit-anchor.json hash mismatch!")
	}
	fmt.Println("✅ Audit Anchor Hash matches")

	fmt.Println("🔍 Cryptographically Verifying Signature...")
	pubBytes, err := base64.StdEncoding.DecodeString(manifest.PublicKey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		log.Fatalf("❌ Invalid public key")
	}

	sigBytes, err := hex.DecodeString(manifest.Signature)
	if err != nil {
		log.Fatalf("❌ Invalid signature encoding")
	}

	msg := []byte(manifest.ResultsHash + manifest.AnchorHash)
	valid := ed25519.Verify(pubBytes, msg, sigBytes)

	if !valid {
		log.Fatalf("❌ Cryptographic signature verification failed! The bundle was tampered with or not signed by the issuer.")
	}

	fmt.Println("✅ Cryptographic Signature Valid")
	fmt.Printf("\n🏆 VERIFICATION SUCCESSFUL\nIssuer: %s (v%s)\n", manifest.Issuer, manifest.Version)
}
