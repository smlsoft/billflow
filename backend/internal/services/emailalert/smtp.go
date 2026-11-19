package emailalert

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

type SendError struct {
	Stage     string
	Code      int
	Uncertain bool
}

func (e *SendError) Error() string {
	if e.Uncertain {
		return "SMTP อาจรับข้อความแล้ว แต่ยืนยันผลไม่ได้ โปรดตรวจสอบก่อนลองใหม่"
	}
	if e.Code != 0 {
		return fmt.Sprintf("ส่งอีเมลไม่สำเร็จที่ %s (SMTP %d)", e.Stage, e.Code)
	}
	return "ส่งอีเมลไม่สำเร็จที่ " + e.Stage + " โปรดตรวจการเชื่อมต่อและค่าบัญชีส่ง"
}

type Message struct{ ID, To, Subject, Body string }
type Mailer interface {
	Send(context.Context, Config, string, Message) error
}
type SMTP struct{ roots *x509.CertPool }

func mimeMessage(c Config, m Message, now time.Time) []byte {
	var content bytes.Buffer
	fmt.Fprintf(&content, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <billflow-%s@%s>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", c.From, m.To, mime.QEncoding.Encode("UTF-8", m.Subject), now.Format(time.RFC1123Z), m.ID, strings.Split(c.From, "@")[1])
	w := quotedprintable.NewWriter(&content)
	_, _ = w.Write([]byte(m.Body))
	_ = w.Close()
	return content.Bytes()
}
func (s SMTP) Send(ctx context.Context, c Config, password string, m Message) error {
	// Config validation is repeated by the worker before use. All credentials are
	// sent only after TLS verification; this transport never offers plaintext mode.
	if !addressOK(c.From) || !addressOK(m.To) || strings.ContainsAny(m.ID, "\r\n<> ") {
		return &SendError{Stage: "ตรวจรูปแบบผู้ส่ง/ผู้รับ"}
	}
	stage := "เชื่อมต่อ"
	uncertain := false
	fail := func(err error) error {
		code := 0
		if e, ok := err.(*textproto.Error); ok {
			code = e.Code
		}
		return &SendError{Stage: stage, Code: code, Uncertain: uncertain}
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(c.Host, fmt.Sprint(c.Port)))
	if err != nil {
		return fail(err)
	}
	defer conn.Close()
	deadline := time.Now().Add(20 * time.Second)
	if t, ok := ctx.Deadline(); ok && t.Before(deadline) {
		deadline = t
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12, RootCAs: s.roots}
	if c.TLSMode == "tls" {
		stage = "TLS"
		secured := tls.Client(conn, tlsConfig)
		if err = secured.HandshakeContext(ctx); err != nil {
			return fail(err)
		}
		conn = secured
	}
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		return fail(err)
	}
	defer client.Close()
	if c.TLSMode == "starttls" {
		stage = "STARTTLS"
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fail(fmt.Errorf("STARTTLS unavailable"))
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return fail(err)
		}
	} else if c.TLSMode != "tls" {
		return fail(fmt.Errorf("TLS required"))
	}
	state, ok := client.TLSConnectionState()
	if !ok || !state.HandshakeComplete {
		return fail(fmt.Errorf("TLS required"))
	}
	stage = "ยืนยันบัญชี SMTP"
	if err = client.Auth(smtp.PlainAuth("", c.Username, password, c.Host)); err != nil {
		return fail(err)
	}
	stage = "ผู้ส่ง"
	if err = client.Mail(c.From); err != nil {
		return fail(err)
	}
	stage = "ผู้รับ"
	if err = client.Rcpt(m.To); err != nil {
		return fail(err)
	}
	stage = "ส่งเนื้อหา"
	w, err := client.Data()
	if err != nil {
		return fail(err)
	}
	// Once DATA starts, a dropped connection may leave an accepted mail on the
	// remote side. Do not automatically retry ambiguous acceptance.
	uncertain = true
	if _, err = w.Write(mimeMessage(c, m, time.Now())); err != nil {
		return fail(err)
	}
	if err = w.Close(); err != nil {
		if _, explicit := err.(*textproto.Error); explicit {
			uncertain = false
		}
		return fail(err)
	}
	_ = client.Quit() // DATA 250 is acceptance; a later QUIT error cannot undo it.
	return nil
}
