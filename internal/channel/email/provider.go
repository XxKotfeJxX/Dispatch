package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"

	"dispatch/internal/config"
	"dispatch/internal/delivery"
)

type Provider struct{ config config.SMTPConfig }

func New(value config.SMTPConfig) *Provider   { return &Provider{config: value} }
func (p *Provider) Channel() delivery.Channel { return delivery.ChannelEmail }
func (p *Provider) Configured() bool {
	return p.config.Host != "" && p.config.Port > 0 && p.config.From != ""
}

func (p *Provider) Deliver(ctx context.Context, message delivery.Message) (delivery.ProviderResult, error) {
	if !p.Configured() {
		return delivery.ProviderResult{}, &delivery.ProviderError{Code: "email_not_configured", Message: "SMTP is not configured"}
	}
	address := net.JoinHostPort(p.config.Host, strconv.Itoa(p.config.Port))
	var auth smtp.Auth
	if p.config.Username != "" {
		auth = smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
	}
	payload := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		p.config.From, message.Destination, message.Subject, message.Body))
	result := make(chan error, 1)
	go func() {
		if !p.config.TLS {
			result <- smtp.SendMail(address, auth, p.config.From, []string{message.Destination}, payload)
			return
		}
		connection, err := tls.Dial("tcp", address, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: p.config.Host})
		if err != nil {
			result <- err
			return
		}
		defer connection.Close()
		client, err := smtp.NewClient(connection, p.config.Host)
		if err != nil {
			result <- err
			return
		}
		defer client.Close()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				result <- err
				return
			}
		}
		if err := client.Mail(p.config.From); err != nil {
			result <- err
			return
		}
		if err := client.Rcpt(message.Destination); err != nil {
			result <- err
			return
		}
		writer, err := client.Data()
		if err == nil {
			_, err = writer.Write(payload)
		}
		if err == nil {
			err = writer.Close()
		}
		result <- err
	}()
	select {
	case <-ctx.Done():
		return delivery.ProviderResult{}, &delivery.ProviderError{Code: "email_timeout", Message: ctx.Err().Error(), Retryable: true}
	case err := <-result:
		if err != nil {
			return delivery.ProviderResult{}, &delivery.ProviderError{Code: "email_provider_error", Message: err.Error(), Retryable: true}
		}
		return delivery.ProviderResult{ResponseCode: 250}, nil
	}
}
