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
	"github.com/nehalshaquib/solana-balance-reporter/internal/solana"
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
func (m *Mailer) SendReport(csvFilePath string, balances []*solana.TokenBalance) error {
	if len(m.emailTo) == 0 {
		return fmt.Errorf("no recipients configured")
	}

	m.logger.Log(fmt.Sprintf("Preparing to send email report with attachment %s to %d recipients",
		csvFilePath, len(m.emailTo)))

	// Get current exact timestamp
	now := time.Now().UTC()
	exactTimestamp := now.Format("2006-01-02 15:04:05 UTC")

	// Extract the time information from the filename
	filename := filepath.Base(csvFilePath)
	timeStr := strings.TrimPrefix(strings.TrimSuffix(filename, ".csv"), "balance_")
	t, err := time.Parse("2006-01-02_15_04_05", timeStr)
	if err != nil {
		// Try the old format if new format fails
		t, err = time.Parse("2006-01-02_15", timeStr)
		if err != nil {
			return fmt.Errorf("failed to parse time from filename: %w", err)
		}
	}

	// Create formatted time strings for the email
	dateStr := t.Format("2 January 2006")
	hourStr := t.Format("15:00")
	nextHourStr := t.Add(time.Hour).Format("15:00")

	// Count successful and failed fetches
	totalAddresses := len(balances)
	successCount := 0
	failedCount := 0

	for _, balance := range balances {
		if balance.FetchError == nil {
			successCount++
		} else {
			failedCount++
		}
	}

	// Format subject and body
	subject := fmt.Sprintf("Token Balance Report for %s, %s - %s UTC", dateStr, hourStr, nextHourStr)
	body := fmt.Sprintf(`Hello,

Attached is the token balance report for %s, %s - %s UTC.

This report contains wallet addresses and their JINGLE token balances.

Summary:
- Total addresses processed: %d
- Successfully fetched: %d
- Failed to fetch: %d
- Failed addresses are marked as "N/A" in the balance column

This report was generated at exactly: %s

Best regards,
Solana Balance Reporter
`, dateStr, hourStr, nextHourStr, totalAddresses, successCount, failedCount, exactTimestamp)

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
	// Set up TLS config
	tlsConfig := &tls.Config{
		ServerName:         m.smtpServer,
		InsecureSkipVerify: false, // Never skip verification in production
		MinVersion:         tls.VersionTLS12,
	}

	// Connect to the SMTP server
	addr := fmt.Sprintf("%s:%d", m.smtpServer, m.smtpPort)

	// Try different email sending methods - sometimes AWS SES requires different approaches
	err := m.sendWithStartTLS(addr, mimeMsg)
	if err != nil {
		m.logger.LogError("Failed to send using StartTLS, trying direct TLS", err)
		err = m.sendWithDirectTLS(addr, tlsConfig, mimeMsg)
	}

	return err
}

// sendWithStartTLS attempts to send email using SMTP StartTLS
func (m *Mailer) sendWithStartTLS(addr string, mimeMsg []byte) error {
	// Set up authentication
	auth := smtp.PlainAuth("", m.smtpUsername, m.smtpPassword, m.smtpServer)

	return smtp.SendMail(addr, auth, m.emailFrom, m.emailTo, mimeMsg)
}

// sendWithDirectTLS attempts to send email using direct TLS connection
func (m *Mailer) sendWithDirectTLS(addr string, tlsConfig *tls.Config, mimeMsg []byte) error {
	// Connect to the SMTP server
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

	// Set up authentication
	auth := smtp.PlainAuth("", m.smtpUsername, m.smtpPassword, m.smtpServer)

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
