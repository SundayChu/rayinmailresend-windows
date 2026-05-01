# rayinmailresend Windows 版

這是一個依照 `json_readme.txt` 建立的 Windows helper 專案，重點是讓 Windows 使用者能準備並驗證 Gmail OAuth JSON：

- `gmail_credentials.json`: Google Cloud 下載的 OAuth Client 設定。
- `gmail_token.json`: 透過本機 OAuth 流程產生的 Gmail 使用者 token。

此版本提供 Windows 可直接執行的 `e.ps1` / `e.bat`，並保留原文件中的檢查參數：

```powershell
.\e.ps1 --once --check-login-only
.\e.ps1 --once --check-gmail-api-login-only
```

## 需求

- Windows 10/11
- Go 1.22 或更新版
- Google Cloud OAuth Client 類型請選「電腦版應用程式」

## 第一次設定

1. 複製設定檔：

   ```powershell
   Copy-Item .\config.properties.example .\config.properties
   ```

2. 從 Google Cloud 下載 OAuth client JSON，改名為：

   ```text
   gmail_credentials.json
   ```

3. 放到本專案目錄：

   ```text
   rayinmailresend-windows\gmail_credentials.json
   ```

4. 產生 Gmail token：

   ```powershell
   .\e.ps1 --generate-gmail-token
   ```

   程式會開啟瀏覽器，完成授權後會寫入：

   ```text
   gmail_token.json
   ```

5. 驗證 Gmail API JSON/token：

   ```powershell
   .\e.ps1 --once --check-gmail-api-login-only
   ```

   成功會看到：

   ```text
   Gmail API 登入檢查成功 account= your@gmail.com
   ```

## POP3 登入檢查

目前預設來源信箱是：

```text
sunday@rayin.com.tw
```

POP3 主機是：

```text
mail.rayin.com.tw:110 + STARTTLS
```

測試時執行：

```powershell
.\test-pop3.ps1
```

它會要求輸入 `sunday@rayin.com.tw` 的 POP3 信箱密碼。

## 開始運作

系統目前設定為：

```text
接收：sunday@rayin.com.tw
轉寄到：java.sunday@gmail.com
```

第一次正式啟動先執行一次：

```powershell
.\run-rayin-to-gmail.ps1 -Once
```

它會要求輸入：

- `sunday@rayin.com.tw` 的 POP3 信箱密碼

第一次執行會把 POP3 裡已存在的信件記錄為已處理，不會轉寄舊信，避免一啟動就大量重寄。

接著啟動常駐模式：

```powershell
.\run-rayin-to-gmail.ps1
```

PowerShell 視窗保持開著，程式會每 60 秒檢查一次 POP3，有新信就用 Gmail API 轉寄。停止時按 `Ctrl+C`。

如果只想看會處理哪些信，不實際寄出：

```powershell
.\run-rayin-to-gmail.ps1 -Once -DryRun
```

如果要固定轉寄收件者，也可以在 `config.properties` 設定：

```properties
resend.to=target@example.com
```

多個收件者用逗號分隔。

## 每天自動啟動

先把 `sunday@rayin.com.tw` 的 POP3 密碼加密保存到目前 Windows 使用者：

```powershell
.\save-pop3-secret.ps1
```

再安裝 Windows 工作排程，預設每天 08:00 啟動：

```powershell
.\install-daily-task.ps1
```

指定啟動時間：

```powershell
.\install-daily-task.ps1 -At "09:30"
```

手動立刻啟動工作排程：

```powershell
Start-ScheduledTask -TaskName "rayinmailresend-rayin-to-gmail"
```

查看狀態：

```powershell
Get-ScheduledTask -TaskName "rayinmailresend-rayin-to-gmail"
```

日誌會寫到：

```text
logs\
```

移除自動啟動：

```powershell
.\uninstall-daily-task.ps1
```

## 建置

開發時可直接用：

```powershell
.\e.ps1 --once --check-gmail-api-login-only
```

`e.ps1` 會在需要時自動執行 `go build`。若要輸出正式 exe：

```powershell
.\build.ps1
```

產物會在：

```text
dist\rayinmailresend.exe
```

## 常見錯誤

`403 access_denied` 或 `app not verified`

Google Cloud App 還在測試模式，而且登入的 Gmail 沒加入測試使用者。到 Google Auth Platform 的「目標對象」加入測試使用者。

`找不到 gmail_credentials.json`

確認檔案放在專案目錄，或修改 `config.properties` 的 `gmail.credentials.file`。

`gmail_token.json 沒有 refresh_token`

重新執行：

```powershell
.\e.ps1 --generate-gmail-token
```

## 安全注意

以下檔案含敏感資料，不要上傳公開 git：

- `gmail_credentials.json`
- `gmail_token.json`
- `config.properties`

`.gitignore` 已預設排除這些檔案。
