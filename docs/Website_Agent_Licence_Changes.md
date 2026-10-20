# Website agent: licence changes (no more `ver`)

Summary of the licence-format change made on the website (migration 092). Contains no secrets; the signing key is only in the host env.

## What changed

- One licence key covers every app version and every build (Microsoft Store, offline Windows zip, Mac, Linux) on the same computer. A key is still per computer.
- The signed payload no longer has `ver`. Fields: `dev` (device hash as entered, trimmed and lower-cased), `iat` (ISO 8601 UTC), `kid`, `lic`, `prod` ("temple-accounts"), `v` (1). Compact JSON, keys sorted, UTF-8.
- Unchanged: signature = Ed25519 over `"vb-licence/v1\n" + payload`; key = `VBL1-` + base64url(payload) + `.` + base64url(signature), no padding; the private key.
- Keys already issued (with `ver`) keep working because the app ignores `ver`; nothing is reissued. The app (`verify_licence`, "different version" check) should drop that check on its side.
- Key generation needs only the Device ID; no operating system is asked for or stored. A new app version never needs a new key.
- Removed: `software.licence_versions`, `licences.ver`, the version chooser, "valid for v1.x" text. My licences shows one key per device (copyable text and `.lic` download with the same text).
- Added: nullable `software.microsoft_store_url` (admin software form), per-OS downloads on the product page and post-purchase page, the four activation steps, and `/temple-accounts/privacy`.

## Test vector (TEST seed only, never the production key)

Seed (hex): `0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20`
Public key (b64): `ebVWLo/mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ=`
kid `test-1`, lic `TAB-TEST-0001`, iat `2026-10-19T12:00:00Z`, prod `temple-accounts`

Payload:

```
{"dev":"9f7828e963f3f765e509de5cddb6c3aee32482b4aec979ca60b8e578f51ca241","iat":"2026-10-19T12:00:00Z","kid":"test-1","lic":"TAB-TEST-0001","prod":"temple-accounts","v":1}
```

Signature (b64url): `t6-RzkzhXUzW0bbsw6v0sWGpHMePxVmTEEXB6qR1-1CLVcItqxUIYZ7goo9RbRwq6oHlR_o46Y8iqoT9Rp5pCg`

Key:

```
VBL1-eyJkZXYiOiI5Zjc4MjhlOTYzZjNmNzY1ZTUwOWRlNWNkZGI2YzNhZWUzMjQ4MmI0YWVjOTc5Y2E2MGI4ZTU3OGY1MWNhMjQxIiwiaWF0IjoiMjAyNi0xMC0xOVQxMjowMDowMFoiLCJraWQiOiJ0ZXN0LTEiLCJsaWMiOiJUQUItVEVTVC0wMDAxIiwicHJvZCI6InRlbXBsZS1hY2NvdW50cyIsInYiOjF9.t6-RzkzhXUzW0bbsw6v0sWGpHMePxVmTEEXB6qR1-1CLVcItqxUIYZ7goo9RbRwq6oHlR_o46Y8iqoT9Rp5pCg
```

Checked by `core/licence/licence_test.go` and independently with Python `cryptography` (same signature).
