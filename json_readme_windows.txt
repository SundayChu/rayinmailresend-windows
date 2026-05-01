rayinmailresend Windows 版 - JSON 檔準備說明

目的
- 本專案使用兩個 JSON：
  1) gmail_credentials.json：OAuth Client 設定（應用程式身分）
  2) gmail_token.json：OAuth 使用者授權 token（實際 Gmail 帳號身分）

放置位置
- 建議都放在 Windows 專案目錄：
  D:\Dropbox\Sunday\general_project\rayinmailresend-windows\
- 預設設定檔對應：
  - gmail.credentials.file=.\gmail_credentials.json
  - gmail.token.file=.\gmail_token.json

------------------------------------------------------------
一、準備 gmail_credentials.json（Google Cloud 下載）
------------------------------------------------------------
1. 進入 Google Cloud Console，選擇要使用的 Project。
2. 進入 Google Auth Platform。
3. 完成 OAuth 設定（品牌、目標對象、聯絡資訊）。
4. 目標對象若為「測試中」：
   - 必須在「測試使用者」加入要登入的 Gmail 帳號。
   - 否則授權時會出現 403 access_denied。
5. 進入「用戶端」→「建立用戶端」。
6. 應用程式類型選「電腦版應用程式」。
7. 建立後按「下載 JSON」。
8. 將下載檔改名為 gmail_credentials.json，放到本專案目錄。

------------------------------------------------------------
二、準備 gmail_token.json（Windows OAuth 授權產生）
------------------------------------------------------------
1. 在本專案目錄執行：

   .\e.ps1 --generate-gmail-token

2. 瀏覽器用目標 Gmail 帳號登入並同意授權。
3. 程式收到授權 callback 後會自動存成 gmail_token.json。
4. 切換 Gmail 帳號時，只要重做本段，重新產生新的 gmail_token.json。

說明
- gmail_credentials.json 可跨多帳號共用（同一個 OAuth client）。
- gmail_token.json 綁定「某一個 Gmail 帳號」。
- 程式設定 gmail.user.id=me 時，會使用 token 對應的帳號。

------------------------------------------------------------
三、驗證 JSON 是否可用
------------------------------------------------------------
在 Windows PowerShell 的專案目錄執行：

1) 只測 POP3

   .\e.ps1 --once --check-login-only

2) 只測 Gmail API（驗證 JSON/token）

   .\e.ps1 --once --check-gmail-api-login-only

成功會看到：

Gmail API 登入檢查成功 account= <你的gmail>

------------------------------------------------------------
四、開始運作（Windows）
------------------------------------------------------------
第一次正式啟動：

   .\run-rayin-to-gmail.ps1 -Once

目前預設：
- 接收：sunday@rayin.com.tw
- POP3：mail.rayin.com.tw:110 + STARTTLS
- 轉寄到：java.sunday@gmail.com

這次會要求輸入 sunday@rayin.com.tw 的 POP3 信箱密碼。

第一次執行會把 POP3 裡已存在的信件記錄成已處理，不會轉寄舊信。

之後啟動常駐模式：

   .\run-rayin-to-gmail.ps1

PowerShell 視窗保持開著，程式會每 60 秒檢查一次 POP3。
停止時按 Ctrl+C。

------------------------------------------------------------
五、常見錯誤對照
------------------------------------------------------------
1) 403 access_denied / app not verified
- 原因：App 在測試模式，但該 Gmail 未加入測試使用者。
- 解法：到 Google Auth Platform -> 目標對象 -> 測試使用者新增該帳號。

2) 讀取 gmail_credentials.json 失敗
- 原因：檔案路徑錯誤或檔案不存在。
- 解法：確認檔案在專案目錄，或修正 config.properties 路徑。

3) gmail oauth token 驗證失敗
- 原因：token 過期/撤銷/不是此 client 產生。
- 解法：重新做 OAuth 授權，重建 gmail_token.json。

------------------------------------------------------------
六、安全建議
------------------------------------------------------------
- 這兩個 JSON 都是敏感檔案，不要上傳公開 git。
- 本專案的 .gitignore 已排除：
  - gmail_credentials.json
  - gmail_token.json
  - config.properties
