plugins {
    id("com.android.application")
}

dependencies {
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
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

    defaultConfig {
        applicationId = "com.kentgram"
        minSdk = 24
        targetSdk = 35
        // GitHub Actions run numbers increase monotonically for this workflow.
        // Local builds keep the safe initial value.
        versionCode = (System.getenv("GITHUB_RUN_NUMBER")?.toIntOrNull() ?: 1).coerceAtLeast(1)
        versionName = "0.1.$versionCode"

        externalNativeBuild {
            cmake {
                cppFlags += listOf("-std=c++17", "-fexceptions", "-frtti")
            }
        }
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

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.22.1"
        }
    }
}
