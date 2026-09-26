package main

import (
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/pem"
    "flag"
    "fmt"
    "math/big"
    "net"
    "os"
    "strings"
    "time"
)

func main() {
    name := flag.String("name", "wbd.local", "certificate DNS name or IP")
    certPath := flag.String("cert", "front.pem", "certificate output")
    keyPath := flag.String("key", "front.key", "private key output")
    days := flag.Int("days", 3650, "validity in days")
    flag.Parse()
    if *days < 1 || strings.TrimSpace(*name) == "" {
        fmt.Fprintln(os.Stderr, "invalid certificate arguments")
        os.Exit(2)
    }
    key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil { fail(err) }
    limit := new(big.Int).Lsh(big.NewInt(1), 128)
    serial, err := rand.Int(rand.Reader, limit)
    if err != nil { fail(err) }
    now := time.Now().UTC()
    tpl := &x509.Certificate{
        SerialNumber: serial,
        Subject: pkix.Name{CommonName: *name},
        NotBefore: now.Add(-5 * time.Minute),
        NotAfter: now.Add(time.Duration(*days) * 24 * time.Hour),
        KeyUsage: x509.KeyUsageDigitalSignature,
        ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid: true,
    }
    if ip := net.ParseIP(*name); ip != nil { tpl.IPAddresses = []net.IP{ip} } else { tpl.DNSNames = []string{*name} }
    der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
    if err != nil { fail(err) }
    keyDER, err := x509.MarshalPKCS8PrivateKey(key)
    if err != nil { fail(err) }
    if err := os.WriteFile(*certPath, pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE", Bytes:der}), 0644); err != nil { fail(err) }
    if err := os.WriteFile(*keyPath, pem.EncodeToMemory(&pem.Block{Type:"PRIVATE KEY", Bytes:keyDER}), 0600); err != nil { fail(err) }
    fmt.Printf("WBD_SERVER_CERT_PASS name=%s cert=%s key=%s\n", *name, *certPath, *keyPath)
}

func fail(err error) { fmt.Fprintln(os.Stderr, "WBD_SERVER_CERT_FAIL", err); os.Exit(1) }
