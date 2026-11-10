package ops

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"time"
)

// SendMail delivers a plain-text UTF-8 message. Port 465 uses TLS from the
// start; other ports upgrade with STARTTLS when the server offers it, and
// never send a password over an unencrypted connection.
func (s Settings) SendMail(ctx context.Context, to []string, subject, body string) error {
	if !s.MailConfigured() {
		return errors.New("e-mail is not configured")
	}
	if len(to) == 0 {
		return nil
	}
	from, err := mail.ParseAddress(s.SMTPFrom)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.SMTPHost, strconv.Itoa(s.SMTPPort))
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	tlsConf := &tls.Config{ServerName: s.SMTPHost}

	var conn net.Conn
	if s.SMTPPort == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConf)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(time.Minute))
	c, err := smtp.NewClient(conn, s.SMTPHost)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	_, secure := conn.(*tls.Conn)
	if ok, _ := c.Extension("STARTTLS"); ok && !secure {
		if err := c.StartTLS(tlsConf); err != nil {
			return err
		}
		secure = true
	}
	if s.SMTPUser != "" {
		if !secure {
			return errors.New("the SMTP server offers no encryption; refusing to send the password")
		}
		if err := c.Auth(smtp.PlainAuth("", s.SMTPUser, s.SMTPPass, s.SMTPHost)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("%s: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(message(from, to, subject, body)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func message(from *mail.Address, to []string, subject, body string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", joinComma(to))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(body))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.Bytes()
}

func joinComma(xs []string) string {
	var b bytes.Buffer
	for i, x := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(x)
	}
	return b.String()
}
