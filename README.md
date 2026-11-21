# kentGram

Native Android social messenger combining Telegram-style communication and Instagram-style social features.

## Stack
- Java for the Android application layer
- Android SDK / AndroidX
- C++ via Android NDK and JNI for native performance-sensitive components
- Gradle Kotlin DSL for build configuration
- Backend API planned as a separate service; realtime transport will use WebSocket

## Project status
Initial Android foundation. The current app is a starter shell, not yet a connected messaging service.

## Build
Open the project in Android Studio with Android SDK 35 and a compatible Android Gradle Plugin. Let Gradle sync, then run the `app` configuration or build the debug APK.

## Planned modules
- Authentication and user profiles
- Chats, groups and channels
- Posts, reactions and comments
- Stories
- Feed and discovery
- Notifications and privacy controls
