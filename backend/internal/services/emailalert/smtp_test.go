package emailalert

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	cert, e := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)
	return cert, roots
}
func TestSMTPRequiresVerifiedTLSAndHonestAcceptance(t *testing.T) {
	for _, tt := range []struct {
		name, transport, outcome string
		trust                    bool
		wantError, wantUncertain bool
	}{
		{"TLS accepted", "tls", "ok", true, false, false}, {"STARTTLS accepted", "starttls", "ok", true, false, false},
		{"untrusted certificate", "tls", "ok", false, true, false}, {"no STARTTLS", "plaintext", "ok", true, true, false},
		{"recipient rejected", "tls", "reject", true, true, false}, {"dropped after DATA", "tls", "drop", true, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cert, roots := testCertificate(t)
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			var auth atomic.Bool
			received := make(chan []byte, 1)
			go func() {
				raw, e := listener.Accept()
				if e != nil {
					return
				}
				defer raw.Close()
				_ = raw.SetDeadline(time.Now().Add(3 * time.Second))
				conn := raw
				if tt.transport == "tls" {
					secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
					if secure.Handshake() != nil {
						return
					}
					conn = secure
				}
				rw := textproto.NewConn(conn)
				_ = rw.PrintfLine("220 localhost test SMTP")
				for {
					line, e := rw.ReadLine()
					if e != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"):
						if tt.transport == "starttls" {
							_ = rw.PrintfLine("250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN")
						} else {
							_ = rw.PrintfLine("250-localhost\r\n250 AUTH PLAIN")
						}
					case line == "STARTTLS":
						_ = rw.PrintfLine("220 ready")
						secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
						if secure.Handshake() != nil {
							return
						}
						conn = secure
						rw = textproto.NewConn(conn)
					case strings.HasPrefix(line, "AUTH "):
						auth.Store(true)
						_ = rw.PrintfLine("235 accepted")
					case strings.HasPrefix(line, "MAIL FROM:"):
						_ = rw.PrintfLine("250 ok")
					case strings.HasPrefix(line, "RCPT TO:"):
						if tt.outcome == "reject" {
							_ = rw.PrintfLine("550 rejected")
						} else {
							_ = rw.PrintfLine("250 ok")
						}
					case line == "DATA":
						_ = rw.PrintfLine("354 continue")
						body, e := rw.ReadDotBytes()
						if e != nil {
							return
						}
						received <- body
						if tt.outcome == "drop" {
							return
						}
						_ = rw.PrintfLine("250 accepted")
					case line == "QUIT":
						_ = rw.PrintfLine("221 bye")
						return
					default:
						_ = rw.PrintfLine("500 unsupported")
					}
				}
			}()
			port, _ := strconv.Atoi(strings.Split(listener.Addr().String(), ":")[1])
			c := validConfig()
			c.Host = "localhost"
			c.Port = port
			c.TLSMode = "tls"
			if tt.transport != "tls" {
				c.TLSMode = "starttls"
			}
			transport := SMTP{}
			if tt.trust {
				transport.roots = roots
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := transport.Send(ctx, c, "synthetic-password", Message{ID: "test-message", To: "owner@example.com", Subject: "ทดสอบภาษาไทย", Body: "ชื่อผู้รับไม่ตรง กรุณาตรวจสอบ"})
			if (err != nil) != tt.wantError {
				t.Fatalf("got %v", err)
			}
			if err != nil {
				e, ok := err.(*SendError)
				if !ok || e.Uncertain != tt.wantUncertain {
					t.Fatalf("wrong uncertainty: %v", err)
				}
				if strings.Contains(err.Error(), "synthetic-password") {
					t.Fatal("secret in error")
				}
			}
			if (!tt.trust || tt.transport == "plaintext") && auth.Load() {
				t.Fatal("credentials sent without verified TLS")
			}
			if !tt.wantError {
				select {
				case raw := <-received:
					m, e := mail.ReadMessage(strings.NewReader(string(raw)))
					if e != nil {
						t.Fatal(e)
					}
					subject, e := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
					if e != nil || subject != "ทดสอบภาษาไทย" {
						t.Fatal("bad Thai subject", e)
					}
					body, e := io.ReadAll(quotedprintable.NewReader(m.Body))
					if e != nil || strings.TrimSpace(string(body)) != "ชื่อผู้รับไม่ตรง กรุณาตรวจสอบ" {
						t.Fatal("bad Thai body", e)
					}
				case <-ctx.Done():
					t.Fatal("message not received")
				}
			}
		})
	}
}
