## Step 1: Initialize and Clean Go Modules

```bash
go mod tidy
```

---

## Step 2: Generate Encryption Keysets

Run the following command to generate encryption keysets:

```bash
go run ./cmd/s3-proxy/main.go cryption-keyset
```

You will receive 3 keysets in the terminal. Copy and store them in your `.env` file as follows:

```env
TINK_KEYSET="PASTE_TINK_KEYSET_HERE"
AES_KEY="PASTE_AES_KEY_HERE"
CHACHA_KEY="PASTE_CHACHA_KEY_HERE"
```

---

## Step 3: Run the S3 Proxy Server

Use the following command to start the proxy server:

```bash
go run ./cmd/s3-proxy/main.go s3-proxy --config=configs/main.yaml
```

### Test Cases of authentication

Use PowerShell commands to test the proxy. These are the same as in the previous response, but I’ll repeat them for clarity, along with expected logs.

1. **Valid PUT Request**

   ```powershell
   Invoke-WebRequest -Method Put -Uri "http://localhost:8080/test-bucket/hello.txt" -Headers @{ "Authorization" = "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250516/us-east-1/s3/aws4_request" } -InFile "D:\MyProjects\s3-proxy\test\testdata\hello.txt" -ContentType "text/plain"
   ```

   **Expected Response**: HTTP 200 OK

2. **Invalid Access Key**

   ```powershell
   Invoke-WebRequest -Method Put -Uri "http://localhost:8080/test-bucket/test-key" -Headers @{ "Authorization" = "AWS4-HMAC-SHA256 Credential=INVALIDKEY/20250516/us-east-1/s3/aws4_request" } -Body "test data"
   ```

   **Expected Response**: HTTP 401 Unauthorized, body: "invalid access key"

3. **Missing Authorization Header**

   ```powershell
   Invoke-WebRequest -Method Put -Uri "http://localhost:8080/test-bucket/test-key" -Body "test data"
   ```

   **Expected Response**: HTTP 401 Unauthorized, body: "missing Authorization header"

4. **Health Check**

   ```powershell
   Invoke-WebRequest -Method Get -Uri "http://localhost:8080/healthz"
   ```

   **Expected Response**: HTTP 200 OK, body: "ok"

5. **Valid GET Request**

   ```powershell
   Invoke-WebRequest -Method Get -Uri "http://localhost:8080/test-bucket/test-key" -Headers @{ "Authorization" = "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250516/us-east-1/s3/aws4_request" }
   ```

   **Expected Response**: HTTP 200 OK, body: decrypted object data

6. **Valid DELETE Request**

   ```powershell
   Invoke-WebRequest -Method Delete -Uri "http://localhost:8080/test-bucket/test-key" -Headers @{ "Authorization" = "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250516/us-east-1/s3/aws4_request" }
   ```

   **Expected Response**: HTTP 200 OK
