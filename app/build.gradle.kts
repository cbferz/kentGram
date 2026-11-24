plugins {
    id("com.android.application")
}

val ciKeystorePath = System.getenv("KENTGRAM_KEYSTORE_PATH")
val ciKeystorePassword = System.getenv("KENTGRAM_KEYSTORE_PASSWORD")
val ciKeyAlias = System.getenv("KENTGRAM_KEY_ALIAS")
val ciKeyPassword = System.getenv("KENTGRAM_KEY_PASSWORD")
val hasCiSigning = !ciKeystorePath.isNullOrBlank()
    && !ciKeystorePassword.isNullOrBlank()
    && !ciKeyAlias.isNullOrBlank()
    && !ciKeyPassword.isNullOrBlank()

android {
    namespace = "com.kentgram"
    compileSdk = 35
    // Match the build-tools the CI installs; otherwise AGP downloads its own
    // default (34.0.0) on every fresh runner.
    buildToolsVersion = "35.0.0"

    defaultConfig {
        applicationId = "com.kentgram"
        minSdk = 24
        targetSdk = 35
        // GitHub Actions run numbers increase monotonically for this workflow.
        // Local builds keep the safe initial value.
        versionCode = (System.getenv("GITHUB_RUN_NUMBER")?.toIntOrNull() ?: 1).coerceAtLeast(1)
        versionName = "0.1.$versionCode"
    }

    signingConfigs {
        if (hasCiSigning) {
            create("ci") {
                storeFile = file(ciKeystorePath!!)
                storePassword = ciKeystorePassword
                keyAlias = ciKeyAlias
                keyPassword = ciKeyPassword
            }
        }
    }

    buildTypes {
        debug {
            // Use the same private keystore for every CI APK once repository
            // secrets are configured. Never commit the keystore to the repo.
            if (hasCiSigning) {
                signingConfig = signingConfigs.getByName("ci")
            }
        }
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
}
