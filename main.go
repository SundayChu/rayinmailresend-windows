package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConfigFile = "config.properties"
	defaultAuthURI    = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenURI   = "https://oauth2.googleapis.com/token"
)

type appConfig struct {
	values  map[string]string
	baseDir string
}

type oauthClientFile struct {
	Installed oauthClientConfig `json:"installed"`
	Web       oauthClientConfig `json:"web"`
}

type oauthClientConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	ProjectID    string   `json:"project_id"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
	RedirectURIs []string `json:"redirect_uris"`
}

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Expiry       string `json:"expiry,omitempty"`
	ExpiryDate   int64  `json:"expiry_date,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type tokenEndpointResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type gmailProfile struct {
	EmailAddress  string `json:"emailAddress"`
	MessagesTotal int64  `json:"messagesTotal"`
	ThreadsTotal  int64  `json:"threadsTotal"`
	HistoryID     string `json:"historyId"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "錯誤:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rayinmailresend", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		configPath    string
		once          bool
		checkPOP3     bool
		checkGmailAPI bool
		generateToken bool
		runResend     bool
		dryRun        bool
		pollSeconds   int
		oauthPort     int
	)

	fs.StringVar(&configPath, "config", defaultConfigFile, "config.properties path")
	fs.BoolVar(&once, "once", false, "compatibility flag for the original script")
	fs.BoolVar(&checkPOP3, "check-login-only", false, "check POP3 login only")
	fs.BoolVar(&checkGmailAPI, "check-gmail-api-login-only", false, "check Gmail API token only")
	fs.BoolVar(&generateToken, "generate-gmail-token", false, "start OAuth flow and write gmail_token.json")
	fs.BoolVar(&runResend, "run", false, "poll POP3 and resend new messages with Gmail API")
	fs.BoolVar(&dryRun, "dry-run", false, "show what would be resent without sending")
	fs.IntVar(&pollSeconds, "poll-seconds", 0, "POP3 polling interval for --run")
	fs.IntVar(&oauthPort, "oauth-port", 0, "OAuth callback port; 0 chooses a free port")

	if err := fs.Parse(args); err != nil {
		return usageError(err)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	ran := false

	if generateToken {
		ran = true
		if err := generateGmailToken(ctx, cfg, oauthPort); err != nil {
			return err
		}
	}

	if checkPOP3 {
		ran = true
		if err := checkPOP3Login(cfg); err != nil {
			return err
		}
	}

	if checkGmailAPI {
		ran = true
		if err := checkGmailAPILogin(ctx, cfg); err != nil {
			return err
		}
	}

	if runResend {
		ran = true
		if err := runResendSystem(ctx, cfg, once, dryRun, pollSeconds); err != nil {
			return err
		}
	}

	if !ran {
		if once {
			fmt.Println("--once 已接受；此 Windows helper 目前提供登入與 OAuth JSON 驗證。")
		}
		printUsage()
	}

	return nil
}

func usageError(err error) error {
	printUsage()
	return err
}

func printUsage() {
	fmt.Println("rayinmailresend Windows helper")
	fmt.Println()
	fmt.Println("常用命令:")
	fmt.Println("  .\\e.ps1 --generate-gmail-token")
	fmt.Println("  .\\e.ps1 --once --check-gmail-api-login-only")
	fmt.Println("  .\\e.ps1 --once --check-login-only")
	fmt.Println("  .\\run.ps1")
	fmt.Println("  .\\run.ps1 -Once")
	fmt.Println()
	fmt.Println("設定檔預設為 .\\config.properties，可用 --config 指定其他路徑。")
}

func loadConfig(path string) (*appConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cfg := &appConfig{
		values: map[string]string{
			"gmail.credentials.file":            ".\\gmail_credentials.json",
			"gmail.token.file":                  ".\\gmail_token.json",
			"gmail.user.id":                     "me",
			"gmail.oauth.scopes":                "https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/gmail.send",
			"pop3.host":                         "mail.rayin.com.tw",
			"pop3.port":                         "110",
			"pop3.tls.mode":                     "starttls",
			"pop3.username":                     "sunday@rayin.com.tw",
			"pop3.password":                     "",
			"pop3.timeout.seconds":              "15",
			"resend.to":                         "java.sunday@gmail.com",
			"resend.from.name":                  "rayinmailresend",
			"resend.subject.prefix":             "[rayinmailresend]",
			"resend.poll.seconds":               "60",
			"resend.state.file":                 ".\\state\\processed_uidl.json",
			"resend.max.messages":               "20",
			"resend.dry.run":                    "false",
			"resend.skip.subject.contains":      "UNIPSG(",
			"resend.skip.from":                  "",
			"resend.skip.existing.on.first.run": "true",
		},
		baseDir: cwd,
	}

	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	cfg.baseDir = filepath.Dir(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && filepath.Base(path) == defaultConfigFile {
			return cfg, nil
		}
		return nil, fmt.Errorf("讀取設定檔失敗 %s: %w", path, err)
	}

	if err := parseProperties(data, cfg.values); err != nil {
		return nil, fmt.Errorf("解析設定檔失敗 %s: %w", path, err)
	}

	return cfg, nil
}

func parseProperties(data []byte, dst map[string]string) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("第 %d 行缺少 '='", lineNo)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("第 %d 行 key 不可為空", lineNo)
		}
		dst[key] = value
	}

	return scanner.Err()
}

func (c *appConfig) get(key string) string {
	return strings.TrimSpace(c.values[key])
}

func (c *appConfig) path(key string) string {
	value := c.get(key)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(c.baseDir, value))
}

func (c *appConfig) intValue(key string, fallback int) int {
	value := c.get(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func (c *appConfig) scopes() []string {
	raw := c.get("gmail.oauth.scopes")
	if raw == "" {
		return []string{"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/gmail.send"}
	}
	return strings.Fields(raw)
}

func generateGmailToken(ctx context.Context, cfg *appConfig, oauthPort int) error {
	credentialsPath := cfg.path("gmail.credentials.file")
	tokenPath := cfg.path("gmail.token.file")

	clientConfig, err := loadOAuthClient(credentialsPath)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(oauthPort)))
	if err != nil {
		return fmt.Errorf("啟動 OAuth callback server 失敗: %w", err)
	}
	defer listener.Close()

	actualPort := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth2callback", actualPort)
	state, err := randomState()
	if err != nil {
		return err
	}

	type authResult struct {
		code string
		err  error
	}
	resultCh := make(chan authResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			resultCh <- authResult{err: errors.New("OAuth state 不一致，授權已中止")}
			return
		}
		if oauthErr := query.Get("error"); oauthErr != "" {
			http.Error(w, oauthErr, http.StatusBadRequest)
			resultCh <- authResult{err: fmt.Errorf("OAuth 授權失敗: %s", oauthErr)}
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			resultCh <- authResult{err: errors.New("OAuth callback 沒有收到 code")}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><meta charset=\"utf-8\"><title>OAuth 完成</title><body>OAuth 授權完成，可以回到 PowerShell 視窗。</body>")
		resultCh <- authResult{code: code}
	})

	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			resultCh <- authResult{err: serveErr}
		}
	}()

	authURL := buildAuthURL(clientConfig, redirectURI, state, cfg.scopes())
	fmt.Println("請在瀏覽器完成 Gmail OAuth 授權。")
	fmt.Println("若瀏覽器未自動開啟，請複製以下 URL：")
	fmt.Println(authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Println("瀏覽器自動開啟失敗，請手動複製上方 URL。")
	}

	var result authResult
	select {
	case result = <-resultCh:
	case <-time.After(5 * time.Minute):
		result.err = errors.New("等待 OAuth 授權逾時")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	if result.err != nil {
		return result.err
	}

	token, err := exchangeCode(ctx, clientConfig, redirectURI, result.code)
	if err != nil {
		return err
	}

	if err := saveToken(tokenPath, token); err != nil {
		return err
	}

	fmt.Printf("已寫入 Gmail token: %s\n", tokenPath)
	return checkGmailAPILogin(ctx, cfg)
}

func buildAuthURL(client oauthClientConfig, redirectURI, state string, scopes []string) string {
	authURI := client.AuthURI
	if authURI == "" {
		authURI = defaultAuthURI
	}

	values := url.Values{}
	values.Set("client_id", client.ClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(scopes, " "))
	values.Set("access_type", "offline")
	values.Set("prompt", "consent")
	values.Set("state", state)

	return authURI + "?" + values.Encode()
}

func exchangeCode(ctx context.Context, client oauthClientConfig, redirectURI, code string) (*oauthToken, error) {
	form := url.Values{}
	form.Set("client_id", client.ClientID)
	form.Set("client_secret", client.ClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)

	response, err := postTokenForm(ctx, client.tokenURI(), form)
	if err != nil {
		return nil, err
	}
	if response.RefreshToken == "" {
		return nil, errors.New("OAuth 回應沒有 refresh_token，請重新執行並確認授權畫面已同意")
	}

	return response.toToken(""), nil
}

func refreshToken(ctx context.Context, client oauthClientConfig, token *oauthToken) (*oauthToken, error) {
	if token.RefreshToken == "" {
		return nil, errors.New("gmail_token.json 沒有 refresh_token，請重新執行 --generate-gmail-token")
	}

	form := url.Values{}
	form.Set("client_id", client.ClientID)
	form.Set("client_secret", client.ClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", token.RefreshToken)

	response, err := postTokenForm(ctx, client.tokenURI(), form)
	if err != nil {
		return nil, err
	}

	refreshed := response.toToken(token.RefreshToken)
	if refreshed.Scope == "" {
		refreshed.Scope = token.Scope
	}
	return refreshed, nil
}

func postTokenForm(ctx context.Context, endpoint string, form url.Values) (*tokenEndpointResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var tokenResp tokenEndpointResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析 OAuth token 回應失敗: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tokenResp.Error != "" {
		if tokenResp.ErrorDescription != "" {
			return nil, fmt.Errorf("OAuth token endpoint 失敗: %s (%s)", tokenResp.Error, tokenResp.ErrorDescription)
		}
		return nil, fmt.Errorf("OAuth token endpoint 失敗: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return &tokenResp, nil
}

func (r *tokenEndpointResponse) toToken(existingRefreshToken string) *oauthToken {
	refreshToken := r.RefreshToken
	if refreshToken == "" {
		refreshToken = existingRefreshToken
	}
	tokenType := r.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}

	expiry := ""
	if r.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}

	return &oauthToken{
		AccessToken:  r.AccessToken,
		TokenType:    tokenType,
		RefreshToken: refreshToken,
		Expiry:       expiry,
		Scope:        r.Scope,
	}
}

func checkGmailAPILogin(ctx context.Context, cfg *appConfig) error {
	credentialsPath := cfg.path("gmail.credentials.file")
	tokenPath := cfg.path("gmail.token.file")

	clientConfig, err := loadOAuthClient(credentialsPath)
	if err != nil {
		return err
	}

	token, err := loadToken(tokenPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("找不到 %s，請先執行 .\\e.ps1 --generate-gmail-token", tokenPath)
		}
		return err
	}

	if token.needsRefresh() {
		refreshed, err := refreshToken(ctx, clientConfig, token)
		if err != nil {
			return err
		}
		token = refreshed
		if err := saveToken(tokenPath, token); err != nil {
			return err
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
			return refreshErr
		}
		token = refreshed
		if saveErr := saveToken(tokenPath, token); saveErr != nil {
			return saveErr
		}
		profile, _, err = fetchGmailProfile(ctx, userID, token)
	}
	if err != nil {
		return err
	}

	fmt.Printf("Gmail API 登入檢查成功 account= %s\n", profile.EmailAddress)
	return nil
}

func fetchGmailProfile(ctx context.Context, userID string, token *oauthToken) (*gmailProfile, int, error) {
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/" + url.PathEscape(userID) + "/profile"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}

	tokenType := token.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	req.Header.Set("Authorization", tokenType+" "+token.AccessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("Gmail API 驗證失敗: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var profile gmailProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("解析 Gmail API 回應失敗: %w", err)
	}
	if profile.EmailAddress == "" {
		return nil, resp.StatusCode, errors.New("Gmail API 回應沒有 emailAddress")
	}

	return &profile, resp.StatusCode, nil
}

func checkPOP3Login(cfg *appConfig) error {
	host := firstNonEmpty(os.Getenv("POP3_HOST"), cfg.get("pop3.host"))
	port := firstNonEmpty(os.Getenv("POP3_PORT"), cfg.get("pop3.port"))
	tlsMode := firstNonEmpty(os.Getenv("POP3_TLS_MODE"), cfg.get("pop3.tls.mode"))
	username := firstNonEmpty(cfg.get("pop3.username"), os.Getenv("POP3_USERNAME"), os.Getenv("GMAIL_POP3_USERNAME"))
	password := firstNonEmpty(cfg.get("pop3.password"), os.Getenv("POP3_PASSWORD"), os.Getenv("GMAIL_APP_PASSWORD"))

	if host == "" {
		return errors.New("pop3.host 不可為空")
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
		return errors.New("pop3.username 未設定，可在 config.properties 設定或使用 POP3_USERNAME 環境變數")
	}
	if password == "" {
		return errors.New("pop3.password 未設定，可在 config.properties 設定或使用 POP3_PASSWORD/GMAIL_APP_PASSWORD 環境變數")
	}

	timeout := time.Duration(cfg.intValue("pop3.timeout.seconds", 15)) * time.Second
	client, err := openPOP3Client(host, port, tlsMode, timeout)
	if err != nil {
		return err
	}
	defer client.close()

	if err := client.commandOK("USER "+username, "USER"); err != nil {
		return err
	}
	if err := client.commandOK("PASS "+password, "PASS"); err != nil {
		return err
	}

	fmt.Printf("POP3 登入檢查成功 server= %s user= %s tls= %s\n", net.JoinHostPort(host, port), username, tlsMode)
	return nil
}

func sendPOP3Command(w io.Writer, command string) error {
	_, err := io.WriteString(w, command+"\r\n")
	return err
}

func expectPOP3OK(reader *bufio.Reader, step string) error {
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("讀取 POP3 %s 回應失敗: %w", step, err)
	}

	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "+OK") {
		return fmt.Errorf("POP3 %s 失敗: %s", step, line)
	}
	return nil
}

func loadOAuthClient(path string) (oauthClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return oauthClientConfig{}, fmt.Errorf("找不到 %s，請先從 Google Cloud 下載並命名為 gmail_credentials.json", path)
		}
		return oauthClientConfig{}, fmt.Errorf("讀取 OAuth client JSON 失敗 %s: %w", path, err)
	}

	var raw oauthClientFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return oauthClientConfig{}, fmt.Errorf("解析 OAuth client JSON 失敗 %s: %w", path, err)
	}

	client := raw.Installed
	if client.ClientID == "" {
		client = raw.Web
	}

	if client.ClientID == "" {
		return oauthClientConfig{}, fmt.Errorf("%s 缺少 installed.client_id 或 web.client_id", path)
	}
	if client.AuthURI == "" {
		client.AuthURI = defaultAuthURI
	}
	if client.TokenURI == "" {
		client.TokenURI = defaultTokenURI
	}

	return client, nil
}

func (c oauthClientConfig) tokenURI() string {
	if c.TokenURI == "" {
		return defaultTokenURI
	}
	return c.TokenURI
}

func loadToken(path string) (*oauthToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var token oauthToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("解析 Gmail token JSON 失敗 %s: %w", path, err)
	}
	if token.AccessToken == "" && token.RefreshToken == "" {
		return nil, fmt.Errorf("%s 缺少 access_token/refresh_token", path)
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	return &token, nil
}

func saveToken(path string, token *oauthToken) error {
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (t *oauthToken) needsRefresh() bool {
	if t.AccessToken == "" {
		return true
	}
	expiry := t.expiryTime()
	if expiry.IsZero() {
		return false
	}
	return time.Now().Add(60 * time.Second).After(expiry)
}

func (t *oauthToken) expiryTime() time.Time {
	if t.Expiry != "" {
		if parsed, err := time.Parse(time.RFC3339, t.Expiry); err == nil {
			return parsed
		}
		if parsed, err := time.Parse("2006-01-02T15:04:05.999999999Z07:00", t.Expiry); err == nil {
			return parsed
		}
	}

	if t.ExpiryDate > 0 {
		if t.ExpiryDate > 1_000_000_000_000 {
			return time.UnixMilli(t.ExpiryDate)
		}
		return time.Unix(t.ExpiryDate, 0)
	}

	return time.Time{}
}

func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
