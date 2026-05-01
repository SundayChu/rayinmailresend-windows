package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type pop3UIDL struct {
	Number int
	UID    string
}

type pop3Client struct {
	conn   net.Conn
	reader *bufio.Reader
}

type resendState struct {
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	ProcessedUIDLs map[string]string `json:"processed_uidls"`
}

type gmailSendRequest struct {
	Raw string `json:"raw"`
}

type gmailSendResponse struct {
	ID      string `json:"id"`
	Thread  string `json:"threadId"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func runResendSystem(parent context.Context, cfg *appConfig, once bool, dryRunFlag bool, pollSecondsFlag int) error {
	pollSeconds := pollSecondsFlag
	if pollSeconds <= 0 {
		pollSeconds = cfg.intValue("resend.poll.seconds", 60)
	}
	if pollSeconds <= 0 {
		pollSeconds = 60
	}

	dryRun := dryRunFlag || cfg.boolValue("resend.dry.run", false)

	if once {
		return processResendCycle(parent, cfg, dryRun)
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := "send"
	if dryRun {
		mode = "dry-run"
	}
	fmt.Printf("rayinmailresend 已開始運作 mode=%s poll=%ds，按 Ctrl+C 停止。\n", mode, pollSeconds)

	for {
		if err := processResendCycle(ctx, cfg, dryRun); err != nil {
			fmt.Printf("運作週期失敗: %v\n", err)
		}

		timer := time.NewTimer(time.Duration(pollSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			fmt.Println("rayinmailresend 已停止。")
			return nil
		case <-timer.C:
		}
	}
}

func processResendCycle(ctx context.Context, cfg *appConfig, dryRun bool) error {
	recipients, err := resendRecipients(cfg)
	if err != nil {
		return err
	}

	token, profile, err := authorizeGmail(ctx, cfg)
	if err != nil {
		return err
	}

	client, err := connectAndLoginPOP3(cfg)
	if err != nil {
		return err
	}
	defer client.close()

	uidls, err := client.uidl()
	if err != nil {
		return err
	}

	statePath := cfg.path("resend.state.file")
	state, isNew, err := loadResendState(statePath)
	if err != nil {
		return err
	}

	if isNew && cfg.boolValue("resend.skip.existing.on.first.run", true) && !dryRun {
		now := time.Now().UTC().Format(time.RFC3339)
		for _, item := range uidls {
			state.ProcessedUIDLs[item.UID] = now
		}
		if err := saveResendState(statePath, state); err != nil {
			return err
		}
		fmt.Printf("第一次啟動：已記錄現有 POP3 郵件 %d 封，不轉寄舊信；之後只處理新信。\n", len(uidls))
		return nil
	}

	pending := make([]pop3UIDL, 0)
	for _, item := range uidls {
		if _, ok := state.ProcessedUIDLs[item.UID]; !ok {
			pending = append(pending, item)
		}
	}

	if len(pending) == 0 {
		fmt.Printf("%s 沒有新信。\n", time.Now().Format("2006-01-02 15:04:05"))
		return nil
	}

	maxMessages := cfg.intValue("resend.max.messages", 20)
	if maxMessages > 0 && len(pending) > maxMessages {
		pending = pending[len(pending)-maxMessages:]
	}

	fmt.Printf("找到新信 %d 封，準備轉寄到 %s。\n", len(pending), strings.Join(recipients, ", "))

	for _, item := range pending {
		rawMessage, err := client.retr(item.Number)
		if err != nil {
			return fmt.Errorf("讀取 POP3 message=%d uid=%s 失敗: %w", item.Number, item.UID, err)
		}

		forwarded, subject := buildForwardEmail(cfg, profile.EmailAddress, recipients, rawMessage)
		if dryRun {
			fmt.Printf("dry-run: 會轉寄 uid=%s subject=%q bytes=%d\n", item.UID, subject, len(rawMessage))
			continue
		}

		messageID, err := sendGmailRaw(ctx, token, cfg.get("gmail.user.id"), forwarded)
		if err != nil {
			return fmt.Errorf("Gmail API 寄送失敗 uid=%s subject=%q: %w", item.UID, subject, err)
		}

		state.ProcessedUIDLs[item.UID] = time.Now().UTC().Format(time.RFC3339)
		if err := saveResendState(statePath, state); err != nil {
			return err
		}
		fmt.Printf("已轉寄 uid=%s gmail_message_id=%s subject=%q\n", item.UID, messageID, subject)
	}

	return nil
}

func authorizeGmail(ctx context.Context, cfg *appConfig) (*oauthToken, *gmailProfile, error) {
	credentialsPath := cfg.path("gmail.credentials.file")
	tokenPath := cfg.path("gmail.token.file")

	clientConfig, err := loadOAuthClient(credentialsPath)
	if err != nil {
		return nil, nil, err
	}

	token, err := loadToken(tokenPath)
	if err != nil {
		return nil, nil, err
	}

	if token.needsRefresh() {
		refreshed, err := refreshToken(ctx, clientConfig, token)
		if err != nil {
			return nil, nil, err
		}
		token = refreshed
		if err := saveToken(tokenPath, token); err != nil {
			return nil, nil, err
		}
	}

	userID := cfg.get("gmail.user.id")
	if userID == "" {
		userID = "me"
	}

	profile, statusCode, err := fetchGmailProfile(ctx, userID, token)
	if statusCode == http.StatusUnauthorized && token.RefreshToken != "" {
		refreshed, refreshErr := refreshToken(ctx, clientConfig, token)
		if refreshErr != nil {
			return nil, nil, refreshErr
		}
		token = refreshed
		if saveErr := saveToken(tokenPath, token); saveErr != nil {
			return nil, nil, saveErr
		}
		profile, _, err = fetchGmailProfile(ctx, userID, token)
	}
	if err != nil {
		return nil, nil, err
	}

	return token, profile, nil
}

func connectAndLoginPOP3(cfg *appConfig) (*pop3Client, error) {
	host := firstNonEmpty(os.Getenv("POP3_HOST"), cfg.get("pop3.host"))
	port := firstNonEmpty(os.Getenv("POP3_PORT"), cfg.get("pop3.port"))
	tlsMode := firstNonEmpty(os.Getenv("POP3_TLS_MODE"), cfg.get("pop3.tls.mode"))
	username := firstNonEmpty(cfg.get("pop3.username"), os.Getenv("POP3_USERNAME"), os.Getenv("GMAIL_POP3_USERNAME"))
	password := firstNonEmpty(cfg.get("pop3.password"), os.Getenv("POP3_PASSWORD"), os.Getenv("GMAIL_APP_PASSWORD"))

	if host == "" {
		return nil, errors.New("pop3.host 不可為空")
	}
	if port == "" {
		port = "995"
	}
	if tlsMode == "" {
		if port == "110" {
			tlsMode = "starttls"
		} else {
			tlsMode = "ssl"
		}
	}
	if username == "" {
		return nil, errors.New("pop3.username 未設定，可在 config.properties 設定或使用 POP3_USERNAME 環境變數")
	}
	if password == "" {
		return nil, errors.New("pop3.password 未設定，可在 config.properties 設定或使用 POP3_PASSWORD/GMAIL_APP_PASSWORD 環境變數")
	}

	timeout := time.Duration(cfg.intValue("pop3.timeout.seconds", 15)) * time.Second
	client, err := openPOP3Client(host, port, tlsMode, timeout)
	if err != nil {
		return nil, err
	}

	if err := client.commandOK("USER "+username, "USER"); err != nil {
		client.close()
		return nil, err
	}
	if err := client.commandOK("PASS "+password, "PASS"); err != nil {
		client.close()
		return nil, err
	}

	return client, nil
}

func openPOP3Client(host string, port string, tlsMode string, timeout time.Duration) (*pop3Client, error) {
	tlsMode = strings.ToLower(strings.TrimSpace(tlsMode))
	if tlsMode == "" {
		tlsMode = "ssl"
	}

	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("連線 POP3 失敗 %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	client := &pop3Client{conn: conn, reader: bufio.NewReader(conn)}
	greeted := false

	switch tlsMode {
	case "ssl", "tls", "implicit":
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.Handshake(); err != nil {
			client.closeRaw()
			return nil, fmt.Errorf("POP3 TLS 握手失敗 %s: %w", addr, err)
		}
		client.conn = tlsConn
		client.reader = bufio.NewReader(tlsConn)
	case "starttls", "stls":
		if err := expectPOP3OK(client.reader, "server greeting"); err != nil {
			client.closeRaw()
			return nil, err
		}
		greeted = true
		if err := client.commandOK("STLS", "STLS"); err != nil {
			client.closeRaw()
			return nil, err
		}
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.Handshake(); err != nil {
			client.closeRaw()
			return nil, fmt.Errorf("POP3 STARTTLS 握手失敗 %s: %w", addr, err)
		}
		client.conn = tlsConn
		client.reader = bufio.NewReader(tlsConn)
	case "plain", "none":
		// Keep the initial clear-text connection.
	default:
		client.closeRaw()
		return nil, fmt.Errorf("不支援的 pop3.tls.mode=%q，請使用 ssl/starttls/plain", tlsMode)
	}

	if !greeted {
		if err := expectPOP3OK(client.reader, "server greeting"); err != nil {
			client.close()
			return nil, err
		}
	}

	return client, nil
}

func (c *pop3Client) close() {
	if c == nil || c.conn == nil {
		return
	}
	_ = sendPOP3Command(c.conn, "QUIT")
	_ = c.conn.Close()
}

func (c *pop3Client) closeRaw() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.conn.Close()
}

func (c *pop3Client) commandOK(command string, step string) error {
	if err := sendPOP3Command(c.conn, command); err != nil {
		return err
	}
	return expectPOP3OK(c.reader, step)
}

func (c *pop3Client) uidl() ([]pop3UIDL, error) {
	if err := sendPOP3Command(c.conn, "UIDL"); err != nil {
		return nil, err
	}
	lines, err := readPOP3Multiline(c.reader, "UIDL")
	if err != nil {
		return nil, err
	}

	items := make([]pop3UIDL, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		number, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		items = append(items, pop3UIDL{Number: number, UID: fields[1]})
	}
	return items, nil
}

func (c *pop3Client) retr(number int) ([]byte, error) {
	if err := sendPOP3Command(c.conn, "RETR "+strconv.Itoa(number)); err != nil {
		return nil, err
	}
	lines, err := readPOP3Multiline(c.reader, "RETR")
	if err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\r\n") + "\r\n"), nil
}

func readPOP3Multiline(reader *bufio.Reader, step string) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("讀取 POP3 %s 回應失敗: %w", step, err)
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "+OK") {
		return nil, fmt.Errorf("POP3 %s 失敗: %s", step, line)
	}

	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("讀取 POP3 %s multiline 失敗: %w", step, err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			break
		}
		if strings.HasPrefix(line, "..") {
			line = line[1:]
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func resendRecipients(cfg *appConfig) ([]string, error) {
	raw := firstNonEmpty(cfg.get("resend.to"), os.Getenv("RESEND_TO"))
	recipients := splitList(raw)
	if len(recipients) == 0 {
		return nil, errors.New("resend.to 未設定，可在 config.properties 設定，或執行 run.ps1 時輸入轉寄目標")
	}

	if _, err := mail.ParseAddressList(strings.Join(recipients, ",")); err != nil {
		return nil, fmt.Errorf("resend.to 收件者格式錯誤: %w", err)
	}
	return recipients, nil
}

func buildForwardEmail(cfg *appConfig, fromEmail string, recipients []string, original []byte) ([]byte, string) {
	headers := parseOriginalHeaders(original)
	originalSubject := firstNonEmpty(headers["Subject"], "(no subject)")
	prefix := cfg.get("resend.subject.prefix")
	subject := strings.TrimSpace(strings.TrimSpace(prefix) + " " + originalSubject)

	fromName := cfg.get("resend.from.name")
	if fromName == "" {
		fromName = "rayinmailresend"
	}

	boundary := "rayinmailresend-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	body := buildForwardBody(headers)

	var buf bytes.Buffer
	writeMailHeader(&buf, "From", (&mail.Address{Name: fromName, Address: fromEmail}).String())
	writeMailHeader(&buf, "To", strings.Join(recipients, ", "))
	writeMailHeader(&buf, "Subject", mime.QEncoding.Encode("UTF-8", sanitizeHeader(subject)))
	writeMailHeader(&buf, "Date", time.Now().Format(time.RFC1123Z))
	writeMailHeader(&buf, "MIME-Version", "1.0")
	writeMailHeader(&buf, "Content-Type", fmt.Sprintf("multipart/mixed; boundary=%q", boundary))
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	buf.WriteString(wrapBase64([]byte(body)))
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: application/octet-stream; name=\"original.eml\"\r\n")
	buf.WriteString("Content-Disposition: attachment; filename=\"original.eml\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	buf.WriteString(wrapBase64(original))
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "--\r\n")
	return buf.Bytes(), subject
}

func parseOriginalHeaders(original []byte) map[string]string {
	result := map[string]string{
		"From":    "",
		"To":      "",
		"Date":    "",
		"Subject": "",
	}

	message, err := mail.ReadMessage(bytes.NewReader(original))
	if err != nil {
		return result
	}

	decoder := new(mime.WordDecoder)
	for key := range result {
		value := message.Header.Get(key)
		if decoded, err := decoder.DecodeHeader(value); err == nil {
			value = decoded
		}
		result[key] = strings.TrimSpace(value)
	}
	return result
}

func buildForwardBody(headers map[string]string) string {
	var b strings.Builder
	b.WriteString("這是一封由 rayinmailresend 自動轉寄的郵件。\r\n\r\n")
	b.WriteString("原始寄件者: " + firstNonEmpty(headers["From"], "(unknown)") + "\r\n")
	b.WriteString("原始收件者: " + firstNonEmpty(headers["To"], "(unknown)") + "\r\n")
	b.WriteString("原始日期: " + firstNonEmpty(headers["Date"], "(unknown)") + "\r\n")
	b.WriteString("原始主旨: " + firstNonEmpty(headers["Subject"], "(no subject)") + "\r\n\r\n")
	b.WriteString("原始郵件已附加為 original.eml。\r\n")
	return b.String()
}

func writeMailHeader(buf *bytes.Buffer, key string, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(sanitizeHeader(value))
	buf.WriteString("\r\n")
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func wrapBase64(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	if encoded == "" {
		return ""
	}

	var b strings.Builder
	for len(encoded) > 76 {
		b.WriteString(encoded[:76])
		b.WriteString("\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded)
	b.WriteString("\r\n")
	return b.String()
}

func sendGmailRaw(ctx context.Context, token *oauthToken, userID string, rawMessage []byte) (string, error) {
	if userID == "" {
		userID = "me"
	}

	payload := gmailSendRequest{Raw: base64.RawURLEncoding.EncodeToString(rawMessage)}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := "https://gmail.googleapis.com/gmail/v1/users/" + url.PathEscape(userID) + "/messages/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	tokenType := token.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	req.Header.Set("Authorization", tokenType+" "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var sendResp gmailSendResponse
	if err := json.Unmarshal(body, &sendResp); err != nil {
		return "", fmt.Errorf("解析 Gmail send 回應失敗: %w", err)
	}
	if sendResp.ID == "" {
		return "", errors.New("Gmail send 回應沒有 message id")
	}
	return sendResp.ID, nil
}

func loadResendState(path string) (*resendState, bool, error) {
	state := &resendState{ProcessedUIDLs: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			now := time.Now().UTC().Format(time.RFC3339)
			state.CreatedAt = now
			state.UpdatedAt = now
			return state, true, nil
		}
		return nil, false, fmt.Errorf("讀取狀態檔失敗 %s: %w", path, err)
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, false, fmt.Errorf("解析狀態檔失敗 %s: %w", path, err)
	}
	if state.ProcessedUIDLs == nil {
		state.ProcessedUIDLs = map[string]string{}
	}
	return state, false, nil
}

func saveResendState(path string, state *resendState) error {
	if state.CreatedAt == "" {
		state.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

func (c *appConfig) boolValue(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(c.values[key]))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func splitList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})

	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}
