# Android CI signing for kentGram

The CI workflow publishes an APK release only after a persistent signing key is configured.
Without these secrets, CI still compiles the app and uploads a short-lived validation artifact, but it deliberately does not publish an APK that could break in-place updates.

## 1. Create the upload keystore locally

Run this on a trusted computer with Java's `keytool` installed:

```bash
keytool -genkeypair -v \
  -keystore kentgram-upload.jks \
  -alias kentgram-upload \
  -keyalg RSA -keysize 2048 -validity 10000
```

Choose strong passwords and keep a secure backup of `kentgram-upload.jks`.
The keystore and passwords are irreplaceable: losing them can prevent future APKs from updating an app signed with this key.

## 2. Encode the keystore

Linux:

```bash
base64 -w 0 kentgram-upload.jks
```

macOS:

```bash
base64 < kentgram-upload.jks | tr -d '\n'
```

Copy the resulting single-line value locally. Do not commit it and do not send it in chat.

## 3. Add GitHub Actions repository secrets

Open **Settings → Secrets and variables → Actions → New repository secret** in the kentGram repository and create:

| Secret name | Value |
|---|---|
| `KENTGRAM_KEYSTORE_BASE64` | The base64 output from step 2 |
| `KENTGRAM_KEYSTORE_PASSWORD` | Keystore password |
| `KENTGRAM_KEY_ALIAS` | `kentgram-upload` |
| `KENTGRAM_KEY_PASSWORD` | Password for the alias |

The workflow decodes the keystore into the temporary GitHub runner directory. It is never written to the repository.

## 4. Important upgrade note

Android permits an in-place update only when the package name and signing certificate match and the incoming `versionCode` is higher.

kentGram keeps the package name `com.kentgram`; CI now derives `versionCode` from the monotonically increasing GitHub Actions workflow run number. The first install after switching from an older runner-generated debug certificate may still require a one-time uninstall. After installing the persistent-key APK, later signed builds can update it in place.

Do not delete or replace the keystore after distributing APKs signed with it.
