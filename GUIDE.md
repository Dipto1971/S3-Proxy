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

---

## Step 4: Test File Upload

To test the server, run this curl command:

```bash
curl -X PUT http://localhost:8080/test-bucket1/hello.txt \
     -H "Content-Type: text/plain" \
     --data-binary @test/testdata/hello.txt
```

---

## Step 5: Verify

Check your MinIO dashboard or S3 client to verify that `hello.txt` has been uploaded to `test-bucket1`.

---
