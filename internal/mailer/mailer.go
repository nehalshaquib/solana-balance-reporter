package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"math"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nehalshaquib/solana-balance-reporter/internal/logger"
)

// Mailer handles sending emails with CSV attachments
type Mailer struct {
	smtpServer   string
	smtpPort     int
	smtpUsername string
	smtpPassword string
	emailFrom    string
	emailTo      []string
	logger       *logger.Logger
	maxRetries   int
	retryDelay   time.Duration
}

// New creates a new Mailer
func New(smtpServer string, smtpPort int, smtpUsername, smtpPassword, emailFrom string, emailTo []string, maxRetries int, logger *logger.Logger) *Mailer {
	return &Mailer{
		smtpServer:   smtpServer,
		smtpPort:     smtpPort,
		smtpUsername: smtpUsername,
		smtpPassword: smtpPassword,
		emailFrom:    emailFrom,
		emailTo:      emailTo,
		logger:       logger,
		maxRetries:   maxRetries,
		retryDelay:   500 * time.Millisecond,
	}
}

// SendReport sends an email with the CSV report attached
func (m *Mailer) SendReport(csvFilePath string) error {
	if len(m.emailTo) == 0 {
		return fmt.Errorf("no recipients configured")
	}

	m.logger.Log(fmt.Sprintf("Preparing to send email report with attachment %s to %d recipients",
		csvFilePath, len(m.emailTo)))

	// Extract the time information from the filename
	filename := filepath.Base(csvFilePath)
	timeStr := strings.TrimPrefix(strings.TrimSuffix(filename, ".csv"), "balance_")
	t, err := time.Parse("2006-01-02_15", timeStr)
	if err != nil {
		return fmt.Errorf("failed to parse time from filename: %w", err)
	}

	// Create formatted time strings for the email
	dateStr := t.Format("2 January 2006")
	hourStr := t.Format("15:00")
	nextHourStr := t.Add(time.Hour).Format("15:00")

	// Format subject and body
	subject := fmt.Sprintf("Token Balance Report for %s, %s - %s UTC", dateStr, hourStr, nextHourStr)
	body := fmt.Sprintf(`Hello,

Attached is the token balance report for %s, %s - %s UTC.

This report contains wallet addresses and their token balances.

Best regards,
Solana Balance Reporter
`, dateStr, hourStr, nextHourStr)

	// Read the CSV file content
	csvContent, err := readFile(csvFilePath)
	if err != nil {
		return fmt.Errorf("failed to read CSV file: %w", err)
	}

	// Create the MIME message with attachment
	boundary := "solanaReportBoundary"
	mimeMsgBytes := createMimeMessage(
		m.emailFrom,
		m.emailTo,
		subject,
		body,
		filename,
		csvContent,
		boundary,
	)

	// Attempt to send the email with retries
	var sendErr error
	for attempt := 0; attempt <= m.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * m.retryDelay
			m.logger.Log(fmt.Sprintf("Retrying email send (attempt %d/%d) after %v",
				attempt, m.maxRetries, backoff))
			time.Sleep(backoff)
		}

		sendErr = m.sendEmail(mimeMsgBytes)
		if sendErr == nil {
			break
		}

		m.logger.LogError(fmt.Sprintf("Email send attempt %d failed", attempt+1), sendErr)
	}

	if sendErr != nil {
		return fmt.Errorf("failed to send email after %d attempts: %w", m.maxRetries+1, sendErr)
	}

	m.logger.Log(fmt.Sprintf("Successfully sent email report to %s", strings.Join(m.emailTo, ", ")))
	return nil
}

// sendEmail sends the email using SMTP
func (m *Mailer) sendEmail(mimeMsg []byte) error {
	// Set up authentication
	auth := smtp.PlainAuth("", m.smtpUsername, m.smtpPassword, m.smtpServer)

	// Set up TLS config
	tlsConfig := &tls.Config{
		ServerName: m.smtpServer,
	}

	// Connect to the SMTP server
	addr := fmt.Sprintf("%s:%d", m.smtpServer, m.smtpPort)
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.smtpServer)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Authenticate
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set the sender and recipients
	if err = client.Mail(m.emailFrom); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	for _, recipient := range m.emailTo {
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	// Send the message
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to start mail data: %w", err)
	}

	_, err = w.Write(mimeMsg)
	if err != nil {
		return fmt.Errorf("failed to write mail data: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close mail data: %w", err)
	}

	return client.Quit()
}

// readFile reads a file's content
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// createMimeMessage creates a MIME message with an attachment
func createMimeMessage(from string, to []string, subject, body, filename string, attachment []byte, boundary string) []byte {
	var message strings.Builder

	// Add headers
	message.WriteString(fmt.Sprintf("From: %s\r\n", from))
	message.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	message.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	message.WriteString(fmt.Sprintf("MIME-Version: 1.0\r\n"))
	message.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n\r\n", boundary))

	// Add text part
	message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	message.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	message.WriteString(body)
	message.WriteString("\r\n\r\n")

	// Add attachment part
	message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	message.WriteString(fmt.Sprintf("Content-Type: text/csv; name=\"%s\"\r\n", filename))
	message.WriteString("Content-Transfer-Encoding: base64\r\n")
	message.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", filename))

	// Encode attachment as base64
	encodedAttachment := base64.StdEncoding.EncodeToString(attachment)

	// Add attachment content in chunks of 76 characters
	chunkSize := 76
	for i := 0; i < len(encodedAttachment); i += chunkSize {
		end := i + chunkSize
		if end > len(encodedAttachment) {
			end = len(encodedAttachment)
		}
		message.WriteString(encodedAttachment[i:end] + "\r\n")
	}

	// Add closing boundary
	message.WriteString(fmt.Sprintf("\r\n--%s--", boundary))

	return []byte(message.String())
}
