package utils

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig holds SMTP server settings.
type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	// From is the envelope/header From address. Defaults to User when empty.
	From string
}

// SMTPConfigFromMap builds SMTPConfig from settings key/value map.
// Returns nil when host is missing (mail not configured).
func SMTPConfigFromMap(m map[string]string) *SMTPConfig {
	host := strings.TrimSpace(m["smtp_host"])
	if host == "" {
		return nil
	}
	port, _ := strconv.Atoi(strings.TrimSpace(m["smtp_port"]))
	if port == 0 {
		port = 587
	}
	user := strings.TrimSpace(m["smtp_user"])
	pass := m["smtp_pass"] // do not trim password (may have intentional spaces — rare)
	from := strings.TrimSpace(m["smtp_from"])
	if from == "" {
		from = user
	}
	return &SMTPConfig{
		Host: host,
		Port: port,
		User: user,
		Pass: pass,
		From: from,
	}
}

// Addr returns the host:port address string.
func (c *SMTPConfig) Addr() string {
	if c.Port == 0 {
		c.Port = 587
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// FromAddr returns the From address used for envelope/header.
func (c *SMTPConfig) FromAddr() string {
	if c == nil {
		return ""
	}
	if c.From != "" {
		return c.From
	}
	return c.User
}

// SendMail sends a plain-text email using the configured SMTP server.
// Supports:
//   - 465: implicit TLS (SMTPS)
//   - 587 / other: plain connect + STARTTLS when available
func (c *SMTPConfig) SendMail(to, subject, body string) error {
	if c == nil || c.Host == "" {
		return fmt.Errorf("SMTP 未配置")
	}
	if to == "" {
		return fmt.Errorf("收件人为空")
	}

	from := c.FromAddr()
	if from == "" {
		return fmt.Errorf("发件人为空，请填写 SMTP 用户/发件邮箱")
	}

	addr := c.Addr()
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n"+
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	var client *smtp.Client
	var err error

	if c.Port == 465 {
		tlsCfg := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
		conn, dialErr := tls.Dial("tcp", addr, tlsCfg)
		if dialErr != nil {
			return fmt.Errorf("TLS 连接失败: %w", dialErr)
		}
		client, err = smtp.NewClient(conn, c.Host)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("SMTP 客户端创建失败: %w", err)
		}
	} else {
		conn, dialErr := net.DialTimeout("tcp", addr, 15*time.Second)
		if dialErr != nil {
			return fmt.Errorf("连接 SMTP 失败: %w", dialErr)
		}
		client, err = smtp.NewClient(conn, c.Host)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("SMTP 客户端创建失败: %w", err)
		}
		// Prefer STARTTLS on submission ports; keep going on plain 25 if server refuses
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsCfg := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
			if err = client.StartTLS(tlsCfg); err != nil {
				_ = client.Close()
				return fmt.Errorf("STARTTLS 失败: %w", err)
			}
		} else if c.Port == 587 {
			_ = client.Close()
			return fmt.Errorf("端口 587 要求 STARTTLS，但服务器未提供")
		}
	}
	defer client.Close()

	if c.User != "" && c.Pass != "" {
		// Prefer AUTH PLAIN; fall back to LOGIN is not in stdlib — PlainAuth is enough for most providers
		auth := smtp.PlainAuth("", c.User, c.Pass, c.Host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}

	if err = client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM 失败: %w", err)
	}
	for _, r := range strings.Split(to, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if err = client.Rcpt(r); err != nil {
			return fmt.Errorf("RCPT TO 失败 (%s): %w", r, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA 失败: %w", err)
	}
	if _, err = w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("写入邮件失败: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("结束 DATA 失败: %w", err)
	}

	if err = client.Quit(); err != nil {
		// Some servers close early after success; treat as soft error only if mail already sent
		return nil
	}
	return nil
}
