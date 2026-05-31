package context

// cryptoEquivalenceClasses returns equivalence classes for cryptography patterns.
func cryptoEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "CRYPTO_ENCRYPT",
		Phrases:    []string{"encrypt", "decrypt", "encryption", "decryption", "cipher", "aes", "symmetric encryption"},
		Targets:    []string{"Cipher", "Encrypt", "Decrypt", "AES", "NewCipher", "GCM", "Block", "Stream"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CRYPTO_HASH",
		Phrases:    []string{"hash function", "sha256", "md5", "digest", "checksum", "hmac"},
		Targets:    []string{"Hash", "SHA256", "MD5", "Sum", "Digest", "HMAC", "New", "Write"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CRYPTO_SIGN",
		Phrases:    []string{"digital signature", "sign message", "verify signature", "rsa signature", "ecdsa"},
		Targets:    []string{"Sign", "Verify", "PrivateKey", "PublicKey", "SignPKCS1v15", "VerifyPKCS1v15", "ECDSA"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CRYPTO_TLS",
		Phrases:    []string{"tls handshake", "tls connection", "certificate", "x509", "certificate authority"},
		Targets:    []string{"TLSConfig", "Certificate", "X509", "CertPool", "LoadX509KeyPair", "Handshake"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CRYPTO_RANDOM",
		Phrases:    []string{"random number", "secure random", "crypto random", "random bytes"},
		Targets:    []string{"Reader", "Read", "GenerateKey", "Prime", "Int"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
	}
}
